package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fullerzz/herdr-plugin-sesh/internal/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAttachRoutesOnlyConfiguredMachines(t *testing.T) {
	cfg := remote.Config{ID: "laptop", HerdrSession: "default", Peers: []remote.Peer{{ID: "server", SSH: "user@server", Session: "work"}}}
	local, err := resolveAttachEndpoint(cfg, "laptop")
	require.NoError(t, err)
	assert.Equal(t, []string{"--session", "default"}, attachArgs(cfg, local))
	peer, err := resolveAttachEndpoint(cfg, "server")
	require.NoError(t, err)
	assert.Equal(t, []string{"--remote", "user@server", "--session", "work"}, attachArgs(cfg, peer))
	_, err = resolveAttachEndpoint(cfg, "untrusted")
	require.Error(t, err)
}

// fakeTransport serves attach requests from in-memory machines.
type fakeTransport struct {
	mu        sync.Mutex
	actions   []string
	call      func(attachEndpoint, remote.Request) (remote.Response, error)
	subscribe func(attachEndpoint) (chan remote.Target, error)
}

func (f *fakeTransport) record(action string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.actions = append(f.actions, action)
}

func (f *fakeTransport) recorded() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.actions...)
}

func (f *fakeTransport) Call(_ context.Context, endpoint attachEndpoint, req remote.Request) (remote.Response, error) {
	f.record(endpoint.id + ":" + req.Op)
	if f.call != nil {
		return f.call(endpoint, req)
	}
	return remote.Response{}, nil
}

func (f *fakeTransport) Subscribe(_ context.Context, endpoint attachEndpoint) (attachSubscription, error) {
	f.record(endpoint.id + ":subscribe")
	ch, err := f.subscribe(endpoint)
	if err != nil {
		return attachSubscription{}, err
	}
	return attachSubscription{
		targets: ch,
		err:     func() error { return errors.New("stream lost") },
		close:   func() { f.record(endpoint.id + ":release") },
		complete: func(_ context.Context, failure error) error {
			f.record(fmt.Sprintf("%s:complete:%v", endpoint.id, failure))
			return nil
		},
	}, nil
}

func exportFor(cfg remote.Config) func(attachEndpoint, remote.Request) (remote.Response, error) {
	return func(endpoint attachEndpoint, req remote.Request) (remote.Response, error) {
		if req.Op != "export" {
			return remote.Response{}, nil
		}
		session := cfg.HerdrSession
		if endpoint.peer != nil {
			session = endpoint.peer.Session
		}
		return remote.Response{Catalog: []remote.Machine{{ID: endpoint.id, Session: session}}}, nil
	}
}

// fakeHerdr records each attachment's arguments, then stays attached.
func fakeHerdr(t *testing.T) (string, *os.File) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "herdr")
	// exec keeps the fake attachment in the wrapper-owned process.
	//nolint:gosec // Executable fixture lives in the test-owned temporary directory.
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\"\nexec /bin/sleep 30\n"), 0700))
	//nolint:gosec // Output is confined to the test-owned temporary directory.
	logFile, err := os.Create(filepath.Join(dir, "clients"))
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, logFile.Close()) })
	return bin, logFile
}

// waitForLog runs then once the attached client has written want.
func waitForLog(ctx context.Context, logFile *os.File, want string, then func()) {
	go func() {
		for ctx.Err() == nil {
			data, err := os.ReadFile(logFile.Name())
			if err == nil && strings.Contains(string(data), want) {
				then()
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
}

func TestAttachReplacesOnlyOwnedClientAndReleasesControllers(t *testing.T) {
	bin, logFile := fakeHerdr(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cfg := remote.Config{ID: "local", HerdrSession: "default", Peers: []remote.Peer{{ID: "remote", SSH: "host", Session: "work"}}}
	transport := &fakeTransport{call: exportFor(cfg)}
	transport.subscribe = func(endpoint attachEndpoint) (chan remote.Target, error) {
		ch := make(chan remote.Target, 1)
		if endpoint.id == "local" {
			waitForLog(ctx, logFile, "--session default", func() { ch <- remote.Target{MachineID: "remote", WorkspaceID: "w2"} })
		} else {
			waitForLog(ctx, logFile, "--remote host --session work", cancel)
		}
		return ch, nil
	}
	restored := 0
	var errOut bytes.Buffer
	err := runAttach(ctx, cfg, attachEndpoint{id: "local"}, transport, bin, &attachTerminal{restore: func() error { restored++; return nil }}, logFile, &errOut)
	require.ErrorIs(t, err, context.Canceled)
	assert.GreaterOrEqual(t, restored, 2)
	assert.Empty(t, errOut.String())
	// The source's switch ends with its release; completing first would reopen it.
	assert.Equal(t, []string{"local:export", "local:subscribe", "remote:export", "remote:subscribe", "remote:focus", "local:release", "remote:release"}, transport.recorded())
	data, err := os.ReadFile(logFile.Name())
	require.NoError(t, err)
	assert.Equal(t, "--session default\n--remote host --session work\n", string(data))
}

func TestAttachKeepsClientWhenControllerIsLost(t *testing.T) {
	saved := resubscribeDelays
	resubscribeDelays = []time.Duration{0, 10 * time.Millisecond}
	t.Cleanup(func() { resubscribeDelays = saved })
	bin, logFile := fakeHerdr(t)
	cfg := remote.Config{ID: "local", HerdrSession: "default"}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var subscriptions atomic.Int32
	transport := &fakeTransport{call: exportFor(cfg)}
	transport.subscribe = func(attachEndpoint) (chan remote.Target, error) {
		switch n := subscriptions.Add(1); {
		case n == 1:
			ch := make(chan remote.Target)
			waitForLog(ctx, logFile, "--session default", func() { close(ch) })
			return ch, nil
		case n < 5:
			// The daemon restarts slower than several retries.
			return nil, errors.New("connection refused")
		default:
			ch := make(chan remote.Target, 1)
			ch <- remote.Target{MachineID: "local", WorkspaceID: "w1"}
			return ch, nil
		}
	}
	transport.call = func(endpoint attachEndpoint, req remote.Request) (remote.Response, error) {
		if req.Op == "focus" {
			cancel()
		}
		return exportFor(cfg)(endpoint, req)
	}
	var errOut bytes.Buffer
	var mu sync.Mutex
	err := runAttach(ctx, cfg, attachEndpoint{id: "local"}, transport, bin, nil, logFile, &lockedWriter{mu: &mu, w: &errOut})
	require.ErrorIs(t, err, context.Canceled)
	assert.NotContains(t, err.Error(), "controller lost")
	assert.Equal(t, int32(5), subscriptions.Load())
	mu.Lock()
	assert.Equal(t, 1, strings.Count(errOut.String(), "Remote switching unavailable"))
	mu.Unlock()
	data, err := os.ReadFile(logFile.Name())
	require.NoError(t, err)
	// The original client stayed attached; switching resumed on the new controller.
	assert.Equal(t, "--session default\n", string(data))
	assert.Contains(t, transport.recorded(), "local:focus")
}

func TestAttachCompletesEachSwitch(t *testing.T) {
	bin, logFile := fakeHerdr(t)
	cfg := remote.Config{ID: "local", HerdrSession: "default"}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	targets := make(chan remote.Target, 2)
	targets <- remote.Target{MachineID: "local", WorkspaceID: "gone"}
	targets <- remote.Target{MachineID: "local", WorkspaceID: "w1"}
	transport := &fakeTransport{subscribe: func(attachEndpoint) (chan remote.Target, error) { return targets, nil }}
	transport.call = func(endpoint attachEndpoint, req remote.Request) (remote.Response, error) {
		if req.Op == "focus" && req.WorkspaceID == "gone" {
			return remote.Response{}, errors.New("workspace no longer exists")
		}
		return exportFor(cfg)(endpoint, req)
	}
	go func() {
		for ctx.Err() == nil {
			if slices.Contains(transport.recorded(), "local:complete:<nil>") {
				cancel()
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	var errOut bytes.Buffer
	var mu sync.Mutex
	err := runAttach(ctx, cfg, attachEndpoint{id: "local"}, transport, bin, nil, logFile, &lockedWriter{mu: &mu, w: &errOut})
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, []string{
		"local:export", "local:subscribe",
		"local:focus", "local:complete:workspace no longer exists",
		"local:focus", "local:complete:<nil>",
		"local:release",
	}, transport.recorded())
	mu.Lock()
	defer mu.Unlock()
	assert.Contains(t, errOut.String(), "Remote switch failed: workspace no longer exists")
}

// lockedWriter lets the test read stderr the attached child also writes.
type lockedWriter struct {
	mu *sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

func TestAttachEnvironmentDropsOnlyHerdrContext(t *testing.T) {
	for _, tc := range []struct {
		entry string
		kept  bool
	}{
		{"PATH=/bin", true},
		{"HERDR_CONFIG_PATH=/my/herdr.toml", true},
		{"HERDR_SESH_CONFIG=/my/sesh.toml", true},
		{"HERDR_SESH_PICKER=native", true},
		{"HERDR_SESH_REMOTE_CONFIG=/my/remote.toml", true},
		{"HERDR_BIN_PATH=/opt/herdr", true},
		{"HERDR_ENV=1", false},
		{"HERDR_SESSION=other", false},
		{"HERDR_SOCKET_PATH=/wrong.sock", false},
		{"HERDR_CLIENT_SOCKET_PATH=/wrong-client.sock", false},
		{"HERDR_WORKSPACE_ID=w1", false},
		{"HERDR_TAB_ID=w1:t1", false},
		{"HERDR_PANE_ID=w1:p1", false},
		{"HERDR_PANE_RUNTIME_ID=r1", false},
		{"HERDR_PLUGIN_STATE_DIR=/state", false},
		{"HERDR_ACTIVE_PANE_CWD=/src", false},
	} {
		got := attachEnvironment([]string{tc.entry})
		if tc.kept {
			assert.Equal(t, []string{tc.entry}, got, tc.entry)
		} else {
			assert.Empty(t, got, tc.entry)
		}
	}
}

func TestDaemonRPCRequiresStream(t *testing.T) {
	a := &App{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}
	for _, args := range [][]string{{"daemon", "rpc"}, {"daemon", "rpc", "--json"}} {
		assert.ErrorContains(t, a.Run(context.Background(), args), "usage: herdr-sesh daemon serve|list|rpc --stream")
	}
}

func TestAttachRejectsWrongDaemonBeforeRegisteringOrStartingClient(t *testing.T) {
	for _, machine := range []remote.Machine{{ID: "wrong", Session: "default"}, {ID: "local", Session: "wrong"}} {
		cfg := remote.Config{ID: "local", HerdrSession: "default"}
		transport := &fakeTransport{call: func(attachEndpoint, remote.Request) (remote.Response, error) {
			return remote.Response{Catalog: []remote.Machine{machine}}, nil
		}}
		var output bytes.Buffer
		err := runAttach(context.Background(), cfg, attachEndpoint{id: "local"}, transport, "missing-executable", nil, &output, &output)
		require.ErrorContains(t, err, "does not match")
		assert.Equal(t, []string{"local:export"}, transport.recorded())
	}
}

package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	assert.Equal(t, []string{"PATH=/bin", "TERM=xterm", "HERDR_SESH_REMOTE_CONFIG=/custom.toml"}, attachEnvironment([]string{"HERDR_SOCKET_PATH=/wrong", "PATH=/bin", "HERDR_PANE_ID=w1:p1", "TERM=xterm", "HERDR_SESH_REMOTE_CONFIG=/custom.toml"}))
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
	return attachSubscription{targets: ch, err: func() error { return errors.New("stream lost") }, close: func() { f.record(endpoint.id + ":release") }}, nil
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
	err := runAttach(ctx, cfg, attachEndpoint{id: "local"}, transport, bin, nil, logFile, &errOut, func() error { restored++; return nil })
	require.ErrorIs(t, err, context.Canceled)
	assert.GreaterOrEqual(t, restored, 2)
	assert.Empty(t, errOut.String())
	assert.Equal(t, []string{"local:export", "local:subscribe", "remote:export", "remote:subscribe", "remote:focus", "local:release", "remote:release"}, transport.recorded())
	data, err := os.ReadFile(logFile.Name())
	require.NoError(t, err)
	assert.Equal(t, "--session default\n--remote host --session work\n", string(data))
}

func TestAttachResubscribesAfterLostStream(t *testing.T) {
	saved := resubscribeDelays
	resubscribeDelays = []time.Duration{0, 10 * time.Millisecond}
	t.Cleanup(func() { resubscribeDelays = saved })
	bin, logFile := fakeHerdr(t)
	cfg := remote.Config{ID: "local", HerdrSession: "default"}
	for _, recovers := range []bool{true, false} {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		subscriptions := 0
		transport := &fakeTransport{call: exportFor(cfg)}
		transport.subscribe = func(attachEndpoint) (chan remote.Target, error) {
			subscriptions++
			switch {
			case subscriptions == 1:
				ch := make(chan remote.Target)
				waitForLog(ctx, logFile, "--session default", func() { close(ch) })
				return ch, nil
			case recovers:
				cancel()
				return make(chan remote.Target), nil
			default:
				return nil, errors.New("another controller is active")
			}
		}
		err := runAttach(ctx, cfg, attachEndpoint{id: "local"}, transport, bin, nil, logFile, io.Discard, func() error { return nil })
		cancel()
		if recovers {
			require.ErrorIs(t, err, context.Canceled)
			assert.Equal(t, 2, subscriptions)
		} else {
			require.ErrorContains(t, err, "attachment controller lost: another controller is active")
			assert.Equal(t, 1+len(resubscribeDelays), subscriptions)
		}
	}
}

func TestDaemonRPCRejectsInvalidAndOversizedRequests(t *testing.T) {
	for _, input := range []string{"not json", `{}` + `{}`, strings.Repeat("x", 16*1024+1)} {
		var out bytes.Buffer
		err := daemonRPC(context.Background(), remote.Config{}, strings.NewReader(input), &out)
		require.Error(t, err)
		assert.Empty(t, out.String())
	}
}

func TestAttachRejectsWrongDaemonBeforeRegisteringOrStartingClient(t *testing.T) {
	for _, machine := range []remote.Machine{{ID: "wrong", Session: "default"}, {ID: "local", Session: "wrong"}} {
		cfg := remote.Config{ID: "local", HerdrSession: "default"}
		transport := &fakeTransport{call: func(attachEndpoint, remote.Request) (remote.Response, error) {
			return remote.Response{Catalog: []remote.Machine{machine}}, nil
		}}
		var output bytes.Buffer
		err := runAttach(context.Background(), cfg, attachEndpoint{id: "local"}, transport, "missing-executable", nil, &output, &output, func() error { return nil })
		require.ErrorContains(t, err, "does not match")
		assert.Equal(t, []string{"local:export"}, transport.recorded())
	}
}

package app

import (
	"bytes"
	"context"
	"errors"
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

func TestAttachReplacesOnlyOwnedClientAndReleasesLeases(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "herdr")
	// exec keeps the fake attachment in the wrapper-owned process.
	//nolint:gosec // Executable fixture lives in the test-owned temporary directory.
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\"\nexec /bin/sleep 30\n"), 0700))
	//nolint:gosec // Output is confined to the test-owned temporary directory.
	logFile, err := os.Create(filepath.Join(dir, "clients"))
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, logFile.Close()) })
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cfg := remote.Config{ID: "local", HerdrSession: "default", Peers: []remote.Peer{{ID: "remote", SSH: "host", Session: "work"}}}
	var mu sync.Mutex
	var actions []string
	var token string
	queued := false
	call := func(ctx context.Context, endpoint attachEndpoint, req remote.Request) (remote.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		actions = append(actions, endpoint.id+":"+req.Op)
		if req.Op == "export" {
			session := cfg.HerdrSession
			if endpoint.peer != nil {
				session = endpoint.peer.Session
			}
			return remote.Response{Catalog: []remote.Machine{{ID: endpoint.id, Session: session}}}, nil
		}
		if req.Token != "" {
			if token == "" {
				token = req.Token
			} else if token != req.Token {
				return remote.Response{}, errors.New("controller capability changed")
			}
		}
		if req.Op == "poll" {
			data, err := os.ReadFile(logFile.Name())
			if err != nil {
				return remote.Response{}, err
			}
			if endpoint.id == "local" && len(data) > 0 && !queued {
				queued = true
				return remote.Response{Switch: &remote.Target{MachineID: "remote", WorkspaceID: "w2", RequestID: "switch-1"}}, nil
			}
			if endpoint.id == "remote" && strings.Contains(string(data), "--remote host --session work") {
				cancel()
				return remote.Response{}, ctx.Err()
			}
		}
		return remote.Response{}, nil
	}
	restored := 0
	var errOut bytes.Buffer
	err = runAttach(ctx, cfg, attachEndpoint{id: "local"}, call, bin, nil, logFile, &errOut, func() error { restored++; return nil })
	require.ErrorIs(t, err, context.Canceled)
	assert.GreaterOrEqual(t, restored, 2)
	assert.Empty(t, errOut.String())
	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, token, 64)
	assert.Contains(t, actions, "remote:focus")
	assert.Contains(t, actions, "local:unregister")
	assert.Contains(t, actions, "remote:unregister")
	data, err := os.ReadFile(logFile.Name())
	require.NoError(t, err)
	assert.Equal(t, "--session default\n--remote host --session work\n", string(data))
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
		var ops []string
		call := func(_ context.Context, _ attachEndpoint, req remote.Request) (remote.Response, error) {
			ops = append(ops, req.Op)
			return remote.Response{Catalog: []remote.Machine{machine}}, nil
		}
		var output bytes.Buffer
		err := runAttach(context.Background(), cfg, attachEndpoint{id: "local"}, call, "missing-executable", nil, &output, &output, func() error { return nil })
		require.ErrorContains(t, err, "does not match")
		assert.Equal(t, []string{"export"}, ops)
	}
}

func TestPollAttachmentRetriesLostResponsesWithoutRepeatingSelection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	updates := make(chan remote.Target)
	failures := make(chan error, 1)
	done := make(chan struct{})
	target := remote.Target{MachineID: "peer", WorkspaceID: "w1", RequestID: "request-1"}
	polls, acknowledgements := 0, 0
	call := func(_ context.Context, _ attachEndpoint, req remote.Request) (remote.Response, error) {
		switch req.Op {
		case "poll":
			polls++
			if polls == 1 {
				return remote.Response{}, errors.New("lost poll response")
			}
			return remote.Response{Switch: &target}, nil
		case "ack":
			acknowledgements++
			if req.RequestID != target.RequestID {
				return remote.Response{}, errors.New("wrong acknowledgement")
			}
			if acknowledgements == 1 {
				return remote.Response{}, errors.New("lost acknowledgement")
			}
			cancel()
			return remote.Response{}, nil
		default:
			return remote.Response{}, errors.New("unexpected operation")
		}
	}
	go func() {
		defer close(done)
		pollAttachment(ctx, attachEndpoint{id: "local"}, "token", call, updates, failures)
	}()
	var selections []remote.Target
loop:
	for {
		select {
		case selection := <-updates:
			selections = append(selections, selection)
		case <-done:
			break loop
		}
	}
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	assert.Equal(t, []remote.Target{target}, selections)
	assert.Equal(t, 3, polls)
	assert.Equal(t, 2, acknowledgements)
	assert.Empty(t, failures)
}

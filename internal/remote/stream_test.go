package remote

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// local is the controller allowlist used against startDaemon's machine.
var local = []string{"local"}

// fakeRequest is one JSON-RPC call received by fakeDaemon.
type fakeRequest struct {
	ID     json.RawMessage
	Method string
	Params rpcParams
}

// fakeDaemon answers hello on conn, then hands every later request to handle.
func fakeDaemon(conn net.Conn, handle func(fakeRequest, *json.Encoder)) {
	defer func() { _ = conn.Close() }()
	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)
	for {
		var m rpcMessage
		if dec.Decode(&m) != nil {
			return
		}
		r := fakeRequest{ID: m.ID, Method: m.Method}
		if m.Params != nil && json.Unmarshal(m.Params, &r.Params) != nil {
			return
		}
		if r.Method == "hello" {
			_ = enc.Encode(rpcReply(r.ID, r.Method, Response{Version: ProtocolVersion}))
			continue
		}
		handle(r, enc)
	}
}

func TestStreamMultiplexesOutOfOrderResponses(t *testing.T) {
	const calls = 3
	var mu sync.Mutex
	var held []fakeRequest
	c := NewStreamClient("fake", func(context.Context) (io.ReadWriteCloser, error) {
		client, server := net.Pipe()
		go fakeDaemon(server, func(r fakeRequest, enc *json.Encoder) {
			mu.Lock()
			defer mu.Unlock()
			held = append(held, r)
			if len(held) < calls {
				return
			}
			// Answer in reverse so ids, not arrival order, must route responses.
			for i := len(held) - 1; i >= 0; i-- {
				_ = enc.Encode(rpcReply(held[i].ID, held[i].Method, Response{Catalog: []Machine{{ID: held[i].Params.WorkspaceID}}}))
			}
		})
		return client, nil
	})
	c.PingInterval = 0
	t.Cleanup(func() { _ = c.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	results := make([]string, calls)
	errs := make([]error, calls)
	var wg sync.WaitGroup
	for i := range calls {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r, err := c.Call(ctx, Request{Op: "list", WorkspaceID: strconv.Itoa(i)})
			errs[i] = err
			if err == nil && len(r.Catalog) == 1 {
				results[i] = r.Catalog[0].ID
			}
		}(i)
	}
	wg.Wait()
	for i := range calls {
		require.NoError(t, errs[i])
		assert.Equal(t, strconv.Itoa(i), results[i])
	}
}

func TestStreamClientReconnectsWithBackoff(t *testing.T) {
	var dials atomic.Int32
	var mu sync.Mutex
	var live net.Conn
	c := NewStreamClient("fake", func(context.Context) (io.ReadWriteCloser, error) {
		if dials.Add(1) == 1 {
			return nil, errors.New("connection refused")
		}
		client, server := net.Pipe()
		mu.Lock()
		live = server
		mu.Unlock()
		go fakeDaemon(server, func(r fakeRequest, enc *json.Encoder) { _ = enc.Encode(rpcReply(r.ID, r.Method, Response{})) })
		return client, nil
	})
	c.PingInterval = 0
	t.Cleanup(func() { _ = c.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := c.Call(ctx, Request{Op: "list"})
	require.ErrorContains(t, err, "connection refused")
	_, err = c.Call(ctx, Request{Op: "list"})
	require.ErrorContains(t, err, "retrying in")
	assert.Equal(t, int32(1), dials.Load())

	c.mu.Lock()
	c.retryAt = time.Time{}
	c.mu.Unlock()
	_, err = c.Call(ctx, Request{Op: "list"})
	require.NoError(t, err)
	assert.Equal(t, int32(2), dials.Load())

	mu.Lock()
	require.NoError(t, live.Close())
	mu.Unlock()
	require.Eventually(t, func() bool {
		_, err := c.Call(ctx, Request{Op: "list"})
		return err == nil
	}, 2*time.Second, 10*time.Millisecond)
	assert.Equal(t, int32(3), dials.Load())
	c.mu.Lock()
	defer c.mu.Unlock()
	assert.Zero(t, c.backoff)
}

func TestStreamHandshakeRejectsOtherProtocolVersions(t *testing.T) {
	c := NewStreamClient("old", func(context.Context) (io.ReadWriteCloser, error) {
		client, server := net.Pipe()
		go func() {
			defer func() { _ = server.Close() }()
			var m rpcMessage
			if json.NewDecoder(server).Decode(&m) == nil {
				_ = json.NewEncoder(server).Encode(rpcReply(m.ID, m.Method, Response{Version: ProtocolVersion + 1}))
			}
		}()
		return client, nil
	})
	t.Cleanup(func() { _ = c.Close() })
	_, err := c.Call(context.Background(), Request{Op: "list"})
	require.ErrorContains(t, err, "handshake failed")
	legacy := NewStreamClient("legacy", func(context.Context) (io.ReadWriteCloser, error) {
		client, server := net.Pipe()
		go func() {
			defer func() { _ = server.Close() }()
			var m rpcMessage
			if json.NewDecoder(server).Decode(&m) == nil {
				_ = json.NewEncoder(server).Encode(Response{Error: "unknown operation"})
			}
		}()
		return client, nil
	})
	t.Cleanup(func() { _ = legacy.Close() })
	_, err = legacy.Call(context.Background(), Request{Op: "list"})
	require.ErrorContains(t, err, "does not speak JSON-RPC 2.0: unknown operation")
	s := NewServer(Config{Enabled: true, ID: "local"}, &fakeClient{})
	assert.Contains(t, s.Handle(context.Background(), Request{Op: "hello", Version: 1}).Error, "protocol version mismatch")
	assert.Equal(t, ProtocolVersion, s.Handle(context.Background(), Request{Op: "hello", Version: ProtocolVersion}).Version)
	assert.Contains(t, s.Handle(context.Background(), Request{Op: "poll"}).Error, "same herdr-sesh version")
}

func startDaemon(t *testing.T, idle time.Duration) Config {
	t.Helper()
	// Unix socket paths are length limited, so avoid the long test temp root.
	dir, err := os.MkdirTemp("/tmp", "sesh-stream-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	require.NoError(t, os.Chmod(dir, 0700)) // #nosec G302 -- private socket directory requires traversal.
	c := Config{Enabled: true, ID: "local", Socket: filepath.Join(dir, "daemon.sock")}
	s := NewServer(c, &fakeClient{})
	if idle > 0 {
		s.IdleTimeout = idle
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		assert.NoError(t, <-done)
	})
	require.Eventually(t, func() bool {
		r, err := Call(context.Background(), c, Request{Op: "export"})
		return err == nil && len(r.Catalog) == 1 && !r.Catalog[0].Stale
	}, 2*time.Second, 10*time.Millisecond)
	return c
}

func TestClosingSubscribeStreamReleasesController(t *testing.T) {
	c := startDaemon(t, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	first := NewUnixClient(c)
	sub, err := first.Subscribe(ctx, local)
	require.NoError(t, err)
	second := NewUnixClient(c)
	t.Cleanup(func() { _ = second.Close() })
	_, err = second.Subscribe(ctx, local)
	require.ErrorContains(t, err, "another controller is active")

	_, err = Call(ctx, c, Request{Op: "switch", MachineID: "local", WorkspaceID: "missing"})
	require.Error(t, err)
	_, err = Call(ctx, c, Request{Op: "switch", MachineID: "local", WorkspaceID: "ws1"})
	require.NoError(t, err)
	select {
	case target := <-sub.Targets():
		assert.Equal(t, Target{MachineID: "local", WorkspaceID: "ws1"}, target)
	case <-ctx.Done():
		require.FailNow(t, "switch was not pushed")
	}

	require.NoError(t, first.Close())
	_, open := <-sub.Targets()
	assert.False(t, open)
	var next *Subscription
	require.Eventually(t, func() bool {
		next, err = second.Subscribe(ctx, local)
		return err == nil
	}, 2*time.Second, 10*time.Millisecond)

	// Unsubscribing releases the controller but keeps the stream usable.
	next.Close()
	_, err = second.Call(ctx, Request{Op: "list"})
	require.NoError(t, err)
	third := NewUnixClient(c)
	t.Cleanup(func() { _ = third.Close() })
	_, err = third.Subscribe(ctx, local)
	require.NoError(t, err)
	_, err = Call(ctx, c, Request{Op: "subscribe"})
	require.ErrorContains(t, err, "requires a daemon stream")
}

func TestIdleStreamReleasesController(t *testing.T) {
	c := startDaemon(t, 100*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	quiet := NewUnixClient(c)
	quiet.PingInterval = 0
	t.Cleanup(func() { _ = quiet.Close() })
	sub, err := quiet.Subscribe(ctx, local)
	require.NoError(t, err)
	select {
	case _, open := <-sub.Targets():
		assert.False(t, open)
	case <-ctx.Done():
		require.FailNow(t, "idle stream stayed open")
	}
	_, err = Call(ctx, c, Request{Op: "switch", MachineID: "local", WorkspaceID: "ws1"})
	require.ErrorContains(t, err, "no active attach controller")
}

func TestRelayStreamsAndReleasesOnStdinClose(t *testing.T) {
	c := startDaemon(t, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	done := make(chan error, 1)
	go func() { done <- Relay(ctx, c, inR, outW); _ = outW.Close() }()
	responses := bufio.NewScanner(outR)
	send := func(r Request) Response {
		t.Helper()
		b, err := json.Marshal(newRPCRequest(1, r))
		require.NoError(t, err)
		_, err = inW.Write(append(b, '\n'))
		require.NoError(t, err)
		require.True(t, responses.Scan())
		var m rpcMessage
		require.NoError(t, json.Unmarshal(responses.Bytes(), &m))
		assert.JSONEq(t, "1", string(m.ID))
		return rpcResponseOf(m)
	}
	assert.Equal(t, ProtocolVersion, send(Request{Op: "hello", Version: ProtocolVersion}).Version)
	assert.Empty(t, send(Request{Op: "subscribe", Machines: local}).Error)
	_, err := Call(ctx, c, Request{Op: "switch", MachineID: "local", WorkspaceID: "ws1"})
	require.NoError(t, err)
	require.True(t, responses.Scan())
	assert.JSONEq(t, `{"jsonrpc":"2.0","method":"switch","params":{"machine_id":"local","workspace_id":"ws1"}}`, responses.Text())

	require.NoError(t, inW.Close())
	go func() { _, _ = io.Copy(io.Discard, outR) }()
	require.NoError(t, <-done)
	other := NewUnixClient(c)
	t.Cleanup(func() { _ = other.Close() })
	require.Eventually(t, func() bool { _, err := other.Subscribe(ctx, local); return err == nil }, 2*time.Second, 10*time.Millisecond)
}

func TestRelayRejectsOversizedRequests(t *testing.T) {
	c := startDaemon(t, 0)
	err := Relay(context.Background(), c, strings.NewReader(strings.Repeat("x", maxRequest+1)), io.Discard)
	require.ErrorContains(t, err, "exceeds")
}

func TestDaemonStreamFollowsJSONRPC(t *testing.T) {
	c := startDaemon(t, 0)
	conn, err := net.Dial("unix", c.Socket)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	require.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))
	replies := bufio.NewScanner(conn)
	exchange := func(line string) string {
		t.Helper()
		_, err := io.WriteString(conn, line+"\n")
		require.NoError(t, err)
		require.True(t, replies.Scan())
		return replies.Text()
	}
	assert.JSONEq(t, `{"jsonrpc":"2.0","id":"a","result":{"version":2}}`, exchange(`{"jsonrpc":"2.0","id":"a","method":"hello","params":{"version":2}}`))
	assert.JSONEq(t, `{"jsonrpc":"2.0","id":null,"error":{"code":-32700,"message":"parse error"}}`, exchange(`{"jsonrpc":`))
	assert.JSONEq(t, `{"jsonrpc":"2.0","id":null,"error":{"code":-32600,"message":"batch requests are not supported"}}`, exchange(`[]`))
	assert.JSONEq(t, `{"jsonrpc":"2.0","id":3,"error":{"code":-32600,"message":"invalid request"}}`, exchange(`{"jsonrpc":"1.0","id":3,"method":"ping"}`))
	assert.JSONEq(t, `{"jsonrpc":"2.0","id":4,"error":{"code":-32601,"message":"method not found"}}`, exchange(`{"jsonrpc":"2.0","id":4,"method":"poll"}`))
	assert.JSONEq(t, `{"jsonrpc":"2.0","id":5,"error":{"code":-32602,"message":"invalid params"}}`, exchange(`{"jsonrpc":"2.0","id":5,"method":"focus","params":["ws1"]}`))
	assert.JSONEq(t, `{"jsonrpc":"2.0","id":6,"error":{"code":-32001,"message":"protocol version mismatch: client speaks v1, daemon speaks v2; install the same herdr-sesh version on both machines"}}`, exchange(`{"jsonrpc":"2.0","id":6,"method":"hello","params":{"version":1}}`))
	// A notification gets no reply, so the next line answers the request after it.
	_, err = io.WriteString(conn, `{"jsonrpc":"2.0","method":"ping"}`+"\n")
	require.NoError(t, err)
	assert.JSONEq(t, `{"jsonrpc":"2.0","id":7,"result":{}}`, exchange(`{"jsonrpc":"2.0","id":7,"method":"ping"}`))
}

func TestSwitchWaitsForControllerToComplete(t *testing.T) {
	c := startDaemon(t, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := NewUnixClient(c)
	t.Cleanup(func() { _ = client.Close() })
	sub, err := client.Subscribe(ctx, local)
	require.NoError(t, err)
	queue := func() error {
		_, err := Call(ctx, c, Request{Op: "switch", MachineID: "local", WorkspaceID: "ws1"})
		return err
	}
	require.NoError(t, queue())
	// Delivery alone must not admit a second selection that could be dropped.
	<-sub.Targets()
	require.ErrorContains(t, queue(), "already in progress")
	require.NoError(t, sub.Complete(ctx, errors.New("workspace no longer exists")))
	listed, err := Call(ctx, c, Request{Op: "list"})
	require.NoError(t, err)
	assert.Equal(t, "workspace no longer exists", listed.SwitchError)
	require.NoError(t, queue())
	<-sub.Targets()
	require.NoError(t, sub.Complete(ctx, nil))
	listed, err = Call(ctx, c, Request{Op: "list"})
	require.NoError(t, err)
	assert.Empty(t, listed.SwitchError)

	// Releasing the controller mid-switch also ends that switch.
	require.NoError(t, queue())
	require.NoError(t, client.Close())
	next := NewUnixClient(c)
	t.Cleanup(func() { _ = next.Close() })
	require.Eventually(t, func() bool { _, err := next.Subscribe(ctx, local); return err == nil }, 2*time.Second, 10*time.Millisecond)
	require.NoError(t, queue())
}

func TestSwitchRejectsMachinesTheControllerCannotReach(t *testing.T) {
	c := startDaemon(t, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := NewUnixClient(c)
	t.Cleanup(func() { _ = client.Close() })
	_, err := client.Subscribe(ctx, nil)
	require.ErrorContains(t, err, "reachable machines")
	_, err = client.Subscribe(ctx, []string{"--bad"})
	require.ErrorContains(t, err, "invalid machine id")
	_, err = client.Subscribe(ctx, local)
	require.NoError(t, err)
	_, err = Call(ctx, c, Request{Op: "switch", MachineID: "elsewhere", WorkspaceID: "ws1"})
	require.ErrorContains(t, err, `machine "elsewhere" is not in the attached wrapper's remote.toml allowlist`)
}

func TestSSHStreamErrorsIncludeStderr(t *testing.T) {
	dir := t.TempDir()
	// The fake ssh fails the way an unapproved host key or key does.
	//nolint:gosec // Executable fixture lives in the test-owned temporary directory.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ssh"), []byte("#!/bin/sh\necho 'Permission denied (publickey).' >&2\nexit 255\n"), 0700))
	t.Setenv("PATH", dir)
	c := NewSSHClient(Peer{ID: "server", SSH: "server"})
	t.Cleanup(func() { _ = c.Close() })
	_, err := c.Call(context.Background(), Request{Op: "list"})
	require.ErrorContains(t, err, "Permission denied (publickey).")
}

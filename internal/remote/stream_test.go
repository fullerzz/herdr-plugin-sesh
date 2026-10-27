package remote

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
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
	// Backoff repeats the dial error verbatim so status logs do not churn.
	for range 2 {
		_, again := c.Call(ctx, Request{Op: "list"})
		require.Error(t, again)
		assert.Equal(t, err.Error(), again.Error())
	}
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

// handshakeError dials a peer that answers hello with reply and returns the
// handshake failure.
func handshakeError(t *testing.T, reply func(id json.RawMessage) string) error {
	t.Helper()
	c := NewStreamClient("peer", func(context.Context) (io.ReadWriteCloser, error) {
		client, server := net.Pipe()
		go func() {
			defer func() { _ = server.Close() }()
			var m rpcMessage
			if json.NewDecoder(server).Decode(&m) == nil {
				if line := reply(m.ID); line != "" {
					_, _ = io.WriteString(server, line+"\n")
				}
			}
		}()
		return client, nil
	})
	t.Cleanup(func() { _ = c.Close() })
	_, err := c.Call(context.Background(), Request{Op: "list"})
	require.Error(t, err)
	return err
}

func encodeLine(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}

func TestStreamHandshakeRejectsOtherProtocolVersions(t *testing.T) {
	const hint = "install the same herdr-sesh version on both machines and restart their daemons"
	newer := handshakeError(t, func(id json.RawMessage) string {
		return encodeLine(t, rpcReply(id, "hello", Response{Version: ProtocolVersion + 1}))
	})
	require.ErrorContains(t, newer, "peer peer: protocol v3 handshake failed; "+hint)
	rejected := handshakeError(t, func(id json.RawMessage) string {
		return encodeLine(t, rpcFailure(id, codeVersionMismatch, "protocol version mismatch"))
	})
	require.ErrorContains(t, rejected, hint+": protocol version mismatch")
	s := NewServer(Config{Enabled: true, ID: "local"}, &fakeClient{})
	assert.Contains(t, s.Handle(context.Background(), Request{Op: "hello", Version: 1}).Error, "protocol version mismatch")
	assert.Equal(t, ProtocolVersion, s.Handle(context.Background(), Request{Op: "hello", Version: ProtocolVersion}).Version)
}

func TestStreamHandshakeDoesNotBlameVersionsForOtherFailures(t *testing.T) {
	const hint = "install the same herdr-sesh version"
	// A peer without the daemon command, such as v0.15.0, exits without replying.
	missing := handshakeError(t, func(json.RawMessage) string { return "" })
	require.ErrorContains(t, missing, "peer peer: stream closed before the protocol handshake; herdr-sesh may be missing from the peer's noninteractive SSH PATH, older than this version, or its daemon may not be running: ")
	assert.NotContains(t, missing.Error(), hint)
	banner := handshakeError(t, func(json.RawMessage) string { return "Welcome to server" })
	require.ErrorContains(t, banner, "peer peer: protocol handshake failed: peer printed non-JSON-RPC output; shell startup files must not write to stdout for noninteractive SSH")
	assert.NotContains(t, banner.Error(), hint)
	other := handshakeError(t, func(id json.RawMessage) string {
		return encodeLine(t, rpcFailure(id, codeDaemonError, "daemon is shutting down"))
	})
	assert.EqualError(t, other, "peer peer: protocol handshake failed: daemon is shutting down")
}

func TestStreamCleansDaemonErrorText(t *testing.T) {
	c := NewStreamClient("fake", func(context.Context) (io.ReadWriteCloser, error) {
		client, server := net.Pipe()
		go fakeDaemon(server, func(r fakeRequest, enc *json.Encoder) {
			_ = enc.Encode(rpcFailure(r.ID, codeDaemonError, "bad\x1b]0;owned\x07\r\nworkspace"))
		})
		return client, nil
	})
	c.PingInterval = 0
	t.Cleanup(func() { _ = c.Close() })
	_, err := c.Call(context.Background(), Request{Op: "list"})
	assert.EqualError(t, err, "bad]0;ownedworkspace")
}

func TestStreamFailsWhenSubscribeOutlivesItsCaller(t *testing.T) {
	var dials atomic.Int32
	returned := make(chan struct{})
	c := NewStreamClient("fake", func(context.Context) (io.ReadWriteCloser, error) {
		first := dials.Add(1) == 1
		client, server := net.Pipe()
		subscribed := false
		go fakeDaemon(server, func(r fakeRequest, enc *json.Encoder) {
			if r.Method != "subscribe" {
				_ = enc.Encode(rpcReply(r.ID, r.Method, Response{}))
				return
			}
			if subscribed {
				_ = enc.Encode(rpcFailure(r.ID, codeDaemonError, "stream is already subscribed"))
				return
			}
			// Like a daemon, install the controller before replying.
			subscribed = true
			if first {
				<-returned
			}
			_ = enc.Encode(rpcReply(r.ID, r.Method, Response{}))
		})
		return client, nil
	})
	c.PingInterval = 0
	t.Cleanup(func() { _ = c.Close() })
	_, err := c.Call(context.Background(), Request{Op: "list"})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = c.Subscribe(ctx, "local", "default", local)
	close(returned)
	require.ErrorIs(t, err, context.DeadlineExceeded)

	sub, err := c.Subscribe(context.Background(), "local", "default", local)
	require.NoError(t, err)
	assert.NotNil(t, sub)
	assert.Equal(t, int32(2), dials.Load())
}

// replyingConn answers each request before Write returns, the way a fast peer
// can reply before the writer is scheduled again.
type replyingConn struct {
	*io.PipeReader

	w        *io.PipeWriter
	s        *stream
	closeNow bool
	cancel   context.CancelFunc
}

func (c *replyingConn) Write(p []byte) (int, error) {
	var m rpcMessage
	if err := json.Unmarshal(p, &m); err != nil {
		return 0, err
	}
	b, err := json.Marshal(rpcReply(m.ID, m.Method, Response{Version: 7}))
	if err != nil {
		return 0, err
	}
	if _, err := c.w.Write(append(b, '\n')); err != nil {
		return 0, err
	}
	// Wait until the reply has been routed to the caller.
	for routed := false; !routed; {
		c.s.mu.Lock()
		routed = len(c.s.pending) == 0
		c.s.mu.Unlock()
	}
	if c.closeNow {
		_ = c.w.Close()
		<-c.s.done
	}
	if c.cancel != nil {
		c.cancel()
	}
	return len(p), nil
}

func (c *replyingConn) Close() error {
	_ = c.w.Close()
	return c.PipeReader.Close()
}

func TestStreamKeepsRepliesRoutedBeforeTheCallEnds(t *testing.T) {
	newReplyingStream := func(closeNow bool, cancel context.CancelFunc) *stream {
		r, w := io.Pipe()
		conn := &replyingConn{PipeReader: r, w: w, closeNow: closeNow, cancel: cancel}
		s := newStream(conn)
		conn.s = s
		t.Cleanup(func() { s.fail(errClientClosed) })
		return s
	}
	t.Run("stream closed", func(t *testing.T) {
		r, err := newReplyingStream(true, nil).call(context.Background(), Request{Op: "ping"})
		require.NoError(t, err)
		assert.Equal(t, 7, r.Version)
	})
	t.Run("caller canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		s := newReplyingStream(false, cancel)
		r, err := s.call(ctx, Request{Op: "ping"})
		require.NoError(t, err)
		assert.Equal(t, 7, r.Version)
	})
}

func startDaemon(t *testing.T, idle time.Duration) Config {
	t.Helper()
	return serveDaemon(t, idle, nil)
}

// serveDaemon starts startDaemon's server after configure adjusts it.
func serveDaemon(t *testing.T, idle time.Duration, configure func(*Server)) Config {
	t.Helper()
	// Unix socket paths are length limited, so avoid the long test temp root.
	dir, err := os.MkdirTemp("/tmp", "sesh-stream-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	require.NoError(t, os.Chmod(dir, 0700)) // #nosec G302 -- private socket directory requires traversal.
	c := Config{Enabled: true, ID: "local", HerdrSession: "default", Socket: filepath.Join(dir, "daemon.sock")}
	s := NewServer(c, &fakeClient{})
	if idle > 0 {
		s.IdleTimeout = idle
	}
	if configure != nil {
		configure(s)
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
	sub, err := first.Subscribe(ctx, "local", "default", local)
	require.NoError(t, err)
	second := NewUnixClient(c)
	t.Cleanup(func() { _ = second.Close() })
	_, err = second.Subscribe(ctx, "local", "default", local)
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
		next, err = second.Subscribe(ctx, "local", "default", local)
		return err == nil
	}, 2*time.Second, 10*time.Millisecond)

	// Unsubscribing releases the controller but keeps the stream usable.
	next.Close()
	_, err = second.Call(ctx, Request{Op: "list"})
	require.NoError(t, err)
	third := NewUnixClient(c)
	t.Cleanup(func() { _ = third.Close() })
	_, err = third.Subscribe(ctx, "local", "default", local)
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
	sub, err := quiet.Subscribe(ctx, "local", "default", local)
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
		resp, err := rpcResponseOf(m)
		require.NoError(t, err)
		return resp
	}
	assert.Equal(t, ProtocolVersion, send(Request{Op: "hello", Version: ProtocolVersion}).Version)
	send(Request{Op: "subscribe", MachineID: "local", Session: "default", Machines: local})
	_, err := Call(ctx, c, Request{Op: "switch", MachineID: "local", WorkspaceID: "ws1"})
	require.NoError(t, err)
	require.True(t, responses.Scan())
	assert.JSONEq(t, `{"jsonrpc":"2.0","method":"switch","params":{"machine_id":"local","workspace_id":"ws1"}}`, responses.Text())

	require.NoError(t, inW.Close())
	go func() { _, _ = io.Copy(io.Discard, outR) }()
	require.NoError(t, <-done)
	other := NewUnixClient(c)
	t.Cleanup(func() { _ = other.Close() })
	require.Eventually(t, func() bool { _, err := other.Subscribe(ctx, "local", "default", local); return err == nil }, 2*time.Second, 10*time.Millisecond)
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
	assert.JSONEq(t, `{"jsonrpc":"2.0","id":"a","result":{"version":3}}`, exchange(`{"jsonrpc":"2.0","id":"a","method":"hello","params":{"version":3}}`))
	assert.JSONEq(t, `{"jsonrpc":"2.0","id":null,"error":{"code":-32700,"message":"parse error"}}`, exchange(`{"jsonrpc":`))
	assert.JSONEq(t, `{"jsonrpc":"2.0","id":null,"error":{"code":-32600,"message":"batch requests are not supported"}}`, exchange(`[]`))
	assert.JSONEq(t, `{"jsonrpc":"2.0","id":3,"error":{"code":-32600,"message":"invalid request"}}`, exchange(`{"jsonrpc":"1.0","id":3,"method":"ping"}`))
	assert.JSONEq(t, `{"jsonrpc":"2.0","id":4,"error":{"code":-32601,"message":"method not found"}}`, exchange(`{"jsonrpc":"2.0","id":4,"method":"poll"}`))
	assert.JSONEq(t, `{"jsonrpc":"2.0","id":5,"error":{"code":-32602,"message":"invalid params"}}`, exchange(`{"jsonrpc":"2.0","id":5,"method":"focus","params":["ws1"]}`))
	assert.JSONEq(t, `{"jsonrpc":"2.0","id":6,"error":{"code":-32001,"message":"protocol version mismatch: client speaks v1, daemon speaks v3; install the same herdr-sesh version on both machines"}}`, exchange(`{"jsonrpc":"2.0","id":6,"method":"hello","params":{"version":1}}`))
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
	sub, err := client.Subscribe(ctx, "local", "default", local)
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
	require.Eventually(t, func() bool { _, err := next.Subscribe(ctx, "local", "default", local); return err == nil }, 2*time.Second, 10*time.Millisecond)
	require.NoError(t, queue())
}

func TestDaemonLogsSwitchLifecycle(t *testing.T) {
	var mu sync.Mutex
	var logs []string
	c := serveDaemon(t, 0, func(s *Server) {
		s.Logf = func(format string, args ...any) {
			mu.Lock()
			defer mu.Unlock()
			logs = append(logs, fmt.Sprintf(format, args...))
		}
	})
	logged := func(want string) bool {
		mu.Lock()
		defer mu.Unlock()
		return slices.ContainsFunc(logs, func(line string) bool { return strings.Contains(line, want) })
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := NewUnixClient(c)
	sub, err := client.Subscribe(ctx, "local", "default", local)
	require.NoError(t, err)
	_, err = Call(ctx, c, Request{Op: "switch", MachineID: "local", WorkspaceID: "ws1"})
	require.NoError(t, err)
	<-sub.Targets()
	require.NoError(t, sub.Complete(ctx, nil))
	require.NoError(t, client.Close())

	for _, want := range []string{
		"stream opened",
		"hello version=3 -> ok",
		"controller registered on conn",
		"switch to local/ws1 queued for controller",
		"switch machine=local workspace=ws1 -> ok",
		"switch to local/ws1 pushed to controller",
		"completed its switch",
		"disconnected",
		"stream closed after",
	} {
		assert.Eventually(t, func() bool { return logged(want) }, 2*time.Second, 10*time.Millisecond, want)
	}
	assert.False(t, logged(" export"), "routine requests stay quiet")
}

func TestReleasingMidSwitchEndsItWithoutAdmittingAnother(t *testing.T) {
	c := startDaemon(t, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	queue := func() error {
		_, err := Call(ctx, c, Request{Op: "switch", MachineID: "local", WorkspaceID: "ws1"})
		return err
	}
	switchError := func() string {
		listed, err := Call(ctx, c, Request{Op: "list"})
		require.NoError(t, err)
		return listed.SwitchError
	}
	client := NewUnixClient(c)
	t.Cleanup(func() { _ = client.Close() })
	sub, err := client.Subscribe(ctx, "local", "default", local)
	require.NoError(t, err)
	require.NoError(t, queue())
	<-sub.Targets()
	require.NoError(t, sub.Complete(ctx, errors.New("workspace no longer exists")))
	require.NoError(t, queue())
	<-sub.Targets()

	// Leaving to finish the switch elsewhere clears the earlier failure, and the
	// switch stays in progress until the controller is gone.
	require.ErrorContains(t, queue(), "already in progress")
	sub.Close()
	require.ErrorContains(t, queue(), "no active attach controller")
	assert.Empty(t, switchError())

	// A stream lost mid-switch cannot report the outcome.
	sub, err = client.Subscribe(ctx, "local", "default", local)
	require.NoError(t, err)
	require.NoError(t, queue())
	<-sub.Targets()
	require.NoError(t, client.Close())
	require.Eventually(t, func() bool { return switchError() == "attach controller disconnected during a switch" }, 2*time.Second, 10*time.Millisecond)
}

func TestSwitchRejectsMachinesTheControllerCannotReach(t *testing.T) {
	c := startDaemon(t, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := NewUnixClient(c)
	t.Cleanup(func() { _ = client.Close() })
	_, err := client.Subscribe(ctx, "local", "default", nil)
	require.ErrorContains(t, err, "reachable machines")
	_, err = client.Subscribe(ctx, "local", "default", []string{"--bad"})
	require.ErrorContains(t, err, "invalid machine id")
	_, err = client.Subscribe(ctx, "local", "default", local)
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

func TestStreamRejectsNullIDError(t *testing.T) {
	client, peer := net.Pipe()
	defer func() { _ = peer.Close() }()
	s := newStream(client)
	defer s.fail(errors.New("test done"))
	written := make(chan error, 1)
	go func() {
		_, err := readLine(bufio.NewReader(peer), maxRequest)
		if err == nil {
			err = json.NewEncoder(peer).Encode(rpcFailure(nil, codeParseError, "parse error"))
		}
		written <- err
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := s.call(ctx, Request{Op: "hello", Version: ProtocolVersion})
	require.ErrorContains(t, err, "daemon rejected stream: parse error")
	require.NoError(t, <-written)
	assert.ErrorContains(t, s.failure(), "parse error")
}

func TestSSHDiagnosticTruncatesAtUnicodeBoundary(t *testing.T) {
	c := &sshConn{waited: make(chan struct{})}
	close(c.waited)
	c.stderr.limit = 4096
	_, err := c.stderr.Write([]byte(strings.Repeat("a", 511) + "é"))
	require.NoError(t, err)
	assert.Equal(t, strings.Repeat("a", 511), c.diagnostic())
}

func TestUnsubscribeRecordsFailureAndRejectsFurtherSwitches(t *testing.T) {
	cfg := startDaemon(t, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := NewUnixClient(cfg)
	defer func() { _ = client.Close() }()
	sub, err := client.Subscribe(ctx, "local", "default", local)
	require.NoError(t, err)
	_, err = client.Call(ctx, Request{Op: "switch", MachineID: "local", WorkspaceID: "ws1"})
	require.NoError(t, err)
	select {
	case <-sub.Targets():
	case <-ctx.Done():
		t.Fatal("switch was not delivered")
	}
	sub.CloseWithFailure(errors.New("restore\nfailed"))
	response, err := client.Call(ctx, Request{Op: "list"})
	require.NoError(t, err)
	assert.Equal(t, "restorefailed", response.SwitchError)
	_, err = client.Call(ctx, Request{Op: "switch", MachineID: "local", WorkspaceID: "ws1"})
	require.ErrorContains(t, err, "no active attach controller")
	// A second close must not erase the failure or affect another subscription.
	sub.Close()
	next, err := client.Subscribe(ctx, "local", "default", local)
	require.NoError(t, err)
	next.Close()
	response, err = client.Call(ctx, Request{Op: "list"})
	require.NoError(t, err)
	assert.Equal(t, "restorefailed", response.SwitchError)
}

func TestSubscribeValidatesIdentityBeforeInstallingController(t *testing.T) {
	cfg := startDaemon(t, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := NewUnixClient(cfg)
	t.Cleanup(func() { _ = client.Close() })
	for _, identity := range []struct{ machine, session string }{
		{"wrong", "default"}, {"local", "wrong"}, {"", "default"}, {"local", ""},
	} {
		sub, err := client.Subscribe(ctx, identity.machine, identity.session, local)
		require.ErrorContains(t, err, "does not match")
		assert.Nil(t, sub)
		_, err = Call(ctx, cfg, Request{Op: "switch", MachineID: "local", WorkspaceID: "ws1"})
		require.ErrorContains(t, err, "no active attach controller")
	}
	sub, err := client.Subscribe(ctx, cfg.ID, cfg.HerdrSession, local)
	require.NoError(t, err)
	sub.Close()
}

func TestSubscribeRevalidatesIdentityAfterReconnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var dials atomic.Int32
	client := NewStreamClient("local", func(context.Context) (io.ReadWriteCloser, error) {
		cfg := Config{ID: "local", HerdrSession: "default"}
		if dials.Add(1) > 1 {
			cfg.HerdrSession = "replacement"
		}
		server := NewServer(cfg, &fakeClient{})
		conn, peer := net.Pipe()
		go server.serveConn(ctx, peer)
		return conn, nil
	})
	client.PingInterval = 0
	t.Cleanup(func() { _ = client.Close() })
	sub, err := client.Subscribe(ctx, "local", "default", local)
	require.NoError(t, err)
	sub.stream.fail(errors.New("connection lost"))
	_, err = client.Subscribe(ctx, "local", "default", local)
	require.ErrorContains(t, err, "does not match")
	require.Equal(t, int32(2), dials.Load())
	// The restarted daemon must not retain a controller for the old identity.
	replacement, err := client.Subscribe(ctx, "local", "replacement", local)
	require.NoError(t, err)
	replacement.Close()
}

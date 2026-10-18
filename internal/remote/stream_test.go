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

// fakeDaemon answers hello on conn, then hands every later request to handle.
func fakeDaemon(conn net.Conn, handle func(Request, *json.Encoder)) {
	defer func() { _ = conn.Close() }()
	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)
	for {
		var r Request
		if dec.Decode(&r) != nil {
			return
		}
		if r.Op == "hello" {
			_ = enc.Encode(Response{ID: r.ID, Version: ProtocolVersion})
			continue
		}
		handle(r, enc)
	}
}

func TestStreamMultiplexesOutOfOrderResponses(t *testing.T) {
	const calls = 3
	var mu sync.Mutex
	var held []Request
	c := NewStreamClient("fake", func(context.Context) (io.ReadWriteCloser, error) {
		client, server := net.Pipe()
		go fakeDaemon(server, func(r Request, enc *json.Encoder) {
			mu.Lock()
			defer mu.Unlock()
			held = append(held, r)
			if len(held) < calls {
				return
			}
			// Answer in reverse so ids, not arrival order, must route responses.
			for i := len(held) - 1; i >= 0; i-- {
				_ = enc.Encode(Response{ID: held[i].ID, Catalog: []Machine{{ID: held[i].WorkspaceID}}})
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
		go fakeDaemon(server, func(r Request, enc *json.Encoder) { _ = enc.Encode(Response{ID: r.ID}) })
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
			var r Request
			if json.NewDecoder(server).Decode(&r) == nil {
				_ = json.NewEncoder(server).Encode(Response{ID: r.ID, Version: ProtocolVersion + 1})
			}
		}()
		return client, nil
	})
	t.Cleanup(func() { _ = c.Close() })
	_, err := c.Call(context.Background(), Request{Op: "list"})
	require.ErrorContains(t, err, "handshake failed")
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
	sub, err := first.Subscribe(ctx)
	require.NoError(t, err)
	second := NewUnixClient(c)
	t.Cleanup(func() { _ = second.Close() })
	_, err = second.Subscribe(ctx)
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
		next, err = second.Subscribe(ctx)
		return err == nil
	}, 2*time.Second, 10*time.Millisecond)

	// Unsubscribing releases the controller but keeps the stream usable.
	next.Close()
	_, err = second.Call(ctx, Request{Op: "list"})
	require.NoError(t, err)
	third := NewUnixClient(c)
	t.Cleanup(func() { _ = third.Close() })
	_, err = third.Subscribe(ctx)
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
	sub, err := quiet.Subscribe(ctx)
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
		b, err := json.Marshal(r)
		require.NoError(t, err)
		_, err = inW.Write(append(b, '\n'))
		require.NoError(t, err)
		require.True(t, responses.Scan())
		var resp Response
		require.NoError(t, json.Unmarshal(responses.Bytes(), &resp))
		return resp
	}
	assert.Equal(t, ProtocolVersion, send(Request{ID: 1, Op: "hello", Version: ProtocolVersion}).Version)
	assert.Empty(t, send(Request{ID: 2, Op: "subscribe"}).Error)
	_, err := Call(ctx, c, Request{Op: "switch", MachineID: "local", WorkspaceID: "ws1"})
	require.NoError(t, err)
	require.True(t, responses.Scan())
	var push Response
	require.NoError(t, json.Unmarshal(responses.Bytes(), &push))
	assert.Equal(t, uint64(2), push.ID)
	require.NotNil(t, push.Switch)

	require.NoError(t, inW.Close())
	go func() { _, _ = io.Copy(io.Discard, outR) }()
	require.NoError(t, <-done)
	other := NewUnixClient(c)
	t.Cleanup(func() { _ = other.Close() })
	require.Eventually(t, func() bool { _, err := other.Subscribe(ctx); return err == nil }, 2*time.Second, 10*time.Millisecond)
}

func TestRelayRejectsOversizedRequests(t *testing.T) {
	c := startDaemon(t, 0)
	err := Relay(context.Background(), c, strings.NewReader(strings.Repeat("x", maxRequest+1)), io.Discard)
	require.ErrorContains(t, err, "exceeds")
}

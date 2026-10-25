package remote

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamWriteDeadlineClosesPartialRequest(t *testing.T) {
	client, server := net.Pipe()
	s := newStream(client)
	t.Cleanup(func() { s.fail(errClientClosed); _ = server.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := s.call(ctx, Request{Op: "ping"}); done <- err }()
	// Consume only a prefix: the incomplete frame must never be reused.
	require.NoError(t, server.SetReadDeadline(time.Now().Add(time.Second)))
	_, err := server.Read(make([]byte, 1))
	require.NoError(t, err)
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(time.Second):
		t.Fatal("stream write ignored the request deadline")
	}
	assert.False(t, s.alive())
	s.mu.Lock()
	defer s.mu.Unlock()
	assert.Empty(t, s.pending)
}

func TestStreamQueuedWriteCancellationPreservesActiveRequest(t *testing.T) {
	client, server := net.Pipe()
	blocked := &blockedWriteConn{Conn: client, entered: make(chan struct{})}
	s := newStream(blocked)
	t.Cleanup(func() { s.fail(errClientClosed); _ = server.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	first := make(chan error, 1)
	go func() { _, err := s.call(ctx, Request{Op: "ping"}); first <- err }()
	select {
	case <-blocked.entered:
	case <-time.After(time.Second):
		t.Fatal("first request never entered its write")
	}
	queuedCtx, cancelQueued := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancelQueued()
	queued := make(chan error, 1)
	go func() { _, err := s.call(queuedCtx, Request{Op: "focus"}); queued <- err }()
	select {
	case err := <-queued:
		require.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(time.Second):
		t.Fatal("queued request ignored cancellation while waiting to write")
	}
	require.True(t, s.alive(), "an unsent canceled request must not interrupt other calls")
	requests := make(chan string, 3)
	go fakeDaemon(server, func(r fakeRequest, enc *json.Encoder) {
		requests <- r.Method
		_ = enc.Encode(rpcReply(r.ID, r.Method, Response{}))
	})
	require.NoError(t, <-first)
	_, err := s.call(ctx, Request{Op: "list"})
	require.NoError(t, err)
	assert.Equal(t, "ping", <-requests)
	assert.Equal(t, "list", <-requests)
}

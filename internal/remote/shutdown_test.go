package remote

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type blockedWriteConn struct {
	net.Conn

	entered chan struct{}
	once    sync.Once
}

func (c *blockedWriteConn) Write(b []byte) (int, error) {
	c.once.Do(func() { close(c.entered) })
	return c.Conn.Write(b)
}

func TestServeShutdownClosesBackpressuredPeer(t *testing.T) {
	// Unix sockets need a shorter path than the test temp root provides.
	dir, err := os.MkdirTemp("/tmp", "sesh-shutdown-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	cfg := Config{Enabled: true, ID: "local", Socket: filepath.Join(dir, "daemon.sock"), Peers: []Peer{{ID: "other", SSH: "host"}}}
	s := NewServer(cfg, &fakeClient{})
	left, right := net.Pipe()
	t.Cleanup(func() { _ = right.Close() })
	blocked := &blockedWriteConn{Conn: left, entered: make(chan struct{})}
	s.peers["other"].current = newStream(blocked)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx) }()
	finished := false
	t.Cleanup(func() {
		cancel()
		_ = right.Close()
		if !finished {
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Error("Serve did not stop after closing the peer")
			}
		}
	})
	select {
	case <-blocked.entered:
	case <-time.After(time.Second):
		t.Fatal("peer refresh did not enter transport Write")
	}
	cancel()
	select {
	case err := <-done:
		finished = true
		require.NoError(t, err)
	case <-time.After(300 * time.Millisecond):
		t.Fatal("Serve cancellation blocked on a peer transport write")
	}
}

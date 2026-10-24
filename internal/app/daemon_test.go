package app

import (
	"context"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/fullerzz/herdr-plugin-sesh/internal/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDisabledManagedDaemonWaitsWithoutStartingServer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "remote.toml")
	socket := filepath.Join(t.TempDir(), "daemon.sock")
	t.Setenv("HERDR_SESH_REMOTE_CONFIG", path)
	t.Setenv("HERDR_BIN_PATH", "/missing/herdr")
	require.NoError(t, remote.SaveConfig(path, remote.Config{Socket: socket}))
	a := &App{Out: io.Discard, Err: io.Discard}
	require.ErrorContains(t, a.daemon(t.Context(), []string{"serve"}), "disabled")
	require.ErrorContains(t, a.daemon(t.Context(), []string{"serve", "--service", "extra"}), "usage")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- a.daemon(ctx, []string{"serve", "--service"}) }()
	select {
	case err := <-done:
		t.Fatalf("disabled service exited before cancellation: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	assert.NoFileExists(t, socket)
	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("disabled service did not exit after cancellation")
	}
}

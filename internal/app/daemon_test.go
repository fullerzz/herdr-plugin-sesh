package app

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fullerzz/herdr-plugin-sesh/internal/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDisabledManagedDaemonWaitsWithoutStartingServer(t *testing.T) {
	for _, kind := range []string{"disabled", "missing", "malformed"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "remote.toml")
			socket := filepath.Join(t.TempDir(), "daemon.sock")
			t.Setenv("HERDR_SESH_REMOTE_CONFIG", path)
			t.Setenv("HERDR_BIN_PATH", "/missing/herdr")
			switch kind {
			case "disabled":
				require.NoError(t, remote.SaveConfig(path, remote.Config{Socket: socket}))
			case "malformed":
				require.NoError(t, os.WriteFile(path, []byte("enabled = ["), 0o600))
				require.Error(t, (&App{Out: io.Discard, Err: io.Discard}).daemon(t.Context(), []string{"serve", "--service"}))
				return
			}
			a := &App{Out: io.Discard, Err: io.Discard}
			if kind == "missing" {
				require.ErrorIs(t, a.daemon(t.Context(), []string{"serve"}), os.ErrNotExist)
			} else {
				require.ErrorContains(t, a.daemon(t.Context(), []string{"serve"}), "disabled")
			}
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
			if kind == "disabled" {
				assert.NoFileExists(t, socket)
			}
			cancel()
			select {
			case err := <-done:
				require.NoError(t, err)
			case <-time.After(time.Second):
				t.Fatal("disabled service did not exit after cancellation")
			}
		})
	}
}

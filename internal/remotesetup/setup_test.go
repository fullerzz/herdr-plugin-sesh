package remotesetup

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/huh/v2"
	"github.com/fullerzz/herdr-plugin-sesh/internal/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMachineIDDefaultIsValid(t *testing.T) {
	for _, hostname := range []string{"Mac.local", "Zach’s Mac", "---", "", strings.Repeat("a", 200), "..machine"} {
		id := machineID(hostname)
		require.NoError(t, (remote.Config{Enabled: true, ID: id}).Validate())
	}
	assert.Equal(t, "Zach-s-Mac", machineID("Zach’s Mac"))
	assert.Equal(t, "machine", machineID("---"))
}

func TestConfigureAccessibleCollectsPeersAndRetriesInvalidInput(t *testing.T) {
	t.Setenv("ACCESSIBLE", "1")
	input := setupInput(t, "local\n\ny\nlocal\npeer\n-invalid\nuser@host\n\nn\n")
	var output bytes.Buffer
	cfg, err := Configure(context.Background(), remote.Config{}, input, &output)
	require.NoError(t, err)
	assert.Equal(t, remote.Config{Enabled: true, ID: "local", HerdrSession: "default", Peers: []remote.Peer{{ID: "peer", SSH: "user@host", Session: "default"}}}, cfg)
	assert.Contains(t, output.String(), "invalid or duplicate peer id")
	assert.Contains(t, output.String(), "invalid SSH destination")
}

func TestConfigureDefaultsAndRetainsInitialSettings(t *testing.T) {
	t.Setenv("ACCESSIBLE", "true")
	initial := remote.Config{ID: "local", Label: "Local machine", Socket: "/tmp/custom.sock", Peers: []remote.Peer{{ID: "peer", SSH: "peer"}}}
	cfg, err := Configure(context.Background(), initial, setupInput(t, "\n\nn\n"), io.Discard)
	require.NoError(t, err)
	assert.True(t, cfg.Enabled)
	assert.Equal(t, initial.Label, cfg.Label)
	assert.Equal(t, initial.Socket, cfg.Socket)
	assert.Equal(t, "default", cfg.HerdrSession)
	require.Len(t, cfg.Peers, 1)
	assert.Equal(t, "default", cfg.Peers[0].Session)
	assert.Empty(t, initial.Peers[0].Session)
	assert.False(t, initial.Enabled)
}

func TestConfigureAbortReturnsNoConfiguration(t *testing.T) {
	initial := remote.Config{ID: "local"}
	cfg, err := configure(initial, "host", func(...huh.Field) error { return huh.ErrUserAborted })
	require.ErrorIs(t, err, huh.ErrUserAborted)
	assert.Empty(t, cfg)
	assert.False(t, initial.Enabled)
}

func TestConfigureAccessibleEOFAndCancellationDoNotEnable(t *testing.T) {
	t.Setenv("ACCESSIBLE", "true")
	cfg, err := Configure(context.Background(), remote.Config{ID: "local"}, setupInput(t, "\n\n"), io.Discard)
	require.ErrorIs(t, err, io.EOF)
	assert.Empty(t, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, err = Configure(ctx, remote.Config{ID: "local"}, setupInput(t, "\n\nn\n"), io.Discard)
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, cfg)
}

func setupInput(t *testing.T, text string) *os.File {
	t.Helper()
	path := filepath.Join(t.TempDir(), "input")
	require.NoError(t, os.WriteFile(path, []byte(text), 0600))
	f, err := os.Open(path) // #nosec G304 -- test-owned temporary input.
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, f.Close()) })
	return f
}

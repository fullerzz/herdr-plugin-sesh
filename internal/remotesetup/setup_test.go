package remotesetup

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/fullerzz/herdr-plugin-sesh/internal/remote"
	"github.com/fullerzz/herdr-plugin-sesh/internal/uitheme"
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
	cfg, err := Configure(context.Background(), remote.Config{}, uitheme.Default(), input, &output)
	require.NoError(t, err)
	assert.Equal(t, remote.Config{Enabled: true, ID: "local", HerdrSession: "default", Peers: []remote.Peer{{ID: "peer", SSH: "user@host", Session: "default"}}}, cfg)
	assert.Contains(t, output.String(), "invalid or duplicate peer id")
	assert.Contains(t, output.String(), "invalid SSH destination")
}

func TestConfigureConfirmsEachPeerBeforeAddingAnotherOrContinuing(t *testing.T) {
	t.Setenv("ACCESSIBLE", "1")
	input := setupInput(t, "local\n\ny\nfirst\nfirst-host\n\ny\nsecond\nuser@second-host\nwork\nn\n")
	var output bytes.Buffer
	cfg, err := Configure(context.Background(), remote.Config{}, uitheme.Default(), input, &output)
	require.NoError(t, err)
	assert.Equal(t, []remote.Peer{
		{ID: "first", SSH: "first-host", Session: "default"},
		{ID: "second", SSH: "user@second-host", Session: "work"},
	}, cfg.Peers)
	transcript := output.String()
	for _, text := range []string{
		"Add an SSH peer before continuing?",
		`Peer "first" added to setup (SSH: first-host).`,
		"Add another SSH peer before continuing?",
		`Peer "second" added to setup (SSH: user@second-host).`,
		"Add another SSH peer before continuing?",
	} {
		_, rest, found := strings.Cut(transcript, text)
		require.True(t, found, "expected %q in remaining output: %s", text, transcript)
		transcript = rest
	}
}

func TestConfigureDefaultsAndRetainsInitialSettings(t *testing.T) {
	t.Setenv("ACCESSIBLE", "true")
	initial := remote.Config{ID: "local", Label: "Local machine", Socket: "/tmp/custom.sock", Peers: []remote.Peer{{ID: "peer", SSH: "peer"}}}
	cfg, err := Configure(context.Background(), initial, uitheme.Default(), setupInput(t, "\n\nn\n"), io.Discard)
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
	cfg, err := configure(initial, "host", uitheme.Default(), io.Discard, func(...huh.Field) error { return huh.ErrUserAborted })
	require.ErrorIs(t, err, huh.ErrUserAborted)
	assert.Empty(t, cfg)
	assert.False(t, initial.Enabled)
}

func TestConfigureAccessibleEOFAndCancellationDoNotEnable(t *testing.T) {
	t.Setenv("ACCESSIBLE", "true")
	cfg, err := Configure(context.Background(), remote.Config{ID: "local"}, uitheme.Default(), setupInput(t, "\n\n"), io.Discard)
	require.ErrorIs(t, err, io.EOF)
	assert.Empty(t, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, err = Configure(ctx, remote.Config{ID: "local"}, uitheme.Default(), setupInput(t, "\n\nn\n"), io.Discard)
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, cfg)
}

func TestConfigureUsesPaletteColors(t *testing.T) {
	t.Setenv("ACCESSIBLE", "1")
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	palette := uitheme.Default()
	palette.Accent = lipgloss.Color("#123456")
	palette.Green = lipgloss.Color("#456789")
	var output bytes.Buffer
	_, err := Configure(context.Background(), remote.Config{}, palette,
		setupInput(t, "local\n\ny\npeer\nhost\n\nn\n"), &output)
	require.NoError(t, err)
	assert.Contains(t, output.String(), lipgloss.NewStyle().Foreground(palette.Accent).Bold(true).Render("Machine ID"))
	assert.Contains(t, output.String(), lipgloss.NewStyle().Foreground(palette.Green).Bold(true).Render(`Peer "peer" added to setup (SSH: host).`))
}

func TestConfigureRespectsNoColor(t *testing.T) {
	t.Setenv("ACCESSIBLE", "1")
	t.Setenv("NO_COLOR", "1")
	var output bytes.Buffer
	_, err := Configure(context.Background(), remote.Config{}, uitheme.Default(),
		setupInput(t, "local\n\ny\npeer\nhost\n\nn\n"), &output)
	require.NoError(t, err)
	assert.NotContains(t, output.String(), "38;")
	assert.Contains(t, output.String(), `Peer "peer" added to setup (SSH: host).`)
}

func TestSetupInputsKeepThemeTextAfterBlur(t *testing.T) {
	palette := uitheme.Default()
	palette.Text = lipgloss.Color("#4c4f69")
	_, err := configure(remote.Config{ID: "local", HerdrSession: "default"}, "local", palette, io.Discard, func(fields ...huh.Field) error {
		for _, field := range fields {
			field.WithTheme(formTheme(palette))
			field.WithWidth(50)
			field.Focus()
			assert.Contains(t, field.View(), "38;2;76;79;105", "focused value must use theme text color")
			field.Blur()
			assert.Contains(t, field.View(), "38;2;76;79;105", "inactive value must keep theme text color")
			field.Focus()
			assert.Contains(t, field.View(), "38;2;76;79;105", "refocused value must keep theme text color")
		}
		return huh.ErrUserAborted
	})
	require.ErrorIs(t, err, huh.ErrUserAborted)
}

func TestSetupInputKeepsEditingAndValidationAfterUpdate(t *testing.T) {
	var value string
	rejected := errors.New("invalid session")
	input := newInput("Herdr session", "", &value, func(value string) error {
		if value != "work" {
			return rejected
		}
		return nil
	})
	input.WithKeyMap(huh.NewDefaultKeyMap())
	input.Focus()
	_, _ = input.Update(tea.PasteMsg{Content: "invalid"})
	_, cmd := input.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.ErrorIs(t, input.Error(), rejected)
	assert.Nil(t, cmd)
	_, _ = input.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	model, _ := input.Update(tea.PasteMsg{Content: "work"})
	assert.Same(t, input, model)
	assert.Equal(t, "work", value)
	_, cmd = input.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NoError(t, input.Error())
	assert.NotNil(t, cmd)
	input.Blur()
	assert.Contains(t, input.View(), "work")
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

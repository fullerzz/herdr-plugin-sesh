package settings

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/fullerzz/herdr-plugin-sesh/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func press(t *testing.T, m Model, code rune, mod tea.KeyMod) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(tea.KeyPressMsg{Code: code, Mod: mod})
	result, ok := next.(Model)
	require.True(t, ok)
	return result, cmd
}

func TestSettingsConfirmAndDiscard(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	original := []byte("version = 1\n[picker]\nshow_icons = false\n")
	require.NoError(t, os.WriteFile(path, original, 0600))
	doc, err := config.OpenSettings(config.LoadOptions{Path: path})
	require.NoError(t, err)
	m := newModel(doc)
	m, _ = press(t, m, tea.KeyEnter, 0)
	assert.Len(t, m.changes(), 1)
	m, _ = press(t, m, 's', tea.ModCtrl)
	assert.Equal(t, review, m.mode)
	assert.Contains(t, m.View().Content, "off → on")
	m, _ = press(t, m, tea.KeyEscape, 0)
	assert.Equal(t, form, m.mode)
	data, err := os.ReadFile(path) //nolint:gosec // Test-owned temporary file.
	require.NoError(t, err)
	assert.Equal(t, original, data)
	m, _ = press(t, m, tea.KeyEscape, 0)
	assert.Equal(t, discard, m.mode)
	m, _ = press(t, m, 'n', 0)
	assert.Equal(t, form, m.mode)
	m, _ = press(t, m, 's', tea.ModCtrl)
	m, cmd := press(t, m, 'y', 0)
	require.NotNil(t, cmd)
	next, _ := m.Update(cmd())
	m = next.(Model)
	assert.Empty(t, m.changes())
	cfg, _, err := config.Load(config.LoadOptions{Path: path})
	require.NoError(t, err)
	assert.True(t, cfg.TUI.ShowIcons)
}

func TestSettingsTextEditAndValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("version = 1\n"), 0600))
	doc, err := config.OpenSettings(config.LoadOptions{Path: path})
	require.NoError(t, err)
	m := newModel(doc)
	for i, f := range m.fields {
		if f.key == "naming.path_components" {
			m.cursor = i
		}
	}
	m, _ = press(t, m, tea.KeyEnter, 0)
	m.input.SetValue("0")
	m, _ = press(t, m, tea.KeyEnter, 0)
	assert.Equal(t, editing, m.mode)
	assert.Empty(t, m.changes())
	m, _ = press(t, m, tea.KeyEscape, 0) // dismiss validation error, keeping the editor
	m.input.SetValue("3")
	m, _ = press(t, m, tea.KeyEnter, 0)
	assert.Equal(t, form, m.mode)
	assert.Equal(t, 3, m.changes()["naming.path_components"])
	m, _ = press(t, m, 'r', tea.ModCtrl)
	assert.Empty(t, m.changes())
}

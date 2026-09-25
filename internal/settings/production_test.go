package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/fullerzz/herdr-plugin-sesh/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSettingsSaveIsAsyncAndReturnsToCaller(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("version = 1\n"), 0600))
	doc, err := config.OpenSettings(config.LoadOptions{Path: path})
	require.NoError(t, err)
	m := newModel(doc)
	m, _ = press(t, m, tea.KeyEnter, 0)
	m, _ = press(t, m, 's', tea.ModCtrl)
	m, cmd := press(t, m, 'y', 0)
	require.NotNil(t, cmd)
	cfg, _, err := config.Load(config.LoadOptions{Path: path})
	require.NoError(t, err)
	assert.False(t, cfg.TUI.ShowIcons, "confirmation must enqueue, not synchronously write")
	_, duplicate := press(t, m, 'y', 0)
	assert.Nil(t, duplicate)
	next, _ := m.Update(cmd())
	m = next.(Model)
	assert.Empty(t, m.changes())
	_, cmd = press(t, m, tea.KeyEscape, 0)
	require.NotNil(t, cmd)
	done, ok := cmd().(DoneMsg)
	require.True(t, ok)
	assert.True(t, done.Result.Saved)
	assert.Equal(t, doc.SelectedPath, done.Result.Path)
}

func TestSettingsViewFitsAndEscapesConfigText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("version = 1\n"), 0600))
	doc, err := config.OpenSettings(config.LoadOptions{Path: path})
	require.NoError(t, err)
	m := newModel(doc)
	m.fields[0].help = "unsafe \x1b[31mred\x1b[0m"
	for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 60, Height: 18}, {Width: 30, Height: 10}} {
		next, _ := m.Update(size)
		m = next.(Model)
		view := m.View().Content
		lines := strings.Split(view, "\n")
		assert.LessOrEqual(t, len(lines), size.Height)
		for _, line := range lines {
			assert.LessOrEqual(t, ansi.StringWidth(line), size.Width)
		}
		assert.NotContains(t, view, "\x1b[31m")
	}
}

func TestSettingsUntouchedEmptyListHasNoChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("version = 1\n"), 0600))
	m, err := Open(config.LoadOptions{Path: path}, nil)
	require.NoError(t, err)
	for i, f := range m.fields {
		if f.key == "list.blacklist" {
			m.cursor = i
		}
	}
	m, _ = press(t, m, tea.KeyEnter, 0)
	require.Equal(t, array, m.mode)
	m, _ = press(t, m, 's', tea.ModCtrl)
	assert.Empty(t, m.changes())
}

func TestSettingsArrayAndMultilineDrafts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	m, err := Open(config.LoadOptions{Path: path}, nil)
	require.NoError(t, err)
	for i, f := range m.fields {
		if f.key == "list.blacklist" {
			m.cursor = i
		}
	}
	m, _ = press(t, m, tea.KeyEnter, 0)
	require.Equal(t, array, m.mode)
	m, _ = press(t, m, 'a', 0)
	m.area.SetValue("[")
	m, _ = press(t, m, 's', tea.ModCtrl)
	m, _ = press(t, m, 's', tea.ModCtrl)
	assert.NotEmpty(t, m.problem)
	assert.Empty(t, m.changes())
	m, _ = press(t, m, tea.KeyEscape, 0)
	m, _ = press(t, m, tea.KeyEnter, 0)
	m.area.SetValue("^scratch$")
	m, _ = press(t, m, 's', tea.ModCtrl)
	m, _ = press(t, m, 's', tea.ModCtrl)
	assert.Equal(t, []string{"^scratch$"}, m.changes()["list.blacklist"])
	for i, f := range m.fields {
		if f.key == "workspace_defaults.startup" {
			m.cursor = i
		}
	}
	m, _ = press(t, m, tea.KeyEnter, 0)
	m.area.SetValue("echo first\necho second")
	m, _ = press(t, m, 's', tea.ModCtrl)
	assert.Equal(t, "echo first\necho second", m.changes()["workspace_defaults.startup"])
	m, _ = press(t, m, 's', tea.ModCtrl)
	m, cmd := press(t, m, 'y', 0)
	require.NotNil(t, cmd)
	next, _ := m.Update(cmd())
	m = next.(Model)
	assert.Empty(t, m.changes())
	cfg, _, err := config.Load(config.LoadOptions{Path: path})
	require.NoError(t, err)
	assert.Equal(t, "echo first\necho second", cfg.DefaultSessionConfig.StartupCommand)
	assert.Equal(t, []string{"^scratch$"}, cfg.Blacklist)
}

func TestSettingsConflictPreservesDraftAndReloads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("version=1\n"), 0600))
	m, err := Open(config.LoadOptions{Path: path}, nil)
	require.NoError(t, err)
	m, _ = press(t, m, tea.KeyEnter, 0)
	m, _ = press(t, m, 's', tea.ModCtrl)
	require.NoError(t, os.WriteFile(path, []byte("version=1\n# external\n"), 0600))
	m, cmd := press(t, m, 'y', 0)
	require.NotNil(t, cmd)
	next, _ := m.Update(cmd())
	m = next.(Model)
	assert.NotEmpty(t, m.changes())
	assert.True(t, m.conflict)
	m, _ = press(t, m, 'r', 0)
	assert.Equal(t, reloadConfirm, m.mode)
	m, cmd = press(t, m, 'y', 0)
	require.NotNil(t, cmd)
	next, _ = m.Update(cmd())
	m = next.(Model)
	assert.Empty(t, m.changes())
	assert.False(t, m.result.Saved)
}

func TestSettingsMigrationRequiresSeparateConfirmation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sesh.toml")
	require.NoError(t, os.WriteFile(path, []byte("dir_length=2\n"), 0600))
	m, err := Open(config.LoadOptions{Path: path}, nil)
	require.NoError(t, err)
	assert.Equal(t, migration, m.mode)
	_, cmd := press(t, m, 'n', 0)
	require.NotNil(t, cmd)
	_, err = os.Stat(filepath.Join(dir, "config.toml"))
	require.ErrorIs(t, err, os.ErrNotExist)
	m, cmd = press(t, m, 'y', 0)
	require.NotNil(t, cmd)
	next, _ := m.Update(cmd())
	m = next.(Model)
	assert.True(t, m.result.Saved)
	assert.Equal(t, form, m.mode)
	assert.Equal(t, 2, m.doc.Config.DirLength)
	assert.Contains(t, m.problem, "HERDR_SESH_CONFIG")
}

func TestSettingsMigrationFromSharedSeshDirTargetsPluginDir(t *testing.T) {
	home := t.TempDir()
	legacy := filepath.Join(home, ".config", "sesh", "sesh.toml")
	require.NoError(t, os.MkdirAll(filepath.Dir(legacy), 0700))
	require.NoError(t, os.WriteFile(legacy, []byte("dir_length=2\n"), 0600))
	m, err := Open(config.LoadOptions{Home: home, Env: map[string]string{"HERDR_SESH_CONFIG": legacy}}, nil)
	require.NoError(t, err)
	require.Equal(t, migration, m.mode)
	assert.Equal(t, filepath.Join(home, ".config", "herdr-sesh", config.NativeFileName), m.migration.NativePath)
}

func TestTextEditorPreservesTabs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	original := "cat <<-'EOF'\n\timportant\\t\n\tEOF"
	doc, err := config.OpenSettings(config.LoadOptions{Path: path})
	require.NoError(t, err)
	require.NoError(t, doc.Save(map[string]any{"workspace_defaults.startup": original}))
	m, err := Open(config.LoadOptions{Path: path}, nil)
	require.NoError(t, err)
	for i := range m.fields {
		if m.fields[i].key == "workspace_defaults.startup" {
			m.cursor = i
		}
	}
	m, _ = press(t, m, tea.KeyEnter, 0)
	m, _ = press(t, m, 's', tea.ModCtrl)
	assert.Equal(t, original, m.fields[m.cursor].value)
	assert.Empty(t, m.changes())
	m, _ = press(t, m, tea.KeyEnter, 0)
	pasted := "\n\tprintf '%s' '\\t'"
	next, _ := m.Update(tea.PasteMsg{Content: pasted})
	m = next.(Model)
	m, _ = press(t, m, 's', tea.ModCtrl)
	assert.Equal(t, original+pasted, m.fields[m.cursor].value)
	m, _ = press(t, m, 's', tea.ModCtrl)
	_, cmd := press(t, m, 'y', 0)
	require.NotNil(t, cmd)
	result := cmd().(savedMsg)
	require.NoError(t, result.err)
	cfg, _, err := config.Load(config.LoadOptions{Path: path})
	require.NoError(t, err)
	assert.Equal(t, original+pasted, cfg.DefaultSessionConfig.StartupCommand)
}

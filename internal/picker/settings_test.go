package picker

import (
	"context"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/fullerzz/herdr-plugin-sesh/internal/config"
	sessionmodel "github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/fullerzz/herdr-plugin-sesh/internal/settings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSettingsRoundTripRestoresPicker(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	items := []sessionmodel.Session{{Name: "alpha", Path: "/alpha"}, {Name: "beta", Path: "/beta"}}
	opts := Options{Context: context.Background(), HidePreview: true}
	opts.OpenSettings = func() (settings.Model, error) { return settings.Open(config.LoadOptions{Path: path}, nil) }
	opts.ReloadSettings = func(result settings.Result) (Options, ReloadResult, error) {
		assert.True(t, result.Saved)
		next := opts
		next.HidePath = true
		next.WorkspaceSort = "recent"
		return next, ReloadResult{Sessions: items}, nil
	}
	m := newTeaModel(items, opts)
	m.width, m.height = 80, 24
	m.input.SetValue("a")
	m.list.Filter("a")
	m.list.Selected = 1
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyF2})
	m = updated.(teaModel)
	require.NotNil(t, cmd)
	updated, _ = m.Update(cmd())
	m = updated.(teaModel)
	require.NotNil(t, m.settings)
	updated, cmd = m.Update(statusRefreshTickMsg{})
	assert.Nil(t, cmd, "background refresh must not run behind settings")
	m = updated.(teaModel)
	updated, cmd = m.Update(settings.DoneMsg{Result: settings.Result{Path: path, Saved: true}})
	require.NotNil(t, cmd)
	m = updated.(teaModel)
	updated, _ = m.Update(cmd())
	m = updated.(teaModel)
	assert.Nil(t, m.settings)
	assert.True(t, m.hidePath)
	assert.Equal(t, "recent", m.workspaceSort)
	assert.Equal(t, "a", m.list.Query)
	current, ok := m.list.Current()
	require.True(t, ok)
	assert.Equal(t, "beta", current.Name)
}

func TestSettingsShortcutDoesNotStealPreviewBinding(t *testing.T) {
	binding := "f2"
	m := newTeaModel(nil, Options{HidePreview: true, CyclePreviewModeKey: &binding, OpenSettings: func() (settings.Model, error) { return settings.Model{}, nil }})
	assert.Equal(t, "ctrl+,", m.settingsKey())
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyF2})
	assert.Nil(t, cmd)
	assert.False(t, next.(teaModel).settingsBusy)
}

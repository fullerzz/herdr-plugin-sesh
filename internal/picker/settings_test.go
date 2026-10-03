package picker

import (
	"context"
	"path/filepath"
	"testing"
	"time"

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
	opts := Options{DisplayOptions: DisplayOptions{Context: context.Background(), HidePreview: true}}
	backend := &fakeBackend{}
	opts.Backend = backend
	backend.openSettings = func() (settings.Model, error) { return settings.Open(config.LoadOptions{Path: path}, nil) }
	backend.reloadSettings = func(_ context.Context, result settings.Result) (DisplayOptions, ReloadResult, error) {
		assert.True(t, result.Saved)
		next := opts.DisplayOptions
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
	assert.Same(t, backend, m.backend)
	updated, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyF2})
	require.NotNil(t, cmd, "settings remains callable after save")
	assert.True(t, updated.(teaModel).settingsBusy)
	assert.True(t, m.hidePath)
	assert.Equal(t, "recent", m.workspaceSort)
	assert.Equal(t, "a", m.list.Query)
	current, ok := m.list.Current()
	require.True(t, ok)
	assert.Equal(t, "beta", current.Name)
}

func TestSettingsShortcutDoesNotStealPreviewBinding(t *testing.T) {
	binding := "f2"
	m := newTeaModel([]sessionmodel.Session{{Name: "alpha", Path: "/alpha"}}, Options{DisplayOptions: DisplayOptions{CyclePreviewModeKey: &binding}, Backend: &fakeBackend{openSettings: func() (settings.Model, error) { return settings.Model{}, nil }}})
	assert.Equal(t, "ctrl+,", m.settingsKey())
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyF2})
	require.NotNil(t, cmd)
	assert.False(t, next.(teaModel).settingsBusy)
	assert.True(t, next.(teaModel).panePreview)
}

func TestSettingsReloadCanBeCancelled(t *testing.T) {
	started := make(chan context.Context, 1)
	m := newTeaModel(nil, Options{DisplayOptions: DisplayOptions{HidePreview: true}, Backend: &fakeBackend{reloadSettings: func(ctx context.Context, _ settings.Result) (DisplayOptions, ReloadResult, error) {
		started <- ctx
		<-ctx.Done()
		return DisplayOptions{}, ReloadResult{}, ctx.Err()
	}}})
	m.settings = &settings.Model{}
	next, reload := m.Update(settings.DoneMsg{Result: settings.Result{Saved: true}})
	m = next.(teaModel)
	require.NotNil(t, reload)
	require.NotNil(t, m.settingsCancel)
	t.Cleanup(m.settingsCancel)
	finished := make(chan tea.Msg, 1)
	go func() { finished <- reload() }()
	var ctx context.Context
	select {
	case ctx = <-started:
	case <-time.After(time.Second):
		t.Fatal("reload did not start")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m = next.(teaModel)
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	select {
	case result := <-finished:
		_, quit := m.Update(result)
		require.NotNil(t, quit)
		assert.IsType(t, tea.QuitMsg{}, quit())
	case <-time.After(time.Second):
		t.Fatal("reload did not stop after Ctrl+C")
	}
}

func TestSettingsOpeningKeepsQuitKeyActive(t *testing.T) {
	m := newTeaModel(nil, Options{DisplayOptions: DisplayOptions{HidePreview: true}})
	m.settingsBusy = true
	_, quit := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	require.NotNil(t, quit)
	assert.IsType(t, tea.QuitMsg{}, quit())
}

func TestSettingsSaveRetainsBackendOperationsAndRefreshesMetadata(t *testing.T) {
	items := []sessionmodel.Session{{Source: "herdr", Name: "alpha", WorkspaceID: "w1"}, {Source: "herdr", Name: "beta", WorkspaceID: "w2"}}
	closed := ""
	refreshed := false
	backend := &fakeBackend{
		closeWorkspace:       func(_ context.Context, id string) error { closed = id; return nil },
		refreshAgentStatuses: func() (map[string]string, error) { refreshed = true; return map[string]string{"w1": "idle"}, nil },
		reloadPicker: func(context.Context) (ReloadResult, error) {
			return ReloadResult{Sessions: []sessionmodel.Session{items[1]}, RecentWorkspaceIDs: []string{"w2"}}, nil
		},
	}
	m := newTeaModel(items, Options{DisplayOptions: DisplayOptions{HidePreview: true, RecentWorkspaceIDs: []string{"w1"}}, Backend: backend})
	m.settings = &settings.Model{}
	updated, _ := m.Update(settingsReloadedMsg{options: DisplayOptions{HidePreview: true, WorkspaceSort: "recent"}, result: ReloadResult{Sessions: items, RecentWorkspaceIDs: []string{"w2", "w1"}, LastWorkspaceID: "w2", HerdrWorkspaces: items}})
	m = updated.(teaModel)
	assert.Same(t, backend, m.backend)
	assert.Equal(t, []string{"w2", "w1"}, m.recentWorkspaceIDs)
	assert.Equal(t, "w2", m.lastWorkspaceID)
	assert.Len(t, m.herdrWorkspaces, 2)
	assert.Equal(t, []string{"beta", "alpha"}, sessionNames(m.list.All))
	_, statusCmd := m.Update(statusRefreshTickMsg{generation: m.refreshGeneration})
	require.NotNil(t, statusCmd)
	statusCmd()
	assert.True(t, refreshed)
	updated, closeCmd := m.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	require.NotNil(t, closeCmd)
	m = updated.(teaModel)
	updated, _ = m.Update(closeCmd())
	m = updated.(teaModel)
	assert.Equal(t, "w1", closed)
	assert.Equal(t, []string{"w2"}, m.recentWorkspaceIDs)
}

func TestSettingsReloadErrorRetainsPickerAndBackend(t *testing.T) {
	backend := &fakeBackend{}
	m := newTeaModel([]sessionmodel.Session{{Name: "alpha"}}, Options{DisplayOptions: DisplayOptions{HidePreview: true}, Backend: backend})
	m.settings = &settings.Model{}
	updated, _ := m.Update(settingsReloadedMsg{err: context.Canceled})
	m = updated.(teaModel)
	assert.Same(t, backend, m.backend)
	assert.Equal(t, []string{"alpha"}, sessionNames(m.list.All))
	assert.Contains(t, m.closeError, "picker reload failed")
}

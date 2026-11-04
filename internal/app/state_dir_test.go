package app

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
	"github.com/fullerzz/herdr-plugin-sesh/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStateDirsKeepEachBackendApart(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("HERDR_PLUGIN_STATE_DIR", "/plugin/state")
	t.Setenv("HERDR_SOCKET_PATH", "")

	t.Setenv("HERDR_SESH_BACKEND", "app")
	t.Setenv("XDG_STATE_HOME", "/xdg/state")
	dir, err := historyStateDir()
	require.NoError(t, err)
	assert.Equal(t, "/xdg/state/herdr-sesh/herdr-app", dir, "a forced app backend ignores the Herdr plugin state")
	assert.Equal(t, "/xdg/state/herdr-sesh/herdr-app", sessionStateDir())

	t.Setenv("XDG_STATE_HOME", "")
	dir, err = historyStateDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".local", "state", "herdr-sesh", "herdr-app"), dir)

	t.Setenv("HERDR_SESH_BACKEND", "herdr")
	dir, err = historyStateDir()
	require.NoError(t, err)
	assert.Equal(t, "/plugin/state", dir)
	assert.Equal(t, "/plugin/state", sessionStateDir())
}

func TestAppSwitchRecordsOriginWithoutHistoryWatcher(t *testing.T) {
	t.Setenv("HERDR_SESH_BACKEND", "app")
	dir := t.TempDir()
	require.NoError(t, state.SaveHistory(dir, state.History{Workspaces: []string{"B", "A"}}))
	require.NoError(t, recordSwitch(dir, "C", "B", 0))
	h, err := state.LoadHistory(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"B", "C", "A"}, h.Workspaces)
}

type liveSpaces struct {
	herdr.Backend

	ids []string
}

func (l liveSpaces) WorkspaceList(context.Context) ([]herdr.Workspace, error) {
	workspaces := make([]herdr.Workspace, 0, len(l.ids))
	for _, id := range l.ids {
		workspaces = append(workspaces, herdr.Workspace{ID: id})
	}
	return workspaces, nil
}

func TestAppLastSkipsSpacesClosedInTheApp(t *testing.T) {
	t.Setenv("HERDR_SESH_BACKEND", "app")
	dir := t.TempDir()
	require.NoError(t, state.SaveHistory(dir, state.History{Workspaces: []string{"C", "B", "A"}}))

	h, err := loadLastHistory(t.Context(), liveSpaces{ids: []string{"A", "C"}}, dir)
	require.NoError(t, err)
	id, ok := h.PreviousWorkspace("C")
	require.True(t, ok)
	assert.Equal(t, "A", id)

	saved, err := state.LoadHistory(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"C", "A"}, saved.Workspaces)
}

func TestAppRecordSwitchWritesAppHistoryUnderAnyBackend(t *testing.T) {
	t.Setenv("HERDR_SESH_BACKEND", "herdr")
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	dir, err := appStateDir()
	require.NoError(t, err)
	require.NoError(t, state.SaveHistory(dir, state.History{Workspaces: []string{"A", "B"}}))

	require.NoError(t, New().Run(t.Context(), []string{"app", "record-switch", "A", "buntu26/sp1"}))

	h, err := state.LoadHistory(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"buntu26/sp1", "A", "B"}, h.Workspaces)
	assert.Error(t, New().Run(t.Context(), []string{"app", "record-switch", "A"}))
}

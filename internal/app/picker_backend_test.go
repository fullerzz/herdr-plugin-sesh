package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/fullerzz/herdr-plugin-sesh/internal/config"
	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/fullerzz/herdr-plugin-sesh/internal/settings"
	"github.com/fullerzz/herdr-plugin-sesh/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPickerBackendClosePrunesHistoryAndFallback(t *testing.T) {
	configureHerdrScript(t, "#!/bin/sh\nexit 0\n")
	dir := t.TempDir()
	require.NoError(t, state.SaveHistory(dir, state.History{Workspaces: []string{"w1", "w2"}}))
	b := &pickerBackend{app: &App{}, cfg: config.Default(), client: herdr.NewCLIClient(), historyDir: dir, herdrWorkspaces: []model.Session{{WorkspaceID: "w1"}, {WorkspaceID: "w2", Worktree: model.WorktreeRelation{Linked: true, ParentWorkspaceID: "w1", ParentWorkspaceName: "parent"}}}}
	require.NoError(t, b.CloseWorkspace(context.Background(), "w1"))
	history, err := state.LoadHistory(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"w2"}, history.Workspaces)
	require.Len(t, b.herdrWorkspaces, 1)
	assert.Equal(t, "w2", b.herdrWorkspaces[0].WorkspaceID)
	assert.Empty(t, b.herdrWorkspaces[0].Worktree.ParentWorkspaceID)
	assert.Empty(t, b.herdrWorkspaces[0].Worktree.ParentWorkspaceName)
	configureFakeSources(t, "")
	result, err := b.ReloadPicker(context.Background())
	require.Error(t, err)
	assert.Equal(t, b.herdrWorkspaces, result.HerdrWorkspaces)
}

func TestPickerBackendSettingsCommitsOnlySuccessfulReload(t *testing.T) {
	configureFakeSources(t, "")
	configureHerdrScript(t, `#!/bin/sh
case "$1 $2" in
"pane current") printf '{"id":"p1","workspace_id":"w2"}\n' ;;
*) exit 1 ;;
esac
`)
	dir := t.TempDir()
	require.NoError(t, state.SaveHistory(dir, state.History{Workspaces: []string{"w2", "w3"}}))
	path := filepath.Join(t.TempDir(), "settings.toml")
	require.NoError(t, os.WriteFile(path, []byte("dir_length = 3\n[tui]\nshow_last_workspace = false\n"), 0600))
	cfg := config.Default()
	b := &pickerBackend{app: &App{}, ctx: context.Background(), client: herdr.NewCLIClient(), cfg: cfg, activeConfigPath: "original", historyDir: dir, herdrWorkspaces: []model.Session{{Source: "herdr", Name: "api", WorkspaceID: "w2"}}}
	_, _, err := b.ReloadSettings(context.Background(), settings.Result{Path: path + "-missing"})
	require.Error(t, err)
	assert.Equal(t, "original", b.activeConfigPath)
	assert.Equal(t, cfg, b.cfg)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = b.ReloadSettings(ctx, settings.Result{Path: path})
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, "original", b.activeConfigPath)
	opts, result, err := b.ReloadSettings(context.Background(), settings.Result{Path: path})
	require.NoError(t, err)
	assert.Equal(t, path, b.activeConfigPath)
	assert.Equal(t, 3, b.cfg.DirLength)
	assert.Equal(t, []string{"w2", "w2", "w3"}, result.RecentWorkspaceIDs)
	assert.Equal(t, result.RecentWorkspaceIDs, opts.RecentWorkspaceIDs)
	assert.Equal(t, b.herdrWorkspaces, opts.HerdrWorkspaces)
	assert.True(t, opts.HideLastWorkspace)
	require.NotEmpty(t, b.warnings)
}

package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

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

func TestPickerBackendSettingsTracksSavedPathAndCommitsConfigOnSuccess(t *testing.T) {
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
	require.NoError(t, os.WriteFile(path, []byte("version = 1\n[naming]\npath_components = 3\n[picker]\nshow_last_workspace = false\n"), 0600))
	invalidPath := filepath.Join(t.TempDir(), "invalid.toml")
	require.NoError(t, os.WriteFile(invalidPath, []byte("[invalid"), 0600))
	cfg := config.Default()
	b := &pickerBackend{app: &App{}, ctx: context.Background(), client: herdr.NewCLIClient(), cfg: cfg, activeConfigPath: "original", historyDir: dir, herdrWorkspaces: []model.Session{{Source: "herdr", Name: "api", WorkspaceID: "w2"}}}
	_, _, err := b.ReloadSettings(context.Background(), settings.Result{Path: invalidPath, Saved: true})
	require.Error(t, err)
	assert.Equal(t, invalidPath, b.activeConfigPath)
	assert.Equal(t, cfg, b.cfg)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = b.ReloadSettings(ctx, settings.Result{Path: path, Saved: true})
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, path, b.activeConfigPath)
	assert.Equal(t, cfg, b.cfg)
	_, err = b.OpenSettings()
	require.NoError(t, err, "reopens the saved file despite the canceled reload")
	opts, result, err := b.ReloadSettings(context.Background(), settings.Result{Path: path, Saved: true})
	require.NoError(t, err)
	assert.Equal(t, path, b.activeConfigPath)
	assert.Equal(t, 3, b.cfg.DirLength)
	assert.Equal(t, []string{"w2", "w2", "w3"}, result.RecentWorkspaceIDs)
	assert.Equal(t, result.RecentWorkspaceIDs, opts.RecentWorkspaceIDs)
	assert.Equal(t, b.herdrWorkspaces, opts.HerdrWorkspaces)
	assert.True(t, opts.HideLastWorkspace)
	require.NotEmpty(t, b.warnings)
}

func TestPickerBackendCanceledSettingsReloadPreservesRuntimeState(t *testing.T) {
	configureFakeSources(t, "")
	marker := filepath.Join(t.TempDir(), "focus-started")
	t.Setenv("FOCUS_MARKER", marker)
	configureHerdrScript(t, `#!/bin/sh
case "$1 $2" in
"workspace list") printf '[{"id":"new","label":"new","cwd":"/new"}]\n' ;;
"pane list") printf '[]\n' ;;
"pane current") touch "$FOCUS_MARKER"; exec sleep 10 ;;
*) exit 1 ;;
esac
`)
	path := filepath.Join(t.TempDir(), "saved.toml")
	require.NoError(t, os.WriteFile(path, []byte("version = 1\n[naming]\npath_components = 3\n"), 0600))
	old := []model.Session{{Source: "herdr", WorkspaceID: "old"}}
	cfg := config.Default()
	b := &pickerBackend{app: &App{}, ctx: context.Background(), client: herdr.NewCLIClient(), cfg: cfg, activeConfigPath: "original", pickerWorkspaceID: "old", herdrWorkspaces: old}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, _, err := b.ReloadSettings(ctx, settings.Result{Path: path, Saved: true})
		done <- err
	}()
	require.Eventually(t, func() bool { _, err := os.Stat(marker); return err == nil }, 5*time.Second, time.Millisecond)
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	assert.Equal(t, path, b.activeConfigPath)
	assert.Equal(t, cfg, b.cfg)
	assert.Equal(t, old, b.herdrWorkspaces)
	assert.Equal(t, "old", b.pickerWorkspaceID)
}

func TestPickerBackendInitializeMetadata(t *testing.T) {
	for _, showLast := range []bool{true, false} {
		name := "hidden"
		if showLast {
			name = "shown"
		}
		t.Run(name, func(t *testing.T) {
			configureFakeSources(t, "")
			configureHerdrScript(t, `#!/bin/sh
case "$1 $2" in
"workspace list") printf '[{"id":"current","label":"api","cwd":"/api"}]\n' ;;
"pane list") printf '[]\n' ;;
*) exit 1 ;;
esac
`)
			dir := t.TempDir()
			t.Setenv("HERDR_PLUGIN_STATE_DIR", dir)
			t.Setenv("HERDR_SOCKET_PATH", "")
			require.NoError(t, state.SaveHistory(dir, state.History{Workspaces: []string{"current", "previous"}}))
			cfg := config.Default()
			cfg.TUI.ShowLastWorkspace = showLast
			b := &pickerBackend{app: &App{}, ctx: context.Background(), client: herdr.NewCLIClient(), cfg: cfg, pickerWorkspaceID: "current"}
			sessions, opts, err := b.initialize(context.Background())
			require.NoError(t, err)
			want := []model.Session{{Source: "herdr", Name: "api", Path: "/api", WorkspaceID: "current"}}
			assert.Equal(t, want, sessions)
			assert.Equal(t, want, opts.HerdrWorkspaces)
			assert.Equal(t, want, b.herdrWorkspaces)
			assert.Equal(t, []string{"current", "current", "previous"}, opts.RecentWorkspaceIDs)
			assert.Equal(t, !showLast, opts.HideLastWorkspace)
			assert.False(t, opts.LastWorkspaceUnknown)
			if showLast {
				assert.Equal(t, "previous", opts.LastWorkspaceID)
			} else {
				assert.Empty(t, opts.LastWorkspaceID)
			}
		})
	}
}

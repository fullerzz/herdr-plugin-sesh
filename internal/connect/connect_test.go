package connect

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/fullerzz/herdr-plugin-sesh/internal/config"
	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/fullerzz/herdr-plugin-sesh/internal/sources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnectFocusesExistingWorkspace(t *testing.T) {
	f := &herdr.FakeClient{}
	_, err := Connect(context.Background(), f, []model.Session{{Name: "api", WorkspaceID: "ws1"}}, "api", Options{})
	require.NoError(t, err)
	require.Len(t, f.FocusedWorkspaces, 1)
	require.Equal(t, "ws1", f.FocusedWorkspaces[0])
	assert.Empty(t, f.CreatedWorkspaces)
}

func TestConnectCreatesWorkspaceForConfigSession(t *testing.T) {
	f := &herdr.FakeClient{}
	res, err := Connect(context.Background(), f, []model.Session{{Source: "config", Name: "api", Path: "/tmp/api"}}, "api", Options{})
	require.NoError(t, err)
	require.True(t, res.Created)
	require.Len(t, f.CreatedWorkspaces, 1)
	require.Equal(t, "/tmp/api", f.CreatedWorkspaces[0].CWD)
	assert.Equal(t, "api", f.CreatedWorkspaces[0].Label)
}

func TestConnectNoFocusScopesStartupCommandToCreatedWorkspace(t *testing.T) {
	f := &herdr.FakeClient{
		Workspaces: []herdr.Workspace{{ID: "existing-workspace"}},
		Panes: []herdr.Pane{
			{ID: "existing-pane", WorkspaceID: "existing-workspace"},
			{ID: "new-pane", WorkspaceID: "new-workspace"},
		},
	}
	session := model.Session{Source: "config", Name: "api", Path: "/tmp/api", StartupCommand: "echo ready"}
	_, err := Connect(context.Background(), f, []model.Session{session}, "api", Options{NoFocus: true})
	require.NoError(t, err)
	require.Len(t, f.CreatedWorkspaces, 1)
	require.False(t, f.CreatedWorkspaces[0].Focus)
	require.Len(t, f.PaneRuns, 1)
	assert.Equal(t, "new-pane:echo ready", f.PaneRuns[0])
}

func TestConnectFocusedScopesStartupCommandToCreatedWorkspace(t *testing.T) {
	f := &herdr.FakeClient{
		Workspaces: []herdr.Workspace{{ID: "existing-workspace"}},
		Panes: []herdr.Pane{
			{ID: "existing-pane", WorkspaceID: "existing-workspace"},
			{ID: "new-pane", WorkspaceID: "new-workspace"},
		},
	}
	session := model.Session{Source: "config", Name: "api", Path: "/tmp/api", StartupCommand: "echo ready"}
	_, err := Connect(context.Background(), f, []model.Session{session}, "api", Options{})
	require.NoError(t, err)
	require.Len(t, f.CreatedWorkspaces, 1)
	require.True(t, f.CreatedWorkspaces[0].Focus)
	require.Len(t, f.PaneRuns, 1)
	assert.Equal(t, "new-pane:echo ready", f.PaneRuns[0])
}

func TestConnectUsesExpandedConfigSessionPath(t *testing.T) {
	cfg := config.Config{SessionConfigs: []config.SessionConfig{{Name: "api", Path: "~/projects/api"}}}
	got, err := sources.ConfigSessions{Config: cfg, Home: "/home/zach"}.List(context.Background())
	require.NoError(t, err)
	f := &herdr.FakeClient{}
	_, err = Connect(context.Background(), f, got.Ordered(), "api", Options{})
	require.NoError(t, err)
	require.Len(t, f.CreatedWorkspaces, 1)
	assert.Equal(t, "/home/zach/projects/api", f.CreatedWorkspaces[0].CWD)
}

func TestConnectExistingWorkspaceSkipsPaneLayout(t *testing.T) {
	f := &herdr.FakeClient{}
	session := model.Session{Name: "api", WorkspaceID: "ws1", StartupCommand: "echo hi", WindowConfigs: []model.WindowConfig{{Name: "dev", Panes: []model.PaneConfig{
		{Name: "a", Startup: "nvim"},
		{Name: "b", SplitFrom: "a", Split: "right"},
	}}}}
	_, err := Connect(context.Background(), f, []model.Session{session}, "api", Options{})
	require.NoError(t, err)
	assert.Empty(t, f.CreatedTabs)
	assert.Empty(t, f.RenamedTabs)
	assert.Empty(t, f.Splits)
	assert.Empty(t, f.PaneRuns)
}

func TestConnectReusesInitialTabWithoutMovingFocus(t *testing.T) {
	for _, noFocus := range []bool{true, false} {
		f := &herdr.FakeClient{}
		session := model.Session{Name: "api", Path: "/tmp/api", WindowConfigs: []model.WindowConfig{
			{Name: "dev", Panes: []model.PaneConfig{
				{Name: "a", Path: "/tmp/api", Env: map[string]string{"EDITOR": "nvim"}},
				{Name: "b", SplitFrom: "a", Split: "right", Path: "/tmp/api/web", Startup: "nvim"},
			}},
			{Name: "git"},
		}}
		_, err := Connect(context.Background(), f, []model.Session{session}, "api", Options{NoFocus: noFocus})
		require.NoError(t, err)
		require.Len(t, f.CreatedWorkspaces, 1)
		assert.Equal(t, herdr.WorkspaceCreateRequest{CWD: "/tmp/api", Label: "api", Env: map[string]string{"EDITOR": "nvim"}, Focus: !noFocus}, f.CreatedWorkspaces[0])
		assert.Equal(t, []string{"initial-tab:dev"}, f.RenamedTabs)
		require.Len(t, f.CreatedTabs, 1, "exactly the configured tabs exist")
		assert.False(t, f.CreatedTabs[0].Focus)
		assert.Empty(t, f.FocusedTabs)
		require.Len(t, f.Splits, 1)
		assert.Equal(t, "initial-pane", f.Splits[0].PaneID)
		assert.Equal(t, []string{"split-1:nvim"}, f.PaneRuns)
	}
}

func TestConnectKeepsInitialTabWhenRootPaneLeavesWorkspacePath(t *testing.T) {
	for _, noFocus := range []bool{false, true} {
		for _, startup := range []string{"", "echo ready"} {
			t.Run(fmt.Sprintf("noFocus=%t/startup=%q", noFocus, startup), func(t *testing.T) {
				f := &herdr.FakeClient{}
				path := t.TempDir()
				rootPath := filepath.Join(path, "web")
				session := model.Session{Source: "config", Name: "api", Path: path, StartupCommand: startup, WindowConfigs: []model.WindowConfig{{Name: "dev", Panes: []model.PaneConfig{
					{Name: "a", Path: rootPath, Env: map[string]string{"EDITOR": "nvim"}},
				}}}}
				_, err := Connect(context.Background(), f, []model.Session{session}, "api", Options{NoFocus: noFocus})
				require.NoError(t, err)
				require.Len(t, f.CreatedWorkspaces, 1)
				assert.Equal(t, herdr.WorkspaceCreateRequest{CWD: path, Label: "api", Focus: !noFocus}, f.CreatedWorkspaces[0])
				assert.Empty(t, f.RenamedTabs)
				require.Len(t, f.CreatedTabs, 1)
				assert.Equal(t, rootPath, f.CreatedTabs[0].CWD)
				assert.False(t, f.CreatedTabs[0].Focus)
				// The workspace keeps reporting its configured path, so path lookup reconnects.
				live, err := sources.HerdrWorkspaces{Client: f}.List(context.Background())
				require.NoError(t, err)
				result, err := Connect(context.Background(), f, live.Ordered(), path, Options{NoFocus: noFocus})
				require.NoError(t, err)
				assert.False(t, result.Created)
				assert.Len(t, f.CreatedWorkspaces, 1)
			})
		}
	}
}

func TestConnectWorkspaceStartupKeepsSeparateInitialTab(t *testing.T) {
	f := &herdr.FakeClient{}
	session := model.Session{Name: "api", Path: "/tmp/api", StartupCommand: "echo ready", WindowConfigs: []model.WindowConfig{{Name: "dev", Panes: []model.PaneConfig{
		{Name: "a", Path: "/tmp/api/web", Env: map[string]string{"EDITOR": "nvim"}, Startup: "nvim"},
	}}}}
	_, err := Connect(context.Background(), f, []model.Session{session}, "api", Options{NoFocus: true})
	require.NoError(t, err)
	require.Len(t, f.CreatedWorkspaces, 1)
	assert.Equal(t, herdr.WorkspaceCreateRequest{CWD: "/tmp/api", Label: "api"}, f.CreatedWorkspaces[0])
	assert.Empty(t, f.RenamedTabs)
	require.Len(t, f.CreatedTabs, 1)
	assert.Equal(t, herdr.TabCreateRequest{WorkspaceID: "new-workspace", CWD: "/tmp/api/web", Label: "dev", Env: map[string]string{"EDITOR": "nvim"}}, f.CreatedTabs[0])
	assert.Equal(t, []string{"initial-pane:echo ready", "new-pane:nvim"}, f.PaneRuns)
}

func TestConnectRootStartupChangingDirectoryPreservesReconnect(t *testing.T) {
	for _, kind := range []string{"tab", "pane"} {
		for _, noFocus := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/noFocus=%t", kind, noFocus), func(t *testing.T) {
				f := &herdr.FakeClient{}
				path := t.TempDir()
				command := "cd web && npm run dev"
				tab := model.WindowConfig{Name: "dev", StartupScript: command}
				if kind == "pane" {
					tab.StartupScript = ""
					tab.Panes = []model.PaneConfig{{Name: "root", Path: path, Startup: command}}
				}
				session := model.Session{Name: "api", Path: path, WindowConfigs: []model.WindowConfig{tab}}
				_, err := Connect(context.Background(), f, []model.Session{session}, "api", Options{NoFocus: noFocus})
				require.NoError(t, err)
				require.Len(t, f.Workspaces, 1)
				// Model the command changing cwd after layout creation. Herdr reports
				// it as ForegroundCWD only if the command's tab is active.
				if len(f.RenamedTabs) > 0 || (len(f.Tabs) > 0 && f.Workspaces[0].ActiveTabID == f.Tabs[0].ID) {
					f.Workspaces[0].ForegroundCWD = filepath.Join(path, "web")
				}
				live, err := sources.HerdrWorkspaces{Client: f}.List(context.Background())
				require.NoError(t, err)
				result, err := Connect(context.Background(), f, live.Ordered(), path, Options{NoFocus: noFocus})
				require.NoError(t, err)
				assert.False(t, result.Created)
				assert.Len(t, f.CreatedWorkspaces, 1)
				assert.Equal(t, []string{"new-pane:" + command}, f.PaneRuns)
			})
		}
	}
}

type workspaceStartupCWDClient struct{ herdr.FakeClient }

func (f *workspaceStartupCWDClient) PaneRun(ctx context.Context, id, cmd string) error {
	if id == "initial-pane" {
		// Model workspace startup changing cwd before configured tabs are created.
		f.Workspaces[0].ForegroundCWD = filepath.Join(f.Workspaces[0].CWD, "web")
	}
	return f.FakeClient.PaneRun(ctx, id, cmd)
}

func TestConnectWorkspaceStartupChangingDirectoryKeepsConfiguredTabFocus(t *testing.T) {
	f := &workspaceStartupCWDClient{}
	path := t.TempDir()
	session := model.Session{
		Name: "api", Path: path, StartupCommand: "cd web && npm run dev",
		WindowConfigs: []model.WindowConfig{{Name: "editor", StartupScript: "nvim"}},
	}
	_, err := Connect(context.Background(), f, []model.Session{session}, "api", Options{})
	require.NoError(t, err)
	live, err := (sources.HerdrWorkspaces{Client: f}).List(context.Background())
	require.NoError(t, err)
	result, err := Connect(context.Background(), f, live.Ordered(), path, Options{})
	require.NoError(t, err)
	assert.False(t, result.Created)
	assert.Len(t, f.CreatedWorkspaces, 1)
	assert.Equal(t, []string{"initial-pane:cd web && npm run dev", "new-pane:nvim"}, f.PaneRuns)
}

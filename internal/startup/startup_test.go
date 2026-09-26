package startup

import (
	"context"
	"errors"
	"testing"

	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyCreatesTabsAndRunsCommands(t *testing.T) {
	f := &herdr.FakeClient{Panes: []herdr.Pane{{ID: "workspace-root", WorkspaceID: "ws1"}}}
	s := model.Session{
		Path: "/tmp/app", StartupCommand: "echo {}",
		WindowConfigs: []model.WindowConfig{{Name: "git", StartupScript: "git -C {} status"}},
	}
	require.NoError(t, Apply(context.Background(), f, Plan{WorkspaceID: "ws1", Session: s}))
	require.Len(t, f.CreatedTabs, 1)
	require.Equal(t, "git", f.CreatedTabs[0].Label)
	assert.Equal(t, []string{"workspace-root:echo /tmp/app", "new-pane:git -C /tmp/app status"}, f.PaneRuns)
}

func TestApplySkipsDisabledStartup(t *testing.T) {
	f := &herdr.FakeClient{}
	s := model.Session{Path: "/tmp/app", StartupCommand: "echo hi", DisableStartupCommand: true}
	require.NoError(t, Apply(context.Background(), f, Plan{WorkspaceID: "ws1", Session: s}))
	assert.Empty(t, f.PaneRuns)
}

func TestApplyFailsClearlyWhenOnlyOffTargetPaneExists(t *testing.T) {
	f := &herdr.FakeClient{Panes: []herdr.Pane{{ID: "existing-pane", WorkspaceID: "existing-workspace"}}}
	s := model.Session{Path: "/tmp/app", StartupCommand: "echo hi"}
	err := Apply(context.Background(), f, Plan{WorkspaceID: "new-workspace", Session: s})
	require.Error(t, err)
	require.Equal(t, `no pane available in workspace "new-workspace"`, err.Error())
	assert.Empty(t, f.PaneRuns)
}

func TestApplyBuildsPaneLayout(t *testing.T) {
	f := &herdr.FakeClient{Panes: []herdr.Pane{{ID: "workspace-root", WorkspaceID: "ws1"}}}
	s := model.Session{
		Name: "app", Path: "/tmp/app", StartupCommand: "echo ws",
		WindowConfigs: []model.WindowConfig{{Name: "dev", Panes: []model.PaneConfig{
			{Name: "editor", Path: "/tmp/app", Env: map[string]string{"EDITOR": "nvim"}, Startup: "nvim"},
			{Name: "server", SplitFrom: "editor", Split: "right", Ratio: 0.25, Path: "/tmp/app/web dir", Env: map[string]string{"NODE_ENV": "dev"}, Startup: "cd {} && npm run dev; echo \"$NODE_ENV\""},
			{Name: "logs", SplitFrom: "server", Split: "down", Path: "/var/log"},
			{Name: "shell", SplitFrom: "editor", Split: "down", Path: "/tmp/app"},
		}}},
	}
	require.NoError(t, Apply(context.Background(), f, Plan{WorkspaceID: "ws1", Session: s, Focus: true}))
	require.Len(t, f.CreatedTabs, 1)
	// Separate panes keep interactive commands from receiving each other's input.
	assert.Equal(t, herdr.TabCreateRequest{WorkspaceID: "ws1", CWD: "/tmp/app", Label: "dev", Env: map[string]string{"EDITOR": "nvim"}, Focus: true}, f.CreatedTabs[0])
	assert.Equal(t, []herdr.PaneSplitRequest{
		{PaneID: "new-pane", Direction: "right", Ratio: 0.75, CWD: "/tmp/app/web dir", Env: map[string]string{"NODE_ENV": "dev"}},
		{PaneID: "split-1", Direction: "down", CWD: "/var/log"},
		{PaneID: "new-pane", Direction: "down", CWD: "/tmp/app"},
	}, f.Splits)
	assert.Equal(t, []string{
		"workspace-root:echo ws",
		"new-pane:nvim",
		"split-1:cd '/tmp/app/web dir' && npm run dev; echo \"$NODE_ENV\"",
	}, f.PaneRuns)
}

func TestApplyRootPanePathOverridesTabPath(t *testing.T) {
	f := &herdr.FakeClient{}
	s := model.Session{Path: "/tmp/app", WindowConfigs: []model.WindowConfig{{Name: "dev", Path: "/tmp/tab", Panes: []model.PaneConfig{{Name: "root", Path: "/tmp/tab/sub"}}}}}
	require.NoError(t, Apply(context.Background(), f, Plan{WorkspaceID: "ws1", Session: s}))
	require.Len(t, f.CreatedTabs, 1)
	assert.Equal(t, "/tmp/tab/sub", f.CreatedTabs[0].CWD)
	assert.False(t, f.CreatedTabs[0].Focus)
}

func TestApplyRunsStartupCommandsUnwrappedInSeparatePanes(t *testing.T) {
	for _, kind := range []string{"tab", "layout"} {
		for _, command := range []string{"lazygit", "true # setup", "true &", "true;", "let value = 42", "$env:VALUE = 'ready'"} {
			t.Run(command+"/"+kind, func(t *testing.T) {
				f := &herdr.FakeClient{Panes: []herdr.Pane{
					{ID: "other", WorkspaceID: "other-workspace"},
					{ID: "workspace-root", WorkspaceID: "ws1"},
				}}
				window := model.WindowConfig{Name: "dev", StartupScript: "nvim"}
				if kind == "layout" {
					window.StartupScript = ""
					window.Panes = []model.PaneConfig{{Name: "root", Path: "/tmp/app", Startup: "nvim"}}
				}
				s := model.Session{
					Path: "/tmp/app", StartupCommand: command,
					WindowConfigs: []model.WindowConfig{window},
				}
				require.NoError(t, Apply(context.Background(), f, Plan{WorkspaceID: "ws1", Session: s}))
				assert.Equal(t, []string{"workspace-root:" + command, "new-pane:nvim"}, f.PaneRuns)
			})
		}
	}
}

func TestApplyStopsAndReportsMidLayoutFailure(t *testing.T) {
	f := &herdr.FakeClient{SplitErr: errors.New("boom"), SplitErrAt: 1, Panes: []herdr.Pane{{ID: "workspace-root", WorkspaceID: "ws1"}}}
	s := model.Session{Name: "app", Path: "/tmp/app", StartupCommand: "echo ws", WindowConfigs: []model.WindowConfig{
		{Name: "dev", Panes: []model.PaneConfig{
			{Name: "a"},
			{Name: "b", SplitFrom: "a", Split: "right"},
			{Name: "c", SplitFrom: "b", Split: "down", Startup: "never"},
		}},
		{Name: "later"},
	}}
	err := Apply(context.Background(), f, Plan{WorkspaceID: "ws1", Session: s})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `workspace "app" tab "dev": pane "c": split from "b": boom`)
	assert.Contains(t, err.Error(), "reconnecting does not retry the layout")
	assert.Len(t, f.Splits, 1)
	assert.Len(t, f.CreatedTabs, 1, "later tabs are not created")
	assert.Equal(t, []string{"workspace-root:echo ws"}, f.PaneRuns)
}

type tabWithoutRootClient struct{ herdr.FakeClient }

func (f *tabWithoutRootClient) TabCreate(ctx context.Context, req herdr.TabCreateRequest) (herdr.Tab, error) {
	tab, err := f.FakeClient.TabCreate(ctx, req)
	tab.PaneID = ""
	return tab, err
}

func TestApplyFindsMissingRootOnlyInCreatedTab(t *testing.T) {
	for _, matching := range []bool{false, true} {
		f := &tabWithoutRootClient{FakeClient: herdr.FakeClient{Panes: []herdr.Pane{
			{ID: "wrong-tab", WorkspaceID: "ws1", TabID: "other-tab"},
			{ID: "wrong-workspace", WorkspaceID: "other-workspace", TabID: "new-tab"},
		}}}
		if matching {
			f.Panes = append(f.Panes, herdr.Pane{ID: "right-root", WorkspaceID: "ws1", TabID: "new-tab"})
		}
		s := model.Session{Name: "app", Path: "/tmp/app", WindowConfigs: []model.WindowConfig{{Name: "dev", Panes: []model.PaneConfig{
			{Name: "root", Path: "/tmp/app", Startup: "nvim"},
			{Name: "child", Path: "/tmp/app", SplitFrom: "root", Split: "right"},
		}}}}
		err := Apply(context.Background(), f, Plan{WorkspaceID: "ws1", Session: s})
		if !matching {
			require.Error(t, err)
			assert.Empty(t, f.PaneRuns)
			assert.Empty(t, f.Splits)
			continue
		}
		require.NoError(t, err)
		assert.Equal(t, []string{"right-root:nvim"}, f.PaneRuns)
		require.Len(t, f.Splits, 1)
		assert.Equal(t, "right-root", f.Splits[0].PaneID)
	}
}

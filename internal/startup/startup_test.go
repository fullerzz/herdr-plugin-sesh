package startup

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fullerzz/herdr-plugin-sesh/internal/config"
	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/fullerzz/herdr-plugin-sesh/internal/sources"
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

func waitLayout() model.Session {
	return model.Session{Name: "app", Path: "/tmp/app", WindowConfigs: []model.WindowConfig{
		{Name: "dev", Panes: []model.PaneConfig{
			{Name: "server", Startup: "npm run dev", WaitFor: &model.PaneWait{Match: "Local:", Timeout: time.Second}},
			{Name: "tests", SplitFrom: "server", Split: "right", Startup: "npm test"},
		}},
		{Name: "later", StartupScript: "lazygit"},
	}}
}

func TestApplyWaitsForReadinessBeforeLaterPanes(t *testing.T) {
	f := &herdr.FakeClient{}
	require.NoError(t, Apply(context.Background(), f, Plan{WorkspaceID: "ws1", Session: waitLayout()}))
	assert.Equal(t, []string{
		"create tab dev",
		"run new-pane:npm run dev",
		"wait new-pane:Local:",
		"split new-pane",
		"run split-1:npm test",
		"create tab later",
		"run new-pane:lazygit",
	}, f.Ops)
}

func TestApplyRejectsMatchEchoedByResolvedStartup(t *testing.T) {
	cfg := config.Config{
		SessionConfigs: []config.SessionConfig{{Name: "app", Path: "/tmp/ready-api", Windows: []string{"dev", "later"}}},
		WindowConfigs: []model.WindowConfig{
			{Name: "dev", Panes: []model.PaneConfig{
				{Name: "server", Startup: "cd {} && npm run dev", WaitFor: &model.PaneWait{Match: "ready", Timeout: time.Second}},
				{Name: "tests", SplitFrom: "server", Split: "right", Startup: "npm test"},
			}},
			{Name: "later", StartupScript: "lazygit"},
		},
	}
	for path, wantErr := range map[string]bool{"/tmp/ready-api": true, "/tmp/api": false} {
		t.Run(path, func(t *testing.T) {
			cfg.SessionConfigs[0].Path = path
			// The pane path comes from the workspace, as a raw-config check cannot see.
			got, err := sources.ConfigSessions{Config: cfg}.List(context.Background())
			require.NoError(t, err)
			f := &herdr.FakeClient{}
			err = Apply(context.Background(), f, Plan{WorkspaceID: "ws1", Session: got.Ordered()[0]})
			if !wantErr {
				require.NoError(t, err)
				assert.Equal(t, []string{"new-pane:ready"}, f.Waits)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), `workspace "app" tab "dev": pane "server": wait_for match "ready" appears in its startup command after {} expands to "/tmp/ready-api"`)
			assert.Contains(t, err.Error(), "reconnecting does not retry the layout")
			assert.Equal(t, []string{"create tab dev"}, f.Ops, "nothing runs, waits, or splits after the collision")
		})
	}
}

func TestApplyStopsLayoutWhenReadinessFails(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for name, tc := range map[string]struct {
		ctx     context.Context
		waitErr error
		want    error
	}{
		"timeout":   {context.Background(), errors.New("timed out"), nil},
		"cancelled": {canceled, nil, context.Canceled},
	} {
		t.Run(name, func(t *testing.T) {
			f := &herdr.FakeClient{WaitErr: tc.waitErr}
			err := Apply(tc.ctx, f, Plan{WorkspaceID: "ws1", Session: waitLayout()})
			require.Error(t, err)
			assert.Contains(t, err.Error(), `workspace "app" tab "dev": pane "server": wait_for "Local:" within 1s`)
			assert.Contains(t, err.Error(), "reconnecting does not retry the layout")
			if tc.want != nil {
				require.ErrorIs(t, err, tc.want)
			}
			assert.Empty(t, f.Splits, "later panes are not created")
			assert.Len(t, f.CreatedTabs, 1, "later tabs are not created")
			assert.Equal(t, []string{"new-pane:npm run dev"}, f.PaneRuns, "later startup commands do not run")
		})
	}
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
			{ID: "wrong-workspace", WorkspaceID: "other-workspace", TabID: "new-tab-1"},
		}}}
		if matching {
			f.Panes = append(f.Panes, herdr.Pane{ID: "right-root", WorkspaceID: "ws1", TabID: "new-tab-1"})
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

func TestApplyReusesInitialTabForFirstConfiguredTab(t *testing.T) {
	f := &herdr.FakeClient{Panes: []herdr.Pane{{ID: "initial-pane", WorkspaceID: "ws1", TabID: "initial-tab"}}}
	s := model.Session{Name: "app", Path: "/tmp/app", WindowConfigs: []model.WindowConfig{
		{Name: "dev", Panes: []model.PaneConfig{
			{Name: "editor", Path: "/tmp/app"},
			{Name: "shell", SplitFrom: "editor", Split: "right", Path: "/tmp/app", Startup: "nvim"},
		}},
		{Name: "git", StartupScript: "lazygit"},
	}}
	require.NoError(t, Apply(context.Background(), f, Plan{WorkspaceID: "ws1", Session: s, Focus: true, InitialTab: PlanInitialTab(s)}))
	assert.Equal(t, []string{"initial-tab:dev"}, f.RenamedTabs)
	require.Len(t, f.CreatedTabs, 1, "only tabs after the first are created")
	assert.Equal(t, herdr.TabCreateRequest{WorkspaceID: "ws1", CWD: "/tmp/app", Label: "git"}, f.CreatedTabs[0])
	require.Len(t, f.Splits, 1)
	assert.Equal(t, "initial-pane", f.Splits[0].PaneID)
	assert.Equal(t, []string{"split-1:nvim", "new-pane:lazygit"}, f.PaneRuns)
	assert.Empty(t, f.FocusedTabs, "the reused tab is already active")
}

func TestApplyKeepsInitialTabForWorkspaceStartup(t *testing.T) {
	f := &herdr.FakeClient{Panes: []herdr.Pane{{ID: "initial-pane", WorkspaceID: "ws1", TabID: "initial-tab"}}}
	s := model.Session{Path: "/tmp/app", StartupCommand: "echo ws", WindowConfigs: []model.WindowConfig{{Name: "dev", StartupScript: "nvim"}}}
	require.NoError(t, Apply(context.Background(), f, Plan{WorkspaceID: "ws1", Session: s, InitialTab: PlanInitialTab(s)}))
	assert.Empty(t, f.RenamedTabs)
	assert.Len(t, f.CreatedTabs, 1)
	assert.Equal(t, []string{"initial-pane:echo ws", "new-pane:nvim"}, f.PaneRuns)
}

func TestApplyReportsInitialTabReuseFailure(t *testing.T) {
	f := &herdr.FakeClient{}
	s := model.Session{Name: "app", Path: "/tmp/app", WindowConfigs: []model.WindowConfig{{Name: "dev"}, {Name: "later"}}}
	err := Apply(context.Background(), f, Plan{WorkspaceID: "ws1", Session: s, InitialTab: PlanInitialTab(s)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `workspace "app" tab "dev": reuse initial tab: no pane available in workspace "ws1" (the workspace was kept)`)
	assert.Empty(t, f.CreatedTabs)
	assert.Empty(t, f.PaneRuns)
}

func TestPlanInitialTabCouplesReuseAndRootEnvironment(t *testing.T) {
	root := func(tabPath, panePath string) model.Session {
		return model.Session{Path: "/tmp/app", WindowConfigs: []model.WindowConfig{{Name: "dev", Path: tabPath, Panes: []model.PaneConfig{{Name: "root", Path: panePath, Env: map[string]string{"A": "1"}}}}}}
	}
	reuse := InitialTabPolicy{Reuse: true, Env: map[string]string{"A": "1"}}
	assert.Equal(t, reuse, PlanInitialTab(root("", "")))
	assert.Equal(t, reuse, PlanInitialTab(root("/tmp/sub", "/tmp/app/")))
	assert.Equal(t, InitialTabPolicy{}, PlanInitialTab(root("/tmp/sub", "")))
	assert.Equal(t, InitialTabPolicy{}, PlanInitialTab(root("", "/tmp/app/web")))
	assert.Equal(t, InitialTabPolicy{}, PlanInitialTab(model.Session{Path: "/tmp/app"}))
	assert.Equal(t, InitialTabPolicy{Reuse: true}, PlanInitialTab(model.Session{Path: "/tmp/app", WindowConfigs: []model.WindowConfig{{Name: "dev"}}}))

	s := root("", "")
	s.StartupCommand = "lazygit"
	assert.Equal(t, InitialTabPolicy{}, PlanInitialTab(s), "workspace startup must not inherit the configured root environment")
	s.DisableStartupCommand = true
	assert.Equal(t, reuse, PlanInitialTab(s))
	s.WindowConfigs[0].Panes[0].Startup = "git status"
	assert.Equal(t, InitialTabPolicy{}, PlanInitialTab(s), "do not infer whether arbitrary shell commands change cwd")
	s.WindowConfigs[0].Panes = nil
	s.WindowConfigs[0].StartupScript = "run-dev"
	assert.Equal(t, InitialTabPolicy{}, PlanInitialTab(s))
}

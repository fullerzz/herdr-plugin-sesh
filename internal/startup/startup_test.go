package startup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyCreatesTabsAndRunsCommands(t *testing.T) {
	f := &herdr.FakeClient{}
	s := model.Session{
		Path: "/tmp/app", StartupCommand: "echo {}",
		WindowConfigs: []model.WindowConfig{{Name: "git", StartupScript: "git -C {} status"}},
	}
	require.NoError(t, Apply(context.Background(), f, Plan{WorkspaceID: "ws1", Session: s}))
	require.Len(t, f.CreatedTabs, 1)
	require.Equal(t, "git", f.CreatedTabs[0].Label)
	assert.Equal(t, []string{"new-pane:git -C /tmp/app status", "new-pane:echo /tmp/app"}, f.PaneRuns, "tabs without panes keep tab-then-workspace order")
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
	f := &herdr.FakeClient{}
	s := model.Session{
		Name: "app", Path: "/tmp/app", StartupCommand: "echo ws",
		WindowConfigs: []model.WindowConfig{{Name: "dev", Panes: []model.PaneConfig{
			{Name: "editor", Env: map[string]string{"EDITOR": "nvim"}, Startup: "nvim"},
			{Name: "server", SplitFrom: "editor", Split: "right", Ratio: 0.25, Path: "web dir", Env: map[string]string{"NODE_ENV": "dev"}, Startup: "cd {} && npm run dev; echo \"$NODE_ENV\""},
			{Name: "logs", SplitFrom: "server", Split: "down", Path: "/var/log"},
			{Name: "shell", SplitFrom: "editor", Split: "down"},
		}}},
	}
	require.NoError(t, Apply(context.Background(), f, Plan{WorkspaceID: "ws1", Session: s, Focus: true}))
	require.Len(t, f.CreatedTabs, 1)
	// One pane run keeps "nvim" from receiving the workspace command as input.
	assert.Equal(t, herdr.TabCreateRequest{WorkspaceID: "ws1", CWD: "/tmp/app", Label: "dev", Env: map[string]string{"EDITOR": "nvim"}, Focus: true}, f.CreatedTabs[0])
	assert.Equal(t, []herdr.PaneSplitRequest{
		{PaneID: "new-pane", Direction: "right", Ratio: 0.75, CWD: "/tmp/app/web dir", Env: map[string]string{"NODE_ENV": "dev"}},
		{PaneID: "split-1", Direction: "down", CWD: "/var/log"},
		{PaneID: "new-pane", Direction: "down", CWD: "/tmp/app"},
	}, f.Splits)
	assert.Equal(t, []string{
		"new-pane:echo ws; nvim",
		"split-1:cd '/tmp/app/web dir' && npm run dev; echo \"$NODE_ENV\"",
	}, f.PaneRuns)
}

func TestApplyRootPanePathOverridesTabPath(t *testing.T) {
	f := &herdr.FakeClient{}
	s := model.Session{Path: "/tmp/app", WindowConfigs: []model.WindowConfig{{Name: "dev", Path: "/tmp/tab", Panes: []model.PaneConfig{{Name: "root", Path: "sub"}}}}}
	require.NoError(t, Apply(context.Background(), f, Plan{WorkspaceID: "ws1", Session: s}))
	require.Len(t, f.CreatedTabs, 1)
	assert.Equal(t, "/tmp/tab/sub", f.CreatedTabs[0].CWD)
	assert.False(t, f.CreatedTabs[0].Focus)
}

func TestApplyStopsAndReportsMidLayoutFailure(t *testing.T) {
	f := &herdr.FakeClient{SplitErr: errors.New("boom"), SplitErrAt: 1}
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
	assert.Equal(t, []string{"new-pane:echo ws"}, f.PaneRuns)
}

func TestPanePathExpandsBareHome(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	assert.Equal(t, home, panePath("/tmp/app", "~/"))
	assert.Equal(t, filepath.Join(home, "logs"), panePath("/tmp/app", "~/logs"))
	assert.Equal(t, "/tmp/app/web", panePath("/tmp/app", "./web"))
}

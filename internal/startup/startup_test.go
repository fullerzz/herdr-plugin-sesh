package startup

import (
	"context"
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
	require.Len(t, f.PaneRuns, 2)
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

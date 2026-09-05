package connect

import (
	"context"
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

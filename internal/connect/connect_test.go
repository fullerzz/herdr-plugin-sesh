package connect

import (
	"context"
	"testing"

	"github.com/fullerzz/herdr-plugin-sesh/internal/config"
	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/fullerzz/herdr-plugin-sesh/internal/sources"
)

func TestConnectFocusesExistingWorkspace(t *testing.T) {
	f := &herdr.FakeClient{}
	_, err := Connect(context.Background(), f, []model.Session{{Name: "api", WorkspaceID: "ws1"}}, "api", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.FocusedWorkspaces) != 1 || f.FocusedWorkspaces[0] != "ws1" {
		t.Fatalf("focused: %#v", f.FocusedWorkspaces)
	}
	if len(f.CreatedWorkspaces) != 0 {
		t.Fatalf("created unexpectedly: %#v", f.CreatedWorkspaces)
	}
}

func TestConnectCreatesWorkspaceForConfigSession(t *testing.T) {
	f := &herdr.FakeClient{}
	res, err := Connect(context.Background(), f, []model.Session{{Source: "config", Name: "api", Path: "/tmp/api"}}, "api", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Created || len(f.CreatedWorkspaces) != 1 {
		t.Fatalf("result=%#v created=%#v", res, f.CreatedWorkspaces)
	}
	if f.CreatedWorkspaces[0].CWD != "/tmp/api" || f.CreatedWorkspaces[0].Label != "api" {
		t.Fatalf("bad request: %#v", f.CreatedWorkspaces[0])
	}
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
	if _, err := Connect(context.Background(), f, []model.Session{session}, "api", Options{NoFocus: true}); err != nil {
		t.Fatal(err)
	}
	if len(f.CreatedWorkspaces) != 1 || f.CreatedWorkspaces[0].Focus {
		t.Fatalf("created workspaces: %#v", f.CreatedWorkspaces)
	}
	if len(f.PaneRuns) != 1 || f.PaneRuns[0] != "new-pane:echo ready" {
		t.Fatalf("pane runs: %#v", f.PaneRuns)
	}
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
	if _, err := Connect(context.Background(), f, []model.Session{session}, "api", Options{}); err != nil {
		t.Fatal(err)
	}
	if len(f.CreatedWorkspaces) != 1 || !f.CreatedWorkspaces[0].Focus {
		t.Fatalf("created workspaces: %#v", f.CreatedWorkspaces)
	}
	if len(f.PaneRuns) != 1 || f.PaneRuns[0] != "new-pane:echo ready" {
		t.Fatalf("pane runs: %#v", f.PaneRuns)
	}
}

func TestConnectUsesExpandedConfigSessionPath(t *testing.T) {
	cfg := config.Config{SessionConfigs: []config.SessionConfig{{Name: "api", Path: "~/projects/api"}}}
	got, err := sources.ConfigSessions{Config: cfg, Home: "/home/zach"}.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	f := &herdr.FakeClient{}
	_, err = Connect(context.Background(), f, got.Ordered(), "api", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.CreatedWorkspaces) != 1 || f.CreatedWorkspaces[0].CWD != "/home/zach/projects/api" {
		t.Fatalf("created workspaces: %#v", f.CreatedWorkspaces)
	}
}

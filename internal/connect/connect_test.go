package connect

import (
	"context"
	"testing"

	"forgejo.local/fullerzz/herdr-plugin-sesh/internal/herdr"
	"forgejo.local/fullerzz/herdr-plugin-sesh/internal/model"
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

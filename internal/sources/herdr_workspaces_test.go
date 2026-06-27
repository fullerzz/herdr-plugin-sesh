package sources

import (
	"context"
	"testing"

	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
)

func TestHerdrWorkspacesUsesPaneCWDWhenWorkspaceListOmitsPath(t *testing.T) {
	src := HerdrWorkspaces{Client: &herdr.FakeClient{
		Workspaces: []herdr.Workspace{{ID: "w1", Label: "api", ActiveTabID: "w1:t2"}},
		Panes: []herdr.Pane{
			{ID: "p1", WorkspaceID: "w1", TabID: "w1:t1", ForegroundCWD: "/tmp/wrong"},
			{ID: "p2", WorkspaceID: "w1", TabID: "w1:t2", CWD: "/tmp/api"},
		},
	}}
	got, err := src.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sessions := got.Ordered()
	if len(sessions) != 1 || sessions[0].Path != "/tmp/api" {
		t.Fatalf("sessions=%#v", sessions)
	}
}

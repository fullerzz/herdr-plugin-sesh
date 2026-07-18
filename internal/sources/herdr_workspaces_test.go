package sources

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
)

func TestHerdrWorkspacesUsesPaneCWDWhenWorkspaceListOmitsPath(t *testing.T) {
	src := HerdrWorkspaces{Client: &herdr.FakeClient{
		Workspaces: []herdr.Workspace{{ID: "w1", Number: 7, Label: "api", ActiveTabID: "w1:t2", AgentStatus: "working", TabCount: 2, PaneCount: 3}},
		Tabs: []herdr.Tab{
			{ID: "w1:t1", WorkspaceID: "w1", Number: 1, Label: "logs"},
			{ID: "w1:t2", WorkspaceID: "w1", Number: 2, Label: "main"},
		},
		Panes: []herdr.Pane{
			{ID: "p1", WorkspaceID: "w1", TabID: "w1:t1", ForegroundCWD: "/tmp/wrong", Title: "tail", AgentStatus: "idle"},
			{ID: "p2", WorkspaceID: "w1", TabID: "w1:t2", CWD: "/tmp/api", DisplayAgent: "Codex", AgentStatus: "working"},
			{ID: "p3", WorkspaceID: "w1", TabID: "w1:t2", Label: "server", AgentStatus: "idle"},
		},
	}}
	got, err := src.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sessions := got.Ordered()
	if len(sessions) != 1 || sessions[0].Path != "/tmp/api" || sessions[0].WorkspaceNumber != 7 || sessions[0].AgentStatus != "working" || sessions[0].TabCount != 2 || sessions[0].PaneCount != 3 || sessions[0].ActiveTabID != "w1:t2" {
		t.Fatalf("sessions=%#v", sessions)
	}
	if len(sessions[0].WorkspaceTabs) != 2 || sessions[0].WorkspaceTabs[1].Number != 2 || sessions[0].WorkspaceTabs[1].Label != "main" {
		t.Fatalf("tabs=%#v", sessions[0].WorkspaceTabs)
	}
	if len(sessions[0].WorkspacePanes) != 3 || sessions[0].WorkspacePanes[1].Label != "Codex" || sessions[0].WorkspacePanes[1].AgentStatus != "working" {
		t.Fatalf("panes=%#v", sessions[0].WorkspacePanes)
	}
}

func TestWorkspaceTabsUsesWorkspaceLocalPositions(t *testing.T) {
	tabs := workspaceTabs("w1", []herdr.Tab{
		{ID: "w1:t1", WorkspaceID: "w1", Number: 1},
		{ID: "w2:t1", WorkspaceID: "w2", Number: 1},
		{ID: "w1:tG", WorkspaceID: "w1", Number: 16},
		{ID: "w1:tH", WorkspaceID: "w1", Number: 17},
	})

	for i, tab := range tabs {
		if want := i + 1; tab.Number != want {
			t.Fatalf("tabs[%d].Number=%d, want workspace position %d: %#v", i, tab.Number, want, tabs)
		}
	}
}

func TestHerdrWorkspaceRefreshRejectsPartialLayoutMetadata(t *testing.T) {
	src := HerdrWorkspaces{Client: &herdr.FakeClient{
		Workspaces:  []herdr.Workspace{{ID: "w1"}},
		PaneListErr: errors.New("pane list failed"),
	}}
	if _, err := src.Refresh(context.Background()); err == nil || !strings.Contains(err.Error(), "pane list failed") {
		t.Fatalf("refresh error=%v", err)
	}
}

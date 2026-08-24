package sources

import (
	"context"
	"testing"

	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
)

type countingHerdrClient struct {
	*herdr.FakeClient

	paneListCalls int
}

func (c *countingHerdrClient) PaneList(ctx context.Context, workspaceID string) ([]herdr.Pane, error) {
	c.paneListCalls++
	return c.FakeClient.PaneList(ctx, workspaceID)
}

func TestHerdrWorkspacesSkipsPaneListWhenWorkspaceHasPath(t *testing.T) {
	client := &countingHerdrClient{FakeClient: &herdr.FakeClient{Workspaces: []herdr.Workspace{
		{ID: "w-foreground", Label: "foreground", ForegroundCWD: "/tmp/foreground"},
		{ID: "w-cwd", Label: "cwd", CWD: "/tmp/cwd"},
	}}}

	got, err := (HerdrWorkspaces{Client: client}).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if client.paneListCalls != 0 {
		t.Fatalf("PaneList calls=%d, want 0", client.paneListCalls)
	}
	paths := got.Ordered()
	if len(paths) != 2 || paths[0].Path != "/tmp/foreground" || paths[1].Path != "/tmp/cwd" {
		t.Fatalf("sessions=%#v", paths)
	}
}

func TestHerdrWorkspacesRelatesLinkedWorktreeToUniqueParent(t *testing.T) {
	repo := "/repos/project/.git"
	src := HerdrWorkspaces{Client: &herdr.FakeClient{Workspaces: []herdr.Workspace{
		{ID: "w-child", Label: "feature", Worktree: &herdr.Worktree{IsLinkedWorktree: true, RepoKey: repo}},
		{ID: "w-other", Label: "other", Worktree: &herdr.Worktree{RepoKey: "/repos/other/.git"}},
		{ID: "w-parent", Label: "project", Worktree: &herdr.Worktree{RepoKey: repo}},
	}}}

	got, err := src.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sessions := got.Ordered()
	if len(sessions) != 3 {
		t.Fatalf("sessions=%#v", sessions)
	}
	child := sessions[0]
	if !child.Worktree.Linked || child.Worktree.ParentWorkspaceID != "w-parent" || child.Worktree.ParentWorkspaceName != "project" {
		t.Fatalf("child relation=%#v", child.Worktree)
	}
	for _, normal := range sessions[1:] {
		if normal.Worktree.Linked || normal.Worktree.ParentWorkspaceID != "" || normal.Worktree.ParentWorkspaceName != "" {
			t.Fatalf("normal workspace %q has relation %#v", normal.Name, normal.Worktree)
		}
	}
}

func TestHerdrWorkspacesFallsBackForUnresolvedLinkedWorktree(t *testing.T) {
	tests := []struct {
		name       string
		workspaces []herdr.Workspace
	}{
		{
			name: "parent absent",
			workspaces: []herdr.Workspace{
				{ID: "w-child", Label: "feature", Worktree: &herdr.Worktree{IsLinkedWorktree: true, RepoKey: "/repos/project/.git"}},
			},
		},
		{
			name: "parent ambiguous",
			workspaces: []herdr.Workspace{
				{ID: "w-child", Label: "feature", Worktree: &herdr.Worktree{IsLinkedWorktree: true, RepoKey: "/repos/project/.git"}},
				{ID: "w-parent-1", Label: "project one", Worktree: &herdr.Worktree{RepoKey: "/repos/project/.git"}},
				{ID: "w-parent-2", Label: "project two", Worktree: &herdr.Worktree{RepoKey: "/repos/project/.git"}},
			},
		},
		{
			name: "repo key missing",
			workspaces: []herdr.Workspace{
				{ID: "w-child", Label: "feature", Worktree: &herdr.Worktree{IsLinkedWorktree: true}},
				{ID: "w-parent", Label: "project", Worktree: &herdr.Worktree{}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := HerdrWorkspaces{Client: &herdr.FakeClient{Workspaces: tt.workspaces}}
			got, err := src.List(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			child := got.Ordered()[0]
			if !child.Worktree.Linked || child.Worktree.ParentWorkspaceID != "" || child.Worktree.ParentWorkspaceName != "" {
				t.Fatalf("child relation=%#v", child.Worktree)
			}
		})
	}
}

func TestHerdrWorkspacesUsesParentIDWhenLabelIsEmpty(t *testing.T) {
	repo := "/repos/project/.git"
	src := HerdrWorkspaces{Client: &herdr.FakeClient{Workspaces: []herdr.Workspace{
		{ID: "w-parent", Worktree: &herdr.Worktree{RepoKey: repo}},
		{ID: "w-child", Label: "feature", Worktree: &herdr.Worktree{IsLinkedWorktree: true, RepoKey: repo}},
	}}}
	got, err := src.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	child := got.Ordered()[1]
	if child.Worktree.ParentWorkspaceName != "w-parent" {
		t.Fatalf("child relation=%#v", child.Worktree)
	}
}

func TestHerdrWorkspacesUsesLinkedWorktreeCheckoutPathAsFallback(t *testing.T) {
	src := HerdrWorkspaces{Client: &herdr.FakeClient{Workspaces: []herdr.Workspace{{
		ID:    "w-child",
		Label: "feature",
		Worktree: &herdr.Worktree{
			CheckoutPath:     "/worktrees/feature",
			IsLinkedWorktree: true,
		},
	}}}}

	got, err := src.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sessions := got.Ordered()
	if len(sessions) != 1 || sessions[0].Path != "/worktrees/feature" {
		t.Fatalf("sessions=%#v", sessions)
	}
}

func TestHerdrWorkspacesUsesPaneCWDWhenWorkspaceListOmitsPath(t *testing.T) {
	src := HerdrWorkspaces{Client: &herdr.FakeClient{
		Workspaces: []herdr.Workspace{{ID: "w1", Label: "api", ActiveTabID: "w1:t2", AgentStatus: "working"}},
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
	if len(sessions) != 1 || sessions[0].Path != "/tmp/api" || sessions[0].AgentStatus != "working" {
		t.Fatalf("sessions=%#v", sessions)
	}
}

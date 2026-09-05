package sources

import (
	"context"
	"testing"

	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, err)
	require.Equal(t, 0, client.paneListCalls)
	paths := got.Ordered()
	require.Len(t, paths, 2)
	require.Equal(t, "/tmp/foreground", paths[0].Path)
	assert.Equal(t, "/tmp/cwd", paths[1].Path)
}

func TestHerdrWorkspacesRelatesLinkedWorktreeToUniqueParent(t *testing.T) {
	repo := "/repos/project/.git"
	src := HerdrWorkspaces{Client: &herdr.FakeClient{Workspaces: []herdr.Workspace{
		{ID: "w-child", Label: "feature", Worktree: &herdr.Worktree{IsLinkedWorktree: true, RepoKey: repo}},
		{ID: "w-other", Label: "other", Worktree: &herdr.Worktree{RepoKey: "/repos/other/.git"}},
		{ID: "w-parent", Label: "project", Worktree: &herdr.Worktree{RepoKey: repo}},
	}}}

	got, err := src.List(context.Background())
	require.NoError(t, err)
	sessions := got.Ordered()
	require.Len(t, sessions, 3)
	child := sessions[0]
	require.True(t, child.Worktree.Linked)
	require.Equal(t, "w-parent", child.Worktree.ParentWorkspaceID)
	require.Equal(t, "project", child.Worktree.ParentWorkspaceName)
	for _, normal := range sessions[1:] {
		require.False(t, normal.Worktree.Linked)
		require.Empty(t, normal.Worktree.ParentWorkspaceID)
		require.Empty(t, normal.Worktree.ParentWorkspaceName)
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
			require.NoError(t, err)
			child := got.Ordered()[0]
			require.True(t, child.Worktree.Linked)
			require.Empty(t, child.Worktree.ParentWorkspaceID)
			assert.Empty(t, child.Worktree.ParentWorkspaceName)
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
	require.NoError(t, err)
	child := got.Ordered()[1]
	assert.Equal(t, "w-parent", child.Worktree.ParentWorkspaceName)
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
	require.NoError(t, err)
	sessions := got.Ordered()
	require.Len(t, sessions, 1)
	assert.Equal(t, "/worktrees/feature", sessions[0].Path)
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
	require.NoError(t, err)
	sessions := got.Ordered()
	require.Len(t, sessions, 1)
	require.Equal(t, "/tmp/api", sessions[0].Path)
	assert.Equal(t, "working", sessions[0].AgentStatus)
}

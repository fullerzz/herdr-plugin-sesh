package herdr

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFakeClientTabFocusUpdatesWorkspaceThroughEitherEntryPoint(t *testing.T) {
	for _, focusOnCreate := range []bool{false, true} {
		t.Run(fmt.Sprintf("focusOnCreate=%t", focusOnCreate), func(t *testing.T) {
			initial := Workspace{ID: "ws", CWD: "/repo", ActiveTabID: "initial", ForegroundCWD: "/repo"}
			other := Workspace{ID: "other", CWD: "/other", ActiveTabID: "other-tab"}
			f := &FakeClient{Workspaces: []Workspace{other, initial}}
			ctx := context.Background()
			tab, err := f.TabCreate(ctx, TabCreateRequest{WorkspaceID: "ws", CWD: "/repo/web", Focus: focusOnCreate})
			require.NoError(t, err)
			if !focusOnCreate {
				assert.Equal(t, initial, f.Workspaces[1], "background creation must not change the active tab")
				assert.Empty(t, f.FocusedTabs)
				require.NoError(t, f.TabFocus(ctx, tab.ID))
				assert.Equal(t, []string{tab.ID}, f.FocusedTabs)
			} else {
				assert.Empty(t, f.FocusedTabs, "only direct TabFocus calls are recorded")
			}
			workspaces, err := f.WorkspaceList(ctx)
			require.NoError(t, err)
			require.Len(t, workspaces, 2)
			assert.Equal(t, other, workspaces[0])
			assert.Equal(t, Workspace{ID: "ws", CWD: "/repo", ActiveTabID: tab.ID, ForegroundCWD: "/repo/web"}, workspaces[1])
		})
	}
}

func TestFakeClientFocusesTheRequestedCreatedTab(t *testing.T) {
	ctx := context.Background()
	f := &FakeClient{Workspaces: []Workspace{{ID: "ws", CWD: "/repo"}}}
	first, err := f.TabCreate(ctx, TabCreateRequest{WorkspaceID: "ws", CWD: "/repo/api"})
	require.NoError(t, err)
	second, err := f.TabCreate(ctx, TabCreateRequest{WorkspaceID: "ws", CWD: "/repo/web"})
	require.NoError(t, err)
	assert.NotEqual(t, first.ID, second.ID)
	for _, tab := range []Tab{second, first} {
		require.NoError(t, f.TabFocus(ctx, tab.ID))
		workspaces, err := f.WorkspaceList(ctx)
		require.NoError(t, err)
		require.Len(t, workspaces, 1)
		assert.Equal(t, tab.ID, workspaces[0].ActiveTabID)
		assert.Equal(t, tab.CWD, workspaces[0].ForegroundCWD)
	}
}

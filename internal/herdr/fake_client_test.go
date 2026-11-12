package herdr

import (
	"context"
	"fmt"
	"sync"
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

// Run with -race: preview may run remote commands from concurrent renders.
func TestFakeClientMachineExecScriptsRepliesAndRecordsCalls(t *testing.T) {
	f := &FakeClient{MachineExecReply: func(_ context.Context, machine string, r MachineExecRequest) ([]byte, []byte, error) {
		return []byte(machine + ":" + r.Argv[0]), nil, nil
	}}
	var wg sync.WaitGroup
	outs := make(chan string, 4)
	for i := range 4 {
		wg.Go(func() {
			out, _, err := f.MachineExec(t.Context(), "buntu26", MachineExecRequest{Dir: "src", Argv: []string{fmt.Sprint(i)}})
			if err != nil {
				out = []byte(err.Error())
			}
			outs <- string(out)
		})
	}
	wg.Wait()
	close(outs)
	var got []string
	for out := range outs {
		got = append(got, out)
	}
	assert.ElementsMatch(t, []string{"buntu26:0", "buntu26:1", "buntu26:2", "buntu26:3"}, got)
	calls := f.MachineExecCalls()
	require.Len(t, calls, 4)
	for _, call := range calls {
		assert.Equal(t, "buntu26", call.Machine)
		assert.Equal(t, "src", call.Request.Dir)
	}

	_, _, err := (&FakeClient{}).MachineExec(t.Context(), "buntu26", MachineExecRequest{Argv: []string{"ls"}})
	require.NoError(t, err, "an unscripted MachineExec succeeds with empty output")
}

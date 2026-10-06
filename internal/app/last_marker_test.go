package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
	"github.com/fullerzz/herdr-plugin-sesh/internal/state"
)

func markerFixture(t *testing.T, history []string, workspaces ...herdr.Workspace) (*herdr.FakeClient, string) {
	t.Helper()
	historyDir := filepath.Join(t.TempDir(), "history")
	require.NoError(t, state.SaveHistory(historyDir, state.History{Workspaces: history}))
	return &herdr.FakeClient{Workspaces: workspaces}, historyDir
}

func markedWorkspaces(client *herdr.FakeClient) []string {
	var marked []string
	for _, w := range client.Workspaces {
		if w.Tokens[lastWorkspaceToken] != "" {
			marked = append(marked, w.ID)
		}
	}
	return marked
}

func TestLastWorkspaceMarkerFollowsFocus(t *testing.T) {
	ctx := context.Background()
	client, historyDir := markerFixture(t, []string{"A"}, herdr.Workspace{ID: "A"}, herdr.Workspace{ID: "B"}, herdr.Workspace{ID: "C"})

	require.NoError(t, state.Record(historyDir, "B"))
	require.NoError(t, syncLastWorkspaceMarker(ctx, client, historyDir, "B"))
	assert.Equal(t, []string{"A"}, markedWorkspaces(client))
	require.NoError(t, syncLastWorkspaceMarker(ctx, client, historyDir, "B"))
	require.NoError(t, state.Record(historyDir, "C"))
	require.NoError(t, syncLastWorkspaceMarker(ctx, client, historyDir, "C"))

	assert.Equal(t, []string{"B"}, markedWorkspaces(client))
	assert.Equal(t, []string{"A:sesh_last=last", "A:sesh_last=", "B:sesh_last=last"}, client.ReportedTokens)
}

// A delayed RecordSwitch can leave another workspace at the history head; the
// marker must still match what `last` picks from the focused workspace.
func TestLastWorkspaceMarkerUsesFocusedWorkspaceNotHistoryHead(t *testing.T) {
	client, historyDir := markerFixture(t, []string{"B", "A", "C"}, herdr.Workspace{ID: "A"}, herdr.Workspace{ID: "B"}, herdr.Workspace{ID: "C"})

	require.NoError(t, syncLastWorkspaceMarker(context.Background(), client, historyDir, "C"))

	assert.Equal(t, []string{"B"}, markedWorkspaces(client))
}

func TestLastWorkspaceMarkerRepairsStaleAndFailedClears(t *testing.T) {
	ctx := context.Background()
	stale := map[string]string{"sesh_last": "last", "other": "x"}
	client, historyDir := markerFixture(t, []string{"B", "A"},
		herdr.Workspace{ID: "A"}, herdr.Workspace{ID: "B"}, herdr.Workspace{ID: "C", Tokens: stale})
	failing := &failingClearClient{FakeClient: client, failID: "C"}

	require.Error(t, syncLastWorkspaceMarker(ctx, failing, historyDir, "B"))
	assert.Equal(t, []string{"A", "C"}, markedWorkspaces(client))
	failing.failID = ""
	require.NoError(t, syncLastWorkspaceMarker(ctx, failing, historyDir, "B"))

	assert.Equal(t, []string{"A"}, markedWorkspaces(client))
	assert.Equal(t, map[string]string{"other": "x"}, client.Workspaces[2].Tokens)
}

type failingClearClient struct {
	*herdr.FakeClient

	failID string
}

func (c *failingClearClient) WorkspaceReportToken(ctx context.Context, id, source, name, value string) error {
	if id == c.failID {
		return errors.New("report failed")
	}
	return c.FakeClient.WorkspaceReportToken(ctx, id, source, name, value)
}

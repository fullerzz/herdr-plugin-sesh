package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fullerzz/herdr-plugin-sesh/internal/config"
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
	require.NoError(t, syncLastWorkspaceMarker(ctx, client, historyDir, "B", config.DefaultLastWorkspaceLabel))
	assert.Equal(t, []string{"A"}, markedWorkspaces(client))
	require.NoError(t, syncLastWorkspaceMarker(ctx, client, historyDir, "B", config.DefaultLastWorkspaceLabel))
	require.NoError(t, state.Record(historyDir, "C"))
	require.NoError(t, syncLastWorkspaceMarker(ctx, client, historyDir, "C", config.DefaultLastWorkspaceLabel))

	assert.Equal(t, []string{"B"}, markedWorkspaces(client))
	assert.Equal(t, []string{"A:sesh_last=last", "A:sesh_last=", "B:sesh_last=last"}, client.ReportedTokens)
}

func TestLastWorkspaceMarkerUpdatesConfiguredLabel(t *testing.T) {
	ctx := context.Background()
	client, historyDir := markerFixture(t, []string{"B", "A"},
		herdr.Workspace{ID: "A", Tokens: map[string]string{lastWorkspaceToken: "last"}},
		herdr.Workspace{ID: "B"},
		herdr.Workspace{ID: "C", Tokens: map[string]string{lastWorkspaceToken: "last"}})

	require.NoError(t, syncLastWorkspaceMarker(ctx, client, historyDir, "B", "previous ↩"))
	assert.Equal(t, "previous ↩", client.Workspaces[0].Tokens[lastWorkspaceToken])
	assert.Equal(t, []string{"A"}, markedWorkspaces(client))
	require.NoError(t, syncLastWorkspaceMarker(ctx, client, historyDir, "B", "previous ↩"))
	assert.Equal(t, []string{"A:sesh_last=previous ↩", "C:sesh_last="}, client.ReportedTokens)

	require.NoError(t, syncLastWorkspaceMarker(ctx, client, historyDir, "A", "previous ↩"))
	assert.Equal(t, []string{"B"}, markedWorkspaces(client))
	assert.Equal(t, "previous ↩", client.Workspaces[1].Tokens[lastWorkspaceToken])
	require.NoError(t, syncLastWorkspaceMarker(ctx, client, historyDir, "A", ""))
	assert.Empty(t, markedWorkspaces(client))
}

// A delayed RecordSwitch can leave another workspace at the history head; the
// marker must still match what `last` picks from the focused workspace.
func TestLastWorkspaceMarkerUsesFocusedWorkspaceNotHistoryHead(t *testing.T) {
	client, historyDir := markerFixture(t, []string{"B", "A", "C"}, herdr.Workspace{ID: "A"}, herdr.Workspace{ID: "B"}, herdr.Workspace{ID: "C"})

	require.NoError(t, syncLastWorkspaceMarker(context.Background(), client, historyDir, "C", config.DefaultLastWorkspaceLabel))

	assert.Equal(t, []string{"B"}, markedWorkspaces(client))
}

// A late RecordSwitch from a picker switch A->B must not move `last` away from
// the marker after the watcher saw focus go B->C->A.
func TestLastWorkspaceMarkerAgreesWithLastAfterLateSwitch(t *testing.T) {
	ctx := context.Background()
	client, historyDir := markerFixture(t, []string{"A"}, herdr.Workspace{ID: "A"}, herdr.Workspace{ID: "B"}, herdr.Workspace{ID: "C"})
	revision, err := state.HistoryRevision(historyDir)
	require.NoError(t, err)
	for _, focused := range []string{"B", "C", "A"} {
		require.NoError(t, state.Record(historyDir, focused))
		require.NoError(t, syncLastWorkspaceMarker(ctx, client, historyDir, focused, config.DefaultLastWorkspaceLabel))
	}

	require.NoError(t, state.RecordSwitch(historyDir, "A", "B", revision))

	history, err := state.LoadHistory(historyDir)
	require.NoError(t, err)
	last, ok := history.PreviousWorkspace("A")
	require.True(t, ok)
	assert.Equal(t, "C", last)
	assert.Equal(t, []string{last}, markedWorkspaces(client))
}

func TestLastWorkspaceMarkerRepairsStaleAndFailedClears(t *testing.T) {
	ctx := context.Background()
	stale := map[string]string{"sesh_last": "last", "other": "x"}
	client, historyDir := markerFixture(t, []string{"B", "A"},
		herdr.Workspace{ID: "A"}, herdr.Workspace{ID: "B"}, herdr.Workspace{ID: "C", Tokens: stale})
	failing := &failingClearClient{FakeClient: client, failID: "C"}

	require.Error(t, syncLastWorkspaceMarker(ctx, failing, historyDir, "B", config.DefaultLastWorkspaceLabel))
	assert.Equal(t, []string{"A", "C"}, markedWorkspaces(client))
	failing.failID = ""
	require.NoError(t, syncLastWorkspaceMarker(ctx, failing, historyDir, "B", config.DefaultLastWorkspaceLabel))

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

// Every Herdr call in one sync shares a single short deadline, so a slow CLI
// cannot hold the watcher's next history mutation for a timeout per call.
func TestLastWorkspaceMarkerSyncSharesOneBoundedDeadline(t *testing.T) {
	client, historyDir := markerFixture(t, []string{"B", "A"},
		herdr.Workspace{ID: "A"}, herdr.Workspace{ID: "B"},
		herdr.Workspace{ID: "C", Tokens: map[string]string{lastWorkspaceToken: "last"}})
	recorder := &deadlineClient{FakeClient: client}

	start := time.Now()
	require.NoError(t, syncLastWorkspaceMarker(context.Background(), recorder, historyDir, "B", config.DefaultLastWorkspaceLabel))
	end := time.Now()

	require.Len(t, recorder.deadlines, 3, "list, set A, clear C")
	for _, deadline := range recorder.deadlines {
		assert.Equal(t, recorder.deadlines[0], deadline)
	}
	assert.WithinRange(t, recorder.deadlines[0], start.Add(markerSyncTimeout), end.Add(markerSyncTimeout))
}

type deadlineClient struct {
	*herdr.FakeClient

	deadlines []time.Time
}

func (c *deadlineClient) record(ctx context.Context) {
	deadline, _ := ctx.Deadline()
	c.deadlines = append(c.deadlines, deadline)
}

func (c *deadlineClient) WorkspaceList(ctx context.Context) ([]herdr.Workspace, error) {
	c.record(ctx)
	return c.FakeClient.WorkspaceList(ctx)
}

func (c *deadlineClient) WorkspaceReportToken(ctx context.Context, id, source, name, value string) error {
	c.record(ctx)
	return c.FakeClient.WorkspaceReportToken(ctx, id, source, name, value)
}

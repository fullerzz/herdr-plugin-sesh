package herdr

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type panePreviewRunner struct {
	calls    [][]string
	snapshot string
	err      error
}

func (r *panePreviewRunner) Run(_ context.Context, _ string, args ...string) ([]byte, []byte, error) {
	r.calls = append(r.calls, args)
	if len(r.calls) == 1 {
		return []byte(r.snapshot), nil, r.err
	}
	return []byte("\x1b[32mactive pane\x1b[0m\n"), nil, r.err
}

func TestWorkspacePaneReadTargetsBackgroundWorkspaceActiveTab(t *testing.T) {
	r := &panePreviewRunner{snapshot: `{"result":{"snapshot":{
		"focused_pane_id":"w1:p1",
		"workspaces":[{"workspace_id":"w1","active_tab_id":"w1:t1"},{"workspace_id":"w2","active_tab_id":"w2:t2"}],
		"layouts":[
			{"workspace_id":"w1","tab_id":"w1:t1","focused_pane_id":"w1:p1"},
			{"workspace_id":"w2","tab_id":"w2:t1","focused_pane_id":"w2:p1"},
			{"workspace_id":"w2","tab_id":"w2:t2","focused_pane_id":"w2:p3"}]
	}}}`}
	c := &CLIClient{Bin: "herdr", Runner: r}
	text, err := c.WorkspacePaneRead(context.Background(), "w2")
	require.NoError(t, err)
	assert.Equal(t, "\x1b[32mactive pane\x1b[0m\n", text)
	want := [][]string{{"api", "snapshot"}, {"pane", "read", "w2:p3", "--source", "visible", "--format", "ansi"}}
	assert.Equal(t, want, r.calls)
}

func TestWorkspacePaneReadRejectsMissingTargetsAndErrors(t *testing.T) {
	for _, tc := range []struct {
		name, id, snapshot string
		err                error
	}{
		{name: "empty workspace"},
		{name: "closed workspace", id: "w2", snapshot: `{"result":{"snapshot":{"workspaces":[]}}}`},
		{name: "missing layout", id: "w2", snapshot: `{"result":{"snapshot":{"workspaces":[{"workspace_id":"w2","active_tab_id":"w2:t2"}]}}}`},
		{name: "invalid JSON", id: "w2", snapshot: `{`},
		{name: "unavailable", id: "w2", err: errors.New("offline")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &panePreviewRunner{snapshot: tc.snapshot, err: tc.err}
			c := &CLIClient{Bin: "herdr", Runner: r}
			_, err := c.WorkspacePaneRead(context.Background(), tc.id)
			require.Error(t, err)
			var wantCalls [][]string
			if tc.id != "" {
				wantCalls = [][]string{{"api", "snapshot"}}
			}
			assert.Equal(t, wantCalls, r.calls)
		})
	}
}

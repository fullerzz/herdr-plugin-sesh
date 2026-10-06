package herdr

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recRunner struct{ calls [][]string }

func (r *recRunner) Run(_ context.Context, bin string, args ...string) ([]byte, []byte, error) {
	r.calls = append(r.calls, append([]string{bin}, args...))
	return []byte(`{"id":"ws1","label":"api","cwd":"/tmp/api"}`), nil, nil
}
func TestCLIClientConstructsWorkspaceCreate(t *testing.T) {
	rr := &recRunner{}
	c := &CLIClient{Bin: "/bin/herdr", Runner: rr}
	_, err := c.WorkspaceCreate(context.Background(), WorkspaceCreateRequest{CWD: "/tmp/api", Label: "api", Focus: true})
	require.NoError(t, err)
	want := [][]string{
		{"/bin/herdr", "workspace", "create", "--cwd", "/tmp/api", "--label", "api"},
		{"/bin/herdr", "workspace", "focus", "ws1"},
	}
	assert.Equal(t, want, rr.calls)
}

func TestCLIClientConstructsWorkspaceCreateNoFocus(t *testing.T) {
	rr := &recRunner{}
	c := &CLIClient{Bin: "/bin/herdr", Runner: rr}
	_, err := c.WorkspaceCreate(context.Background(), WorkspaceCreateRequest{CWD: "/tmp/api", Label: "api"})
	require.NoError(t, err)
	want := [][]string{{"/bin/herdr", "workspace", "create", "--cwd", "/tmp/api", "--label", "api", "--no-focus"}}
	assert.Equal(t, want, rr.calls)
}

func TestCLIClientConstructsWorkspaceClose(t *testing.T) {
	rr := &recRunner{}
	c := &CLIClient{Bin: "/bin/herdr", Runner: rr}
	require.NoError(t, c.WorkspaceClose(context.Background(), "ws1"))
	want := [][]string{{"/bin/herdr", "workspace", "close", "ws1"}}
	assert.Equal(t, want, rr.calls)
}

func TestCLIClientDecodesWorkspaceListEnvelope(t *testing.T) {
	c := &CLIClient{Bin: "/bin/herdr", Runner: fixedRunner{stdout: []byte(`{"result":{"workspaces":[{"workspace_id":"w1","label":"api","agent_status":"working"}]}}`)}}
	got, err := c.WorkspaceList(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "w1", got[0].ID)
	require.Equal(t, "api", got[0].Label)
	assert.Equal(t, "working", got[0].AgentStatus)
}

func TestCLIClientDecodesWorkspaceListWorktreeMetadata(t *testing.T) {
	c := &CLIClient{Bin: "/bin/herdr", Runner: fixedRunner{stdout: []byte(`{"result":{"workspaces":[{"workspace_id":"w-root","label":"project","worktree":{"checkout_path":"/repos/project","is_linked_worktree":false,"repo_key":"/repos/project/.git","repo_name":"project","repo_root":"/repos/project"}},{"workspace_id":"w-child","label":"feature","worktree":{"checkout_path":"/worktrees/feature","is_linked_worktree":true,"repo_key":"/repos/project/.git","repo_name":"project","repo_root":"/repos/project"}}]}}`)}}
	got, err := c.WorkspaceList(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 2)
	root, child := got[0].Worktree, got[1].Worktree
	require.NotNil(t, root)
	require.False(t, root.IsLinkedWorktree)
	require.Equal(t, "/repos/project", root.CheckoutPath)
	require.Equal(t, "/repos/project/.git", root.RepoKey)
	require.Equal(t, "project", root.RepoName)
	require.Equal(t, "/repos/project", root.RepoRoot)
	require.NotNil(t, child)
	require.True(t, child.IsLinkedWorktree)
	require.Equal(t, "/worktrees/feature", child.CheckoutPath)
	assert.Equal(t, root.RepoKey, child.RepoKey)
}

func TestCLIClientDecodesWorkspaceListArrayWithoutWorktreeMetadata(t *testing.T) {
	c := &CLIClient{Bin: "/bin/herdr", Runner: fixedRunner{stdout: []byte(`[{"id":"w1","label":"api","agent_status":"blocked"}]`)}}
	got, err := c.WorkspaceList(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "w1", got[0].ID)
	require.Equal(t, "api", got[0].Label)
	require.Equal(t, "blocked", got[0].AgentStatus)
	require.Nil(t, got[0].Worktree)
}

func TestCLIClientDecodesWorkspaceCreateEnvelope(t *testing.T) {
	c := &CLIClient{Bin: "/bin/herdr", Runner: fixedRunner{stdout: []byte(`{"result":{"root_pane":{"cwd":"/tmp/api","pane_id":"p1"},"workspace":{"workspace_id":"w1","label":"api"}}}`)}}
	got, err := c.WorkspaceCreate(context.Background(), WorkspaceCreateRequest{CWD: "/tmp/api", Label: "api", Focus: true})
	require.NoError(t, err)
	require.Equal(t, "w1", got.ID)
	require.Equal(t, "api", got.Label)
	assert.Equal(t, "/tmp/api", got.CWD)
}

func TestCLIClientDecodesTabCreateEnvelope(t *testing.T) {
	c := &CLIClient{Bin: "/bin/herdr", Runner: fixedRunner{stdout: []byte(`{"result":{"root_pane":{"cwd":"/tmp/api","pane_id":"p1"},"tab":{"tab_id":"w1:t2","workspace_id":"w1","label":"api"}}}`)}}
	got, err := c.TabCreate(context.Background(), TabCreateRequest{WorkspaceID: "w1", CWD: "/tmp/api", Label: "api", Focus: true})
	require.NoError(t, err)
	require.Equal(t, "w1:t2", got.ID)
	require.Equal(t, "w1", got.WorkspaceID)
	require.Equal(t, "/tmp/api", got.CWD)
	assert.Equal(t, "p1", got.PaneID)
}

func TestCLIClientDecodesTabListArray(t *testing.T) {
	c := &CLIClient{Bin: "/bin/herdr", Runner: fixedRunner{stdout: []byte(`[{"id":"w1:t1","workspace_id":"w1","label":"api"}]`)}}
	got, err := c.TabList(context.Background(), "w1")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "w1:t1", got[0].ID)
	require.Equal(t, "w1", got[0].WorkspaceID)
	assert.Equal(t, "api", got[0].Label)
}

func TestCLIClientDecodesPaneListEnvelope(t *testing.T) {
	c := &CLIClient{Bin: "/bin/herdr", Runner: fixedRunner{stdout: []byte(`{"result":{"panes":[{"pane_id":"p1","workspace_id":"w1","tab_id":"w1:t1","cwd":"/tmp/api","foreground_cwd":"/tmp/api/sub","focused":true}]}}`)}}
	got, err := c.PaneList(context.Background(), "")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "p1", got[0].ID)
	require.Equal(t, "w1", got[0].WorkspaceID)
	require.Equal(t, "/tmp/api/sub", got[0].ForegroundCWD)
	assert.True(t, got[0].Focused)
}

func TestCLIClientDecodesPaneCurrentEnvelope(t *testing.T) {
	c := &CLIClient{Bin: "/bin/herdr", Runner: fixedRunner{stdout: []byte(`{"result":{"pane":{"pane_id":"p1","workspace_id":"w1","tab_id":"w1:t1","cwd":"/tmp/api"}}}`)}}
	got, err := c.PaneCurrent(context.Background())
	require.NoError(t, err)
	require.Equal(t, "p1", got.ID)
	require.Equal(t, "w1", got.WorkspaceID)
	require.Equal(t, "w1:t1", got.TabID)
	assert.Equal(t, "/tmp/api", got.CWD)
}

func TestCLIClientPaneFocusedOmitsCallerPane(t *testing.T) {
	d := t.TempDir()
	bin := filepath.Join(d, "herdr")
	script := `#!/bin/sh
if [ -n "$HERDR_PANE_ID" ]; then
  printf '{"workspace_id":"caller"}\n'
else
  printf '{"workspace_id":"focused"}\n'
fi
`
	//nolint:gosec // test creates a local executable fixture.
	require.NoError(t, os.WriteFile(bin, []byte(script), 0700))
	t.Setenv("HERDR_PANE_ID", "stale-pane")
	c := &CLIClient{Bin: bin, Runner: ExecRunner{}}

	caller, err := c.PaneCurrent(context.Background())
	require.NoError(t, err)
	focused, err := c.PaneFocused(context.Background())
	require.NoError(t, err)
	require.Equal(t, "caller", caller.WorkspaceID)
	assert.Equal(t, "focused", focused.WorkspaceID)
}
func TestFakeClientRecordsPaneRun(t *testing.T) {
	f := &FakeClient{}
	_ = f.PaneRun(context.Background(), "p1", "npm test")
	assert.Equal(t, "p1:npm test", f.PaneRuns[0])
}

type fixedRunner struct {
	stdout []byte
	stderr []byte
	err    error
}

func (r fixedRunner) Run(context.Context, string, ...string) ([]byte, []byte, error) {
	return r.stdout, r.stderr, r.err
}

func TestCLIClientReturnsDecodeErrors(t *testing.T) {
	c := &CLIClient{Bin: "/bin/herdr", Runner: fixedRunner{stdout: []byte("not json")}}
	_, err := c.WorkspaceList(context.Background())
	require.Error(t, err)
	require.ErrorContains(t, err, "decode herdr workspace list JSON")
}

func TestCLIClientIncludesStderrOnCommandFailure(t *testing.T) {
	c := &CLIClient{Bin: "/bin/herdr", Runner: fixedRunner{stderr: []byte("boom\n"), err: errors.New("exit status 1")}}
	_, err := c.WorkspaceList(context.Background())
	require.Error(t, err)
	require.ErrorContains(t, err, "boom")
}

func TestCLIClientConstructsPaneSplit(t *testing.T) {
	rr := &recRunner{}
	c := &CLIClient{Bin: "/bin/herdr", Runner: rr}
	_, err := c.PaneSplit(context.Background(), PaneSplitRequest{PaneID: "w1-1", Direction: "right", Ratio: 1 - 0.35, CWD: "/tmp/web", Env: map[string]string{"Z": "last", "A": "a b"}})
	require.NoError(t, err)
	want := [][]string{{"/bin/herdr", "pane", "split", "w1-1", "--direction", "right", "--no-focus", "--ratio", "0.65", "--cwd", "/tmp/web", "--env", "A=a b", "--env", "Z=last"}}
	assert.Equal(t, want, rr.calls)
}

func TestCLIClientConstructsPaneWaitOutput(t *testing.T) {
	rr := &recRunner{}
	c := &CLIClient{Bin: "/bin/herdr", Runner: rr}
	require.NoError(t, c.PaneWaitOutput(context.Background(), "w1-2", "--ready", 15*time.Second))
	want := [][]string{{"/bin/herdr", "pane", "wait-output", "w1-2", "--match=--ready", "--source=recent-unwrapped", "--timeout=15000"}}
	assert.Equal(t, want, rr.calls)
}

func TestCLIClientReportsCanceledWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := &CLIClient{Bin: "/bin/herdr", Runner: fixedRunner{err: errors.New("signal: killed")}}
	err := c.PaneWaitOutput(ctx, "w1-2", "ready", time.Second)
	require.ErrorIs(t, err, context.Canceled)
}

func TestCLIClientCanceledCommandBoundsInheritedPipes(t *testing.T) {
	c := &CLIClient{Bin: "sh", Runner: ExecRunner{}, Timeout: 50 * time.Millisecond}
	start := time.Now()
	// The short-lived child keeps stdout open after cancellation kills the shell.
	out, err := c.runFor(context.Background(), 0, "-c", "printf ready; sleep 1 & wait")
	elapsed := time.Since(start)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, "ready", string(out))
	assert.Less(t, elapsed, 750*time.Millisecond, "cancellation must not wait for the child's inherited pipe")
	t.Logf("command returned after %s", elapsed)
}

func TestCLIClientDecodesPaneSplitEnvelope(t *testing.T) {
	c := &CLIClient{Bin: "/bin/herdr", Runner: fixedRunner{stdout: []byte(`{"result":{"type":"pane_info","pane":{"pane_id":"w1-3"}}}`)}}
	got, err := c.PaneSplit(context.Background(), PaneSplitRequest{PaneID: "w1-1", Direction: "down"})
	require.NoError(t, err)
	assert.Equal(t, "w1-3", got.ID)
	c.Runner = fixedRunner{stdout: []byte(`{"result":{"type":"pane_info","pane":{}}}`)}
	_, err = c.PaneSplit(context.Background(), PaneSplitRequest{PaneID: "w1-1", Direction: "down"})
	assert.ErrorContains(t, err, "no pane id")
}

func TestCLIClientTabCreatePassesEnv(t *testing.T) {
	rr := &recRunner{}
	c := &CLIClient{Bin: "/bin/herdr", Runner: rr}
	_, err := c.TabCreate(context.Background(), TabCreateRequest{WorkspaceID: "w1", CWD: "/tmp", Label: "dev", Env: map[string]string{"EDITOR": "nvim"}})
	require.NoError(t, err)
	require.Len(t, rr.calls, 1)
	assert.Equal(t, []string{"/bin/herdr", "tab", "create", "--workspace", "w1", "--cwd", "/tmp", "--label", "dev", "--env", "EDITOR=nvim", "--no-focus"}, rr.calls[0])
}

func TestCLIClientRedactsEnvValuesInErrors(t *testing.T) {
	c := &CLIClient{Bin: "/bin/herdr", Runner: fixedRunner{stderr: []byte("pane not found"), err: errors.New("exit status 1")}}
	_, err := c.PaneSplit(context.Background(), PaneSplitRequest{PaneID: "w1-1", Direction: "right", Env: map[string]string{"API_TOKEN": "s3cret=value"}})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "s3cret")
	assert.Contains(t, err.Error(), "--env API_TOKEN=<redacted>")
	_, err = c.TabCreate(context.Background(), TabCreateRequest{WorkspaceID: "w1", Env: map[string]string{"API_TOKEN": "s3cret"}})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "s3cret")
}

func TestCLIClientWorkspaceCreatePassesEnvAndTabRename(t *testing.T) {
	rr := &recRunner{}
	c := &CLIClient{Bin: "/bin/herdr", Runner: rr}
	_, err := c.WorkspaceCreate(context.Background(), WorkspaceCreateRequest{CWD: "/tmp", Label: "api", Env: map[string]string{"EDITOR": "nvim"}})
	require.NoError(t, err)
	require.NoError(t, c.TabRename(context.Background(), "w1:t1", "dev"))
	assert.Equal(t, [][]string{
		{"/bin/herdr", "workspace", "create", "--cwd", "/tmp", "--label", "api", "--env", "EDITOR=nvim", "--no-focus"},
		{"/bin/herdr", "tab", "rename", "w1:t1", "dev"},
	}, rr.calls)
}

func TestCLIClientPaneFocusUsesSocketAPI(t *testing.T) {
	listener, socketPath := listenTestSocket(t)
	requests := make(chan map[string]any, 2)
	go func() {
		for _, response := range []map[string]any{
			{"id": "herdr-sesh-pane-focus", "result": map[string]any{"type": "pane_info"}},
			{"id": "herdr-sesh-pane-focus", "error": map[string]any{"code": "pane_not_found", "message": "pane w1:p9 not found"}},
		} {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			var request map[string]any
			_ = json.NewDecoder(conn).Decode(&request)
			requests <- request
			_ = json.NewEncoder(conn).Encode(response)
			_ = conn.Close()
		}
	}()
	c := &CLIClient{Bin: "/bin/herdr", Runner: &recRunner{}, SocketPath: socketPath}

	require.NoError(t, c.PaneFocus(context.Background(), "w1:p2"))
	assert.Equal(t, map[string]any{"id": "herdr-sesh-pane-focus", "method": "pane.focus", "params": map[string]any{"pane_id": "w1:p2"}}, <-requests)
	err := c.PaneFocus(context.Background(), "w1:p9")
	require.EqualError(t, err, "focus pane w1:p9: pane_not_found: pane w1:p9 not found")
	<-requests

	c.SocketPath = ""
	require.ErrorContains(t, c.PaneFocus(context.Background(), "w1:p2"), "HERDR_SOCKET_PATH is not set")
}

func TestCLIClientWorkspaceReportTokenSetsAndClears(t *testing.T) {
	rr := &recRunner{}
	c := &CLIClient{Bin: "/bin/herdr", Runner: rr}
	require.NoError(t, c.WorkspaceReportToken(context.Background(), "w1", "src", "mark", "on"))
	require.NoError(t, c.WorkspaceReportToken(context.Background(), "w1", "src", "mark", ""))
	assert.Equal(t, [][]string{
		{"/bin/herdr", "workspace", "report-metadata", "w1", "--source", "src", "--token", "mark=on"},
		{"/bin/herdr", "workspace", "report-metadata", "w1", "--source", "src", "--clear-token", "mark"},
	}, rr.calls)
}

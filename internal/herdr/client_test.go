package herdr

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
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
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"/bin/herdr", "workspace", "create", "--cwd", "/tmp/api", "--label", "api"},
		{"/bin/herdr", "workspace", "focus", "ws1"},
	}
	if !reflect.DeepEqual(rr.calls, want) {
		t.Fatalf("got %#v want %#v", rr.calls, want)
	}
}

func TestCLIClientConstructsWorkspaceCreateNoFocus(t *testing.T) {
	rr := &recRunner{}
	c := &CLIClient{Bin: "/bin/herdr", Runner: rr}
	_, err := c.WorkspaceCreate(context.Background(), WorkspaceCreateRequest{CWD: "/tmp/api", Label: "api"})
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"/bin/herdr", "workspace", "create", "--cwd", "/tmp/api", "--label", "api", "--no-focus"}}
	if !reflect.DeepEqual(rr.calls, want) {
		t.Fatalf("got %#v want %#v", rr.calls, want)
	}
}

func TestCLIClientConstructsWorkspaceClose(t *testing.T) {
	rr := &recRunner{}
	c := &CLIClient{Bin: "/bin/herdr", Runner: rr}
	if err := c.WorkspaceClose(context.Background(), "ws1"); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"/bin/herdr", "workspace", "close", "ws1"}}
	if !reflect.DeepEqual(rr.calls, want) {
		t.Fatalf("got %#v want %#v", rr.calls, want)
	}
}

func TestCLIClientConstructsWorkspaceReportMetadata(t *testing.T) {
	rr := &recRunner{}
	c := &CLIClient{Bin: "/bin/herdr", Runner: rr}
	err := c.WorkspaceReportMetadata(context.Background(), WorkspaceMetadataRequest{
		WorkspaceID: "w1",
		Source:      "fullerzz.sesh",
		Tokens:      map[string]string{"sesh": "◀ sesh"},
		ClearTokens: []string{"old"},
		TTLMS:       3000,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{{
		"/bin/herdr", "workspace", "report-metadata", "w1",
		"--source", "fullerzz.sesh",
		"--token", "sesh=◀ sesh",
		"--clear-token", "old",
		"--ttl-ms", "3000",
	}}
	if !reflect.DeepEqual(rr.calls, want) {
		t.Fatalf("got %#v want %#v", rr.calls, want)
	}
}

func TestCLIClientDecodesWorkspaceListEnvelope(t *testing.T) {
	c := &CLIClient{Bin: "/bin/herdr", Runner: fixedRunner{stdout: []byte(`{"result":{"workspaces":[{"workspace_id":"w1","label":"api","agent_status":"working"}]}}`)}}
	got, err := c.WorkspaceList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "w1" || got[0].Label != "api" || got[0].AgentStatus != "working" {
		t.Fatalf("workspaces=%#v", got)
	}
}

func TestCLIClientDecodesWorkspaceListWorktreeMetadata(t *testing.T) {
	c := &CLIClient{Bin: "/bin/herdr", Runner: fixedRunner{stdout: []byte(`{"result":{"workspaces":[{"workspace_id":"w-root","label":"project","worktree":{"checkout_path":"/repos/project","is_linked_worktree":false,"repo_key":"/repos/project/.git","repo_name":"project","repo_root":"/repos/project"}},{"workspace_id":"w-child","label":"feature","worktree":{"checkout_path":"/worktrees/feature","is_linked_worktree":true,"repo_key":"/repos/project/.git","repo_name":"project","repo_root":"/repos/project"}}]}}`)}}
	got, err := c.WorkspaceList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("workspaces=%#v", got)
	}
	root, child := got[0].Worktree, got[1].Worktree
	if root == nil || root.IsLinkedWorktree || root.CheckoutPath != "/repos/project" || root.RepoKey != "/repos/project/.git" || root.RepoName != "project" || root.RepoRoot != "/repos/project" {
		t.Fatalf("root worktree=%#v", root)
	}
	if child == nil || !child.IsLinkedWorktree || child.CheckoutPath != "/worktrees/feature" || child.RepoKey != root.RepoKey {
		t.Fatalf("child worktree=%#v", child)
	}
}

func TestCLIClientDecodesWorkspaceListArrayWithoutWorktreeMetadata(t *testing.T) {
	c := &CLIClient{Bin: "/bin/herdr", Runner: fixedRunner{stdout: []byte(`[{"id":"w1","label":"api","agent_status":"blocked"}]`)}}
	got, err := c.WorkspaceList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "w1" || got[0].Label != "api" || got[0].AgentStatus != "blocked" || got[0].Worktree != nil {
		t.Fatalf("workspaces=%#v", got)
	}
}

func TestCLIClientDecodesWorkspaceCreateEnvelope(t *testing.T) {
	c := &CLIClient{Bin: "/bin/herdr", Runner: fixedRunner{stdout: []byte(`{"result":{"root_pane":{"cwd":"/tmp/api","pane_id":"p1"},"workspace":{"workspace_id":"w1","label":"api"}}}`)}}
	got, err := c.WorkspaceCreate(context.Background(), WorkspaceCreateRequest{CWD: "/tmp/api", Label: "api", Focus: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "w1" || got.Label != "api" || got.CWD != "/tmp/api" {
		t.Fatalf("workspace=%#v", got)
	}
}

func TestCLIClientDecodesTabCreateEnvelope(t *testing.T) {
	c := &CLIClient{Bin: "/bin/herdr", Runner: fixedRunner{stdout: []byte(`{"result":{"root_pane":{"cwd":"/tmp/api","pane_id":"p1"},"tab":{"tab_id":"w1:t2","workspace_id":"w1","label":"api"}}}`)}}
	got, err := c.TabCreate(context.Background(), TabCreateRequest{WorkspaceID: "w1", CWD: "/tmp/api", Label: "api", Focus: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "w1:t2" || got.WorkspaceID != "w1" || got.CWD != "/tmp/api" || got.PaneID != "p1" {
		t.Fatalf("tab=%#v", got)
	}
}

func TestCLIClientDecodesTabListArray(t *testing.T) {
	c := &CLIClient{Bin: "/bin/herdr", Runner: fixedRunner{stdout: []byte(`[{"id":"w1:t1","workspace_id":"w1","label":"api"}]`)}}
	got, err := c.TabList(context.Background(), "w1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "w1:t1" || got[0].WorkspaceID != "w1" || got[0].Label != "api" {
		t.Fatalf("tabs=%#v", got)
	}
}

func TestCLIClientDecodesPaneListEnvelope(t *testing.T) {
	c := &CLIClient{Bin: "/bin/herdr", Runner: fixedRunner{stdout: []byte(`{"result":{"panes":[{"pane_id":"p1","workspace_id":"w1","tab_id":"w1:t1","cwd":"/tmp/api","foreground_cwd":"/tmp/api/sub","focused":true}]}}`)}}
	got, err := c.PaneList(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "p1" || got[0].WorkspaceID != "w1" || got[0].ForegroundCWD != "/tmp/api/sub" || !got[0].Focused {
		t.Fatalf("panes=%#v", got)
	}
}

func TestCLIClientDecodesPaneCurrentEnvelope(t *testing.T) {
	c := &CLIClient{Bin: "/bin/herdr", Runner: fixedRunner{stdout: []byte(`{"result":{"pane":{"pane_id":"p1","workspace_id":"w1","tab_id":"w1:t1","cwd":"/tmp/api"}}}`)}}
	got, err := c.PaneCurrent(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "p1" || got.WorkspaceID != "w1" || got.TabID != "w1:t1" || got.CWD != "/tmp/api" {
		t.Fatalf("pane=%#v", got)
	}
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
	if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_PANE_ID", "stale-pane")
	c := &CLIClient{Bin: bin, Runner: ExecRunner{}}

	caller, err := c.PaneCurrent(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	focused, err := c.PaneFocused(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if caller.WorkspaceID != "caller" || focused.WorkspaceID != "focused" {
		t.Fatalf("caller=%q focused=%q", caller.WorkspaceID, focused.WorkspaceID)
	}
}
func TestFakeClientRecordsPaneRun(t *testing.T) {
	f := &FakeClient{}
	_ = f.PaneRun(context.Background(), "p1", "npm test")
	if f.PaneRuns[0] != "p1:npm test" {
		t.Fatal(f.PaneRuns)
	}
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
	if err == nil {
		t.Fatal("expected decode error")
	}
	if !strings.Contains(err.Error(), "decode herdr workspace list JSON") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCLIClientIncludesStderrOnCommandFailure(t *testing.T) {
	c := &CLIClient{Bin: "/bin/herdr", Runner: fixedRunner{stderr: []byte("boom\n"), err: errors.New("exit status 1")}}
	_, err := c.WorkspaceList(context.Background())
	if err == nil {
		t.Fatal("expected command error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("unexpected error: %v", err)
	}
}

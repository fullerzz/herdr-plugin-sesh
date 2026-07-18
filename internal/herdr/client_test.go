package herdr

import (
	"context"
	"errors"
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

func TestCLIClientDecodesWorkspaceListEnvelope(t *testing.T) {
	c := &CLIClient{Bin: "/bin/herdr", Runner: fixedRunner{stdout: []byte(`{"result":{"workspaces":[{"workspace_id":"w1","label":"api","agent_status":"working","tab_count":2,"pane_count":3}]}}`)}}
	got, err := c.WorkspaceList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "w1" || got[0].Label != "api" || got[0].AgentStatus != "working" || got[0].TabCount != 2 || got[0].PaneCount != 3 {
		t.Fatalf("workspaces=%#v", got)
	}
}

func TestCLIClientDecodesWorkspaceListArray(t *testing.T) {
	c := &CLIClient{Bin: "/bin/herdr", Runner: fixedRunner{stdout: []byte(`[{"id":"w1","label":"api","agent_status":"blocked"}]`)}}
	got, err := c.WorkspaceList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "w1" || got[0].Label != "api" || got[0].AgentStatus != "blocked" {
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
	c := &CLIClient{Bin: "/bin/herdr", Runner: fixedRunner{stdout: []byte(`[{"id":"w1:t1","workspace_id":"w1","number":1,"label":"api"}]`)}}
	got, err := c.TabList(context.Background(), "w1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "w1:t1" || got[0].WorkspaceID != "w1" || got[0].Number != 1 || got[0].Label != "api" {
		t.Fatalf("tabs=%#v", got)
	}
}

func TestCLIClientDecodesPaneListEnvelope(t *testing.T) {
	c := &CLIClient{Bin: "/bin/herdr", Runner: fixedRunner{stdout: []byte(`{"result":{"panes":[{"pane_id":"p1","workspace_id":"w1","tab_id":"w1:t1","cwd":"/tmp/api","foreground_cwd":"/tmp/api/sub","focused":true,"label":"api","agent":"codex","title":"editing","display_agent":"Codex","agent_status":"working"}]}}`)}}
	got, err := c.PaneList(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "p1" || got[0].WorkspaceID != "w1" || got[0].ForegroundCWD != "/tmp/api/sub" || !got[0].Focused || got[0].DisplayAgent != "Codex" || got[0].AgentStatus != "working" {
		t.Fatalf("panes=%#v", got)
	}
}

func TestCLIClientDecodesPaneLayoutEnvelope(t *testing.T) {
	runner := &recordingFixedRunner{stdout: []byte(`{"result":{"layout":{"workspace_id":"w1","tab_id":"w1:t1","zoomed":false,"area":{"x":0,"y":0,"width":120,"height":40},"focused_pane_id":"w1:p1","panes":[{"pane_id":"w1:p1","focused":true,"rect":{"x":0,"y":0,"width":60,"height":40}},{"pane_id":"w1:p2","focused":false,"rect":{"x":60,"y":0,"width":60,"height":40}}]}}}`)}
	c := &CLIClient{Bin: "/bin/herdr", Runner: runner}

	got, err := c.PaneLayout(context.Background(), "w1:p1")
	if err != nil {
		t.Fatal(err)
	}
	if want := [][]string{{"/bin/herdr", "pane", "layout", "--pane", "w1:p1"}}; !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls=%#v want %#v", runner.calls, want)
	}
	if got.WorkspaceID != "w1" || got.TabID != "w1:t1" || got.Area.Width != 120 || got.FocusedPaneID != "w1:p1" || len(got.Panes) != 2 || got.Panes[1].Rect.X != 60 {
		t.Fatalf("layout=%#v", got)
	}
}

func TestCLIClientDecodesPaneRunningCommandEnvelope(t *testing.T) {
	runner := &recordingFixedRunner{stdout: []byte(`{"result":{"process_info":{"pane_id":"w1:p1","shell_pid":100,"foreground_process_group_id":200,"foreground_processes":[{"pid":201,"name":"helper","cmdline":"helper"},{"pid":200,"name":"npm","argv":["npm","run","dev"],"cmdline":"npm run dev"}]}}}`)}
	c := &CLIClient{Bin: "/bin/herdr", Runner: runner}

	got, err := c.PaneRunningCommand(context.Background(), "w1:p1")
	if err != nil {
		t.Fatal(err)
	}
	if want := [][]string{{"/bin/herdr", "pane", "process-info", "--pane", "w1:p1"}}; !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls=%#v want %#v", runner.calls, want)
	}
	if got != "npm run dev" {
		t.Fatalf("command=%q, want %q", got, "npm run dev")
	}

	idle := &CLIClient{Bin: "/bin/herdr", Runner: fixedRunner{stdout: []byte(`{"result":{"process_info":{"shell_pid":100,"foreground_process_group_id":100,"foreground_processes":[{"pid":100,"name":"zsh","cmdline":"zsh"}]}}}`)}}
	got, err = idle.PaneRunningCommand(context.Background(), "w1:p1")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("idle shell command=%q, want empty", got)
	}

	unresolved := &CLIClient{Bin: "/bin/herdr", Runner: fixedRunner{stdout: []byte(`{"result":{"process_info":{"shell_pid":100,"foreground_process_group_id":200,"foreground_processes":[{"pid":200,"name":"unknown"}]}}}`)}}
	got, err = unresolved.PaneRunningCommand(context.Background(), "w1:p1")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("unresolved command=%q, want empty", got)
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

type recordingFixedRunner struct {
	stdout []byte
	calls  [][]string
}

func (r *recordingFixedRunner) Run(_ context.Context, bin string, args ...string) ([]byte, []byte, error) {
	r.calls = append(r.calls, append([]string{bin}, args...))
	return r.stdout, nil, nil
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

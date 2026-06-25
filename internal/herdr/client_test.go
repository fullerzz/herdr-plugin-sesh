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
	c := &CLIClient{Bin: "/bin/herdr", Runner: fixedRunner{stdout: []byte(`{"result":{"workspaces":[{"workspace_id":"w1","label":"api"}]}}`)}}
	got, err := c.WorkspaceList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "w1" || got[0].Label != "api" {
		t.Fatalf("workspaces=%#v", got)
	}
}

func TestCLIClientDecodesWorkspaceListArray(t *testing.T) {
	c := &CLIClient{Bin: "/bin/herdr", Runner: fixedRunner{stdout: []byte(`[{"id":"w1","label":"api"}]`)}}
	got, err := c.WorkspaceList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "w1" || got[0].Label != "api" {
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

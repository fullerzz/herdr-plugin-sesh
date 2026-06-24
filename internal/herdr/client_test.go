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
	want := []string{"/bin/herdr", "workspace", "create", "--cwd", "/tmp/api", "--label", "api", "--focus", "--json"}
	if !reflect.DeepEqual(rr.calls[0], want) {
		t.Fatalf("got %#v want %#v", rr.calls[0], want)
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

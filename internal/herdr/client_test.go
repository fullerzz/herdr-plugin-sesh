package herdr

import (
	"context"
	"reflect"
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

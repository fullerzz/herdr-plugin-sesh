package herdr

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// appRunner answers by "<bin> <first arg> [<second arg>]" and records every call.
type appRunner struct {
	replies map[string][]string
	calls   [][]string
}

func (r *appRunner) Run(_ context.Context, bin string, args ...string) ([]byte, []byte, error) {
	r.calls = append(r.calls, append([]string{bin}, args...))
	key := bin + " " + args[0]
	if args[0] == "do" {
		key += " " + args[1]
	}
	queue := r.replies[key]
	if len(queue) == 0 {
		return []byte(`{"ok":true}`), nil, nil
	}
	out := queue[0]
	if len(queue) > 1 {
		r.replies[key] = queue[1:]
	}
	if strings.Contains(out, `"error"`) {
		return []byte(out), nil, errors.New("exit status 1")
	}
	return []byte(out), nil, nil
}

func newTestAppClient(replies map[string][]string) (*AppClient, *appRunner) {
	r := &appRunner{replies: replies}
	return &AppClient{AppBin: "herdr-app", CoreBin: "hc", Runner: r, PollInterval: time.Millisecond}, r
}

const appLS = `{"spaces":[
 {"id":"sp1","name":"api","folder":"/src/api","machine":null,"status":"working","tabs":[
  {"id":"tb1","name":"1","active":false,"terminals":[{"terminal":"t1","pane":"p1"}]},
  {"id":"tb2","name":"2","active":true,"terminals":[{"terminal":"t2","pane":"p2"}]}]},
 {"id":"buntu/sp9","name":"web","folder":"/home/z/web","machine":"buntu","status":null,"tabs":[
  {"id":"buntu/tb9","name":"1","active":true,"terminals":[{"terminal":"buntu/t9","pane":"p9"}]}]}]}`

const hcLS = `{"terminals":[
 {"address":"t1","cwd":"/src/api"},
 {"address":"t2","cwd":"/src/api","foreground":{"cwd":"/src/api/cmd"}}]}`

func TestAppClientListsSpacesAsWorkspaces(t *testing.T) {
	c, _ := newTestAppClient(map[string][]string{"herdr-app ls": {appLS}, "hc ls": {hcLS}})
	got, err := c.WorkspaceList(context.Background())
	require.NoError(t, err)
	want := []Workspace{
		{ID: "sp1", Label: "api", CWD: "/src/api", ForegroundCWD: "/src/api/cmd", ActiveTabID: "tb2", AgentStatus: "working"},
		{ID: "buntu/sp9", Label: "web", CWD: "/home/z/web", ActiveTabID: "buntu/tb9"},
	}
	assert.Equal(t, want, got)
}

func TestAppClientUnfocusedWorkspaceCreateRestoresShownTabAndExportsEnv(t *testing.T) {
	c, r := newTestAppClient(map[string][]string{
		"herdr-app status":       {`{"space":"sp0","tab":"tb0","focused_pane":"p0"}`},
		"herdr-app do space.new": {`{"space":"sp1","tab":"tb1","terminal":"t7"}`},
	})
	w, err := c.WorkspaceCreate(context.Background(), WorkspaceCreateRequest{CWD: "/src/api", Label: "api", Env: map[string]string{"B": "it's", "A": "1"}})
	require.NoError(t, err)
	assert.Equal(t, Workspace{ID: "sp1", Label: "api", CWD: "/src/api", ActiveTabID: "tb1"}, w)
	want := [][]string{
		{"herdr-app", "status"},
		{"herdr-app", "do", "space.new", `{"folder":"/src/api","name":"api"}`},
		{"hc", "send", "t7", `export A='1' B='it'\''s'`},
		{"herdr-app", "do", "tab.select", `{"id":"tb0"}`},
	}
	assert.Equal(t, want, r.calls)
}

func TestAppClientFocusedTabCreateLeavesFocus(t *testing.T) {
	c, r := newTestAppClient(map[string][]string{
		"herdr-app do tab.new": {`{"space":"sp1","tab":"tb3","terminal":"t3"}`},
		"herdr-app ls":         {`{"spaces":[{"id":"sp1","tabs":[{"id":"tb3","terminals":[{"terminal":"t3","pane":"p3"}]}]}]}`},
	})
	tab, err := c.TabCreate(context.Background(), TabCreateRequest{WorkspaceID: "sp1", CWD: "/src", Label: "logs", Focus: true})
	require.NoError(t, err)
	assert.Equal(t, Tab{ID: "tb3", WorkspaceID: "sp1", Label: "logs", CWD: "/src", PaneID: "p3"}, tab)
	want := [][]string{
		{"herdr-app", "do", "tab.new", `{"folder":"/src","name":"logs","space":"sp1"}`},
		{"herdr-app", "ls"},
	}
	assert.Equal(t, want, r.calls)
}

func TestAppClientSplitTypesDirectoryThenRunsInNewTerminal(t *testing.T) {
	c, r := newTestAppClient(map[string][]string{"herdr-app do pane.split": {`{"pane":"p5","terminal":"t5"}`}})
	p, err := c.PaneSplit(context.Background(), PaneSplitRequest{PaneID: "p1", Direction: "down", CWD: "/src/web", Ratio: 0.3})
	require.NoError(t, err)
	require.NoError(t, c.PaneRun(context.Background(), p.ID, "npm run dev"))
	want := [][]string{
		{"herdr-app", "do", "pane.split", `{"direction":"down","pane":"p1"}`},
		{"hc", "send", "t5", `cd -- '/src/web'`},
		{"hc", "send", "t5", "npm run dev"},
	}
	assert.Equal(t, want, r.calls)
}

func TestAppClientReportsHerdrAppErrors(t *testing.T) {
	c, _ := newTestAppClient(map[string][]string{"herdr-app do space.select": {`{"error":"no such space"}`}})
	err := c.WorkspaceFocus(context.Background(), "nope")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no such space")
}

func TestAppClientWaitsForOutputAlreadyPrintedOrLater(t *testing.T) {
	c, _ := newTestAppClient(map[string][]string{
		"herdr-app ls": {appLS},
		"hc read":      {"booting", "booting\nready on :3000"},
	})
	require.NoError(t, c.PaneWaitOutput(context.Background(), "p2", "ready on", time.Second))
}

func TestAppClientWaitForOutputTimesOut(t *testing.T) {
	c, _ := newTestAppClient(map[string][]string{"herdr-app ls": {appLS}, "hc read": {"booting"}})
	err := c.PaneWaitOutput(context.Background(), "p2", "ready on", 20*time.Millisecond)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestAppClientFocusedPaneIsShownSpace(t *testing.T) {
	c, _ := newTestAppClient(map[string][]string{"herdr-app status": {`{"space":"sp1","tab":"tb2","focused_pane":null}`}})
	p, err := c.PaneFocused(context.Background())
	require.NoError(t, err)
	assert.Equal(t, Pane{WorkspaceID: "sp1", TabID: "tb2", Focused: true}, p)
}

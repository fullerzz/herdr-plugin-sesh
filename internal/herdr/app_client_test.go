package herdr

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
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
 {"id":"sp1","name":"api","folder":"/src/api","machine":null,"tabs":[
  {"id":"tb1","name":"1","active":false,"terminals":[{"terminal":"t1","pane":"p1","status":"idle"}]},
  {"id":"tb2","name":"2","active":true,"terminals":[{"terminal":"t2","pane":"p2","status":"working"},{"terminal":"t3","pane":"p3","status":null}]}]},
 {"id":"buntu/sp9","name":"web","folder":"/home/z/web","machine":"buntu","tabs":[
  {"id":"buntu/tb9","name":"1","active":true,"terminals":[{"terminal":"buntu/t9","pane":"buntu/p9","status":null}]}]}]}`

// decodeSetup returns the commands an encoded terminal setup line runs.
func decodeSetup(t *testing.T, line string) string {
	t.Helper()
	const prefix, suffix = ` eval "$(printf '`, `')"`
	require.True(t, strings.HasPrefix(line, prefix) && strings.HasSuffix(line, suffix), "setup line %q", line)
	body := strings.TrimSuffix(strings.TrimPrefix(line, prefix), suffix)
	var out []byte
	for len(body) >= 4 && body[0] == '\\' {
		n, err := strconv.ParseUint(body[1:4], 8, 8)
		require.NoError(t, err)
		out = append(out, byte(n))
		body = body[4:]
	}
	require.Empty(t, body, "setup line must be fully octal-encoded")
	return string(out)
}

const hcLS = `{"terminals":[
 {"address":"t1","cwd":"/src/api"},
 {"address":"t2","cwd":"/src/api","foreground":{"cwd":"/src/api/cmd"}}]}`

func TestAppClientListsSpacesAsWorkspaces(t *testing.T) {
	c, _ := newTestAppClient(map[string][]string{"herdr-app ls": {appLS}, "hc ls": {hcLS}})
	got, err := c.WorkspaceList(context.Background())
	require.NoError(t, err)
	// herdr-app reports status per terminal; a space shows its most urgent one.
	want := []Workspace{
		{ID: "sp1", Label: "api", CWD: "/src/api", ForegroundCWD: "/src/api/cmd", ActiveTabID: "tb2", AgentStatus: "working"},
		{ID: "buntu/sp9", Machine: "buntu", Label: "web", CWD: "/home/z/web", ActiveTabID: "buntu/tb9"},
	}
	assert.Equal(t, want, got)
}

func TestAppClientKeepsRemoteFolderAndQualifiedSelection(t *testing.T) {
	c, r := newTestAppClient(map[string][]string{
		"herdr-app ls": {appLS},
		"hc ls":        {`{"terminals":[{"address":"buntu/t9","cwd":"/local/wrong"}]}`},
	})
	got, err := c.WorkspaceList(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "buntu", got[1].Machine)
	assert.Empty(t, got[1].ForegroundCWD)
	assert.Equal(t, "/home/z/web", got[1].CWD)
	require.NoError(t, c.WorkspaceFocus(context.Background(), got[1].ID))
	assert.Equal(t, []string{"herdr-app", "do", "space.select", `{"id":"buntu/sp9"}`}, r.calls[len(r.calls)-1])
}

func TestAppClientReadsRemotePaneThroughQualifiedTerminal(t *testing.T) {
	c, r := newTestAppClient(map[string][]string{
		"herdr-app ls":     {appLS},
		"herdr-app status": {`{"space":"sp1","focused_pane":"p2"}`},
		"hc read":          {"remote terminal output"},
	})
	got, err := c.WorkspacePaneRead(context.Background(), "buntu/sp9")
	require.NoError(t, err)
	assert.Equal(t, "remote terminal output", got)
	assert.Equal(t, []string{"hc", "read", "buntu/t9"}, r.calls[len(r.calls)-1])
}

func TestAppClientUnfocusedWorkspaceCreateRestoresShownTabAndExportsEnv(t *testing.T) {
	c, r := newTestAppClient(map[string][]string{
		"herdr-app status":       {`{"space":"sp0","tab":"tb0","focused_pane":"p0"}`},
		"herdr-app do space.new": {`{"space":"sp1","tab":"tb1","terminal":"t7"}`},
	})
	w, err := c.WorkspaceCreate(context.Background(), WorkspaceCreateRequest{CWD: "/src/api", Label: "api", Env: map[string]string{"B": "it's", "A": "1"}})
	require.NoError(t, err)
	assert.Equal(t, Workspace{ID: "sp1", Label: "api", CWD: "/src/api", ActiveTabID: "tb1"}, w)
	require.Len(t, r.calls, 4)
	assert.Equal(t, []string{"herdr-app", "status"}, r.calls[0])
	assert.Equal(t, []string{"herdr-app", "do", "space.new", `{"folder":"/src/api","name":"api"}`}, r.calls[1])
	assert.Equal(t, []string{"hc", "send", "t7"}, r.calls[2][:3])
	assert.Equal(t, `export A='1' B='it'\''s'`, decodeSetup(t, r.calls[2][3]))
	assert.Equal(t, []string{"herdr-app", "do", "tab.select", `{"id":"tb0"}`}, r.calls[3])
}

func TestAppClientRestoresShownTabWhenSetupFailsAndHidesEnvValues(t *testing.T) {
	c, r := newTestAppClient(map[string][]string{
		"herdr-app status":       {`{"space":"sp0","tab":"tb0","focused_pane":"p0"}`},
		"herdr-app do space.new": {`{"space":"sp1","tab":"tb1","terminal":"t7"}`},
		"hc send":                {`{"error":"terminal is gone"}`},
	})
	_, err := c.WorkspaceCreate(context.Background(), WorkspaceCreateRequest{Label: "api", Env: map[string]string{"API_TOKEN": "s3cr3t"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "terminal is gone")
	assert.NotContains(t, err.Error(), "s3cr3t")
	assert.NotContains(t, err.Error(), "API_TOKEN")
	assert.Equal(t, []string{"herdr-app", "do", "tab.select", `{"id":"tb0"}`}, r.calls[len(r.calls)-1])
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
	var sends [][]string
	for _, call := range r.calls {
		if call[0] == "hc" && call[1] == "send" {
			sends = append(sends, call)
		}
	}
	require.Len(t, sends, 2)
	assert.Equal(t, `cd -- '/src/web'`, decodeSetup(t, sends[0][3]))
	assert.Equal(t, []string{"hc", "send", "t5", "npm run dev"}, sends[1])
	assert.Contains(t, r.calls, []string{"herdr-app", "do", "pane.split", `{"direction":"down","pane":"p1"}`})
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

func TestAppClientSpaceStatusIsMostUrgentTerminalStatus(t *testing.T) {
	c, _ := newTestAppClient(map[string][]string{"herdr-app ls": {`{"spaces":[{"id":"sp1","tabs":[
	 {"id":"tb1","terminals":[{"terminal":"t1","pane":"p1","status":"working"},{"terminal":"t2","pane":"p2","status":"blocked"}]},
	 {"id":"tb2","terminals":[{"terminal":"t3","pane":"p3","status":"done"}]}]}]}`}})
	got, err := c.WorkspaceList(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "blocked", got[0].AgentStatus)
}

func TestAppClientSplitGivesFocusBackInShownTab(t *testing.T) {
	c, r := newTestAppClient(map[string][]string{
		"herdr-app status":        {`{"space":"sp1","tab":"tb1","focused_pane":"p1"}`, `{"space":"sp1","tab":"tb1","focused_pane":"p5"}`},
		"herdr-app do pane.split": {`{"pane":"p5","terminal":"t5"}`},
	})
	_, err := c.PaneSplit(context.Background(), PaneSplitRequest{PaneID: "p1", Direction: "right"})
	require.NoError(t, err)
	assert.Equal(t, []string{"herdr-app", "do", "pane.focus", `{"pane":"p1"}`}, r.calls[len(r.calls)-1])
}

func TestAppClientSplitInHiddenTabLeavesShownFocus(t *testing.T) {
	c, r := newTestAppClient(map[string][]string{
		"herdr-app status":        {`{"space":"sp0","tab":"tb0","focused_pane":"p0"}`},
		"herdr-app do pane.split": {`{"pane":"p5","terminal":"t5"}`},
	})
	_, err := c.PaneSplit(context.Background(), PaneSplitRequest{PaneID: "p1", Direction: "right"})
	require.NoError(t, err)
	for _, call := range r.calls {
		assert.NotEqual(t, "pane.focus", call[len(call)-2], "split in a hidden tab must not change focus")
	}
}

func TestAppClientWaitForIgnoresEchoedSetupLine(t *testing.T) {
	c, r := newTestAppClient(map[string][]string{"herdr-app do pane.split": {`{"pane":"p5","terminal":"t5"}`}})
	_, err := c.PaneSplit(context.Background(), PaneSplitRequest{PaneID: "p1", Direction: "down", CWD: "/srv/ready", Env: map[string]string{"PORT": "8080"}})
	require.NoError(t, err)
	var echo string
	for _, call := range r.calls {
		if call[0] == "hc" && call[1] == "send" {
			echo = call[3]
		}
	}
	r.replies["hc read"] = []string{echo + "\nnpm start"}
	for _, match := range []string{"8080", "ready", "PORT"} {
		err := c.PaneWaitOutput(context.Background(), "p5", match, 20*time.Millisecond)
		require.ErrorIs(t, err, context.DeadlineExceeded, "match %q", match)
	}
	err = c.PaneWaitOutput(context.Background(), "p5", "printf", time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "terminal setup line")
}

func TestAppClientListsPanesWithFocusAndLiveDirectory(t *testing.T) {
	c, _ := newTestAppClient(map[string][]string{
		"herdr-app ls":     {appLS},
		"hc ls":            {hcLS},
		"herdr-app status": {`{"space":"sp1","tab":"tb2","focused_pane":"p2"}`},
	})
	c.CallerTerminal = "t1"
	got, err := c.PaneList(context.Background(), "sp1")
	require.NoError(t, err)
	want := []Pane{
		{ID: "p1", WorkspaceID: "sp1", TabID: "tb1", CWD: "/src/api"},
		{ID: "p2", WorkspaceID: "sp1", TabID: "tb2", CWD: "/src/api/cmd", Focused: true},
		{ID: "p3", WorkspaceID: "sp1", TabID: "tb2"},
	}
	assert.Equal(t, want, got)
	current, err := c.PaneCurrent(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "p1", current.ID)
}

// Run with -race: the picker calls the client from concurrent commands.
func TestAppClientIsSafeForConcurrentUse(t *testing.T) {
	c := &AppClient{AppBin: "herdr-app", CoreBin: "hc", Runner: fixedRunner{stdout: []byte(appLS)}}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Go(func() {
			_, err := c.WorkspaceList(context.Background())
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
}

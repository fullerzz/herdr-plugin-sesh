package herdr

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
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

// wrapper is the shell command every argv-started terminal runs.
const wrapper = `cd -- "$1"; shift; exec env "$@" "${SHELL:-/bin/sh}" -l`

// assertDo checks a herdr-app do call's command and JSON arguments.
func assertDo(t *testing.T, call []string, command string, args map[string]any) {
	t.Helper()
	require.Len(t, call, 4)
	assert.Equal(t, []string{"herdr-app", "do", command}, call[:3])
	want, err := json.Marshal(args)
	require.NoError(t, err)
	assert.JSONEq(t, string(want), call[3])
}

func assertNoSend(t *testing.T, calls [][]string) {
	t.Helper()
	for _, call := range calls {
		assert.False(t, call[0] == "hc" && call[1] == "send", "unexpected terminal input %q", call)
	}
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

func TestAppClientUnfocusedWorkspaceCreateRestoresShownTabAndStartsShellWithEnv(t *testing.T) {
	c, r := newTestAppClient(map[string][]string{
		"herdr-app status":       {`{"space":"sp0","tab":"tb0","focused_pane":"p0"}`},
		"herdr-app do space.new": {`{"space":"sp1","tab":"tb1","terminal":"t7"}`},
	})
	w, err := c.WorkspaceCreate(context.Background(), WorkspaceCreateRequest{CWD: "/src/api", Label: "api", Env: map[string]string{"B": "it's two", "A": "1"}})
	require.NoError(t, err)
	assert.Equal(t, Workspace{ID: "sp1", Label: "api", CWD: "/src/api", ActiveTabID: "tb1"}, w)
	require.Len(t, r.calls, 3)
	assert.Equal(t, []string{"herdr-app", "status"}, r.calls[0])
	assertDo(t, r.calls[1], "space.new", map[string]any{"name": "api", "folder": "/src/api",
		"argv": []string{"/bin/sh", "-c", wrapper, "sh", ".", "A=1", "B=it's two"}})
	assert.Equal(t, []string{"herdr-app", "do", "tab.select", `{"id":"tb0"}`}, r.calls[2])
}

func TestAppClientWorkspaceCreateWithoutEnvSendsNoArgv(t *testing.T) {
	c, r := newTestAppClient(map[string][]string{"herdr-app do space.new": {`{"space":"sp1","tab":"tb1","terminal":"t7"}`}})
	_, err := c.WorkspaceCreate(context.Background(), WorkspaceCreateRequest{CWD: "/src/api", Label: "api", Focus: true})
	require.NoError(t, err)
	require.Len(t, r.calls, 1)
	assertDo(t, r.calls[0], "space.new", map[string]any{"name": "api", "folder": "/src/api"})
}

// herdr-app may echo its argv in a message, raw or JSON-escaped. A create that
// carries env withholds any message that contains a value in either form.
func TestAppClientCreateErrorsWithholdMessageThatEchoesEnvValue(t *testing.T) {
	// serde_json, which herdr-app uses, leaves & unescaped where Go writes \u0026.
	//nolint:gosec // a fake value that must stay out of error text.
	const secret = "p&ss\"word\n"
	// serdeEscaped is the value as serde_json writes it inside a JSON string.
	const serdeEscaped = `p&ss\"word\n`
	replies := map[string]func(body string) (stdout, stderr string){
		"JSON error":           func(string) (string, string) { return `{"error":"cannot spawn API_TOKEN=p&ss\"word\n"}`, "" },
		"JSON error with body": func(body string) (string, string) { return `{"error":` + strconv.Quote(body) + `}`, "" },
		"JSON error with serde env": func(string) (string, string) {
			return `{"error":` + strconv.Quote(`bad env ["API_TOKEN=`+serdeEscaped+`"]`) + `}`, ""
		},
		"stderr":                func(string) (string, string) { return "", "spawn failed: env API_TOKEN=" + secret },
		"stderr with body":      func(body string) (string, string) { return "", "invalid request body: " + body + "\n" },
		"stderr with serde env": func(string) (string, string) { return "", `bad env ["API_TOKEN=` + serdeEscaped + `"]` + "\n" },
	}
	for command, create := range envCreates(map[string]string{"API_TOKEN": secret}) {
		for name, reply := range replies {
			t.Run(command+" "+name, func(t *testing.T) {
				err := create(failingCreateClient(command, reply))
				require.Error(t, err)
				assert.Equal(t, "herdr-app do "+command+": exit status 1 (herdr-app's message is withheld because the request carried env values)", err.Error())
			})
		}
	}
}

// A message that repeats no env value is shown, so an unknown machine, a
// missing folder or a bad pane ID is visible. The body still stays out.
func TestAppClientCreateErrorsShowMessageWithoutEnvValues(t *testing.T) {
	replies := map[string]struct {
		reply func(body string) (stdout, stderr string)
		want  string
	}{
		"JSON error": {func(string) (string, string) { return `{"error":"unknown machine \"nosuchbox\""}`, "" }, `unknown machine "nosuchbox"`},
		"stderr":     {func(string) (string, string) { return "", "no such folder: /src/typo\n" }, "exit status 1: no such folder: /src/typo"},
	}
	for command, create := range envCreates(map[string]string{"API_TOKEN": "hunter2", "EMPTY": ""}) {
		for name, tc := range replies {
			t.Run(command+" "+name, func(t *testing.T) {
				err := create(failingCreateClient(command, tc.reply))
				require.EqualError(t, err, "herdr-app do "+command+": "+tc.want)
			})
		}
	}
}

// envCreates runs each herdr-app create that can carry env.
func envCreates(env map[string]string) map[string]func(c *AppClient) error {
	return map[string]func(c *AppClient) error{
		"space.new": func(c *AppClient) error {
			_, err := c.WorkspaceCreate(context.Background(), WorkspaceCreateRequest{Machine: "nosuchbox", CWD: "/src/typo", Label: "api", Focus: true, Env: env})
			return err
		},
		"tab.new": func(c *AppClient) error {
			_, err := c.TabCreate(context.Background(), TabCreateRequest{WorkspaceID: "sp1", CWD: "/src/typo", Label: "logs", Focus: true, Env: env})
			return err
		},
		"pane.split": func(c *AppClient) error {
			_, err := c.PaneSplit(context.Background(), PaneSplitRequest{PaneID: "p1", Direction: "right", Env: env})
			return err
		},
	}
}

// failingCreateClient fails command with reply's output and exit status 1.
func failingCreateClient(command string, reply func(body string) (stdout, stderr string)) *AppClient {
	c, r := newTestAppClient(nil)
	c.Runner = runnerFunc(func(ctx context.Context, bin string, args ...string) ([]byte, []byte, error) {
		if bin == "herdr-app" && args[0] == "do" && args[1] == command {
			stdout, stderr := reply(args[2])
			return []byte(stdout), []byte(stderr), errors.New("exit status 1")
		}
		return r.Run(ctx, bin, args...)
	})
	return c
}

// Without env there is nothing to hide: the error keeps herdr-app's message
// and names the request, which says what a select or focus was aimed at.
func TestAppClientErrorsWithoutEnvNameTheRequest(t *testing.T) {
	c, _ := newTestAppClient(map[string][]string{
		"herdr-app do space.new":    {`{"error":"unknown machine \"nosuchbox\""}`},
		"herdr-app do space.select": {`{"error":"no such space"}`},
	})
	_, err := c.WorkspaceCreate(context.Background(), WorkspaceCreateRequest{Machine: "nosuchbox", Label: "api", Focus: true})
	require.EqualError(t, err, `herdr-app do space.new {"folder":"","machine":"nosuchbox","name":"api"}: unknown machine "nosuchbox"`)
	require.EqualError(t, c.WorkspaceFocus(context.Background(), "sp7"), `herdr-app do space.select {"id":"sp7"}: no such space`)
}

// runShellArgv runs shellArgv's program from dir as herdr-app would, with
// typed as the startup command hc send delivers afterwards.
func runShellArgv(t *testing.T, argv []string, dir, shell, typed string) (string, error) {
	t.Helper()
	//nolint:gosec // the test runs the argv shellArgv built.
	cmd := exec.Command(argv[0], argv[1:]...)
	home := t.TempDir()
	cmd.Dir = dir
	cmd.Env = []string{"SHELL=" + shell, "HOME=" + home, "ZDOTDIR=" + home, "PATH=/usr/bin:/bin", "ENV=/dev/null"}
	cmd.Stdin = strings.NewReader(typed)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestShellArgvStartsShellInDirectoryWithValuesIntact(t *testing.T) {
	dir := filepath.Join(t.TempDir(), `it's $(x) "d" -`)
	require.NoError(t, os.Mkdir(dir, 0o750))
	value := "a'b\"c $(touch pwned) `x` \\n\nline2 *"
	for _, shell := range []string{"/bin/bash", "/bin/zsh", "/bin/sh", ""} {
		t.Run(shell, func(t *testing.T) {
			if shell == "/bin/zsh" {
				if _, err := os.Stat(shell); os.IsNotExist(err) {
					t.Skip("optional zsh is not installed")
				}
			}
			out, err := runShellArgv(t, shellArgv(dir, map[string]string{"V": value}), t.TempDir(), shell, "printf '%s|' \"$V\"; pwd -P\n")
			require.NoError(t, err, shell)
			assert.Contains(t, out, value+"|", shell)
			assert.Contains(t, out, filepath.Base(dir), shell)
			assert.NoFileExists(t, filepath.Join(dir, "pwned"), shell)
		})
	}
}

// A pane path that does not exist must not end the terminal: the startup
// command is typed into it next, as it was after the typed cd this replaced.
func TestShellArgvStartsShellWhenDirectoryIsMissing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "typo")
	out, err := runShellArgv(t, shellArgv(missing, map[string]string{"PORT": "8080"}), t.TempDir(), "/bin/bash", "echo \"STARTUP-RAN $PORT\"\n")
	require.NoError(t, err, out)
	assert.Contains(t, out, missing)
	assert.Contains(t, out, "STARTUP-RAN 8080")
}

// herdr-app starts a space's or tab's program in folder, so the wrapper must
// not enter a relative folder a second time.
func TestAppClientSpaceAndTabShellsStayInTheirFolder(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, "src", "api"), 0o750))
	creates := map[string]func(c *AppClient, folder string) error{
		"space.new": func(c *AppClient, folder string) error {
			_, err := c.WorkspaceCreate(context.Background(), WorkspaceCreateRequest{Machine: "buntu26", CWD: folder, Label: "api", Focus: true, Env: map[string]string{"PORT": "8080"}})
			return err
		},
		"tab.new": func(c *AppClient, folder string) error {
			_, err := c.TabCreate(context.Background(), TabCreateRequest{WorkspaceID: "buntu26/sp1", CWD: folder, Label: "logs", Focus: true, Env: map[string]string{"PORT": "8080"}})
			return err
		},
	}
	for command, create := range creates {
		for name, folder := range map[string]string{"relative": "src/api", "absolute": filepath.Join(home, "src", "api")} {
			t.Run(command+" "+name, func(t *testing.T) {
				var out string
				c, r := newTestAppClient(map[string][]string{"herdr-app ls": {`{"spaces":[{"id":"buntu26/sp1","tabs":[{"id":"tb3","terminals":[{"terminal":"t3","pane":"p3"}]}]}]}`}})
				c.Runner = runnerFunc(func(ctx context.Context, bin string, args ...string) ([]byte, []byte, error) {
					if bin != "herdr-app" || args[0] != "do" || args[1] != command {
						return r.Run(ctx, bin, args...)
					}
					var body struct {
						Folder string   `json:"folder"`
						Argv   []string `json:"argv"`
					}
					require.NoError(t, json.Unmarshal([]byte(args[2]), &body))
					dir := body.Folder
					if !filepath.IsAbs(dir) {
						dir = filepath.Join(home, dir)
					}
					var err error
					out, err = runShellArgv(t, body.Argv, dir, "/bin/bash", "printf '%s|' \"$PORT\"; pwd -P\n")
					require.NoError(t, err, out)
					return []byte(`{"space":"buntu26/sp1","tab":"tb3","terminal":"t3"}`), nil, nil
				})
				require.NoError(t, create(c, folder))
				assert.NotContains(t, out, "No such file or directory")
				assert.Contains(t, out, "8080|")
				assert.Contains(t, out, filepath.Join("src", "api"))
			})
		}
	}
}

type runnerFunc func(context.Context, string, ...string) ([]byte, []byte, error)

func (f runnerFunc) Run(ctx context.Context, bin string, args ...string) ([]byte, []byte, error) {
	return f(ctx, bin, args...)
}

func TestAppClientTabCreateStartsShellWithEnv(t *testing.T) {
	c, r := newTestAppClient(map[string][]string{
		"herdr-app do tab.new": {`{"space":"sp1","tab":"tb3","terminal":"t3"}`},
		"herdr-app ls":         {`{"spaces":[{"id":"sp1","tabs":[{"id":"tb3","terminals":[{"terminal":"t3","pane":"p3"}]}]}]}`},
	})
	_, err := c.TabCreate(context.Background(), TabCreateRequest{WorkspaceID: "sp1", CWD: "/src", Label: "logs", Focus: true, Env: map[string]string{"PORT": "8080"}})
	require.NoError(t, err)
	assertDo(t, r.calls[0], "tab.new", map[string]any{"space": "sp1", "name": "logs", "folder": "/src",
		"argv": []string{"/bin/sh", "-c", wrapper, "sh", ".", "PORT=8080"}})
	assertNoSend(t, r.calls)
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

func TestAppClientSplitStartsShellInDirectoryThenRunsInNewTerminal(t *testing.T) {
	c, r := newTestAppClient(map[string][]string{"herdr-app do pane.split": {`{"pane":"p5","terminal":"t5"}`}})
	p, err := c.PaneSplit(context.Background(), PaneSplitRequest{PaneID: "p1", Direction: "down", CWD: "/src/web", Ratio: 0.3})
	require.NoError(t, err)
	assertDo(t, r.calls[1], "pane.split", map[string]any{"pane": "p1", "direction": "down"})
	assert.Equal(t, []string{"hc", "send", "t5"}, r.calls[2][:3])
	assert.NotContains(t, r.calls[2][3], "/src/web")
	require.NoError(t, c.PaneRun(context.Background(), p.ID, "npm run dev"))
	assert.Equal(t, []string{"hc", "send", "t5", "npm run dev"}, r.calls[len(r.calls)-1])
}

func TestAppClientSplitWithOnlyEnvStaysInInheritedDirectory(t *testing.T) {
	c, r := newTestAppClient(map[string][]string{"herdr-app do pane.split": {`{"pane":"p5","terminal":"t5"}`}})
	_, err := c.PaneSplit(context.Background(), PaneSplitRequest{PaneID: "p1", Direction: "right", Env: map[string]string{"PORT": "8080"}})
	require.NoError(t, err)
	assertDo(t, r.calls[1], "pane.split", map[string]any{"pane": "p1", "direction": "right",
		"argv": []string{"/bin/sh", "-c", wrapper, "sh", ".", "PORT=8080"}})
	assertNoSend(t, r.calls)
}

func TestAppClientSplitDirectoryWinsOverLoginProfile(t *testing.T) {
	for _, withEnv := range []bool{false, true} {
		t.Run(strconv.FormatBool(withEnv), func(t *testing.T) {
			home := t.TempDir()
			dir := filepath.Join(t.TempDir(), "it's $(touch injected) web")
			require.NoError(t, os.Mkdir(dir, 0o750))
			require.NoError(t, os.WriteFile(filepath.Join(home, ".bash_profile"), []byte("cd \"$HOME\"\nalias profile_alias='echo PROFILE_LOADED'\n"), 0o600))
			c, r := newTestAppClient(map[string][]string{"herdr-app do pane.split": {`{"pane":"p5","terminal":"remote/t5"}`}})
			var env map[string]string
			if withEnv {
				env = map[string]string{"PORT": "8080"}
			}
			p, err := c.PaneSplit(t.Context(), PaneSplitRequest{PaneID: "p1", Direction: "right", CWD: dir, Env: env})
			require.NoError(t, err)
			require.NoError(t, c.PaneRun(t.Context(), p.ID, "profile_alias; printf 'PORT=%s\\n' \"$PORT\"; pwd -P"))
			argv := []string{"/bin/bash", "-l"}
			var input strings.Builder
			for _, call := range r.calls {
				if len(call) > 3 && call[0] == "herdr-app" && call[2] == "pane.split" {
					var body struct {
						Argv []string `json:"argv"`
					}
					require.NoError(t, json.Unmarshal([]byte(call[3]), &body))
					if len(body.Argv) > 0 {
						argv = body.Argv
					}
				}
				if len(call) > 3 && call[0] == "hc" && call[1] == "send" {
					assert.Equal(t, "remote/t5", call[2])
					input.WriteString(call[3] + "\n")
				}
			}
			//nolint:gosec // execute the actual create argv against an isolated profile.
			cmd := exec.Command(argv[0], argv[1:]...)
			cmd.Dir = home
			cmd.Env = []string{"HOME=" + home, "SHELL=/bin/bash", "PATH=/usr/bin:/bin", "BASH_ENV=/dev/null"}
			cmd.Stdin = strings.NewReader("shopt -s expand_aliases\n" + input.String())
			out, err := cmd.CombinedOutput()
			require.NoError(t, err, string(out))
			physical, err := filepath.EvalSymlinks(dir)
			require.NoError(t, err)
			assert.Contains(t, string(out), physical+"\n")
			assert.Contains(t, string(out), "PROFILE_LOADED")
			if withEnv {
				assert.Contains(t, string(out), "PORT=8080")
			}
			assert.NoFileExists(t, filepath.Join(home, "injected"))
		})
	}
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

func TestAppClientWaitForRejectsEchoedDirectorySetup(t *testing.T) {
	c, r := newTestAppClient(map[string][]string{
		"herdr-app do pane.split": {`{"pane":"p5","terminal":"t5"}`},
		"hc read":                 {"ready from application"},
	})
	p, err := c.PaneSplit(t.Context(), PaneSplitRequest{PaneID: "p1", Direction: "right", CWD: "/src/listening"})
	require.NoError(t, err)
	count := len(r.calls)
	require.ErrorContains(t, c.PaneWaitOutput(t.Context(), p.ID, "printf", time.Second), "directory setup line")
	assert.Len(t, r.calls, count)
	r.replies["hc read"] = []string{r.calls[2][3]}
	require.ErrorIs(t, c.PaneWaitOutput(t.Context(), p.ID, "listening", 20*time.Millisecond), context.DeadlineExceeded)
	r.replies["hc read"] = []string{r.calls[2][3] + "\nlistening from application"}
	require.NoError(t, c.PaneWaitOutput(t.Context(), p.ID, "listening", time.Second))
	r.replies["hc read"] = []string{"ready from application"}
	require.NoError(t, c.PaneWaitOutput(t.Context(), p.ID, "ready from application", time.Second))
}

func TestAppClientSplitRestoresFocusWhenDirectorySendFails(t *testing.T) {
	c, r := newTestAppClient(map[string][]string{
		"herdr-app status":        {`{"focused_pane":"p1"}`, `{"focused_pane":"p5"}`},
		"herdr-app do pane.split": {`{"pane":"p5","terminal":"t5"}`},
		"hc send":                 {`{"error":"terminal unavailable"}`},
	})
	p, err := c.PaneSplit(t.Context(), PaneSplitRequest{PaneID: "p1", Direction: "right", CWD: "/src/web"})
	require.ErrorContains(t, err, "terminal unavailable")
	assert.Equal(t, "p5", p.ID)
	assert.Equal(t, []string{"herdr-app", "do", "pane.focus", `{"pane":"p1"}`}, r.calls[len(r.calls)-1])
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
	assertDo(t, r.calls[1], "pane.split", map[string]any{"pane": "p1", "direction": "right"})
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

func TestAppClientWorkspaceCreateOnMachine(t *testing.T) {
	c, r := newTestAppClient(map[string][]string{"herdr-app do space.new": {`{"space":"buntu26/sp1","tab":"buntu26/tb1","terminal":"buntu26/t7"}`}})
	w, err := c.WorkspaceCreate(context.Background(), WorkspaceCreateRequest{Machine: "buntu26", CWD: "/home/zach/api", Label: "api", Focus: true, Env: map[string]string{"A": "1"}})
	require.NoError(t, err)
	assert.Equal(t, Workspace{ID: "buntu26/sp1", Label: "api", CWD: "/home/zach/api", ActiveTabID: "buntu26/tb1", Machine: "buntu26"}, w)
	require.Len(t, r.calls, 1)
	assertDo(t, r.calls[0], "space.new", map[string]any{"name": "api", "machine": "buntu26", "folder": "/home/zach/api",
		"argv": []string{"/bin/sh", "-c", wrapper, "sh", ".", "A=1"}})
}

func TestAppClientMachineHome(t *testing.T) {
	c, r := newTestAppClient(map[string][]string{"hc exec": {"/home/zach"}})
	home, err := c.MachineHome(context.Background(), "buntu26")
	require.NoError(t, err)
	assert.Equal(t, "/home/zach", home)
	assert.Equal(t, [][]string{{"hc", "exec", "--on", "buntu26", "--", "sh", "-c", `printf %s "$HOME"`}}, r.calls)

	// hc's reply to `hc exec --on nosuchbox`: exit 4, empty stdout, JSON on stderr.
	c.Runner = runnerFunc(func(context.Context, string, ...string) ([]byte, []byte, error) {
		return nil, []byte(`{"error":{"hint":"machines: buntu26","kind":"machine_unknown","message":"unknown machine \"nosuchbox\""}}` + "\n"), errors.New("exit status 4")
	})
	_, err = c.MachineHome(context.Background(), "nosuchbox")
	require.EqualError(t, err, `home on machine "nosuchbox": unknown machine "nosuchbox"`)

	c, _ = newTestAppClient(map[string][]string{"hc exec": {""}})
	_, err = c.MachineHome(context.Background(), "buntu26")
	require.ErrorContains(t, err, "not an absolute path")
}

func TestAppClientMachineExecBuildsHCExec(t *testing.T) {
	c, r := newTestAppClient(map[string][]string{"hc exec": {"listing"}})
	out, _, err := c.MachineExec(t.Context(), "buntu26", MachineExecRequest{
		Dir: "src/api", Env: []string{"LC_ALL=C", "TOKEN=s3cret"}, Timeout: 2500 * time.Millisecond,
		Argv: []string{"git", "status", "--short"},
	})
	require.NoError(t, err)
	assert.Equal(t, "listing", string(out))
	assert.Equal(t, [][]string{{"hc", "exec", "--on", "buntu26", "--cwd", "src/api", "--env", "LC_ALL=C", "--env", "TOKEN=s3cret",
		"--timeout", "3", "--", "git", "status", "--short"}}, r.calls)

	_, _, err = c.MachineExec(t.Context(), "buntu26", MachineExecRequest{Argv: []string{"ls"}})
	require.NoError(t, err)
	assert.Equal(t, []string{"hc", "exec", "--on", "buntu26", "--", "ls"}, r.calls[1], "no Dir, Env or Timeout adds no flags")
}

// Callers decode hc's own report from stderr and word their own messages, so
// MachineExec returns output unchanged and keeps env values out of the error.
func TestAppClientMachineExecFailureKeepsOutputAndHidesEnvValues(t *testing.T) {
	stderr := []byte(`{"error":{"kind":"spawn_failed","message":"cannot start git: No such file or directory"}}` + "\n")
	c, _ := newTestAppClient(nil)
	c.Runner = runnerFunc(func(context.Context, string, ...string) ([]byte, []byte, error) {
		return []byte("partial"), stderr, errors.New("exit status 4")
	})
	out, gotStderr, err := c.MachineExec(t.Context(), "buntu26", MachineExecRequest{Env: []string{"TOKEN=s3cret"}, Argv: []string{"git"}})
	require.Error(t, err)
	assert.Equal(t, "partial", string(out))
	assert.Equal(t, stderr, gotStderr)
	kind, _ := HCError(gotStderr)
	assert.Equal(t, "spawn_failed", kind)
	assert.NotContains(t, err.Error(), "s3cret")
	assert.Contains(t, err.Error(), "hc exec --on buntu26 --env TOKEN -- git: exit status 4")
}

func TestAppClientMachineExecTimeoutAndCancellation(t *testing.T) {
	waitRunner := runnerFunc(func(ctx context.Context, _ string, _ ...string) ([]byte, []byte, error) {
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
			return []byte("done"), nil, nil
		}
	})
	c := &AppClient{CoreBin: "hc", Runner: waitRunner, Timeout: 10 * time.Second}
	_, _, err := c.MachineExec(t.Context(), "buntu26", MachineExecRequest{Timeout: 20 * time.Millisecond, Argv: []string{"ls"}})
	require.ErrorIs(t, err, context.DeadlineExceeded, "Timeout bounds the call below the client's default")

	c.Timeout = 20 * time.Millisecond
	out, _, err := c.MachineExec(t.Context(), "buntu26", MachineExecRequest{Timeout: 5 * time.Second, Argv: []string{"ls"}})
	require.NoError(t, err, "Timeout replaces a shorter client default")
	assert.Equal(t, "done", string(out))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, _, err = c.MachineExec(ctx, "buntu26", MachineExecRequest{Argv: []string{"ls"}})
	require.ErrorIs(t, err, context.Canceled)
}

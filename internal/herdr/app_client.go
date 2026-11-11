package herdr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/fullerzz/herdr-plugin-sesh/internal/config"
)

// AppClient drives the Herdr desktop app through `herdr-app` and its terminal
// core through `hc`. Workspaces are herdr-app spaces.
//
// herdr-app's create commands always focus what they create. AppClient restores
// the shown tab after unfocused creates and refocuses the pane a split took
// focus from. Split directories are sent after shell startup; environments
// are supplied through shellArgv. For a
// space or tab, herdr-app runs that argv inside its own login shell, so startup
// files run twice; spaces and tabs get argv only when they need an environment.
//
// An AppClient is safe for concurrent use.
type AppClient struct {
	AppBin, CoreBin string
	Runner          Runner
	Timeout         time.Duration
	// PollInterval spaces PaneWaitOutput's screen reads.
	PollInterval time.Duration
	// CallerTerminal is the hc address of the terminal running herdr-sesh.
	CallerTerminal string

	mu                sync.Mutex
	terminals         map[string]string // pane ID -> terminal address
	directoryCommands map[string]string // terminal address -> echoed directory setup
}

var (
	_ Client        = (*AppClient)(nil)
	_ MachineClient = (*AppClient)(nil)
)

func NewAppClient() *AppClient {
	return &AppClient{AppBin: "herdr-app", CoreBin: "hc", Runner: ExecRunner{}, Timeout: 10 * time.Second,
		PollInterval: 250 * time.Millisecond, CallerTerminal: os.Getenv("HC_TERMINAL_ID")}
}

type appLayout struct {
	Spaces []appSpace `json:"spaces"`
}

type appSpace struct {
	Machine string   `json:"machine"`
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Folder  string   `json:"folder"`
	Tabs    []appTab `json:"tabs"`
}

type appTab struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Active    bool          `json:"active"`
	Terminals []appTerminal `json:"terminals"`
}

// appTerminal's Status is its agent status, null for a plain program.
type appTerminal struct {
	Terminal string  `json:"terminal"`
	Pane     string  `json:"pane"`
	Status   *string `json:"status"`
}

type appStatus struct {
	Space       string `json:"space"`
	Tab         string `json:"tab"`
	FocusedPane string `json:"focused_pane"`
}

type appCreated struct {
	Space    string `json:"space"`
	Tab      string `json:"tab"`
	Pane     string `json:"pane"`
	Terminal string `json:"terminal"`
}

func (c *AppClient) exec(ctx context.Context, bin string, args ...string) ([]byte, error) {
	out, _, err := c.execNamed(ctx, bin+" "+strings.Join(args, " "), false, bin, args...)
	return out, err
}

// execNamed reports failures as name, so arguments that may hold secrets stay
// out of error text. With secret set, the program's own message is withheld
// too: it may echo its arguments in any encoding, so no match on the values
// can clear it.
func (c *AppClient) execNamed(ctx context.Context, name string, secret bool, bin string, args ...string) (out, stderr []byte, err error) {
	runner := c.Runner
	if runner == nil {
		runner = ExecRunner{}
	}
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	out, stderr, err = runner.Run(ctx, bin, args...)
	if err == nil {
		return out, stderr, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return out, stderr, fmt.Errorf("%s: %w", name, ctxErr)
	}
	if secret {
		return out, stderr, fmt.Errorf("%s: %w (herdr-app's message is withheld because the request carried env values)", name, err)
	}
	// herdr-app reports command failures as {"error": "..."} on stdout.
	var reply struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(out, &reply) == nil && reply.Error != "" {
		return out, stderr, fmt.Errorf("%s: %s", name, reply.Error)
	}
	return out, stderr, fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(string(stderr)))
}

func (c *AppClient) do(ctx context.Context, command string, args map[string]any, reply any) error {
	return c.create(ctx, command, args, nil, reply)
}

// create runs a herdr-app command whose argv may carry env. With env set,
// neither the body nor herdr-app's own message reaches error text; without,
// the error names the request, which says what a select or focus was aimed at.
func (c *AppClient) create(ctx context.Context, command string, args map[string]any, env map[string]string, reply any) error {
	body, err := json.Marshal(args)
	if err != nil {
		return err
	}
	name := c.AppBin + " do " + command
	if len(env) == 0 {
		name += " " + string(body)
	}
	out, _, err := c.execNamed(ctx, name, len(env) > 0, c.AppBin, "do", command, string(body))
	if err != nil || reply == nil {
		return err
	}
	if err := json.Unmarshal(out, reply); err != nil {
		return fmt.Errorf("decode herdr-app %s JSON: %w", command, err)
	}
	return nil
}

func (c *AppClient) layout(ctx context.Context) (appLayout, error) {
	out, err := c.exec(ctx, c.AppBin, "ls")
	if err != nil {
		return appLayout{}, err
	}
	var l appLayout
	if err := json.Unmarshal(out, &l); err != nil {
		return appLayout{}, fmt.Errorf("decode herdr-app ls JSON: %w", err)
	}
	for _, s := range l.Spaces {
		for _, t := range s.Tabs {
			for _, term := range t.Terminals {
				c.remember(term.Pane, term.Terminal)
			}
		}
	}
	return l, nil
}

func (c *AppClient) status(ctx context.Context) (appStatus, error) {
	out, err := c.exec(ctx, c.AppBin, "status")
	if err != nil {
		return appStatus{}, err
	}
	var s appStatus
	if err := json.Unmarshal(out, &s); err != nil {
		return appStatus{}, fmt.Errorf("decode herdr-app status JSON: %w", err)
	}
	return s, nil
}

// terminalCWDs maps local terminal addresses to their live directory. Remote
// terminals are absent, so their spaces fall back to the folder they were made in.
func (c *AppClient) terminalCWDs(ctx context.Context) map[string]string {
	out, err := c.exec(ctx, c.CoreBin, "ls", "--json")
	if err != nil {
		return nil
	}
	var resp struct {
		Terminals []struct {
			Address    string `json:"address"`
			CWD        string `json:"cwd"`
			Foreground *struct {
				CWD string `json:"cwd"`
			} `json:"foreground"`
		} `json:"terminals"`
	}
	if json.Unmarshal(out, &resp) != nil {
		return nil
	}
	cwds := make(map[string]string, len(resp.Terminals))
	for _, t := range resp.Terminals {
		cwds[t.Address] = t.CWD
		if t.Foreground != nil && t.Foreground.CWD != "" {
			cwds[t.Address] = t.Foreground.CWD
		}
	}
	return cwds
}

func (c *AppClient) remember(pane, terminal string) {
	if pane == "" || terminal == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.terminals == nil {
		c.terminals = make(map[string]string)
	}
	c.terminals[pane] = terminal
}

func (c *AppClient) lookupTerminal(pane string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t, ok := c.terminals[pane]
	return t, ok
}

func (c *AppClient) lookupPane(terminal string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for pane, t := range c.terminals {
		if t == terminal {
			return pane, true
		}
	}
	return "", false
}

func (c *AppClient) terminal(ctx context.Context, pane string) (string, error) {
	if t, ok := c.lookupTerminal(pane); ok {
		return t, nil
	}
	if _, err := c.layout(ctx); err != nil {
		return "", err
	}
	if t, ok := c.lookupTerminal(pane); ok {
		return t, nil
	}
	return "", fmt.Errorf("herdr-app pane %q has no terminal", pane)
}

func (c *AppClient) paneOf(ctx context.Context, terminal string) (string, error) {
	if pane, ok := c.lookupPane(terminal); ok {
		return pane, nil
	}
	if _, err := c.layout(ctx); err != nil {
		return "", err
	}
	if pane, ok := c.lookupPane(terminal); ok {
		return pane, nil
	}
	return "", fmt.Errorf("herdr-app terminal %q has no pane", terminal)
}

// keepShown returns a function that reselects the tab shown now, undoing the
// focus herdr-app gives everything it creates.
func (c *AppClient) keepShown(ctx context.Context, focus bool) (func() error, error) {
	if focus {
		return func() error { return nil }, nil
	}
	shown, err := c.status(ctx)
	if err != nil {
		return nil, err
	}
	return func() error {
		if shown.Tab == "" {
			return nil
		}
		return c.do(ctx, "tab.select", map[string]any{"id": shown.Tab}, nil)
	}, nil
}

// shellArgv starts the user's login shell in dir with env exported. herdr-app
// runs it as the terminal's program, so nothing is typed into the terminal.
// Values reach the shell as arguments, never through shell parsing. An empty
// dir keeps the directory herdr-app starts the program in. A dir that cannot
// be entered prints cd's error and still starts the shell: exiting instead
// would end the terminal before its startup command arrives.
func shellArgv(dir string, env map[string]string) []string {
	if dir == "" {
		dir = "."
	}
	argv := make([]string, 0, 5+len(env))
	argv = append(argv, "/bin/sh", "-c", `cd -- "$1"; shift; exec env "$@" "${SHELL:-/bin/sh}" -l`, "sh", dir)
	for _, k := range slices.Sorted(maps.Keys(env)) {
		argv = append(argv, k+"="+env[k])
	}
	return argv
}

// statusRank orders agent statuses by urgency, as herdr-app does for a space.
var statusRank = map[string]int{"idle": 1, "working": 2, "done": 3, "blocked": 4}

func (c *AppClient) WorkspaceList(ctx context.Context) ([]Workspace, error) {
	l, err := c.layout(ctx)
	if err != nil {
		return nil, err
	}
	cwds := c.terminalCWDs(ctx)
	ws := make([]Workspace, 0, len(l.Spaces))
	for _, s := range l.Spaces {
		w := Workspace{ID: s.ID, Label: s.Name, CWD: s.Folder, Machine: s.Machine}
		for _, t := range s.Tabs {
			for _, term := range t.Terminals {
				if term.Status != nil && statusRank[*term.Status] > statusRank[w.AgentStatus] {
					w.AgentStatus = *term.Status
				}
			}
			if t.Active {
				w.ActiveTabID = t.ID
				if len(t.Terminals) > 0 && s.Machine == "" {
					w.ForegroundCWD = cwds[t.Terminals[0].Terminal]
				}
			}
		}
		ws = append(ws, w)
	}
	return ws, nil
}

func (c *AppClient) WorkspaceCreate(ctx context.Context, r WorkspaceCreateRequest) (w Workspace, err error) {
	restore, err := c.keepShown(ctx, r.Focus)
	if err != nil {
		return Workspace{}, err
	}
	args := map[string]any{"name": r.Label, "folder": r.CWD}
	if r.Machine != "" {
		args["machine"] = r.Machine
	}
	if len(r.Env) > 0 {
		// folder already puts the program in r.CWD; entering a relative one
		// again from there would fail.
		args["argv"] = shellArgv("", r.Env)
	}
	var made appCreated
	if err := c.create(ctx, "space.new", args, r.Env, &made); err != nil {
		return Workspace{}, err
	}
	defer func() { err = errors.Join(err, restore()) }()
	return Workspace{ID: made.Space, Label: r.Label, CWD: r.CWD, ActiveTabID: made.Tab, Machine: r.Machine}, nil
}

// MachineHome asks the machine itself: herdr-app does not expand ~ in folders.
func (c *AppClient) MachineHome(ctx context.Context, machine string) (string, error) {
	args := []string{"exec", "--on", machine, "--", "sh", "-c", `printf %s "$HOME"`}
	out, stderr, err := c.execNamed(ctx, c.CoreBin+" "+strings.Join(args, " "), false, c.CoreBin, args...)
	if err != nil {
		// hc reports its own failures, such as an unknown machine, on stderr.
		if _, message := HCError(stderr); message != "" {
			return "", fmt.Errorf("home on machine %q: %s", machine, message)
		}
		return "", fmt.Errorf("home on machine %q: %w", machine, err)
	}
	home := string(out)
	if !strings.HasPrefix(home, "/") {
		return "", fmt.Errorf("home on machine %q: got %q, not an absolute path", machine, home)
	}
	return home, nil
}

// HCError decodes hc's own {"error":{"kind":...,"message":...}} report, which
// hc writes to stderr.
func HCError(stderr []byte) (kind, message string) {
	var reply struct {
		Error struct {
			Kind    string `json:"kind"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(stderr, &reply) != nil {
		return "", ""
	}
	return reply.Error.Kind, reply.Error.Message
}

func (c *AppClient) WorkspaceFocus(ctx context.Context, id string) error {
	return c.do(ctx, "space.select", map[string]any{"id": id}, nil)
}

func (c *AppClient) WorkspaceClose(ctx context.Context, id string) error {
	return c.do(ctx, "space.close", map[string]any{"id": id, "confirmed": true}, nil)
}

func (c *AppClient) TabList(ctx context.Context, wid string) ([]Tab, error) {
	l, err := c.layout(ctx)
	if err != nil {
		return nil, err
	}
	var tabs []Tab
	for _, s := range l.Spaces {
		if s.ID != wid {
			continue
		}
		for _, t := range s.Tabs {
			tab := Tab{ID: t.ID, WorkspaceID: s.ID, Label: t.Name}
			if len(t.Terminals) > 0 {
				tab.PaneID = t.Terminals[0].Pane
			}
			tabs = append(tabs, tab)
		}
	}
	return tabs, nil
}

func (c *AppClient) TabCreate(ctx context.Context, r TabCreateRequest) (tab Tab, err error) {
	restore, err := c.keepShown(ctx, r.Focus)
	if err != nil {
		return Tab{}, err
	}
	args := map[string]any{"space": r.WorkspaceID, "name": r.Label}
	if r.CWD != "" {
		args["folder"] = r.CWD
	}
	if len(r.Env) > 0 {
		args["argv"] = shellArgv("", r.Env)
	}
	var made appCreated
	if err := c.create(ctx, "tab.new", args, r.Env, &made); err != nil {
		return Tab{}, err
	}
	defer func() { err = errors.Join(err, restore()) }()
	tab = Tab{ID: made.Tab, WorkspaceID: r.WorkspaceID, Label: r.Label, CWD: r.CWD}
	tab.PaneID, err = c.paneOf(ctx, made.Terminal)
	return tab, err
}

func (c *AppClient) TabFocus(ctx context.Context, id string) error {
	return c.do(ctx, "tab.select", map[string]any{"id": id}, nil)
}

func (c *AppClient) TabRename(ctx context.Context, id, label string) error {
	return c.do(ctx, "tab.rename", map[string]any{"id": id, "name": label}, nil)
}

// PaneList lists every pane when wid is empty.
func (c *AppClient) PaneList(ctx context.Context, wid string) ([]Pane, error) {
	l, err := c.layout(ctx)
	if err != nil {
		return nil, err
	}
	shown, _ := c.status(ctx)
	cwds := c.terminalCWDs(ctx)
	var panes []Pane
	for _, s := range l.Spaces {
		if wid != "" && s.ID != wid {
			continue
		}
		for _, t := range s.Tabs {
			for _, term := range t.Terminals {
				panes = append(panes, Pane{ID: term.Pane, WorkspaceID: s.ID, TabID: t.ID,
					CWD: cwds[term.Terminal], Focused: term.Pane == shown.FocusedPane})
			}
		}
	}
	return panes, nil
}

// PaneCurrent is the pane showing the terminal herdr-sesh runs in.
func (c *AppClient) PaneCurrent(ctx context.Context) (Pane, error) {
	if c.CallerTerminal == "" {
		return Pane{}, errors.New("HC_TERMINAL_ID is not set; run herdr-sesh from a Herdr terminal")
	}
	panes, err := c.PaneList(ctx, "")
	if err != nil {
		return Pane{}, err
	}
	for _, p := range panes {
		if t, _ := c.lookupTerminal(p.ID); t == c.CallerTerminal {
			return p, nil
		}
	}
	return Pane{}, fmt.Errorf("terminal %q is not in a herdr-app pane", c.CallerTerminal)
}

// PaneFocused reports the window's shown space and tab. The pane ID is empty
// while an overlay terminal, such as the sesh picker, has the keyboard.
func (c *AppClient) PaneFocused(ctx context.Context) (Pane, error) {
	s, err := c.status(ctx)
	if err != nil {
		return Pane{}, err
	}
	return Pane{ID: s.FocusedPane, WorkspaceID: s.Space, TabID: s.Tab, Focused: true}, nil
}

func (c *AppClient) PaneRun(ctx context.Context, id, cmd string) error {
	t, err := c.terminal(ctx, id)
	if err != nil {
		return err
	}
	_, err = c.exec(ctx, c.CoreBin, "send", t, cmd)
	return err
}

// PaneSplit ignores Ratio: herdr-app splits evenly. herdr-app focuses the new
// pane within its tab; when that tab is shown, the pane focused before the split
// gets focus back. A tab that isn't shown keeps the new pane focused.
func (c *AppClient) PaneSplit(ctx context.Context, r PaneSplitRequest) (Pane, error) {
	before, err := c.status(ctx)
	if err != nil {
		return Pane{}, err
	}
	args := map[string]any{"pane": r.PaneID, "direction": r.Direction}
	if len(r.Env) > 0 {
		args["argv"] = shellArgv("", r.Env)
	}
	var made appCreated
	if err := c.create(ctx, "pane.split", args, r.Env, &made); err != nil {
		return Pane{}, err
	}
	c.remember(made.Pane, made.Terminal)
	p := Pane{ID: made.Pane, CWD: r.CWD}
	if r.CWD != "" {
		// Apply after login files, which may change directory themselves.
		command := config.SubstitutePath(" cd -- {}", r.CWD)
		c.mu.Lock()
		if c.directoryCommands == nil {
			c.directoryCommands = make(map[string]string)
		}
		c.directoryCommands[made.Terminal] = command
		c.mu.Unlock()
		_, err = c.exec(ctx, c.CoreBin, "send", made.Terminal, command)
	}
	after, statusErr := c.status(ctx)
	if statusErr == nil && before.FocusedPane != "" && after.FocusedPane == made.Pane {
		statusErr = c.PaneFocus(ctx, before.FocusedPane)
	}
	return p, errors.Join(err, statusErr)
}

func (c *AppClient) PaneFocus(ctx context.Context, id string) error {
	return c.do(ctx, "pane.focus", map[string]any{"pane": id}, nil)
}

// PaneWaitOutput polls the terminal's screen and history, so output printed
// before the call counts.
func (c *AppClient) PaneWaitOutput(ctx context.Context, id, match string, timeout time.Duration) error {
	t, err := c.terminal(ctx, id)
	if err != nil {
		return err
	}
	c.mu.Lock()
	setup := c.directoryCommands[t]
	c.mu.Unlock()
	if setup != "" && strings.Contains(setup, match) {
		return fmt.Errorf("terminal %s: wait_for match %q appears in the directory setup line it echoes", t, match)
	}
	interval := c.PollInterval
	if interval == 0 {
		interval = 250 * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		out, err := c.exec(ctx, c.CoreBin, "read", t, "--lines", "1000")
		if err == nil && strings.Contains(string(out), match) {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("terminal %s: %w", t, ctx.Err())
		case <-time.After(interval):
		}
	}
}

func (c *AppClient) PluginPaneOpen(context.Context, string, string, string) error {
	return errors.New("herdr-app has no plugin panes; open the picker with the sesh mod")
}

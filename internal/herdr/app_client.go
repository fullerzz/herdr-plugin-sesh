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
)

// AppClient drives the Herdr desktop app through `herdr-app` and its terminal
// core through `hc`. Workspaces are herdr-app spaces.
//
// herdr-app's create commands always focus what they create and cannot set a
// split's directory or any terminal environment. AppClient restores the shown
// tab after unfocused creates, refocuses the pane a split took focus from, and
// types a setup line into new terminals. The setup line needs a shell that runs
// `eval "$(printf '\ooo')"`: sh, bash, zsh, or fish 3.4 and newer.
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

	mu        sync.Mutex
	terminals map[string]string // pane ID -> terminal address
	setups    map[string]string // terminal address -> its typed setup line
}

var _ Client = (*AppClient)(nil)

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
	return c.execNamed(ctx, bin+" "+strings.Join(args, " "), bin, args...)
}

// execNamed reports failures as name, so arguments that may hold secrets stay
// out of error text.
func (c *AppClient) execNamed(ctx context.Context, name, bin string, args ...string) ([]byte, error) {
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
	out, stderr, err := runner.Run(ctx, bin, args...)
	if err == nil {
		return out, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return out, fmt.Errorf("%s: %w", name, ctxErr)
	}
	// herdr-app reports command failures as {"error": "..."} on stdout.
	var reply struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(out, &reply) == nil && reply.Error != "" {
		return out, fmt.Errorf("%s: %s", name, reply.Error)
	}
	return out, fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(string(stderr)))
}

func (c *AppClient) do(ctx context.Context, command string, args map[string]any, reply any) error {
	body, err := json.Marshal(args)
	if err != nil {
		return err
	}
	out, err := c.exec(ctx, c.AppBin, "do", command, string(body))
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

// prelude types a directory change and exports into a new terminal, which
// herdr-app cannot set when it creates one. The terminal echoes what is typed,
// so the commands go out octal-encoded: neither values nor names appear in the
// scrollback that PaneWaitOutput searches. The leading space keeps the line out
// of shell history where the shell ignores space-prefixed commands.
func (c *AppClient) prelude(ctx context.Context, terminal, cwd string, env map[string]string) error {
	var parts []string
	if cwd != "" {
		parts = append(parts, "cd -- "+shellQuote(cwd))
	}
	if len(env) > 0 {
		exports := make([]string, 0, len(env))
		for _, k := range slices.Sorted(maps.Keys(env)) {
			exports = append(exports, k+"="+shellQuote(env[k]))
		}
		parts = append(parts, "export "+strings.Join(exports, " "))
	}
	if len(parts) == 0 {
		return nil
	}
	line := ` eval "$(printf '` + octal(strings.Join(parts, " && ")) + `')"`
	c.mu.Lock()
	if c.setups == nil {
		c.setups = make(map[string]string)
	}
	c.setups[terminal] = line
	c.mu.Unlock()
	_, err := c.execNamed(ctx, c.CoreBin+" send "+terminal+" <terminal setup>", c.CoreBin, "send", terminal, line)
	return err
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// octal escapes every byte for printf, so the text survives any quoting.
func octal(s string) string {
	var b strings.Builder
	for i := range len(s) {
		fmt.Fprintf(&b, `\%03o`, s[i])
	}
	return b.String()
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
	var made appCreated
	if err := c.do(ctx, "space.new", map[string]any{"name": r.Label, "folder": r.CWD}, &made); err != nil {
		return Workspace{}, err
	}
	defer func() { err = errors.Join(err, restore()) }()
	w = Workspace{ID: made.Space, Label: r.Label, CWD: r.CWD, ActiveTabID: made.Tab}
	return w, c.prelude(ctx, made.Terminal, "", r.Env)
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
	var made appCreated
	if err := c.do(ctx, "tab.new", args, &made); err != nil {
		return Tab{}, err
	}
	defer func() { err = errors.Join(err, restore()) }()
	tab = Tab{ID: made.Tab, WorkspaceID: r.WorkspaceID, Label: r.Label, CWD: r.CWD}
	if tab.PaneID, err = c.paneOf(ctx, made.Terminal); err != nil {
		return tab, err
	}
	return tab, c.prelude(ctx, made.Terminal, "", r.Env)
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
	var made appCreated
	if err := c.do(ctx, "pane.split", map[string]any{"pane": r.PaneID, "direction": r.Direction}, &made); err != nil {
		return Pane{}, err
	}
	c.remember(made.Pane, made.Terminal)
	p := Pane{ID: made.Pane, CWD: r.CWD}
	err = c.prelude(ctx, made.Terminal, r.CWD, r.Env)
	if after, statusErr := c.status(ctx); statusErr != nil {
		err = errors.Join(err, statusErr)
	} else if before.FocusedPane != "" && after.FocusedPane == made.Pane {
		err = errors.Join(err, c.PaneFocus(ctx, before.FocusedPane))
	}
	return p, err
}

func (c *AppClient) PaneFocus(ctx context.Context, id string) error {
	return c.do(ctx, "pane.focus", map[string]any{"pane": id}, nil)
}

// PaneWaitOutput polls the terminal's screen and history, so output printed
// before the call counts. The echoed setup line is part of that history, so a
// match it contains is refused, like a match in the startup command.
func (c *AppClient) PaneWaitOutput(ctx context.Context, id, match string, timeout time.Duration) error {
	t, err := c.terminal(ctx, id)
	if err != nil {
		return err
	}
	c.mu.Lock()
	setup := c.setups[t]
	c.mu.Unlock()
	if setup != "" && strings.Contains(setup, match) {
		return fmt.Errorf("terminal %s: wait_for match %q appears in the terminal setup line it echoes", t, match)
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

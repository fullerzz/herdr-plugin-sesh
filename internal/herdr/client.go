package herdr

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"
)

type Worktree struct {
	CheckoutPath     string `json:"checkout_path"`
	IsLinkedWorktree bool   `json:"is_linked_worktree"`
	RepoKey          string `json:"repo_key"`
	RepoName         string `json:"repo_name"`
	RepoRoot         string `json:"repo_root"`
}

type Workspace struct {
	ID            string    `json:"id"`
	Label         string    `json:"label"`
	CWD           string    `json:"cwd"`
	ForegroundCWD string    `json:"foreground_cwd"`
	ActiveTabID   string    `json:"active_tab_id"`
	AgentStatus   string    `json:"agent_status"`
	Worktree      *Worktree `json:"worktree,omitempty"`
}
type Tab struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	Label       string `json:"label"`
	CWD         string `json:"cwd"`
	PaneID      string `json:"pane_id"`
}
type Pane struct {
	ID            string `json:"id"`
	WorkspaceID   string `json:"workspace_id"`
	TabID         string `json:"tab_id"`
	CWD           string `json:"cwd"`
	ForegroundCWD string `json:"foreground_cwd"`
	Focused       bool   `json:"focused"`
}

type PaneLayout struct {
	Zoomed bool `json:"zoomed"`
}

func (w *Workspace) UnmarshalJSON(data []byte) error {
	type workspace Workspace
	var v struct {
		workspace

		WorkspaceID string `json:"workspace_id"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*w = Workspace(v.workspace)
	if w.ID == "" {
		w.ID = v.WorkspaceID
	}
	return nil
}

func (t *Tab) UnmarshalJSON(data []byte) error {
	type tab Tab
	var v struct {
		tab

		TabID string `json:"tab_id"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*t = Tab(v.tab)
	if t.ID == "" {
		t.ID = v.TabID
	}
	return nil
}

func (p *Pane) UnmarshalJSON(data []byte) error {
	type pane Pane
	var v struct {
		pane

		PaneID string `json:"pane_id"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*p = Pane(v.pane)
	if p.ID == "" {
		p.ID = v.PaneID
	}
	return nil
}

// WorkspaceCreateRequest's Env applies only to the initial tab's root pane.
type WorkspaceCreateRequest struct {
	CWD, Label string
	Env        map[string]string
	Focus      bool
}
type TabCreateRequest struct {
	WorkspaceID, CWD, Label string
	Env                     map[string]string
	Focus                   bool
}

// PaneSplitRequest never focuses the new pane. Ratio uses Herdr's semantics:
// the split pane's (first child's) share; zero leaves Herdr's default.
type PaneSplitRequest struct {
	PaneID, Direction, CWD string
	Ratio                  float64
	Env                    map[string]string
}

type Client interface {
	WorkspaceList(context.Context) ([]Workspace, error)
	WorkspaceCreate(context.Context, WorkspaceCreateRequest) (Workspace, error)
	WorkspaceFocus(context.Context, string) error
	TabList(context.Context, string) ([]Tab, error)
	TabCreate(context.Context, TabCreateRequest) (Tab, error)
	TabFocus(context.Context, string) error
	TabRename(context.Context, string, string) error
	PaneList(context.Context, string) ([]Pane, error)
	PaneCurrent(context.Context) (Pane, error)
	PaneRun(context.Context, string, string) error
	PaneSplit(context.Context, PaneSplitRequest) (Pane, error)
	// PaneFocus also focuses the pane's tab and workspace.
	PaneFocus(context.Context, string) error
	// PaneWaitOutput waits until a line of the pane's recent output, including
	// output printed before the call, contains match.
	PaneWaitOutput(ctx context.Context, paneID, match string, timeout time.Duration) error
	PluginPaneOpen(context.Context, string, string, string) error
}
type Runner interface {
	Run(context.Context, string, ...string) ([]byte, []byte, error)
}
type ExecRunner struct{}

type omitCallerPaneKey struct{}

func (ExecRunner) Run(ctx context.Context, bin string, args ...string) ([]byte, []byte, error) {
	//nolint:gosec // HERDR_BIN_PATH may intentionally point at a user-selected herdr binary.
	c := exec.CommandContext(ctx, bin, args...)
	if omit, _ := ctx.Value(omitCallerPaneKey{}).(bool); omit {
		c.Env = environmentWithout(os.Environ(), "HERDR_PANE_ID")
	}
	var out, errb bytes.Buffer
	c.Stdout = &out
	c.Stderr = &errb
	err := c.Run()
	return out.Bytes(), errb.Bytes(), err
}

func environmentWithout(environ []string, name string) []string {
	prefix := name + "="
	filtered := make([]string, 0, len(environ))
	for _, entry := range environ {
		if !strings.HasPrefix(entry, prefix) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

type CLIClient struct {
	Bin     string
	Runner  Runner
	Timeout time.Duration
	// SocketPath serves socket API methods that the herdr CLI does not expose.
	SocketPath string
}

func NewCLIClient() *CLIClient {
	bin := os.Getenv("HERDR_BIN_PATH")
	if bin == "" {
		bin = "herdr"
	}
	return &CLIClient{Bin: bin, Runner: ExecRunner{}, Timeout: 10 * time.Second, SocketPath: os.Getenv("HERDR_SOCKET_PATH")}
}
func (c *CLIClient) run(ctx context.Context, args ...string) ([]byte, error) {
	return c.runFor(ctx, 0, args...)
}

// runFor extends the command timeout by extra for commands that block by design.
func (c *CLIClient) runFor(ctx context.Context, extra time.Duration, args ...string) ([]byte, error) {
	if c.Runner == nil {
		c.Runner = ExecRunner{}
	}
	if c.Timeout == 0 {
		c.Timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, c.Timeout+extra)
	defer cancel()
	out, stderr, err := c.Runner.Run(ctx, c.Bin, args...)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return out, fmt.Errorf("herdr %s: %w", strings.Join(redactEnvArgs(args), " "), ctxErr)
		}
		return out, fmt.Errorf("herdr %s: %w: %s", strings.Join(redactEnvArgs(args), " "), err, strings.TrimSpace(string(stderr)))
	}
	return out, nil
}

func responseJSON(out []byte, command string) (json.RawMessage, bool, error) {
	if !bytes.HasPrefix(bytes.TrimSpace(out), []byte("{")) {
		return out, false, nil
	}
	var env struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		return nil, false, fmt.Errorf("decode herdr %s JSON: %w", command, err)
	}
	if len(env.Result) > 0 {
		return env.Result, true, nil
	}
	return out, false, nil
}

func (c *CLIClient) WorkspaceList(ctx context.Context) ([]Workspace, error) {
	out, err := c.run(ctx, "workspace", "list")
	if err != nil {
		return nil, err
	}
	raw, wrapped, err := responseJSON(out, "workspace list")
	if err != nil {
		return nil, err
	}
	if wrapped {
		var resp struct {
			Workspaces []Workspace `json:"workspaces"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil, fmt.Errorf("decode herdr workspace list JSON: %w", err)
		}
		return resp.Workspaces, nil
	}
	var ws []Workspace
	if err := json.Unmarshal(raw, &ws); err != nil {
		return nil, fmt.Errorf("decode herdr workspace list JSON: %w", err)
	}
	return ws, nil
}
func (c *CLIClient) WorkspaceCreate(ctx context.Context, r WorkspaceCreateRequest) (Workspace, error) {
	args := []string{"workspace", "create", "--cwd", r.CWD, "--label", r.Label}
	args = appendEnvArgs(args, r.Env)
	if !r.Focus {
		args = append(args, "--no-focus")
	}
	out, err := c.run(ctx, args...)
	if err != nil {
		return Workspace{}, err
	}
	raw, wrapped, err := responseJSON(out, "workspace create")
	if err != nil {
		return Workspace{}, err
	}
	if wrapped {
		var resp struct {
			Workspace Workspace `json:"workspace"`
			RootPane  Pane      `json:"root_pane"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return Workspace{}, fmt.Errorf("decode herdr workspace create JSON: %w", err)
		}
		if resp.Workspace.CWD == "" {
			resp.Workspace.CWD = resp.RootPane.CWD
		}
		if resp.Workspace.ForegroundCWD == "" {
			resp.Workspace.ForegroundCWD = resp.RootPane.CWD
		}
		if r.Focus && resp.Workspace.ID != "" {
			if err := c.WorkspaceFocus(ctx, resp.Workspace.ID); err != nil {
				return Workspace{}, err
			}
		}
		return resp.Workspace, nil
	}
	var w Workspace
	if err := json.Unmarshal(raw, &w); err != nil {
		return Workspace{}, fmt.Errorf("decode herdr workspace create JSON: %w", err)
	}
	if r.Focus && w.ID != "" {
		if err := c.WorkspaceFocus(ctx, w.ID); err != nil {
			return Workspace{}, err
		}
	}
	return w, nil
}
func (c *CLIClient) WorkspaceFocus(ctx context.Context, id string) error {
	_, err := c.run(ctx, "workspace", "focus", id)
	return err
}
func (c *CLIClient) WorkspaceClose(ctx context.Context, id string) error {
	_, err := c.run(ctx, "workspace", "close", id)
	return err
}
func (c *CLIClient) TabList(ctx context.Context, wid string) ([]Tab, error) {
	args := []string{"tab", "list"}
	if wid != "" {
		args = append(args, "--workspace", wid)
	}
	out, err := c.run(ctx, args...)
	if err != nil {
		return nil, err
	}
	raw, wrapped, err := responseJSON(out, "tab list")
	if err != nil {
		return nil, err
	}
	if wrapped {
		var resp struct {
			Tabs []Tab `json:"tabs"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil, fmt.Errorf("decode herdr tab list JSON: %w", err)
		}
		return resp.Tabs, nil
	}
	var tabs []Tab
	if err := json.Unmarshal(raw, &tabs); err != nil {
		return nil, fmt.Errorf("decode herdr tab list JSON: %w", err)
	}
	return tabs, nil
}
func (c *CLIClient) TabCreate(ctx context.Context, r TabCreateRequest) (Tab, error) {
	args := []string{"tab", "create", "--workspace", r.WorkspaceID, "--cwd", r.CWD, "--label", r.Label}
	args = appendEnvArgs(args, r.Env)
	if !r.Focus {
		args = append(args, "--no-focus")
	}
	out, err := c.run(ctx, args...)
	if err != nil {
		return Tab{}, err
	}
	raw, wrapped, err := responseJSON(out, "tab create")
	if err != nil {
		return Tab{}, err
	}
	if wrapped {
		var resp struct {
			Tab      Tab  `json:"tab"`
			RootPane Pane `json:"root_pane"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return Tab{}, fmt.Errorf("decode herdr tab create JSON: %w", err)
		}
		if resp.Tab.CWD == "" {
			resp.Tab.CWD = resp.RootPane.CWD
		}
		if resp.Tab.PaneID == "" {
			resp.Tab.PaneID = resp.RootPane.ID
		}
		if r.Focus && resp.Tab.ID != "" {
			if err := c.TabFocus(ctx, resp.Tab.ID); err != nil {
				return Tab{}, err
			}
		}
		return resp.Tab, nil
	}
	var t Tab
	if err := json.Unmarshal(raw, &t); err != nil {
		return Tab{}, fmt.Errorf("decode herdr tab create JSON: %w", err)
	}
	if r.Focus && t.ID != "" {
		if err := c.TabFocus(ctx, t.ID); err != nil {
			return Tab{}, err
		}
	}
	return t, nil
}
func (c *CLIClient) TabFocus(ctx context.Context, id string) error {
	_, err := c.run(ctx, "tab", "focus", id)
	return err
}
func (c *CLIClient) TabRename(ctx context.Context, id, label string) error {
	_, err := c.run(ctx, "tab", "rename", id, label)
	return err
}
func (c *CLIClient) PaneList(ctx context.Context, wid string) ([]Pane, error) {
	args := []string{"pane", "list"}
	if wid != "" {
		args = append(args, "--workspace", wid)
	}
	out, err := c.run(ctx, args...)
	if err != nil {
		return nil, err
	}
	raw, wrapped, err := responseJSON(out, "pane list")
	if err != nil {
		return nil, err
	}
	if wrapped {
		var resp struct {
			Panes []Pane `json:"panes"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil, fmt.Errorf("decode herdr pane list JSON: %w", err)
		}
		return resp.Panes, nil
	}
	var panes []Pane
	if err := json.Unmarshal(raw, &panes); err != nil {
		return nil, fmt.Errorf("decode herdr pane list JSON: %w", err)
	}
	return panes, nil
}
func (c *CLIClient) PaneCurrent(ctx context.Context) (Pane, error) {
	return c.paneCurrent(ctx)
}
func (c *CLIClient) PaneFocused(ctx context.Context) (Pane, error) {
	return c.paneCurrent(context.WithValue(ctx, omitCallerPaneKey{}, true))
}
func (c *CLIClient) paneCurrent(ctx context.Context) (Pane, error) {
	out, err := c.run(ctx, "pane", "current")
	if err != nil {
		return Pane{}, err
	}
	return decodePane(out, "pane current")
}

// decodePane accepts both the {"result":{"pane":...}} envelope and a bare pane.
func decodePane(out []byte, command string) (Pane, error) {
	raw, wrapped, err := responseJSON(out, command)
	if err != nil {
		return Pane{}, err
	}
	var p Pane
	if wrapped {
		var resp struct {
			Pane Pane `json:"pane"`
		}
		err = json.Unmarshal(raw, &resp)
		p = resp.Pane
	} else {
		err = json.Unmarshal(raw, &p)
	}
	if err != nil {
		return Pane{}, fmt.Errorf("decode herdr %s JSON: %w", command, err)
	}
	return p, nil
}
func (c *CLIClient) PaneRun(ctx context.Context, id, cmd string) error {
	_, err := c.run(ctx, "pane", "run", id, cmd)
	return err
}
func (c *CLIClient) PaneSplit(ctx context.Context, r PaneSplitRequest) (Pane, error) {
	args := []string{"pane", "split", r.PaneID, "--direction", r.Direction, "--no-focus"}
	if r.Ratio != 0 {
		args = append(args, "--ratio", strconv.FormatFloat(r.Ratio, 'f', -1, 32))
	}
	if r.CWD != "" {
		args = append(args, "--cwd", r.CWD)
	}
	args = appendEnvArgs(args, r.Env)
	out, err := c.run(ctx, args...)
	if err != nil {
		return Pane{}, err
	}
	p, err := decodePane(out, "pane split")
	if err != nil {
		return Pane{}, err
	}
	if p.ID == "" {
		return Pane{}, errors.New("herdr pane split returned no pane id")
	}
	return p, nil
}

// PaneFocus calls the socket API because Herdr's CLI only moves focus by direction.
func (c *CLIClient) PaneFocus(ctx context.Context, id string) error {
	if c.SocketPath == "" {
		return errors.New("focus pane: HERDR_SOCKET_PATH is not set; run herdr-sesh from a Herdr pane")
	}
	ctx, cancel := context.WithTimeout(ctx, cmp.Or(c.Timeout, 10*time.Second))
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", c.SocketPath)
	if err != nil {
		return fmt.Errorf("focus pane %s: %w", id, err)
	}
	defer func() { _ = conn.Close() }()
	stopClose := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopClose()

	request := struct {
		ID     string `json:"id"`
		Method string `json:"method"`
		Params struct {
			PaneID string `json:"pane_id"`
		} `json:"params"`
	}{ID: "herdr-sesh-pane-focus", Method: "pane.focus"}
	request.Params.PaneID = id
	var response struct {
		Error *apiError `json:"error"`
	}
	err = json.NewEncoder(conn).Encode(request)
	if err == nil {
		err = json.NewDecoder(conn).Decode(&response)
	}
	if ctxErr := ctx.Err(); err != nil && ctxErr != nil {
		err = ctxErr
	}
	if err != nil {
		return fmt.Errorf("focus pane %s: %w", id, err)
	}
	if response.Error != nil {
		return fmt.Errorf("focus pane %s: %s: %s", id, response.Error.Code, response.Error.Message)
	}
	return nil
}

// PaneWaitOutput leaves the timeout to Herdr so its error explains the failure.
func (c *CLIClient) PaneWaitOutput(ctx context.Context, id, match string, timeout time.Duration) error {
	// --flag=value keeps a match starting with "-" from parsing as an option.
	// recent-unwrapped joins soft-wrapped lines, so a narrow pane cannot split
	// the match; Herdr's CLI help still lists recent as the default.
	_, err := c.runFor(ctx, timeout, "pane", "wait-output", id, "--match="+match, "--source=recent-unwrapped", "--timeout="+strconv.FormatInt(timeout.Milliseconds(), 10))
	return err
}

// redactEnvArgs hides --env values, which may hold secrets, from error text.
func redactEnvArgs(args []string) []string {
	out := slices.Clone(args)
	for i := 1; i < len(out); i++ {
		if out[i-1] == "--env" {
			key, _, _ := strings.Cut(out[i], "=")
			out[i] = key + "=<redacted>"
		}
	}
	return out
}

// appendEnvArgs sorts keys so argument order is deterministic.
func appendEnvArgs(args []string, env map[string]string) []string {
	for _, key := range slices.Sorted(maps.Keys(env)) {
		args = append(args, "--env", key+"="+env[key])
	}
	return args
}
func (c *CLIClient) PaneLayout(ctx context.Context, id string) (PaneLayout, error) {
	out, err := c.run(ctx, "pane", "layout", "--pane", id)
	if err != nil {
		return PaneLayout{}, err
	}
	raw, wrapped, err := responseJSON(out, "pane layout")
	if err != nil {
		return PaneLayout{}, err
	}
	if wrapped {
		var resp struct {
			Layout PaneLayout `json:"layout"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return PaneLayout{}, fmt.Errorf("decode herdr pane layout JSON: %w", err)
		}
		return resp.Layout, nil
	}
	var layout PaneLayout
	if err := json.Unmarshal(raw, &layout); err != nil {
		return PaneLayout{}, fmt.Errorf("decode herdr pane layout JSON: %w", err)
	}
	return layout, nil
}
func (c *CLIClient) PaneZoom(ctx context.Context, id string) error {
	_, err := c.run(ctx, "pane", "zoom", id, "--on")
	return err
}
func (c *CLIClient) PluginPaneOpen(ctx context.Context, plugin, entry, placement string) error {
	_, err := c.run(ctx, "plugin", "pane", "open", "--plugin", plugin, "--entrypoint", entry, "--placement", placement)
	return err
}

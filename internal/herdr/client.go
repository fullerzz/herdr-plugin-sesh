package herdr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Workspace struct {
	ID            string `json:"id"`
	Number        int    `json:"number"`
	Label         string `json:"label"`
	CWD           string `json:"cwd"`
	ForegroundCWD string `json:"foreground_cwd"`
	ActiveTabID   string `json:"active_tab_id"`
	AgentStatus   string `json:"agent_status"`
	TabCount      int    `json:"tab_count"`
	PaneCount     int    `json:"pane_count"`
}
type Tab struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	Number      int    `json:"number"`
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
	Label         string `json:"label"`
	Agent         string `json:"agent"`
	Title         string `json:"title"`
	DisplayAgent  string `json:"display_agent"`
	AgentStatus   string `json:"agent_status"`
}

type PaneRect struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

type PaneLayoutPane struct {
	ID      string   `json:"pane_id"`
	Focused bool     `json:"focused"`
	Rect    PaneRect `json:"rect"`
	Command string   `json:"-"`
}

type PaneLayout struct {
	WorkspaceID   string           `json:"workspace_id"`
	TabID         string           `json:"tab_id"`
	Zoomed        bool             `json:"zoomed"`
	Area          PaneRect         `json:"area"`
	FocusedPaneID string           `json:"focused_pane_id"`
	Panes         []PaneLayoutPane `json:"panes"`
}

type paneProcessInfo struct {
	ShellPID                 uint32        `json:"shell_pid"`
	ForegroundProcessGroupID uint32        `json:"foreground_process_group_id"`
	ForegroundProcesses      []paneProcess `json:"foreground_processes"`
}

type paneProcess struct {
	PID     uint32   `json:"pid"`
	Name    string   `json:"name"`
	Argv0   string   `json:"argv0"`
	Argv    []string `json:"argv"`
	Cmdline string   `json:"cmdline"`
}

func (info paneProcessInfo) runningCommand() string {
	if info.ForegroundProcessGroupID == 0 || info.ForegroundProcessGroupID == info.ShellPID {
		return ""
	}
	var process *paneProcess
	for i := range info.ForegroundProcesses {
		if info.ForegroundProcesses[i].PID == info.ForegroundProcessGroupID {
			process = &info.ForegroundProcesses[i]
			break
		}
	}
	if process == nil {
		for i := range info.ForegroundProcesses {
			if info.ForegroundProcesses[i].PID != info.ShellPID {
				process = &info.ForegroundProcesses[i]
				break
			}
		}
	}
	if process == nil {
		return ""
	}
	for _, candidate := range []string{process.Cmdline, strings.Join(process.Argv, " "), process.Argv0, process.Name} {
		command := strings.TrimSpace(candidate)
		if command != "" && !strings.EqualFold(command, "unknown") {
			return command
		}
	}
	return ""
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

type WorkspaceCreateRequest struct {
	CWD, Label string
	Focus      bool
}
type TabCreateRequest struct {
	WorkspaceID, CWD, Label string
	Focus                   bool
}

type Client interface {
	WorkspaceList(context.Context) ([]Workspace, error)
	WorkspaceCreate(context.Context, WorkspaceCreateRequest) (Workspace, error)
	WorkspaceFocus(context.Context, string) error
	TabList(context.Context, string) ([]Tab, error)
	TabCreate(context.Context, TabCreateRequest) (Tab, error)
	TabFocus(context.Context, string) error
	PaneList(context.Context, string) ([]Pane, error)
	PaneCurrent(context.Context) (Pane, error)
	PaneRun(context.Context, string, string) error
	PluginPaneOpen(context.Context, string, string, string) error
}
type Runner interface {
	Run(context.Context, string, ...string) ([]byte, []byte, error)
}
type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, bin string, args ...string) ([]byte, []byte, error) {
	//nolint:gosec // HERDR_BIN_PATH may intentionally point at a user-selected herdr binary.
	c := exec.CommandContext(ctx, bin, args...)
	var out, errb bytes.Buffer
	c.Stdout = &out
	c.Stderr = &errb
	err := c.Run()
	return out.Bytes(), errb.Bytes(), err
}

type CLIClient struct {
	Bin     string
	Runner  Runner
	Timeout time.Duration
}

func NewCLIClient() *CLIClient {
	bin := os.Getenv("HERDR_BIN_PATH")
	if bin == "" {
		bin = "herdr"
	}
	return &CLIClient{Bin: bin, Runner: ExecRunner{}, Timeout: 10 * time.Second}
}
func (c *CLIClient) run(ctx context.Context, args ...string) ([]byte, error) {
	if c.Runner == nil {
		c.Runner = ExecRunner{}
	}
	if c.Timeout == 0 {
		c.Timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	out, stderr, err := c.Runner.Run(ctx, c.Bin, args...)
	if err != nil {
		return out, fmt.Errorf("herdr %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(stderr)))
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
func (c *CLIClient) PaneLayout(ctx context.Context, paneID string) (PaneLayout, error) {
	out, err := c.run(ctx, "pane", "layout", "--pane", paneID)
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
func (c *CLIClient) PaneRunningCommand(ctx context.Context, paneID string) (string, error) {
	out, err := c.run(ctx, "pane", "process-info", "--pane", paneID)
	if err != nil {
		return "", err
	}
	raw, wrapped, err := responseJSON(out, "pane process info")
	if err != nil {
		return "", err
	}
	var info paneProcessInfo
	if wrapped {
		var resp struct {
			ProcessInfo paneProcessInfo `json:"process_info"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return "", fmt.Errorf("decode herdr pane process info JSON: %w", err)
		}
		info = resp.ProcessInfo
	} else if err := json.Unmarshal(raw, &info); err != nil {
		return "", fmt.Errorf("decode herdr pane process info JSON: %w", err)
	}
	return info.runningCommand(), nil
}
func (c *CLIClient) PaneCurrent(ctx context.Context) (Pane, error) {
	out, err := c.run(ctx, "pane", "current")
	if err != nil {
		return Pane{}, err
	}
	raw, wrapped, err := responseJSON(out, "pane current")
	if err != nil {
		return Pane{}, err
	}
	if wrapped {
		var resp struct {
			Pane Pane `json:"pane"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return Pane{}, fmt.Errorf("decode herdr pane current JSON: %w", err)
		}
		return resp.Pane, nil
	}
	var p Pane
	if err := json.Unmarshal(raw, &p); err != nil {
		return Pane{}, fmt.Errorf("decode herdr pane current JSON: %w", err)
	}
	return p, nil
}
func (c *CLIClient) PaneRun(ctx context.Context, id, cmd string) error {
	_, err := c.run(ctx, "pane", "run", id, cmd)
	return err
}
func (c *CLIClient) PluginPaneOpen(ctx context.Context, plugin, entry, placement string) error {
	_, err := c.run(ctx, "plugin", "pane", "open", "--plugin", plugin, "--entrypoint", entry, "--placement", placement)
	return err
}

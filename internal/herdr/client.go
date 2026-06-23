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
	Label         string `json:"label"`
	CWD           string `json:"cwd"`
	ForegroundCWD string `json:"foreground_cwd"`
}
type Tab struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	Label       string `json:"label"`
	CWD         string `json:"cwd"`
	PaneID      string `json:"pane_id"`
}
type Pane struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	TabID       string `json:"tab_id"`
	CWD         string `json:"cwd"`
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
	PaneCurrent(context.Context) (Pane, error)
	PaneRun(context.Context, string, string) error
	PluginPaneOpen(context.Context, string, string, string) error
}
type Runner interface {
	Run(context.Context, string, ...string) ([]byte, []byte, error)
}
type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, bin string, args ...string) ([]byte, []byte, error) {
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
func (c *CLIClient) WorkspaceList(ctx context.Context) ([]Workspace, error) {
	out, err := c.run(ctx, "workspace", "list", "--json")
	if err != nil {
		return nil, err
	}
	var ws []Workspace
	_ = json.Unmarshal(out, &ws)
	return ws, nil
}
func (c *CLIClient) WorkspaceCreate(ctx context.Context, r WorkspaceCreateRequest) (Workspace, error) {
	args := []string{"workspace", "create", "--cwd", r.CWD, "--label", r.Label}
	if r.Focus {
		args = append(args, "--focus")
	} else {
		args = append(args, "--no-focus")
	}
	args = append(args, "--json")
	out, err := c.run(ctx, args...)
	if err != nil {
		return Workspace{}, err
	}
	var w Workspace
	_ = json.Unmarshal(out, &w)
	return w, nil
}
func (c *CLIClient) WorkspaceFocus(ctx context.Context, id string) error {
	_, err := c.run(ctx, "workspace", "focus", id)
	return err
}
func (c *CLIClient) TabList(ctx context.Context, wid string) ([]Tab, error) {
	args := []string{"tab", "list", "--json"}
	if wid != "" {
		args = append(args, "--workspace", wid)
	}
	out, err := c.run(ctx, args...)
	if err != nil {
		return nil, err
	}
	var tabs []Tab
	_ = json.Unmarshal(out, &tabs)
	return tabs, nil
}
func (c *CLIClient) TabCreate(ctx context.Context, r TabCreateRequest) (Tab, error) {
	args := []string{"tab", "create", "--workspace", r.WorkspaceID, "--cwd", r.CWD, "--label", r.Label}
	if r.Focus {
		args = append(args, "--focus")
	} else {
		args = append(args, "--no-focus")
	}
	args = append(args, "--json")
	out, err := c.run(ctx, args...)
	if err != nil {
		return Tab{}, err
	}
	var t Tab
	_ = json.Unmarshal(out, &t)
	return t, nil
}
func (c *CLIClient) TabFocus(ctx context.Context, id string) error {
	_, err := c.run(ctx, "tab", "focus", id)
	return err
}
func (c *CLIClient) PaneCurrent(ctx context.Context) (Pane, error) {
	out, err := c.run(ctx, "pane", "current", "--json")
	if err != nil {
		return Pane{}, err
	}
	var p Pane
	_ = json.Unmarshal(out, &p)
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

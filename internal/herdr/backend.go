package herdr

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Backend is what herdr-sesh commands need from the Herdr they run under:
// the Herdr TUI server (CLIClient) or the Herdr desktop app (AppClient).
type Backend interface {
	Client
	WorkspaceClose(context.Context, string) error
	PaneFocused(context.Context) (Pane, error)
	// WorkspacePaneRead reads the screen of a workspace's selected pane.
	WorkspacePaneRead(context.Context, string) (string, error)
	// CurrentWorkspaceID is the workspace herdr-sesh was opened from, or "".
	CurrentWorkspaceID(context.Context) string
}

// UsesApp reports whether herdr-sesh should drive herdr-app.
// HERDR_SESH_BACKEND=app or =herdr forces a backend. Otherwise a Herdr pane
// (HERDR_SOCKET_PATH) selects the TUI server, and an hc terminal without one
// selects herdr-app.
func UsesApp() bool {
	switch strings.ToLower(os.Getenv("HERDR_SESH_BACKEND")) {
	case "app":
		return true
	case "herdr":
		return false
	}
	return os.Getenv("HERDR_SOCKET_PATH") == "" && os.Getenv("HC_TERMINAL_SOCKET") != ""
}

//nolint:ireturn // The backend is chosen at run time from the environment.
func NewBackend() Backend {
	if UsesApp() {
		return NewAppClient()
	}
	return NewCLIClient()
}

func (c *CLIClient) CurrentWorkspaceID(context.Context) string {
	return os.Getenv("HERDR_WORKSPACE_ID")
}

// CurrentWorkspaceID is the window's shown space; the picker overlay never
// becomes a space of its own. It is "" when herdr-app doesn't answer.
func (c *AppClient) CurrentWorkspaceID(ctx context.Context) string {
	p, err := c.PaneFocused(ctx)
	if err != nil {
		return ""
	}
	return p.WorkspaceID
}

// WorkspacePaneRead reads the focused pane of the shown space, or the first
// pane of another space's active tab.
func (c *AppClient) WorkspacePaneRead(ctx context.Context, workspaceID string) (string, error) {
	if workspaceID == "" {
		return "", errors.New("no herdr-app space selected")
	}
	l, err := c.layout(ctx)
	if err != nil {
		return "", err
	}
	shown, _ := c.status(ctx)
	for _, s := range l.Spaces {
		if s.ID != workspaceID {
			continue
		}
		for _, t := range s.Tabs {
			if !t.Active || len(t.Terminals) == 0 {
				continue
			}
			terminal := t.Terminals[0].Terminal
			for _, term := range t.Terminals {
				if term.Pane == shown.FocusedPane {
					terminal = term.Terminal
				}
			}
			out, err := c.exec(ctx, c.CoreBin, "read", terminal)
			return string(out), err
		}
	}
	return "", fmt.Errorf("no active pane available for space %s", workspaceID)
}

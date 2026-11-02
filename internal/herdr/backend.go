package herdr

import (
	"context"
	"os"
	"strings"
)

// Backend is what herdr-sesh commands need from the Herdr they run under:
// the Herdr TUI server (CLIClient) or the Herdr desktop app (AppClient).
type Backend interface {
	Client
	WorkspaceClose(context.Context, string) error
	PaneFocused(context.Context) (Pane, error)
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
// becomes a space of its own.
func (c *AppClient) CurrentWorkspaceID(ctx context.Context) string {
	p, err := c.PaneFocused(ctx)
	if err != nil {
		return ""
	}
	return p.WorkspaceID
}

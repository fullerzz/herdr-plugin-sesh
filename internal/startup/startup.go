package startup

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/fullerzz/herdr-plugin-sesh/internal/config"
	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
)

type Plan struct {
	WorkspaceID string
	Path        string
	Session     model.Session
	// Focus lets the first configured tab take focus if its root keeps the
	// workspace path; background creation leaves it false.
	Focus      bool
	InitialTab InitialTabPolicy
}

// InitialTabPolicy couples initial-tab reuse with the environment that must be
// supplied to WorkspaceCreate. Pass the same policy to Apply in the Plan.
type InitialTabPolicy struct {
	Reuse bool
	Env   map[string]string
}

// PlanInitialTab decides whether the first configured tab can take over the
// initial tab. A workspace startup command keeps the initial tab to itself so
// it never shares a terminal with a tab or pane startup command. The root pane
// must also start in the workspace path: Herdr reports the active pane's
// directory as the workspace path, which path lookups rely on.
func PlanInitialTab(s model.Session) InitialTabPolicy {
	if len(s.WindowConfigs) == 0 || runsWorkspaceStartup(s) {
		return InitialTabPolicy{}
	}
	cwd, env := rootPane(s.WindowConfigs[0], s.Path)
	if filepath.Clean(cwd) != filepath.Clean(s.Path) {
		return InitialTabPolicy{}
	}
	return InitialTabPolicy{Reuse: true, Env: env}
}

func rootPane(w model.WindowConfig, path string) (string, map[string]string) {
	cwd, env := path, map[string]string(nil)
	if w.Path != "" {
		cwd = w.Path
	}
	if len(w.Panes) > 0 {
		env = w.Panes[0].Env
		if w.Panes[0].Path != "" {
			cwd = w.Panes[0].Path
		}
	}
	return cwd, env
}

func runsWorkspaceStartup(s model.Session) bool {
	return !s.DisableStartupCommand && s.StartupCommand != ""
}

func Apply(ctx context.Context, client herdr.Client, p Plan) error {
	if client == nil {
		return nil
	}
	path := p.Path
	if path == "" {
		path = p.Session.Path
	}
	// Run workspace startup before creating tabs, in the initial workspace pane.
	// Interactive workspace and tab commands must never share a terminal input.
	if runsWorkspaceStartup(p.Session) {
		pane, err := findPane(ctx, client, p.WorkspaceID, "")
		if err != nil {
			return err
		}
		if err := client.PaneRun(ctx, pane.ID, config.SubstitutePath(p.Session.StartupCommand, path)); err != nil {
			return fmt.Errorf("workspace %q: run startup: %w", p.Session.Name, err)
		}
	}
	for i, w := range p.Session.WindowConfigs {
		cwd := path
		if w.Path != "" {
			cwd = w.Path
		}
		var tab herdr.Tab
		var err error
		if i == 0 && p.InitialTab.Reuse {
			// The initial tab is already active, including in a background
			// workspace, so it needs no focus call.
			tab, err = claimInitialTab(ctx, client, p.WorkspaceID, w.Name)
		} else {
			// Herdr's tab focus also focuses the workspace, and it has no way to
			// set a background workspace's active tab, so --no-focus leaves the
			// workspace on its initial tab.
			req := herdr.TabCreateRequest{WorkspaceID: p.WorkspaceID, Label: w.Name}
			// Herdr can only set a root pane's cwd and env when creating its tab.
			req.CWD, req.Env = rootPane(w, path)
			// Keep the initial tab active when the configured root is elsewhere:
			// Herdr reports the active pane's directory for path-based reconnects.
			req.Focus = p.Focus && i == 0 && filepath.Clean(req.CWD) == filepath.Clean(path)
			if tab, err = client.TabCreate(ctx, req); err != nil {
				err = fmt.Errorf("create tab: %w", err)
			}
		}
		if err != nil {
			return fmt.Errorf("workspace %q tab %q: %w (the workspace was kept)", p.Session.Name, w.Name, err)
		}
		if len(w.Panes) == 0 && w.StartupScript == "" {
			continue
		}
		if tab.PaneID == "" {
			if tab.ID == "" {
				return fmt.Errorf("workspace %q tab %q: Herdr returned neither tab nor root pane ID", p.Session.Name, w.Name)
			}
			root, err := findPane(ctx, client, p.WorkspaceID, tab.ID)
			if err != nil {
				return fmt.Errorf("workspace %q tab %q: find root pane: %w", p.Session.Name, w.Name, err)
			}
			tab.PaneID = root.ID
		}
		if len(w.Panes) > 0 {
			if err := applyPanes(ctx, client, tab.PaneID, w.Panes); err != nil {
				return fmt.Errorf("workspace %q tab %q: %w (the workspace was kept; reconnecting does not retry the layout)", p.Session.Name, w.Name, err)
			}
			continue
		}
		if w.StartupScript != "" {
			if err := client.PaneRun(ctx, tab.PaneID, config.SubstitutePath(w.StartupScript, cwd)); err != nil {
				return err
			}
		}
	}
	return nil
}

// claimInitialTab labels the new workspace's only tab so it becomes the first
// configured tab.
func claimInitialTab(ctx context.Context, client herdr.Client, workspaceID, label string) (herdr.Tab, error) {
	pane, err := findPane(ctx, client, workspaceID, "")
	if err != nil {
		return herdr.Tab{}, fmt.Errorf("reuse initial tab: %w", err)
	}
	if pane.TabID == "" {
		return herdr.Tab{}, fmt.Errorf("reuse initial tab: Herdr returned no tab ID for pane %q", pane.ID)
	}
	if err := client.TabRename(ctx, pane.TabID, label); err != nil {
		return herdr.Tab{}, fmt.Errorf("reuse initial tab: %w", err)
	}
	return herdr.Tab{ID: pane.TabID, WorkspaceID: workspaceID, Label: label, PaneID: pane.ID}, nil
}

// An empty tabID is used only before configured tabs are created.
func findPane(ctx context.Context, client herdr.Client, workspaceID, tabID string) (herdr.Pane, error) {
	panes, err := client.PaneList(ctx, workspaceID)
	if err != nil {
		return herdr.Pane{}, err
	}
	for _, pane := range panes {
		if pane.ID != "" && pane.WorkspaceID == workspaceID && (tabID == "" || pane.TabID == tabID) {
			return pane, nil
		}
	}
	if tabID != "" {
		return herdr.Pane{}, fmt.Errorf("no pane available in workspace %q tab %q", workspaceID, tabID)
	}
	return herdr.Pane{}, fmt.Errorf("no pane available in workspace %q", workspaceID)
}

// applyPanes reuses rootPane for the first pane, then splits earlier panes in
// declaration order. Config validation guarantees every split_from is earlier.
func applyPanes(ctx context.Context, client herdr.Client, rootPane string, panes []model.PaneConfig) error {
	ids := make(map[string]string, len(panes))
	for i, pane := range panes {
		id := rootPane
		if i > 0 {
			req := herdr.PaneSplitRequest{PaneID: ids[pane.SplitFrom], Direction: pane.Split, CWD: pane.Path, Env: pane.Env}
			if pane.Ratio != 0 {
				// Config ratios are the new pane's share; Herdr's is the split pane's.
				req.Ratio = 1 - pane.Ratio
			}
			created, err := client.PaneSplit(ctx, req)
			if err != nil {
				return fmt.Errorf("pane %q: split from %q: %w", pane.Name, pane.SplitFrom, err)
			}
			id = created.ID
		}
		ids[pane.Name] = id
		cmd := config.SubstitutePath(pane.Startup, pane.Path)
		if cmd != "" {
			if err := client.PaneRun(ctx, id, cmd); err != nil {
				return fmt.Errorf("pane %q: run startup: %w", pane.Name, err)
			}
		}
	}
	return nil
}

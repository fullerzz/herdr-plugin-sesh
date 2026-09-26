package startup

import (
	"context"
	"fmt"

	"github.com/fullerzz/herdr-plugin-sesh/internal/config"
	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
)

type Plan struct {
	WorkspaceID string
	Path        string
	Session     model.Session
	// Focus lets the first configured tab take focus; background creation
	// leaves it false so Herdr focus stays where it was.
	Focus bool
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
	if !p.Session.DisableStartupCommand && p.Session.StartupCommand != "" {
		paneID, err := findPane(ctx, client, p.WorkspaceID, "")
		if err != nil {
			return err
		}
		if err := client.PaneRun(ctx, paneID, config.SubstitutePath(p.Session.StartupCommand, path)); err != nil {
			return fmt.Errorf("workspace %q: run startup: %w", p.Session.Name, err)
		}
	}
	for i, w := range p.Session.WindowConfigs {
		cwd := path
		if w.Path != "" {
			cwd = w.Path
		}
		// Herdr's tab focus also focuses the workspace, and it has no way to
		// set a background workspace's active tab, so --no-focus leaves the
		// workspace on its initial tab.
		req := herdr.TabCreateRequest{WorkspaceID: p.WorkspaceID, CWD: cwd, Label: w.Name, Focus: p.Focus && i == 0}
		if len(w.Panes) > 0 {
			// Herdr can only set a root pane's cwd and env when creating its tab.
			req.CWD = w.Panes[0].Path
			req.Env = w.Panes[0].Env
		}
		tab, err := client.TabCreate(ctx, req)
		if err != nil {
			return fmt.Errorf("workspace %q tab %q: create tab: %w (the workspace was kept)", p.Session.Name, w.Name, err)
		}
		if len(w.Panes) == 0 && w.StartupScript == "" {
			continue
		}
		if tab.PaneID == "" {
			if tab.ID == "" {
				return fmt.Errorf("workspace %q tab %q: Herdr returned neither tab nor root pane ID", p.Session.Name, w.Name)
			}
			tab.PaneID, err = findPane(ctx, client, p.WorkspaceID, tab.ID)
			if err != nil {
				return fmt.Errorf("workspace %q tab %q: find root pane: %w", p.Session.Name, w.Name, err)
			}
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

// An empty tabID is used only before configured tabs are created.
func findPane(ctx context.Context, client herdr.Client, workspaceID, tabID string) (string, error) {
	panes, err := client.PaneList(ctx, workspaceID)
	if err != nil {
		return "", err
	}
	for _, pane := range panes {
		if pane.ID != "" && pane.WorkspaceID == workspaceID && (tabID == "" || pane.TabID == tabID) {
			return pane.ID, nil
		}
	}
	if tabID != "" {
		return "", fmt.Errorf("no pane available in workspace %q tab %q", workspaceID, tabID)
	}
	return "", fmt.Errorf("no pane available in workspace %q", workspaceID)
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

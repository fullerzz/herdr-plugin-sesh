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
	var firstPane, workspaceStartup string
	if !p.Session.DisableStartupCommand {
		workspaceStartup = config.SubstitutePath(p.Session.StartupCommand, path)
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
			req.CWD = panePath(cwd, w.Panes[0].Path)
			req.Env = w.Panes[0].Env
		}
		tab, err := client.TabCreate(ctx, req)
		if err != nil {
			return err
		}
		if i == 0 {
			firstPane = tab.PaneID
		}
		if len(w.Panes) > 0 {
			rootPrefix := ""
			if i == 0 {
				rootPrefix, workspaceStartup = workspaceStartup, ""
			}
			if err := applyPanes(ctx, client, tab.PaneID, cwd, w.Panes, rootPrefix); err != nil {
				return fmt.Errorf("workspace %q tab %q: %w (the workspace was kept; reconnecting does not retry the layout)", p.Session.Name, w.Name, err)
			}
			continue
		}
		if w.StartupScript != "" && tab.PaneID != "" {
			if err := client.PaneRun(ctx, tab.PaneID, config.SubstitutePath(w.StartupScript, cwd)); err != nil {
				return err
			}
		}
	}
	if workspaceStartup == "" {
		return nil
	}
	if firstPane != "" {
		return client.PaneRun(ctx, firstPane, workspaceStartup)
	}
	panes, err := client.PaneList(ctx, p.WorkspaceID)
	if err != nil {
		return err
	}
	for _, pane := range panes {
		if pane.ID != "" && pane.WorkspaceID == p.WorkspaceID {
			return client.PaneRun(ctx, pane.ID, workspaceStartup)
		}
	}
	return fmt.Errorf("no pane available in workspace %q", p.WorkspaceID)
}

// applyPanes reuses rootPane for the first pane, then splits earlier panes in
// declaration order. Config validation guarantees every split_from is earlier.
// rootPrefix is the workspace startup command, sent in the same shell line as
// the root pane's command: pane run only types input, so a second run could
// land in the first command's program (such as nvim) instead of the shell.
func applyPanes(ctx context.Context, client herdr.Client, rootPane, tabPath string, panes []model.PaneConfig, rootPrefix string) error {
	if rootPane == "" {
		return fmt.Errorf("pane %q: herdr returned no root pane", panes[0].Name)
	}
	ids := make(map[string]string, len(panes))
	for i, pane := range panes {
		cwd := panePath(tabPath, pane.Path)
		id := rootPane
		if i > 0 {
			req := herdr.PaneSplitRequest{PaneID: ids[pane.SplitFrom], Direction: pane.Split, CWD: cwd, Env: pane.Env}
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
		cmd := config.SubstitutePath(pane.Startup, cwd)
		if i == 0 && rootPrefix != "" {
			if cmd == "" {
				cmd = rootPrefix
			} else {
				// Evaluate each quoted command separately in the same shell so
				// comments and terminators cannot consume the next command, while
				// workspace exports and directory changes remain available to it.
				cmd = config.SubstitutePath("eval {}", rootPrefix) + "; " + config.SubstitutePath("eval {}", cmd)
			}
		}
		if cmd != "" {
			if err := client.PaneRun(ctx, id, cmd); err != nil {
				return fmt.Errorf("pane %q: run startup: %w", pane.Name, err)
			}
		}
	}
	return nil
}

// panePath expands ~/ and resolves relative paths against the tab path.
func panePath(tabPath, path string) string {
	path = config.ExpandHome(path, "")
	if path == "" {
		return tabPath
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(tabPath, path)
}

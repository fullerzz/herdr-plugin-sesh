package startup

import (
	"context"

	"forgejo.local/fullerzz/herdr-plugin-sesh/internal/config"
	"forgejo.local/fullerzz/herdr-plugin-sesh/internal/herdr"
	"forgejo.local/fullerzz/herdr-plugin-sesh/internal/model"
)

type Plan struct {
	WorkspaceID string
	Path        string
	Session     model.Session
}

func Apply(ctx context.Context, client herdr.Client, p Plan) error {
	if client == nil {
		return nil
	}
	path := p.Path
	if path == "" {
		path = p.Session.Path
	}
	var firstPane string
	for i, w := range p.Session.WindowConfigs {
		cwd := path
		if w.Path != "" {
			cwd = w.Path
		}
		tab, err := client.TabCreate(ctx, herdr.TabCreateRequest{WorkspaceID: p.WorkspaceID, CWD: cwd, Label: w.Name, Focus: i == 0})
		if err != nil {
			return err
		}
		if i == 0 && tab.PaneID != "" {
			firstPane = tab.PaneID
		}
		if w.StartupScript != "" && tab.PaneID != "" {
			if err := client.PaneRun(ctx, tab.PaneID, config.SubstitutePath(w.StartupScript, cwd)); err != nil {
				return err
			}
		}
	}
	if p.Session.DisableStartupCommand || p.Session.StartupCommand == "" {
		return nil
	}
	paneID := firstPane
	if paneID == "" {
		pane, err := client.PaneCurrent(ctx)
		if err != nil {
			return err
		}
		paneID = pane.ID
	}
	return client.PaneRun(ctx, paneID, config.SubstitutePath(p.Session.StartupCommand, path))
}

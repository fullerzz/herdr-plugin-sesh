package sources

import (
	"cmp"
	"context"

	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
)

type HerdrWorkspaces struct{ Client herdr.Client }

func (HerdrWorkspaces) Name() string { return "herdr" }
func (s HerdrWorkspaces) List(ctx context.Context) (model.Sessions, error) {
	out := model.NewSessions()
	if s.Client == nil {
		return out, nil
	}
	ws, err := s.Client.WorkspaceList(ctx)
	if err != nil {
		return out, err
	}
	panes, err := s.Client.PaneList(ctx, "")
	if err != nil {
		panes = nil
	}
	tabs, err := s.Client.TabList(ctx, "")
	if err != nil {
		tabs = nil
	}
	for _, w := range ws {
		path := workspacePath(w, panes)
		out.Add(model.Session{
			Source:         "herdr",
			Name:           w.Label,
			Path:           path,
			WorkspaceID:    w.ID,
			AgentStatus:    w.AgentStatus,
			TabCount:       w.TabCount,
			PaneCount:      w.PaneCount,
			ActiveTabID:    w.ActiveTabID,
			WorkspaceTabs:  workspaceTabs(w.ID, tabs),
			WorkspacePanes: workspacePanes(w.ID, panes),
		})
	}
	return out, nil
}

func workspaceTabs(workspaceID string, tabs []herdr.Tab) []model.WorkspaceTab {
	var out []model.WorkspaceTab
	for _, tab := range tabs {
		if tab.WorkspaceID != workspaceID {
			continue
		}
		out = append(out, model.WorkspaceTab{ID: tab.ID, Number: tab.Number, Label: tab.Label})
	}
	return out
}

func workspacePanes(workspaceID string, panes []herdr.Pane) []model.WorkspacePane {
	var out []model.WorkspacePane
	for _, pane := range panes {
		if pane.WorkspaceID != workspaceID {
			continue
		}
		label := cmp.Or(pane.DisplayAgent, pane.Label, pane.Title, pane.Agent)
		out = append(out, model.WorkspacePane{ID: pane.ID, TabID: pane.TabID, Label: label, AgentStatus: pane.AgentStatus})
	}
	return out
}

func workspacePath(w herdr.Workspace, panes []herdr.Pane) string {
	if w.ForegroundCWD != "" {
		return w.ForegroundCWD
	}
	if w.CWD != "" {
		return w.CWD
	}
	var first string
	for _, p := range panes {
		if p.WorkspaceID != w.ID {
			continue
		}
		path := panePath(p)
		if path == "" {
			continue
		}
		if p.TabID == w.ActiveTabID || p.Focused {
			return path
		}
		if first == "" {
			first = path
		}
	}
	return first
}

func panePath(p herdr.Pane) string {
	if p.ForegroundCWD != "" {
		return p.ForegroundCWD
	}
	return p.CWD
}

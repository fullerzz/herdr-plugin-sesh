package sources

import (
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
	for _, w := range ws {
		path := workspacePath(w, panes)
		out.Add(model.Session{Source: "herdr", Name: w.Label, Path: path, WorkspaceID: w.ID})
	}
	return out, nil
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

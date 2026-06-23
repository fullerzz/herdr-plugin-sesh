package sources

import (
	"context"
	"forgejo.local/fullerzz/herdr-plugin-sesh/internal/herdr"
	"forgejo.local/fullerzz/herdr-plugin-sesh/internal/model"
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
	for _, w := range ws {
		path := w.CWD
		if path == "" {
			path = w.ForegroundCWD
		}
		out.Add(model.Session{Source: "herdr", Name: w.Label, Path: path, WorkspaceID: w.ID})
	}
	return out, nil
}

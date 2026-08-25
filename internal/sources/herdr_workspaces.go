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
	var panes []herdr.Pane
	for _, workspace := range ws {
		if workspace.ForegroundCWD == "" && workspace.CWD == "" {
			panes, err = s.Client.PaneList(ctx, "")
			if err != nil {
				panes = nil
			}
			break
		}
	}
	parentsByRepo := worktreeParentsByRepo(ws)
	for _, w := range ws {
		path := workspacePath(w, panes)
		out.Add(model.Session{
			Source:      "herdr",
			Name:        w.Label,
			Path:        path,
			WorkspaceID: w.ID,
			AgentStatus: w.AgentStatus,
			Worktree:    worktreeRelation(w, parentsByRepo),
		})
	}
	return out, nil
}

func worktreeParentsByRepo(workspaces []herdr.Workspace) map[string][]herdr.Workspace {
	parents := make(map[string][]herdr.Workspace)
	for _, workspace := range workspaces {
		worktree := workspace.Worktree
		if worktree == nil || worktree.IsLinkedWorktree || worktree.RepoKey == "" {
			continue
		}
		parents[worktree.RepoKey] = append(parents[worktree.RepoKey], workspace)
	}
	return parents
}

func worktreeRelation(workspace herdr.Workspace, parentsByRepo map[string][]herdr.Workspace) model.WorktreeRelation {
	worktree := workspace.Worktree
	if worktree == nil || !worktree.IsLinkedWorktree {
		return model.WorktreeRelation{}
	}
	relation := model.WorktreeRelation{Linked: true}
	parents := parentsByRepo[worktree.RepoKey]
	if len(parents) != 1 {
		return relation
	}
	parent := parents[0]
	relation.ParentWorkspaceID = parent.ID
	relation.ParentWorkspaceName = parent.Label
	if relation.ParentWorkspaceName == "" {
		relation.ParentWorkspaceName = parent.ID
	}
	return relation
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
	if first == "" && w.Worktree != nil && w.Worktree.IsLinkedWorktree {
		first = w.Worktree.CheckoutPath
	}
	return first
}

func panePath(p herdr.Pane) string {
	if p.ForegroundCWD != "" {
		return p.ForegroundCWD
	}
	return p.CWD
}

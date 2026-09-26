package herdr

import (
	"context"
	"fmt"
)

type FakeClient struct {
	Workspaces        []Workspace
	Tabs              []Tab
	Panes             []Pane
	CreatedWorkspaces []WorkspaceCreateRequest
	CreatedTabs       []TabCreateRequest
	FocusedWorkspaces []string
	FocusedTabs       []string
	RenamedTabs       []string
	PaneRuns          []string
	Splits            []PaneSplitRequest
	// SplitErr, when set, fails the split whose index equals SplitErrAt.
	SplitErr      error
	SplitErrAt    int
	OpenedPlugins []string
}

func (f *FakeClient) WorkspaceList(context.Context) ([]Workspace, error) { return f.Workspaces, nil }
func (f *FakeClient) WorkspaceCreate(_ context.Context, r WorkspaceCreateRequest) (Workspace, error) {
	f.CreatedWorkspaces = append(f.CreatedWorkspaces, r)
	w := Workspace{ID: "new-workspace", Label: r.Label, CWD: r.CWD, ActiveTabID: "initial-tab"}
	f.Workspaces = append(f.Workspaces, w)
	// Like Herdr, every new workspace starts with one tab and root pane.
	f.Panes = append(f.Panes, Pane{ID: "initial-pane", WorkspaceID: w.ID, TabID: w.ActiveTabID, CWD: r.CWD})
	return w, nil
}
func (f *FakeClient) WorkspaceFocus(_ context.Context, id string) error {
	f.FocusedWorkspaces = append(f.FocusedWorkspaces, id)
	return nil
}
func (f *FakeClient) TabList(context.Context, string) ([]Tab, error) { return f.Tabs, nil }
func (f *FakeClient) TabCreate(_ context.Context, r TabCreateRequest) (Tab, error) {
	f.CreatedTabs = append(f.CreatedTabs, r)
	t := Tab{ID: "new-tab", WorkspaceID: r.WorkspaceID, Label: r.Label, CWD: r.CWD, PaneID: "new-pane"}
	f.Tabs = append(f.Tabs, t)
	if r.Focus {
		for i := range f.Workspaces {
			if f.Workspaces[i].ID == r.WorkspaceID {
				f.Workspaces[i].ActiveTabID = t.ID
				f.Workspaces[i].ForegroundCWD = r.CWD
			}
		}
	}
	return t, nil
}
func (f *FakeClient) TabFocus(_ context.Context, id string) error {
	f.FocusedTabs = append(f.FocusedTabs, id)
	return nil
}
func (f *FakeClient) TabRename(_ context.Context, id, label string) error {
	f.RenamedTabs = append(f.RenamedTabs, id+":"+label)
	return nil
}
func (f *FakeClient) PaneList(context.Context, string) ([]Pane, error) { return f.Panes, nil }
func (f *FakeClient) PaneCurrent(context.Context) (Pane, error) {
	if len(f.Panes) > 0 {
		return f.Panes[0], nil
	}
	return Pane{ID: "pane", WorkspaceID: "ws", TabID: "tab"}, nil
}
func (f *FakeClient) PaneRun(_ context.Context, id, cmd string) error {
	f.PaneRuns = append(f.PaneRuns, id+":"+cmd)
	return nil
}
func (f *FakeClient) PaneSplit(_ context.Context, r PaneSplitRequest) (Pane, error) {
	if f.SplitErr != nil && len(f.Splits) == f.SplitErrAt {
		return Pane{}, f.SplitErr
	}
	f.Splits = append(f.Splits, r)
	return Pane{ID: fmt.Sprintf("split-%d", len(f.Splits))}, nil
}
func (f *FakeClient) PluginPaneOpen(_ context.Context, p, e, pl string) error {
	f.OpenedPlugins = append(f.OpenedPlugins, p+":"+e+":"+pl)
	return nil
}

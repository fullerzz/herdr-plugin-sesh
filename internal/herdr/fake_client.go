package herdr

import "context"

type FakeClient struct {
	Workspaces        []Workspace
	Tabs              []Tab
	Panes             []Pane
	CreatedWorkspaces []WorkspaceCreateRequest
	CreatedTabs       []TabCreateRequest
	FocusedWorkspaces []string
	FocusedTabs       []string
	PaneRuns          []string
	OpenedPlugins     []string
}

func (f *FakeClient) WorkspaceList(context.Context) ([]Workspace, error) { return f.Workspaces, nil }
func (f *FakeClient) WorkspaceCreate(_ context.Context, r WorkspaceCreateRequest) (Workspace, error) {
	f.CreatedWorkspaces = append(f.CreatedWorkspaces, r)
	w := Workspace{ID: "new-workspace", Label: r.Label, CWD: r.CWD}
	f.Workspaces = append(f.Workspaces, w)
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
	return t, nil
}
func (f *FakeClient) TabFocus(_ context.Context, id string) error {
	f.FocusedTabs = append(f.FocusedTabs, id)
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
func (f *FakeClient) PluginPaneOpen(_ context.Context, p, e, pl string) error {
	f.OpenedPlugins = append(f.OpenedPlugins, p+":"+e+":"+pl)
	return nil
}

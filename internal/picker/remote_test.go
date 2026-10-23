package picker

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoteRowsKeepIdentityAndStatus(t *testing.T) {
	remote := model.Session{Source: model.SourceRemote, Name: "same", AgentStatus: "working", Remote: &model.RemoteRef{Machine: "peer", WorkspaceID: "w1", Offline: true}}
	m := New([]model.Session{{Source: "herdr", Name: "same", WorkspaceID: "w1"}, remote})
	m.UpdateAgentStatuses(map[string]string{"w1": "idle"})
	assert.Equal(t, "working", m.All[1].AgentStatus)
	m.Filter("peer")
	require.Len(t, m.Filtered, 1)
	assert.Equal(t, remote, m.Filtered[0])
	m.Filter("offline")
	require.Len(t, m.Filtered, 1)
	assert.Contains(t, fzfInput([]model.Session{remote}, false), "same @ peer (offline)")
	command := fzfPreviewCommand()
	assert.Less(t, strings.Index(command, `"$source" = remote`), strings.Index(command, `[ ! -d "$item_path" ]`))
	tm := newTeaModel([]model.Session{remote}, Options{})
	_, cmd := tm.closeSelectedWorkspace()
	assert.Nil(t, cmd)
}

func TestRemoteOnlyFilterKeepsSearchAndRefreshes(t *testing.T) {
	peer := model.Session{Source: model.SourceRemote, Name: "api", Remote: &model.RemoteRef{Machine: "server", WorkspaceID: "w1"}}
	offline := model.Session{Source: model.SourceRemote, Name: "api-offline", Remote: &model.RemoteRef{Machine: "backup", WorkspaceID: "w2", Offline: true}}
	items := []model.Session{{Source: "herdr", Name: "api-local", WorkspaceID: "w1"}, {Source: "config", Name: "api-config"}, peer, offline}
	m := newTeaModel(items, Options{DisplayOptions: DisplayOptions{WorkspaceSort: "agent", HidePreview: true, HideBottomInfoRow: true}})
	m.width, m.height = 120, 24
	m.input.SetValue("api")
	m.list.Filter("api")
	m.list.Selected = 3
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyF3})
	m = updated.(teaModel)
	assert.True(t, m.list.RemoteOnly)
	assert.Equal(t, "api", m.input.Value())
	assert.Equal(t, []model.Session{peer, offline}, m.list.Filtered)
	assert.Zero(t, m.list.Selected)
	assert.Contains(t, ansi.Strip(m.View().Content), "REMOTE WORKSPACES · f3 all")
	assert.Contains(t, ansi.Strip(m.View().Content), "2/4 workspaces")
	updated, _ = m.Update(agentStatusesMsg{statuses: map[string]string{"w1": "idle"}})
	m = updated.(teaModel)
	assert.Equal(t, []model.Session{peer, offline}, m.list.Filtered)
	updated, _ = m.Update(settingsReloadedMsg{options: DisplayOptions{HidePreview: true}, result: ReloadResult{Sessions: items}})
	m = updated.(teaModel)
	assert.True(t, m.list.RemoteOnly)
	assert.Equal(t, []model.Session{peer, offline}, m.list.Filtered)
	m = m.filter("backup")
	require.Equal(t, []model.Session{offline}, m.list.Filtered)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Equal(t, offline, updated.(teaModel).choice)
	m = m.filter("api")
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyF3})
	m = updated.(teaModel)
	assert.False(t, m.list.RemoteOnly)
	assert.Len(t, m.list.Filtered, len(items))
}

func TestRemoteOnlyFilterEmptyAndPreviewShortcut(t *testing.T) {
	binding := "f3"
	m := newTeaModel([]model.Session{{Source: "herdr", Name: "local"}}, Options{DisplayOptions: DisplayOptions{CyclePreviewModeKey: &binding}})
	defer func() { m.cancelActivePreview() }()
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyF3})
	m = updated.(teaModel)
	assert.True(t, m.panePreview)
	assert.False(t, m.list.RemoteOnly)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'g', Mod: tea.ModCtrl})
	m = updated.(teaModel)
	assert.True(t, m.list.RemoteOnly)
	assert.Empty(t, m.list.Filtered)
	assert.Empty(t, m.previewKey)
	assert.Contains(t, ansi.Strip(m.View().Content), "REMOTE WORKSPACES · ctrl+g all")
	assert.Contains(t, ansi.Strip(m.View().Content), "No matching workspaces")
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.False(t, updated.(teaModel).chosen)
}

func TestRemoteOnlyFilterIgnoredWhileClosingWorkspace(t *testing.T) {
	remote := model.Session{Source: model.SourceRemote, Name: "peer", Remote: &model.RemoteRef{Machine: "server", WorkspaceID: "w2"}}
	m := newTeaModel([]model.Session{{Source: "herdr", Name: "api", WorkspaceID: "w1"}, remote}, Options{DisplayOptions: DisplayOptions{HidePreview: true}, Backend: &fakeBackend{closeWorkspace: func(context.Context, string) error { return nil }}})
	m, _ = m.closeSelectedWorkspace()
	t.Cleanup(func() {
		if m.cancelWorkspaceClose != nil {
			m.cancelWorkspaceClose()
		}
	})
	require.Equal(t, "w1", m.closingWorkspaceID)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyF3})
	m = updated.(teaModel)
	assert.Nil(t, cmd)
	assert.False(t, m.list.RemoteOnly)
	assert.Len(t, m.list.Filtered, 2)
}

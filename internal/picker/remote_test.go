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

func TestAsyncRemoteRowsPreserveChosenLocalIdentity(t *testing.T) {
	local := model.Session{Source: "config", Name: "local", Path: "/local"}
	peer := model.Session{Source: model.SourceRemote, Name: "peer", Remote: &model.RemoteRef{Machine: "server", WorkspaceID: "w1"}}
	release := make(chan struct{})
	started := make(chan struct{})
	m := newTeaModel([]model.Session{local}, Options{DisplayOptions: DisplayOptions{
		HidePreview: true,
		LoadRemoteRows: func(ctx context.Context) ([]model.Session, error) {
			close(started)
			select {
			case <-release:
				return []model.Session{peer}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
	}})
	result := make(chan tea.Msg, 1)
	command := m.remoteRowsCommand()
	go func() { result <- command() }()
	<-started
	current, ok := m.list.Current()
	require.True(t, ok)
	assert.Equal(t, local, current, "local rows remain usable while remote loading waits")
	select {
	case <-result:
		t.Fatal("remote request should still be waiting")
	default:
	}
	close(release)
	updated, _ := m.Update(<-result)
	m = updated.(teaModel)
	assert.Equal(t, []model.Session{peer, local}, m.list.Filtered)
	current, ok = m.list.Current()
	require.True(t, ok)
	assert.Equal(t, model.Key(local), model.Key(current))
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Equal(t, local, updated.(teaModel).choice)
}

func TestAsyncRemoteRowsRejectPreviousPickerState(t *testing.T) {
	local := model.Session{Source: "herdr", Name: "local", WorkspaceID: "local"}
	peer := model.Session{Source: model.SourceRemote, Name: "peer", Remote: &model.RemoteRef{Machine: "server", WorkspaceID: "w1"}}
	m := newTeaModel([]model.Session{local}, Options{DisplayOptions: DisplayOptions{HidePreview: true}, Backend: &fakeBackend{}})
	old := remoteRowsMsg{generation: m.remoteGeneration, sessions: []model.Session{peer}}
	m, _ = m.closeSelectedWorkspace()
	updated, _ := m.Update(old)
	m = updated.(teaModel)
	assert.Equal(t, []model.Session{local}, m.list.All)
	updated, _ = m.Update(workspaceCloseMsg{workspaceID: "local", reloadRan: true, result: ReloadResult{Sessions: []model.Session{}}})
	m = updated.(teaModel)
	updated, _ = m.Update(old)
	assert.Empty(t, updated.(teaModel).list.All, "closed state must not accept an earlier catalog")

	m = newTeaModel([]model.Session{local}, Options{DisplayOptions: DisplayOptions{HidePreview: true}, Backend: &fakeBackend{}})
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyF2})
	m = updated.(teaModel)
	updated, _ = m.Update(old)
	assert.Equal(t, []model.Session{local}, updated.(teaModel).list.All)
	updated, _ = m.Update(settingsReloadedMsg{options: DisplayOptions{HidePreview: true}, result: ReloadResult{Sessions: []model.Session{local}}})
	m = updated.(teaModel)
	updated, _ = m.Update(old)
	assert.Equal(t, []model.Session{local}, updated.(teaModel).list.All, "settings reload must invalidate old catalogs")
}

func TestAsyncRemoteRowsPopulateEmptyRemoteFilter(t *testing.T) {
	peer := model.Session{Source: model.SourceRemote, Name: "api", Remote: &model.RemoteRef{Machine: "server", WorkspaceID: "w1"}}
	m := newTeaModel(nil, Options{DisplayOptions: DisplayOptions{HidePreview: true}})
	m.list.RemoteOnly = true
	m.list.Filter("api")
	updated, _ := m.Update(remoteRowsMsg{sessions: []model.Session{peer}})
	assert.Equal(t, []model.Session{peer}, updated.(teaModel).list.Filtered)
}

func TestAsyncRemoteRowsHonorPickerContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	m := newTeaModel(nil, Options{DisplayOptions: DisplayOptions{Context: ctx, HidePreview: true,
		LoadRemoteRows: func(ctx context.Context) ([]model.Session, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}})
	command := m.remoteRowsCommand()
	result := make(chan tea.Msg, 1)
	go func() { result <- command() }()
	<-started
	cancel()
	msg := (<-result).(remoteRowsMsg)
	assert.ErrorIs(t, msg.err, context.Canceled)
}

func TestAsyncRemoteWarningsAreHandledOnReceipt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var warnings []error
	m := newTeaModel(nil, Options{DisplayOptions: DisplayOptions{Context: ctx, HidePreview: true,
		LoadRemoteRows:  func(ctx context.Context) ([]model.Session, error) { return nil, ctx.Err() },
		RemoteRowsError: func(err error) { warnings = append(warnings, err) },
	}})
	msg := m.remoteRowsCommand()()
	assert.Empty(t, warnings, "background jobs must not mutate backend warnings")
	_, _ = m.Update(msg)
	require.Len(t, warnings, 1)
	assert.ErrorIs(t, warnings[0], context.Canceled)
}

func TestSettingsReloadPreservesPendingRemoteSelection(t *testing.T) {
	locals := []model.Session{
		{Source: "herdr", Name: "match-first", WorkspaceID: "first"},
		{Source: "herdr", Name: "match-second", WorkspaceID: "second"},
	}
	peers := []model.Session{
		{Source: model.SourceRemote, Name: "match-peer-first", Remote: &model.RemoteRef{Machine: "server", WorkspaceID: "first"}},
		{Source: model.SourceRemote, Name: "match-peer-second", Remote: &model.RemoteRef{Machine: "server", WorkspaceID: "second"}},
	}
	reload := settingsReloadedMsg{options: DisplayOptions{HidePreview: true}, result: ReloadResult{Sessions: locals}}
	for _, tt := range []struct {
		name       string
		remoteOnly bool
		messages   []tea.Msg
		want       model.Session
	}{
		{name: "all rows", want: peers[1]},
		{name: "remote only", remoteOnly: true, want: peers[1]},
		{name: "another settings save", messages: []tea.Msg{tea.KeyPressMsg{Code: tea.KeyF2}, reload}, want: peers[1]},
		{name: "user selects local", messages: []tea.Msg{tea.KeyPressMsg{Code: tea.KeyUp}}, want: locals[0]},
		{name: "user changes query", messages: []tea.Msg{tea.PasteMsg{Content: "match"}}, want: locals[0]},
		{name: "user toggles remote filter", messages: []tea.Msg{tea.KeyPressMsg{Code: tea.KeyF3}}, want: peers[0]},
		{name: "user changes sort", messages: []tea.Msg{tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl}}, want: locals[0]},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := newTeaModel(append(append([]model.Session{}, locals...), peers...), Options{DisplayOptions: DisplayOptions{HidePreview: true}, Backend: &fakeBackend{}})
			m.list.RemoteOnly = tt.remoteOnly
			m.list.Filter("")
			m.list.Selected = len(m.list.Filtered) - 1
			m.listFocused = true
			m.input.Blur()
			updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyF2})
			m = updated.(teaModel)
			updated, _ = m.Update(reload)
			m = updated.(teaModel)
			updated, quit := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			require.Nil(t, quit, "Enter must not activate the temporary local fallback")
			m = updated.(teaModel)
			require.False(t, m.chosen)
			for _, msg := range tt.messages {
				updated, _ = m.Update(msg)
				m = updated.(teaModel)
			}
			updated, _ = m.Update(remoteRowsMsg{generation: m.remoteGeneration, sessions: peers})
			m = updated.(teaModel)
			selected, ok := m.list.Current()
			require.True(t, ok)
			assert.Equal(t, model.Key(tt.want), model.Key(selected))
			updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			assert.Equal(t, model.Key(tt.want), model.Key(updated.(teaModel).choice))
		})
	}
}

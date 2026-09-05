package picker

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
)

func TestPanePreviewToggleRefreshAndSelection(t *testing.T) {
	oldPane, oldPreview := renderPanePreview, renderPreview
	t.Cleanup(func() { renderPanePreview, renderPreview = oldPane, oldPreview })
	renderPanePreview = func(_ context.Context, s model.Session) (string, error) { return "pane " + s.WorkspaceID, nil }
	renderPreview = func(_ context.Context, _ model.Session, command string) (string, error) { return command, nil }
	m := newTeaModel([]model.Session{
		{Source: "herdr", Name: "one", WorkspaceID: "w1"},
		{Source: "herdr", Name: "two", WorkspaceID: "w2"},
	}, Options{DefaultPreviewCommand: "configured preview"})
	initialContext, initialID, initialKey := m.previewContext, m.previewRequestID, m.previewKey
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	m = updated.(teaModel)
	require.True(t, m.panePreview)
	require.Error(t, initialContext.Err())
	require.NotNil(t, cmd)
	updated, tick := m.Update(cmd())
	m = updated.(teaModel)
	require.Equal(t, "pane w1", m.preview)
	require.NotNil(t, tick)
	require.Contains(t, ansi.Strip(m.previewTitle()), "PANE [ctrl+o]")
	// A completed read schedules one refresh; refreshing preserves the visible text.
	refreshID := m.previewRequestID
	refreshTick := tick().(panePreviewTickMsg)
	require.Equal(t, refreshID, refreshTick.requestID)
	updated, cmd = m.Update(refreshTick)
	m = updated.(teaModel)
	require.NotNil(t, cmd)
	require.Equal(t, "pane w1", m.preview)
	staleResult := cmd()
	refreshContext := m.previewContext
	m.list.Move(1)
	m, cmd = m.refreshPreview()
	require.Error(t, refreshContext.Err())
	require.Equal(t, "Loading preview...", m.preview)
	updated, _ = m.Update(cmd())
	m = updated.(teaModel)
	for _, stale := range []tea.Msg{staleResult, previewMsg{key: initialKey, requestID: initialID, text: "stale command"}, panePreviewTickMsg{requestID: refreshID}} {
		updated, cmd = m.Update(stale)
		m = updated.(teaModel)
		require.Equal(t, "pane w2", m.preview)
		require.Nil(t, cmd)
	}
	paneID := m.previewRequestID
	updated, cmd = m.Update(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	m = updated.(teaModel)
	updated, _ = m.Update(cmd())
	m = updated.(teaModel)
	require.False(t, m.panePreview)
	require.Equal(t, "configured preview", m.preview)
	updated, cmd = m.Update(panePreviewTickMsg{requestID: paneID})
	m = updated.(teaModel)
	require.Nil(t, cmd)
	m = m.cancelActivePreview()
}

func TestPanePreviewHiddenAndEmptyPicker(t *testing.T) {
	for _, opts := range []Options{{HidePreview: true}, {}} {
		m := newTeaModel(nil, opts)
		updated, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
		m = updated.(teaModel)
		require.Nil(t, cmd)
		if opts.HidePreview {
			assert.False(t, m.panePreview)
		}
	}
}

func TestCtrlVPastesWithoutChangingPreviewMode(t *testing.T) {
	for _, tc := range []struct {
		name         string
		hidden, pane bool
	}{
		{name: "configured preview"},
		{name: "pane preview", pane: true},
		{name: "hidden preview", hidden: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newTeaModel([]model.Session{{Name: "api"}}, Options{HidePreview: tc.hidden})
			m.panePreview = tc.pane
			updated, cmd := m.Update(tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl})
			m = updated.(teaModel)
			require.NotNil(t, cmd)
			require.Equal(t, tc.pane, m.panePreview)
			m = m.cancelActivePreview()
		})
	}
}

func TestInitialPreviewMode(t *testing.T) {
	oldPane, oldPreview := renderPanePreview, renderPreview
	t.Cleanup(func() { renderPanePreview, renderPreview = oldPane, oldPreview })
	for _, tc := range []struct {
		name, mode, want string
		hidden           bool
	}{
		{name: "default", want: "command"},
		{name: "command", mode: "command", want: "command"},
		{name: "pane", mode: "pane", want: "pane"},
		{name: "hidden pane", mode: "pane", hidden: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var rendered string
			renderPanePreview = func(context.Context, model.Session) (string, error) {
				rendered += "pane"
				return rendered, nil
			}
			renderPreview = func(context.Context, model.Session, string) (string, error) {
				rendered += "command"
				return rendered, nil
			}
			m := newTeaModel([]model.Session{{Source: "herdr", WorkspaceID: "w1"}}, Options{PreviewMode: tc.mode, HidePreview: tc.hidden})
			executeTeaCommand(m.Init())
			require.Equal(t, tc.want, rendered)
			if !tc.hidden {
				rendered = ""
				updated, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
				m = updated.(teaModel)
				executeTeaCommand(cmd)
				require.NotEmpty(t, rendered)
				require.NotEqual(t, tc.want, rendered)
			}
			_ = m.cancelActivePreview()
		})
	}
}

func TestCyclePreviewModeKey(t *testing.T) {
	for _, binding := range []string{"alt+p", ""} {
		t.Run(binding, func(t *testing.T) {
			m := newTeaModel(nil, Options{CyclePreviewModeKey: &binding})
			updated, _ := m.Update(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
			m = updated.(teaModel)
			assert.False(t, m.panePreview)
			assert.NotContains(t, ansi.Strip(m.previewTitle()), "[ctrl+o]")
			updated, _ = m.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModAlt})
			m = updated.(teaModel)
			assert.Equal(t, binding != "", m.panePreview)
			if binding != "" {
				assert.Contains(t, ansi.Strip(m.previewTitle()), "["+binding+"]")
				updated, _ = m.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModAlt})
				assert.False(t, updated.(teaModel).panePreview)
			} else {
				assert.NotContains(t, ansi.Strip(m.previewTitle()), "[]")
			}
		})
	}
}

func TestCyclePreviewModeShiftedPrintableText(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{
		{Code: 'p', Mod: tea.ModShift, Text: "P"},
		{Code: 'P', Text: "P"},
		{Code: '/', Mod: tea.ModShift, Text: "?"},
	} {
		binding := key.Text
		m := newTeaModel(nil, Options{CyclePreviewModeKey: &binding})
		updated, _ := m.Update(key)
		assert.True(t, updated.(teaModel).panePreview, "binding %q", binding)
	}
}

func TestCyclePreviewModeModifiedShiftUsesBaseKey(t *testing.T) {
	for _, tc := range []struct {
		binding string
		key     tea.KeyPressMsg
	}{
		{"ctrl+shift+p", tea.KeyPressMsg{Code: 'p', ShiftedCode: 'P', Mod: tea.ModCtrl | tea.ModShift}},
		{"alt+shift+/", tea.KeyPressMsg{Code: '/', ShiftedCode: '?', Mod: tea.ModAlt | tea.ModShift}},
	} {
		m := newTeaModel(nil, Options{CyclePreviewModeKey: &tc.binding})
		updated, _ := m.Update(tc.key)
		assert.True(t, updated.(teaModel).panePreview, "binding %q", tc.binding)
	}
}

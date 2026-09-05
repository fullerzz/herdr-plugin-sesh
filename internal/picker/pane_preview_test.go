package picker

import (
	"context"
	"strings"
	"testing"

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
	if !m.panePreview || initialContext.Err() == nil || cmd == nil {
		t.Fatal("toggle must cancel the old preview and start pane mode")
	}
	updated, tick := m.Update(cmd())
	m = updated.(teaModel)
	if m.preview != "pane w1" || tick == nil || !strings.Contains(ansi.Strip(m.previewTitle()), "PANE [ctrl+o]") {
		t.Fatalf("preview=%q tick=%v title=%q", m.preview, tick, m.previewTitle())
	}
	// A completed read schedules one refresh; refreshing preserves the visible text.
	refreshID := m.previewRequestID
	updated, cmd = m.Update(panePreviewTickMsg{requestID: refreshID})
	m = updated.(teaModel)
	if cmd == nil || m.preview != "pane w1" {
		t.Fatal("refresh should retain pane contents")
	}
	staleResult := cmd()
	refreshContext := m.previewContext
	m.list.Move(1)
	m, cmd = m.refreshPreview()
	if refreshContext.Err() == nil {
		t.Fatal("selection change must cancel refresh")
	}
	updated, _ = m.Update(cmd())
	m = updated.(teaModel)
	for _, stale := range []tea.Msg{staleResult, previewMsg{key: initialKey, requestID: initialID, text: "stale command"}, panePreviewTickMsg{requestID: refreshID}} {
		updated, cmd = m.Update(stale)
		m = updated.(teaModel)
		if m.preview != "pane w2" || cmd != nil {
			t.Fatal("stale preview changed selection or scheduled work")
		}
	}
	paneID := m.previewRequestID
	updated, cmd = m.Update(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	m = updated.(teaModel)
	updated, _ = m.Update(cmd())
	m = updated.(teaModel)
	if m.panePreview || m.preview != "configured preview" {
		t.Fatalf("toggle back: %q", m.preview)
	}
	updated, cmd = m.Update(panePreviewTickMsg{requestID: paneID})
	m = updated.(teaModel)
	if cmd != nil {
		t.Fatal("pane timer survived mode switch")
	}
	m = m.cancelActivePreview()
}

func TestPanePreviewHiddenAndEmptyPicker(t *testing.T) {
	for _, opts := range []Options{{HidePreview: true}, {}} {
		m := newTeaModel(nil, opts)
		updated, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
		m = updated.(teaModel)
		if cmd != nil || opts.HidePreview && m.panePreview {
			t.Fatal("unexpected pane preview work")
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
			if cmd == nil || m.panePreview != tc.pane {
				t.Fatal("Ctrl+V must request clipboard paste without changing the preview")
			}
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
			if rendered != tc.want {
				t.Fatalf("initial renderer = %q, want %q", rendered, tc.want)
			}
			if !tc.hidden {
				rendered = ""
				updated, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
				m = updated.(teaModel)
				executeTeaCommand(cmd)
				if rendered == "" || rendered == tc.want {
					t.Fatalf("Ctrl+O did not change renderer: %q", rendered)
				}
			}
			_ = m.cancelActivePreview()
		})
	}
}

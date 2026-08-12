package picker

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"charm.land/bubbles/v2/cursor"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
)

func TestMain(m *testing.M) {
	if err := os.Setenv("HERDR_SESH_SMEAR_PRESET", "crisp"); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func TestTeaModelFiltersMovesAndChooses(t *testing.T) {
	m := newTeaModel([]model.Session{
		{Name: "api-service", Path: "/tmp/api"},
		{Name: "web", Path: "/tmp/web"},
	}, Options{SeparatorAware: true})
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "api service"})
	m = updated.(teaModel)
	cur, ok := m.list.Current()
	if !ok || cur.Name != "api-service" {
		t.Fatalf("current = %#v ok=%v", cur, ok)
	}
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(teaModel)
	if cmd == nil || !m.chosen || m.choice.Name != "api-service" {
		t.Fatalf("chosen=%v choice=%#v cmd=%v", m.chosen, m.choice, cmd)
	}
}

func TestTeaModelMovesSelection(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "api"}, {Name: "web"}}, Options{})
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(teaModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(teaModel)
	cur, ok := m.list.Current()
	if !ok || cur.Name != "web" {
		t.Fatalf("current = %#v ok=%v", cur, ok)
	}
}

func TestTeaModelCtrlJKMovesSelection(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "api"}, {Name: "web"}}, Options{})

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl})
	m = updated.(teaModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl})
	m = updated.(teaModel)
	if current, ok := m.list.Current(); !ok || current.Name != "web" {
		t.Fatalf("current = %#v ok=%v, want web", current, ok)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})
	m = updated.(teaModel)
	if current, ok := m.list.Current(); !ok || current.Name != "api" {
		t.Fatalf("current = %#v ok=%v, want api", current, ok)
	}
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "enter select · ctrl+j/k") || !strings.Contains(view, "LAST WORKSPACE · None recorded") {
		t.Fatalf("default-width view missing navigation help or last workspace:\n%s", view)
	}
}

func TestTeaModelCtrlKDeletesAfterFilterCursor(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "api-web"}}, Options{})
	m.input.SetValue("api-web")
	m.input.SetCursor(3)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})
	m = updated.(teaModel)

	if got := m.input.Value(); got != "api" {
		t.Fatalf("input=%q, want %q", got, "api")
	}
}

func TestTeaModelCtrlXClosesSelectedHerdrWorkspace(t *testing.T) {
	var closed string
	m := newTeaModel([]model.Session{
		{Source: "herdr", Name: "api-close", WorkspaceID: "w1"},
		{Source: "config", Name: "api-selected"},
		{Source: "herdr", Name: "api-other", WorkspaceID: "w2"},
	}, Options{CloseWorkspace: func(_ context.Context, id string) error {
		closed = id
		return nil
	}})
	m.list.Filter("api")
	m, _ = m.refreshPreview()

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	m = updated.(teaModel)
	if cmd == nil {
		t.Fatal("ctrl+x did not return a close command")
	}
	updated, enterCmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(teaModel)
	if enterCmd != nil || m.chosen {
		t.Fatalf("closing workspace remained selectable: cmd=%v chosen=%v", enterCmd, m.chosen)
	}
	m.list.Move(1)
	m, _ = m.refreshPreview()
	updated, _ = m.Update(cmd())
	m = updated.(teaModel)

	if closed != "w1" {
		t.Fatalf("closed workspace=%q, want w1", closed)
	}
	if got, want := sessionNames(m.list.All), []string{"api-selected", "api-other"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("remaining sessions=%v want %v", got, want)
	}
	if current, ok := m.list.Current(); !ok || current.Name != "api-selected" {
		t.Fatalf("current=%#v ok=%v, want api-selected", current, ok)
	}
	if current, _ := m.list.Current(); m.previewKey != model.Key(current) {
		t.Fatalf("preview key=%q, want selected session %q", m.previewKey, model.Key(current))
	}
}

func TestTeaModelCtrlXUpdatesLastWorkspace(t *testing.T) {
	m := newTeaModel([]model.Session{
		{Source: "herdr", Name: "previous", WorkspaceID: "w1"},
		{Source: "herdr", Name: "older", WorkspaceID: "w2"},
	}, Options{
		RecentWorkspaceIDs: []string{"current", "current", "w1", "w2"},
		LastWorkspaceID:    "w1",
		HerdrWorkspaces: []model.Session{
			{Source: "herdr", Name: "previous", WorkspaceID: "w1"},
			{Source: "herdr", Name: "older", WorkspaceID: "w2"},
		},
		CloseWorkspace: func(context.Context, string) error { return nil },
		ReloadPicker: func(context.Context) (ReloadResult, error) {
			return ReloadResult{
				Sessions:        []model.Session{{Source: "herdr", Name: "older", WorkspaceID: "w2"}},
				HerdrWorkspaces: []model.Session{{Source: "herdr", Name: "older", WorkspaceID: "w2"}},
				LastWorkspaceID: "w2",
			}, nil
		},
	})

	updated, closeCmd := m.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	m = updated.(teaModel)
	updated, _ = m.Update(closeCmd())
	m = updated.(teaModel)

	if m.lastWorkspaceID != "w2" {
		t.Fatalf("last workspace=%q, want w2", m.lastWorkspaceID)
	}
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "LAST WORKSPACE · older") {
		t.Fatalf("view missing updated last workspace:\n%s", view)
	}
}

func TestTeaModelDoesNotQuitWhileWorkspaceCloseIsPending(t *testing.T) {
	tests := []struct {
		name string
		key  tea.KeyPressMsg
		move bool
	}{
		{name: "enter another workspace", key: tea.KeyPressMsg{Code: tea.KeyEnter}, move: true},
		{name: "escape", key: tea.KeyPressMsg{Code: tea.KeyEscape}},
		{name: "control c", key: tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTeaModel([]model.Session{
				{Source: "herdr", Name: "api", WorkspaceID: "w1"},
				{Source: "herdr", Name: "web", WorkspaceID: "w2"},
			}, Options{CloseWorkspace: func(context.Context, string) error { return nil }})
			updated, _ := m.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
			m = updated.(teaModel)
			if tt.move {
				m.list.Move(1)
			}

			updated, cmd := m.Update(tt.key)
			m = updated.(teaModel)

			if cmd != nil || m.chosen || m.closingWorkspaceID != "w1" {
				t.Fatalf("pending close quit picker: cmd=%v chosen=%v closing=%q", cmd, m.chosen, m.closingWorkspaceID)
			}
		})
	}
}

func TestTeaModelCtrlCCancelsWorkspaceCloseAndQuitsAfterResult(t *testing.T) {
	tests := []struct {
		name string
		opts Options
	}{
		{name: "close", opts: Options{CloseWorkspace: func(ctx context.Context, _ string) error {
			<-ctx.Done()
			return ctx.Err()
		}}},
		{name: "reload", opts: Options{
			CloseWorkspace: func(context.Context, string) error { return nil },
			ReloadPicker: func(ctx context.Context) (ReloadResult, error) {
				<-ctx.Done()
				return ReloadResult{}, ctx.Err()
			},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTeaModel([]model.Session{
				{Source: "herdr", Name: "api", WorkspaceID: "w1"},
				{Source: "herdr", Name: "web", WorkspaceID: "w2"},
			}, tt.opts)
			updated, closeCmd := m.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
			m = updated.(teaModel)
			updated, _ = m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
			m = updated.(teaModel)

			updated, quitCmd := m.Update(closeCmd())
			_ = updated.(teaModel)
			if quitCmd == nil {
				t.Fatal("picker did not quit after cancelled close returned")
			}
			msg := quitCmd()
			if _, ok := msg.(tea.QuitMsg); !ok {
				t.Fatalf("command returned %T, want tea.QuitMsg", msg)
			}
		})
	}
}

func TestTeaModelCtrlXRestoresDeduplicatedSessionAfterClose(t *testing.T) {
	m := newTeaModel([]model.Session{
		{Source: "herdr", Name: "api", WorkspaceID: "w1"},
		{Source: "herdr", Name: "web", WorkspaceID: "w2"},
	}, Options{
		CloseWorkspace: func(context.Context, string) error { return nil },
		ReloadPicker: func(context.Context) (ReloadResult, error) {
			return ReloadResult{Sessions: []model.Session{
				{Source: "config", Name: "api", Path: "/configured/api"},
				{Source: "herdr", Name: "web", WorkspaceID: "w2"},
			}}, nil
		},
	})

	updated, closeCmd := m.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	m = updated.(teaModel)
	m.list.Move(1)
	updated, _ = m.Update(closeCmd())
	m = updated.(teaModel)

	if got, want := sessionNames(m.list.All), []string{"api", "web"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("sessions=%v want %v", got, want)
	}
	if current, ok := m.list.Current(); !ok || current.Name != "web" {
		t.Fatalf("current=%#v ok=%v, want web", current, ok)
	}
}

func TestTeaModelCtrlXRetainsActiveWorkspacesWhenReloadFails(t *testing.T) {
	m := newTeaModel([]model.Session{
		{Source: "herdr", Name: "api", WorkspaceID: "w1"},
		{Source: "herdr", Name: "web", WorkspaceID: "w2"},
	}, Options{
		CloseWorkspace: func(context.Context, string) error { return nil },
		ReloadPicker: func(context.Context) (ReloadResult, error) {
			return ReloadResult{LastWorkspaceUnknown: true}, errors.New("workspace list failed")
		},
	})

	updated, closeCmd := m.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	m = updated.(teaModel)
	updated, _ = m.Update(closeCmd())
	m = updated.(teaModel)

	if got, want := sessionNames(m.list.All), []string{"web"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("sessions=%v want %v", got, want)
	}
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "Workspace closed, but sessions could not be r…") {
		t.Fatalf("view missing reload failure:\n%s", view)
	} else if !strings.Contains(view, "LAST WORKSPACE · Unavailable") {
		t.Fatalf("view did not refresh last workspace after reload failure:\n%s", view)
	}
}

func TestTeaModelCtrlXRefreshesHerdrMetadataWhenSessionReloadFails(t *testing.T) {
	m := newTeaModel([]model.Session{
		{Source: "herdr", Name: "closing", WorkspaceID: "w1"},
		{Source: "herdr", Name: "old label", WorkspaceID: "w2"},
	}, Options{
		LastWorkspaceID: "w2",
		HerdrWorkspaces: []model.Session{{Source: "herdr", Name: "old label", WorkspaceID: "w2"}},
		CloseWorkspace:  func(context.Context, string) error { return nil },
		ReloadPicker: func(context.Context) (ReloadResult, error) {
			return ReloadResult{
				HerdrWorkspaces: []model.Session{{Source: "herdr", Name: "new label", WorkspaceID: "w2"}},
				LastWorkspaceID: "w2",
			}, errors.New("config refresh failed")
		},
	})

	updated, closeCmd := m.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	m = updated.(teaModel)
	updated, _ = m.Update(closeCmd())
	m = updated.(teaModel)

	if got, want := sessionNames(m.list.All), []string{"old label"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("sessions=%v want retained rows %v", got, want)
	}
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "LAST WORKSPACE · new label") {
		t.Fatalf("view kept stale Herdr metadata:\n%s", view)
	}
}

func TestTeaModelCtrlXKeepsWorkspaceWhenCloseFails(t *testing.T) {
	m := newTeaModel([]model.Session{
		{Source: "herdr", Name: "api", WorkspaceID: "w1"},
		{Source: "herdr", Name: "web", WorkspaceID: "w2"},
	}, Options{
		CloseWorkspace: func(context.Context, string) error { return errors.New("close failed") },
	})

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	m = updated.(teaModel)
	m.list.Move(1)
	m, _ = m.refreshPreview()
	stalePreview := previewMsg{key: m.previewKey, text: "stale preview"}
	updated, previewCmd := m.Update(cmd())
	m = updated.(teaModel)
	if m.preview == "Closing workspace..." {
		t.Fatalf("failed close left stale preview: cmd=%v", previewCmd)
	}
	updated, _ = m.Update(stalePreview)
	m = updated.(teaModel)

	if len(m.list.All) != 2 {
		t.Fatalf("workspace removed after close failure: %#v", m.list.All)
	}
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "close failed") {
		t.Fatalf("view missing close failure after selection and preview changed:\n%s", view)
	}
}

func TestTeaModelCtrlXRefreshesPreviewWhenCloseFails(t *testing.T) {
	m := newTeaModel([]model.Session{{Source: "herdr", Name: "api", WorkspaceID: "w1"}}, Options{
		CloseWorkspace: func(context.Context, string) error { return errors.New("close failed") },
	})

	updated, closeCmd := m.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	m = updated.(teaModel)
	updated, previewCmd := m.Update(closeCmd())
	m = updated.(teaModel)

	if previewCmd == nil || m.preview == "Closing workspace..." {
		t.Fatalf("failed close did not refresh preview: preview=%q cmd=%v", m.preview, previewCmd)
	}
}

func TestTeaModelCtrlXIgnoresNonHerdrSession(t *testing.T) {
	called := false
	m := newTeaModel([]model.Session{{Source: "config", Name: "api"}}, Options{
		CloseWorkspace: func(context.Context, string) error {
			called = true
			return nil
		},
	})

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	_ = updated.(teaModel)
	if cmd != nil || called {
		t.Fatalf("non-Herdr ctrl+x returned cmd=%v called=%v", cmd, called)
	}
}

func TestTeaModelDownTransfersCursorFromFilterToList(t *testing.T) {
	t.Setenv("HERDR_SESH_REDUCE_MOTION", "")
	m := newTeaModel([]model.Session{{Name: "workspace-api"}, {Name: "workspace-web"}}, Options{})
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'w', Text: "work"})
	m = updated.(teaModel)
	if view := ansi.Strip(m.listView(40, 2)); strings.Contains(view, "┃") {
		t.Fatalf("list cursor visible while filter is focused:\n%s", view)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(teaModel)
	if m.input.Focused() {
		t.Fatal("filter remained focused after moving into the list")
	}
	if m.list.Selected != 0 {
		t.Fatalf("selected row=%d, want first filtered row", m.list.Selected)
	}
	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	startColumn := visualColumn(lines[3], "┃")
	wantStartColumn := horizontalPadding + lipgloss.Width(defaultPrompt+"work")
	if startColumn != wantStartColumn {
		t.Fatalf("transfer cursor column=%d, want typed-text endpoint %d:\n%s", startColumn, wantStartColumn, strings.Join(lines, "\n"))
	}

	updated, _ = m.Update(smearTickMsg{})
	m = updated.(teaModel)
	lines = strings.Split(ansi.Strip(m.View().Content), "\n")
	nextColumn := visualColumn(lines[4], "┃")
	if nextColumn < horizontalPadding || nextColumn >= startColumn {
		t.Fatalf("transfer cursor did not move down-left: start=%d next=%d\n%s", startColumn, nextColumn, strings.Join(lines, "\n"))
	}

	for range 10 {
		updated, _ = m.Update(smearTickMsg{})
		m = updated.(teaModel)
	}
	view := ansi.Strip(m.View().Content)
	if strings.Count(view, "┃") != 1 || !strings.Contains(ansi.Strip(m.listView(40, 2)), "┃") {
		t.Fatalf("cursor did not settle as the single list rail:\n%s", view)
	}
}

func TestTeaModelUpTransfersCursorFromListToFilter(t *testing.T) {
	t.Setenv("HERDR_SESH_REDUCE_MOTION", "")
	m := newTeaModel([]model.Session{{Name: "workspace-api"}, {Name: "workspace-web"}}, Options{})
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'w', Text: "work"})
	m = updated.(teaModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(teaModel)
	for range 10 {
		updated, _ = m.Update(smearTickMsg{})
		m = updated.(teaModel)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m = updated.(teaModel)
	if m.input.Focused() {
		t.Fatal("filter refocused before the reverse smear completed")
	}
	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	startColumn := visualColumn(lines[listFirstRowIndex], "┃")
	if startColumn != horizontalPadding {
		t.Fatalf("reverse cursor column=%d, want list rail %d:\n%s", startColumn, horizontalPadding, strings.Join(lines, "\n"))
	}

	updated, _ = m.Update(smearTickMsg{})
	m = updated.(teaModel)
	lines = strings.Split(ansi.Strip(m.View().Content), "\n")
	nextColumn := visualColumn(lines[listFirstRowIndex-1], "┃")
	if nextColumn <= startColumn {
		t.Fatalf("reverse cursor did not move up-right: start=%d next=%d\n%s", startColumn, nextColumn, strings.Join(lines, "\n"))
	}

	for range 10 {
		updated, _ = m.Update(smearTickMsg{})
		m = updated.(teaModel)
	}
	if !m.input.Focused() || strings.Contains(ansi.Strip(m.listView(40, 2)), "┃") {
		t.Fatalf("cursor did not settle in the filter:\n%s", ansi.Strip(m.View().Content))
	}
}

func TestTeaModelRightTransfersCursorFromListToFilter(t *testing.T) {
	t.Setenv("HERDR_SESH_REDUCE_MOTION", "")
	m := newTeaModel([]model.Session{{Name: "workspace-api"}, {Name: "workspace-web"}}, Options{})
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'w', Text: "work"})
	m = updated.(teaModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(teaModel)
	for range 10 {
		updated, _ = m.Update(smearTickMsg{})
		m = updated.(teaModel)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	m = updated.(teaModel)
	if m.input.Focused() || !m.focusSmearActive || m.focusSmearDirection != -1 {
		t.Fatalf("right arrow skipped reverse smear: inputFocused=%v active=%v direction=%d", m.input.Focused(), m.focusSmearActive, m.focusSmearDirection)
	}

	updated, _ = m.Update(smearTickMsg{})
	m = updated.(teaModel)
	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	if column := visualColumn(lines[listFirstRowIndex-1], "┃"); column <= horizontalPadding {
		t.Fatalf("right-arrow cursor did not smear up-right: column=%d\n%s", column, strings.Join(lines, "\n"))
	}
}

func TestTeaModelTypingTransfersCursorFromListToFilter(t *testing.T) {
	t.Setenv("HERDR_SESH_REDUCE_MOTION", "")
	m := newTeaModel([]model.Session{{Name: "workspace-api"}, {Name: "workspace-web"}}, Options{})
	m.list.Selected = 1
	m.listFocused = true
	m.input.Blur()

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
	m = updated.(teaModel)
	if m.input.Value() != "w" || m.list.Query != "w" {
		t.Fatalf("typed key was not applied during transfer: input=%q query=%q", m.input.Value(), m.list.Query)
	}
	if m.input.Focused() || !m.focusSmearActive || m.focusSmearDirection != -1 {
		t.Fatalf("typing skipped reverse smear: inputFocused=%v active=%v direction=%d", m.input.Focused(), m.focusSmearActive, m.focusSmearDirection)
	}

	updated, _ = m.Update(smearTickMsg{})
	m = updated.(teaModel)
	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	if column := visualColumn(lines[listFirstRowIndex], "┃"); column <= horizontalPadding {
		t.Fatalf("typed cursor did not smear up-right: column=%d\n%s", column, strings.Join(lines, "\n"))
	}
}

func TestTeaModelPasteTransfersCursorFromListToFilter(t *testing.T) {
	t.Setenv("HERDR_SESH_REDUCE_MOTION", "")
	m := newTeaModel([]model.Session{{Name: "workspace-api"}, {Name: "workspace-web"}}, Options{})
	m.listFocused = true
	m.input.Blur()

	updated, _ := m.Update(tea.PasteMsg{Content: "workspace"})
	m = updated.(teaModel)
	if m.input.Value() != "workspace" || m.list.Query != "workspace" {
		t.Fatalf("pasted text was not applied during transfer: input=%q query=%q", m.input.Value(), m.list.Query)
	}
	if m.input.Focused() || !m.focusSmearActive || m.focusSmearDirection != -1 {
		t.Fatalf("paste skipped reverse smear: inputFocused=%v active=%v direction=%d", m.input.Focused(), m.focusSmearActive, m.focusSmearDirection)
	}
}

func TestTeaModelAcceleratesLongFocusTransfers(t *testing.T) {
	t.Setenv("HERDR_SESH_REDUCE_MOTION", "")
	items := make([]model.Session, 40)
	for i := range items {
		items[i] = model.Session{Name: "workspace"}
	}
	m := newTeaModel(items, Options{})
	m.width = 100
	m.height = 100
	m.list.Selected = 30
	m.listFocused = true
	m.input.Blur()

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	m = updated.(teaModel)
	distance := m.focusSmearSteps
	wantDistance := listFirstRowIndex + m.list.Selected - filterLineIndex
	if distance != wantDistance {
		t.Fatalf("transfer distance=%d, want selected-row distance %d", distance, wantDistance)
	}
	previousStep := m.focusSmearStep
	ticks := 0
	largestAdvance := 0
	for m.focusSmearActive {
		updated, _ = m.Update(smearTickMsg{})
		m = updated.(teaModel)
		ticks++
		largestAdvance = max(largestAdvance, previousStep-m.focusSmearStep)
		previousStep = m.focusSmearStep
	}

	if ticks >= distance || largestAdvance <= 1 {
		t.Fatalf("long transfer did not accelerate: distance=%d ticks=%d largestAdvance=%d", distance, ticks, largestAdvance)
	}
}

func TestTeaModelGooeyReverseTransferEasesOut(t *testing.T) {
	t.Setenv("HERDR_SESH_REDUCE_MOTION", "")
	t.Setenv("HERDR_SESH_SMEAR_PRESET", "gooey")
	m := newTeaModel([]model.Session{{Name: "workspace-api"}, {Name: "workspace-web"}}, Options{})
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'w', Text: "work"})
	m = updated.(teaModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(teaModel)
	for range 10 {
		updated, _ = m.Update(smearTickMsg{})
		m = updated.(teaModel)
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m = updated.(teaModel)

	columns := make([]int, 0, 3)
	for frame := range 3 {
		lines := strings.Split(ansi.Strip(m.View().Content), "\n")
		column := visualColumn(lines[listFirstRowIndex-frame], "█")
		if column < 0 {
			t.Fatalf("frame %d missing Gooey cursor:\n%s", frame, strings.Join(lines, "\n"))
		}
		columns = append(columns, column)
		if frame < 2 {
			updated, _ = m.Update(smearTickMsg{})
			m = updated.(teaModel)
		}
	}

	firstMove := columns[1] - columns[0]
	secondMove := columns[2] - columns[1]
	if firstMove <= secondMove {
		t.Fatalf("reverse Gooey movement accelerated into the input: columns=%v moves=%d,%d", columns, firstMove, secondMove)
	}
}

func TestTeaModelSmearPresets(t *testing.T) {
	tests := []struct {
		name          string
		head          string
		transferTrail []string
		rowTrail      string
		rowTrailCells int
	}{
		{name: "crisp", head: "┃", transferTrail: []string{"╱"}, rowTrail: "╷│╵", rowTrailCells: 2},
		{name: "gooey", head: "█", transferTrail: []string{"▓", "▒"}, rowTrail: "▓▒", rowTrailCells: 4},
		{name: "ghost", head: "◆", transferTrail: []string{"·"}, rowTrail: "·", rowTrailCells: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HERDR_SESH_SMEAR_PRESET", tt.name)
			items := make([]model.Session, 6)
			for i := range items {
				items[i] = model.Session{Name: "workspace"}
			}
			m := newTeaModel(items, Options{})
			updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
			m = updated.(teaModel)
			for range 2 {
				updated, _ = m.Update(smearTickMsg{})
				m = updated.(teaModel)
			}
			transfer := ansi.Strip(m.View().Content)
			for _, glyph := range append([]string{tt.head}, tt.transferTrail...) {
				if !strings.Contains(transfer, glyph) {
					t.Fatalf("%s transfer missing %q:\n%s", tt.name, glyph, transfer)
				}
			}

			for range 10 {
				updated, _ = m.Update(smearTickMsg{})
				m = updated.(teaModel)
			}
			if view := ansi.Strip(m.listView(40, 6)); !strings.Contains(view, tt.head) {
				t.Fatalf("%s settled cursor missing %q:\n%s", tt.name, tt.head, view)
			}
			for range len(items) - 1 {
				updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
				m = updated.(teaModel)
			}
			rowView := ansi.Strip(m.listView(40, 6))
			trailCells := 0
			for _, glyph := range rowView {
				if strings.ContainsRune(tt.rowTrail, glyph) {
					trailCells++
				}
			}
			if trailCells != tt.rowTrailCells {
				t.Fatalf("%s row trail cells=%d, want %d:\n%s", tt.name, trailCells, tt.rowTrailCells, rowView)
			}
		})
	}
}

func TestTeaModelSmearsRapidSelectionMoves(t *testing.T) {
	t.Setenv("HERDR_SESH_REDUCE_MOTION", "")
	m := newTeaModel([]model.Session{{Name: "api"}, {Name: "web"}, {Name: "docs"}}, Options{})
	m.listFocused = true
	m.input.Blur()
	for range 2 {
		updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		m = updated.(teaModel)
	}

	lines := strings.Split(strings.TrimSuffix(ansi.Strip(m.listView(40, 3)), "\n"), "\n")
	for i, want := range []string{"╷ ", "│ ", "┃ "} {
		if !strings.HasPrefix(lines[i], want) {
			t.Fatalf("row %d = %q, want rail %q\n%s", i, lines[i], want, strings.Join(lines, "\n"))
		}
	}
}

func TestTeaModelSmearRetracts(t *testing.T) {
	t.Setenv("HERDR_SESH_REDUCE_MOTION", "")
	oldPreview := renderPreview
	renderPreview = func(context.Context, model.Session, string) (string, error) { return "preview", nil }
	t.Cleanup(func() { renderPreview = oldPreview })

	m := newTeaModel([]model.Session{{Name: "api"}, {Name: "web"}}, Options{})
	m.listFocused = true
	m.input.Blur()
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(teaModel)
	if !strings.HasPrefix(ansi.Strip(m.listView(40, 2)), "╷ ") {
		t.Fatalf("moving selection did not start the smear:\n%s", ansi.Strip(m.listView(40, 2)))
	}

	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("move command = %T, want preview and animation batch", msg)
	}
	tickHandled := false
	for _, child := range batch {
		msg := child()
		if _, ok := msg.(previewMsg); ok {
			continue
		}
		updated, _ = m.Update(msg)
		m = updated.(teaModel)
		tickHandled = true
	}
	if !tickHandled {
		t.Fatal("move batch did not contain an animation tick")
	}
	if view := ansi.Strip(m.listView(40, 2)); strings.HasPrefix(view, "╷ ") {
		t.Fatalf("smear remained after settling:\n%s", view)
	}
}

func TestTeaModelCapsSmearSettleTime(t *testing.T) {
	t.Setenv("HERDR_SESH_REDUCE_MOTION", "")
	items := make([]model.Session, 100)
	for i := range items {
		items[i] = model.Session{Name: "workspace"}
	}
	m := newTeaModel(items, Options{})
	m.listFocused = true
	m.input.Blur()
	for range len(items) - 1 {
		updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		m = updated.(teaModel)
	}

	for ticks := 1; m.smearActive; ticks++ {
		if ticks > 3 {
			t.Fatalf("smear still active after %d settle ticks", ticks-1)
		}
		updated, _ := m.Update(smearTickMsg{})
		m = updated.(teaModel)
	}
}

func TestTeaModelQueryChangeClearsSmear(t *testing.T) {
	t.Setenv("HERDR_SESH_REDUCE_MOTION", "")
	m := newTeaModel([]model.Session{
		{Name: "workspace-0"},
		{Name: "workspace-1"},
		{Name: "workspace-2"},
		{Name: "workspace-3"},
	}, Options{})
	m.list.Selected = 2
	m.listFocused = true
	m.input.Blur()
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(teaModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'w', Text: "workspace"})
	m = updated.(teaModel)

	view := ansi.Strip(m.listView(40, 4))
	if strings.Contains(view, "╵ ") || strings.Contains(view, "│ ") {
		t.Fatalf("query change left a smear on reordered rows:\n%s", view)
	}
}

func TestTeaModelReducedMotionSkipsSmear(t *testing.T) {
	t.Setenv("HERDR_SESH_REDUCE_MOTION", "1")
	m := newTeaModel([]model.Session{{Name: "api"}, {Name: "web"}}, Options{})
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(teaModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(teaModel)

	current, ok := m.list.Current()
	if !ok || current.Name != "web" {
		t.Fatalf("current = %#v ok=%v", current, ok)
	}
	view := ansi.Strip(m.listView(40, 2))
	if strings.Contains(view, "╷ ") || strings.Contains(view, "│ ") {
		t.Fatalf("reduced motion rendered a smear:\n%s", view)
	}
}

func TestTeaModelForwardsTextInputNonKeyMessages(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "api"}}, Options{})
	updated, cmd := m.Update(cursor.Blink())
	m = updated.(teaModel)
	if cmd == nil {
		t.Fatal("expected textinput to handle non-key cursor message")
	}
	if m.list.Query != m.input.Value() {
		t.Fatalf("query=%q input=%q", m.list.Query, m.input.Value())
	}
}

func TestTeaModelViewRendersStyledShell(t *testing.T) {
	oldPreview := renderPreview
	renderPreview = func(context.Context, model.Session, string) (string, error) {
		return "preview content", nil
	}
	t.Cleanup(func() { renderPreview = oldPreview })

	m := newTeaModel([]model.Session{
		{Source: "herdr", Name: "workspace-api", Path: "/tmp/workspace-api", WorkspaceID: "ws-api", AgentStatus: "working"},
		{Source: "zoxide", Name: "tools", Path: "/tmp/tools"},
		{Source: "config", Name: "api", Path: "/tmp/api"},
	}, Options{
		Prompt:          "Find> ",
		Placeholder:     "Search sessions",
		ShowIcons:       true,
		LastWorkspaceID: "ws-api",
		HerdrWorkspaces: []model.Session{{Source: "herdr", Name: "workspace-api", Path: "/tmp/workspace-api", WorkspaceID: "ws-api"}},
	})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
	m = updated.(teaModel)
	updated, _ = m.Update(previewCommand(m.previewKey, m.list.Filtered[m.list.Selected], m.defaultPreviewCommand)())
	m = updated.(teaModel)
	view := ansi.Strip(m.View().Content)
	for _, want := range []string{"herdr / sesh", "3 workspaces", "Find> ", "Search sessions", "LAST WORKSPACE · workspace-api  /tmp/workspace-api", "WORKSPACES", "PREVIEW · workspace-api · working", herdrSourceIcon + " herdr", zoxideSourceIcon + " zoxide", configSourceIcon + " config", "api", "preview content", "enter select"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "+-") || strings.Contains(view, "| ") {
		t.Fatalf("view still contains ASCII box chrome:\n%s", view)
	}
	if got, want := maxLineWidth(view), 160; got != want {
		t.Fatalf("view width=%d, want %d:\n%s", got, want, view)
	}
}

func TestTeaModelLastWorkspaceFallsBackToRecordedID(t *testing.T) {
	m := newTeaModel([]model.Session{{Source: "herdr", Name: "current", WorkspaceID: "current"}}, Options{LastWorkspaceID: "unavailable-workspace"})

	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "LAST WORKSPACE · unavailable-workspace") {
		t.Fatalf("view missing recorded workspace ID:\n%s", view)
	}
}

func TestTeaModelLastWorkspaceUsesRawHerdrMetadata(t *testing.T) {
	m := newTeaModel([]model.Session{{Source: "config", Name: "api", Path: "/configured/api"}}, Options{
		LastWorkspaceID: "ws-api",
		HerdrWorkspaces: []model.Session{{Source: "herdr", Name: "api", Path: "/live/api", WorkspaceID: "ws-api"}},
	})

	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "LAST WORKSPACE · api  /live/api") {
		t.Fatalf("view did not use raw Herdr workspace metadata:\n%s", view)
	}
}

func TestTeaModelLastWorkspaceSharesFooterWithKeybinds(t *testing.T) {
	m := newTeaModel(nil, Options{LastWorkspaceID: "ws-api"})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	m = updated.(teaModel)

	view := ansi.Strip(m.View().Content)
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "LAST WORKSPACE") {
			if !strings.Contains(line, "enter select") {
				t.Fatalf("last workspace is not on keybind row: %q", line)
			}
			if want := "LAST WORKSPACE · ws-api"; !strings.HasSuffix(strings.TrimSpace(line), want) {
				t.Fatalf("last workspace is not right-aligned: %q", line)
			}
			return
		}
	}
	t.Fatalf("view missing last workspace footer:\n%s", view)
}

func TestTeaModelLastWorkspaceUnavailable(t *testing.T) {
	m := newTeaModel(nil, Options{LastWorkspaceUnknown: true})

	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "LAST WORKSPACE · Unavailable") {
		t.Fatalf("view missing unavailable state:\n%s", view)
	}
}

func TestTeaModelUnnamedLastWorkspaceDoesNotRepeatCompactedPath(t *testing.T) {
	t.Setenv("HOME", "/home/test")
	m := newTeaModel(nil, Options{
		LastWorkspaceID: "ws-api",
		HerdrWorkspaces: []model.Session{{Source: "herdr", Path: "/home/test/api", WorkspaceID: "ws-api"}},
	})

	view := ansi.Strip(m.View().Content)
	if strings.Count(view, "~/api") != 1 {
		t.Fatalf("view repeated compacted workspace path:\n%s", view)
	}
}

func TestTeaModelShowIconsControlsSourceIcons(t *testing.T) {
	items := []model.Session{{Source: "herdr", Name: "api"}}
	withoutIcons := ansi.Strip(newTeaModel(items, Options{}).View().Content)
	if strings.Contains(withoutIcons, herdrSourceIcon) {
		t.Fatalf("view unexpectedly contains source icon:\n%s", withoutIcons)
	}

	withIcons := ansi.Strip(newTeaModel(items, Options{ShowIcons: true}).View().Content)
	if !strings.Contains(withIcons, herdrSourceIcon+" herdr") {
		t.Fatalf("view missing source icon:\n%s", withIcons)
	}
}

func TestRowUsesSourceCategoryColors(t *testing.T) {
	tests := []struct {
		source string
		color  string
	}{
		{source: "herdr", color: "38;2;125;207;255"},
		{source: "config", color: "38;2;224;175;104"},
		{source: "zoxide", color: "38;2;158;206;106"},
		{source: "dir", color: "38;2;187;154;247"},
	}
	for _, tt := range tests {
		got := row(model.Session{Source: tt.source, Name: tt.source}, false, 80, true, "")
		if !strings.Contains(got, tt.color) {
			t.Fatalf("row for source %q missing color %s:\n%q", tt.source, tt.color, got)
		}
	}
}

func TestRowUsesAgentStatusIndicators(t *testing.T) {
	tests := []struct {
		status string
		glyph  string
		color  string
	}{
		{status: "working", glyph: "⢄", color: "38;2;224;175;104"},
		{status: "blocked", glyph: "◉", color: "38;2;247;118;142"},
		{status: "idle", glyph: "✓", color: "38;2;158;206;106"},
		{status: "done", glyph: "●", color: "38;2;125;207;255"},
	}
	for _, tt := range tests {
		got := row(model.Session{Source: "herdr", Name: "api", AgentStatus: tt.status}, false, 80, true, "")
		if !strings.Contains(ansi.Strip(got), tt.glyph) || !strings.Contains(got, tt.color) {
			t.Fatalf("row for status %q missing glyph/color:\n%q", tt.status, got)
		}
	}
	for _, status := range []string{"", "unknown", "future"} {
		got := ansi.Strip(row(model.Session{Source: "herdr", Name: "api", AgentStatus: status}, false, 80, true, ""))
		if strings.ContainsAny(got, "⢄◉✓●") {
			t.Fatalf("row for status %q unexpectedly contains indicator: %q", status, got)
		}
	}
}

func TestTeaModelAnimatesWorkingAgentStatusIndicator(t *testing.T) {
	m := newTeaModel([]model.Session{{Source: "herdr", Name: "api", AgentStatus: "working"}}, Options{})
	before := ansi.Strip(m.listView(80, 1))

	updated, cmd := m.Update(spinner.TickMsg{})
	m = updated.(teaModel)
	after := ansi.Strip(m.listView(80, 1))

	if !strings.Contains(before, "⢄") || !strings.Contains(after, "⢂") {
		t.Fatalf("working indicator did not advance jump frame:\nbefore: %q\nafter:  %q", before, after)
	}
	if cmd == nil {
		t.Fatal("working indicator did not schedule its next jump frame")
	}
}

func TestTeaModelStartsAgentStatusSpinner(t *testing.T) {
	m := newTeaModel(nil, Options{RefreshAgentStatuses: func() (map[string]string, error) { return nil, nil }})
	batch, ok := m.Init()().(tea.BatchMsg)
	if !ok || len(batch) == 0 {
		t.Fatalf("init command = %#v, want batch", batch)
	}
	if msg := batch[len(batch)-1](); reflect.TypeOf(msg) != reflect.TypeOf(spinner.TickMsg{}) {
		t.Fatalf("last init message = %T, want spinner.TickMsg", msg)
	}
}

func TestRowCompactsHomeAndNeverWraps(t *testing.T) {
	t.Setenv("HOME", "/Users/picker")
	s := model.Session{
		Source:      "herdr",
		Name:        "workspace-with-a-name-that-is-longer-than-the-column",
		Path:        "/Users/picker/Code/Go/workspace-with-a-path-that-is-longer-than-the-row",
		AgentStatus: "working",
	}
	wide := ansi.Strip(strings.TrimSuffix(row(s, true, 76, true, ""), "\n"))
	if strings.Contains(wide, "\n") || lipgloss.Width(wide) != 76 {
		t.Fatalf("wide row width=%d or wrapped:\n%q", lipgloss.Width(wide), wide)
	}
	if !strings.Contains(wide, "~/Code/Go/") || strings.Contains(wide, "/Users/picker") {
		t.Fatalf("wide row did not compact home path: %q", wide)
	}
	narrow := ansi.Strip(strings.TrimSuffix(row(s, false, 48, true, ""), "\n"))
	if strings.Contains(narrow, "~/") || strings.Contains(narrow, "/Users/picker") {
		t.Fatalf("narrow row should omit its path: %q", narrow)
	}
	if strings.Contains(narrow, "\n") || lipgloss.Width(narrow) != 48 {
		t.Fatalf("narrow row width=%d or wrapped: %q", lipgloss.Width(narrow), narrow)
	}
}

func TestTeaModelPreviewUsesConfiguredCommand(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "api", Path: "/tmp/api"}}, Options{DefaultPreviewCommand: "printf preview:%s {}"})
	msg := previewCommand(m.previewKey, m.list.Filtered[m.list.Selected], m.defaultPreviewCommand)()
	preview := msg.(previewMsg)
	if got := strings.TrimSpace(preview.text); got != "preview:/tmp/api" {
		t.Fatalf("preview=%q", preview.text)
	}
}

func TestTeaModelRefreshesPreviewWhenSelectionChanges(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "api", Path: "/tmp/api"}, {Name: "web", Path: "/tmp/web"}}, Options{})
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(teaModel)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(teaModel)
	if cmd == nil || !strings.Contains(m.preview, "Loading preview") {
		t.Fatalf("cmd=%v preview=%q", cmd, m.preview)
	}
	current, ok := m.list.Current()
	if !ok || current.Name != "web" || m.previewKey != model.Key(current) {
		t.Fatalf("current=%#v ok=%v previewKey=%q", current, ok, m.previewKey)
	}
}

func TestTeaModelRefreshesAgentStatuses(t *testing.T) {
	m := newTeaModel([]model.Session{
		{Source: "herdr", Name: "api", WorkspaceID: "w1", AgentStatus: "working"},
		{Source: "config", Name: "local", Path: "/tmp/local"},
	}, Options{RefreshAgentStatuses: func() (map[string]string, error) {
		return map[string]string{"w1": "blocked"}, nil
	}})
	m.list.Filter("api")

	updated, cmd := m.Update(statusRefreshTickMsg{})
	m = updated.(teaModel)
	if cmd == nil {
		t.Fatal("status refresh tick did not fetch statuses")
	}

	updated, next := m.Update(cmd())
	m = updated.(teaModel)
	current, ok := m.list.Current()
	if !ok || current.AgentStatus != "blocked" {
		t.Fatalf("current=%#v ok=%v", current, ok)
	}
	if m.list.Query != "api" || len(m.list.Filtered) != 1 {
		t.Fatalf("query=%q filtered=%#v", m.list.Query, m.list.Filtered)
	}
	if next == nil {
		t.Fatal("status refresh did not schedule the next tick")
	}
}

func TestPreviewViewUsesConstantHeight(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "api"}}, Options{})
	m.preview = "one line"
	short := m.previewView(40, 4)
	m.preview = strings.Repeat("wrapped preview content ", 20)
	long := m.previewView(40, 4)
	if lipgloss.Height(short) != lipgloss.Height(long) {
		t.Fatalf("preview heights changed: short=%d long=%d\nshort:\n%s\nlong:\n%s", lipgloss.Height(short), lipgloss.Height(long), short, long)
	}
	if got, want := lipgloss.Height(short), 4+previewTitleRows; got != want {
		t.Fatalf("preview height=%d, want %d\n%s", got, want, short)
	}
	if !strings.Contains(long, "...") {
		t.Fatalf("long preview missing truncation marker:\n%s", long)
	}
	if strings.Contains(ansi.Strip(long), "+-") {
		t.Fatalf("preview still contains box chrome:\n%s", long)
	}
}

func TestTeaModelUsesAvailableWindowHeight(t *testing.T) {
	items := make([]model.Session, 30)
	for i := range items {
		items[i] = model.Session{Name: "workspace"}
	}
	m := newTeaModel(items, Options{})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(teaModel)
	if got := m.previewBodyLines(); got <= defaultVisibleRows {
		t.Fatalf("preview body lines=%d, want more than fallback %d", got, defaultVisibleRows)
	}
	view := ansi.Strip(m.View().Content)
	if got, want := lipgloss.Height(view), 40; got != want {
		t.Fatalf("view height=%d, want %d", got, want)
	}
	lines := strings.Split(view, "\n")
	if lines[0] != "" {
		t.Fatalf("expected top padding row, got %q\n%s", lines[0], view)
	}
	if header := lines[1]; !strings.Contains(header, "herdr / sesh") {
		t.Fatalf("expected navigator header after padding, got %q\n%s", header, view)
	}
	if got, want := maxLineWidth(view), 120; got != want {
		t.Fatalf("view width=%d, want %d:\n%s", got, want, view)
	}
	if last := lines[len(lines)-1]; strings.TrimSpace(last) != "" {
		t.Fatalf("expected bottom breathing room, got %q\n%s", last, view)
	}
}

func TestSelectedRowUsesRailAndPreservesSourceColor(t *testing.T) {
	got := row(model.Session{Source: "herdr", Name: "herdr-plugin-sesh", Path: "/tmp/herdr-plugin-sesh", AgentStatus: "working"}, true, 80, true, "")
	plain := ansi.Strip(got)
	if !strings.Contains(plain, "┃") {
		t.Fatalf("selected row missing navigation rail:\n%q", got)
	}
	for _, want := range []string{"38;2;125;207;255", "38;2;224;175;104", herdrSourceIcon + " herdr", "herdr-plugin-sesh", "/tmp/herdr-plugin-sesh"} {
		if !strings.Contains(got, want) {
			t.Fatalf("selected row missing %q:\n%q", want, got)
		}
	}
	if strings.Contains(got, "48;2;") || strings.Contains(got, "48;5;") {
		t.Fatalf("selected row should not use a background fill:\n%q", got)
	}
}

func TestListViewHighlightsCaseInsensitiveQueryMatches(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "workspace-API", Path: "/tmp/workspace-API"}}, Options{})
	m.list.Filter("api")

	got := m.listView(80, 1)
	want := lipgloss.NewStyle().Foreground(violetColor).Bold(true).Render("API")
	if matches := strings.Count(got, want); matches != 2 {
		t.Fatalf("highlighted matches=%d, want 2:\n%q", matches, got)
	}
}

func TestListViewPreservesUnicodeWhenHighlightingFoldedMatch(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "workspace-Ⱥ", Path: "/tmp/workspace-Ⱥ"}}, Options{})
	m.list.Filter("ⱥ")

	got := m.listView(80, 1)
	want := matchStyle.Render("Ⱥ")
	if !utf8.ValidString(got) {
		t.Fatalf("highlighted row is invalid UTF-8: %q", got)
	}
	if matches := strings.Count(got, want); matches != 2 {
		t.Fatalf("highlighted matches=%d, want 2:\n%q", matches, got)
	}
}

func TestTeaModelStacksPreviewAtNarrowWidth(t *testing.T) {
	m := newTeaModel([]model.Session{{Source: "herdr", Name: "api", Path: "/tmp/api", AgentStatus: "blocked"}}, Options{})
	m.preview = "preview content"
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 70, Height: 28})
	m = updated.(teaModel)
	view := ansi.Strip(m.View().Content)
	if got, want := lipgloss.Height(view), 28; got != want {
		t.Fatalf("view height=%d, want %d:\n%s", got, want, view)
	}
	if !strings.Contains(view, "WORKSPACES") || !strings.Contains(view, "PREVIEW · api · blocked") {
		t.Fatalf("narrow view missing stacked sections:\n%s", view)
	}
	if strings.Contains(view, "│") {
		t.Fatalf("narrow view should not contain a vertical pane divider:\n%s", view)
	}
}

func TestTeaModelSplitsPreviewAtTerminalThreshold(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "api"}}, Options{})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: previewSplitWidth, Height: 28})
	m = updated.(teaModel)
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "│") {
		t.Fatalf("preview should split at width %d:\n%s", previewSplitWidth, view)
	}
}

func TestTeaModelHeaderShowsFilteredCountWhenAllRowsMatch(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "workspace-api"}, {Name: "workspace-web"}}, Options{})
	m.list.Filter("workspace")
	if got := ansi.Strip(m.header(80)); !strings.Contains(got, "2/2 workspaces") {
		t.Fatalf("filtered header=%q, want total-aware count", got)
	}
}

func TestTeaModelCyclesHerdrWorkspaceSortModes(t *testing.T) {
	items := []model.Session{
		{Source: "config", Name: "configured"},
		{Source: "herdr", Name: "first", WorkspaceID: "w1"},
		{Source: "zoxide", Name: "recent-directory"},
		{Source: "herdr", Name: "second", WorkspaceID: "w2"},
		{Source: "herdr", Name: "third", WorkspaceID: "w3"},
	}
	m := newTeaModel(items, Options{RecentWorkspaceIDs: []string{"w3", "w1"}})

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	m = updated.(teaModel)
	if got, want := sessionNames(m.list.All), []string{"configured", "third", "recent-directory", "first", "second"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("recent order=%v want %v", got, want)
	}
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "ctrl+r recent") {
		t.Fatalf("view missing recent sort mode:\n%s", view)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	m = updated.(teaModel)
	if got, want := sessionNames(m.list.All), []string{"configured", "first", "recent-directory", "second", "third"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("workspace order=%v want %v", got, want)
	}
}

func TestTeaModelStartsWithConfiguredWorkspaceSort(t *testing.T) {
	m := newTeaModel([]model.Session{
		{Source: "herdr", Name: "first", WorkspaceID: "w1"},
		{Source: "herdr", Name: "second", WorkspaceID: "w2"},
	}, Options{RecentWorkspaceIDs: []string{"w2", "w1"}, RecentWorkspaceSort: true})

	if got, want := sessionNames(m.list.All), []string{"second", "first"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("initial order=%v want %v", got, want)
	}
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "ctrl+r recent") {
		t.Fatalf("view missing recent sort mode:\n%s", view)
	}
}

func TestTeaModelSearchRailDoesNotTruncateAtWindowEdge(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "api"}}, Options{})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 28})
	m = updated.(teaModel)
	for _, line := range strings.Split(ansi.Strip(m.View().Content), "\n") {
		if strings.Contains(line, defaultPrompt) && strings.Contains(line, "…") {
			t.Fatalf("search rail was truncated at the window edge: %q", line)
		}
	}
}

func TestListViewUsesDirectionalOverflowMarkers(t *testing.T) {
	items := make([]model.Session, 20)
	for i := range items {
		items[i] = model.Session{Name: "workspace"}
	}
	m := newTeaModel(items, Options{})
	m.list.Selected = 10
	view := ansi.Strip(m.listView(60, 6))
	if !strings.Contains(view, "↑ 7 more") || !strings.Contains(view, "↓ 9 more") || strings.Contains(view, "...") {
		t.Fatalf("list view missing directional overflow markers:\n%s", view)
	}
}

func TestListViewKeepsSelectionVisibleWithTwoRows(t *testing.T) {
	items := []model.Session{{Name: "workspace-0"}, {Name: "workspace-1"}, {Name: "workspace-2"}, {Name: "workspace-3"}, {Name: "workspace-4"}}
	m := newTeaModel(items, Options{})
	m.list.Selected = 2
	if view := ansi.Strip(m.listView(60, 2)); !strings.Contains(view, "workspace-2") {
		t.Fatalf("two-row list hid the selected workspace:\n%s", view)
	}
}

func maxLineWidth(s string) int {
	maxWidth := 0
	for _, line := range strings.Split(s, "\n") {
		maxWidth = max(maxWidth, lipgloss.Width(line))
	}
	return maxWidth
}

func visualColumn(line, marker string) int {
	index := strings.Index(line, marker)
	if index < 0 {
		return -1
	}
	return lipgloss.Width(line[:index])
}

func sessionNames(items []model.Session) []string {
	names := make([]string, len(items))
	for i := range items {
		names[i] = items[i].Name
	}
	return names
}

package picker

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"charm.land/bubbles/v2/cursor"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
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
		{Source: "herdr", Name: "workspace-api", Path: "/tmp/workspace-api", AgentStatus: "working"},
		{Source: "zoxide", Name: "tools", Path: "/tmp/tools"},
		{Source: "config", Name: "api", Path: "/tmp/api"},
	}, Options{
		Prompt:      "Find> ",
		Placeholder: "Search sessions",
		ShowIcons:   true,
	})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
	m = updated.(teaModel)
	updated, _ = m.Update(previewCommand(m.previewKey, m.list.Filtered[m.list.Selected], m.defaultPreviewCommand)())
	m = updated.(teaModel)
	view := ansi.Strip(m.View().Content)
	for _, want := range []string{"herdr / sesh", "3 workspaces", "Find> ", "Search sessions", "WORKSPACES", "PREVIEW · workspace-api · working", "LAYOUT", "Not a running Herdr workspace", herdrSourceIcon + " herdr", zoxideSourceIcon + " zoxide", configSourceIcon + " config", "api", "preview content", "enter select"} {
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

func TestTeaModelPreviewTitleShowsSelectedWorkspaceCounts(t *testing.T) {
	m := newTeaModel([]model.Session{
		{Source: "herdr", Name: "api", WorkspaceID: "w1", TabCount: 1, PaneCount: 2},
		{Source: "herdr", Name: "web", WorkspaceID: "w2", TabCount: 3, PaneCount: 1},
	}, Options{})

	if got, want := ansi.Strip(m.previewTitle()), "PREVIEW · 1 tab · 2 panes · api"; got != want {
		t.Fatalf("preview title=%q, want %q", got, want)
	}
	m.list.Selected = 1
	if got, want := ansi.Strip(m.previewTitle()), "PREVIEW · 3 tabs · 1 pane · web"; got != want {
		t.Fatalf("preview title=%q, want %q", got, want)
	}
}

func TestTeaModelRendersActiveTabPaneLayout(t *testing.T) {
	m := newTeaModel([]model.Session{{
		Source:      "herdr",
		Name:        "api",
		WorkspaceID: "w1",
		TabCount:    2,
		PaneCount:   3,
		ActiveTabID: "w1:t1",
		WorkspaceTabs: []model.WorkspaceTab{
			{ID: "w1:t1", Number: 1, Label: "main"},
			{ID: "w1:t2", Number: 2, Label: "logs"},
		},
		WorkspacePanes: []model.WorkspacePane{
			{ID: "w1:p1", TabID: "w1:t1", Label: "Codex", AgentStatus: "working"},
			{ID: "w1:p2", TabID: "w1:t1", Label: "shell"},
			{ID: "w1:p3", TabID: "w1:t2", Label: "tail"},
		},
	}}, Options{})
	m.preview = "preview content"
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = updated.(teaModel)
	updated, _ = m.Update(paneLayoutMsg{key: m.layoutKey, request: m.layoutRequest, layout: herdr.PaneLayout{
		WorkspaceID:   "w1",
		TabID:         "w1:t1",
		Area:          herdr.PaneRect{Width: 120, Height: 40},
		FocusedPaneID: "w1:p1",
		Panes: []herdr.PaneLayoutPane{
			{ID: "w1:p1", Focused: true, Rect: herdr.PaneRect{Width: 72, Height: 40}, Command: "codex --full-auto"},
			{ID: "w1:p2", Rect: herdr.PaneRect{X: 72, Width: 48, Height: 40}, Command: "go test ./..."},
		},
	}})
	m = updated.(teaModel)

	view := ansi.Strip(m.View().Content)
	for _, want := range []string{"PREVIEW · api", "LAYOUT · 2 tabs · 3 panes", "▶ 1 main · 2 panes", "Codex", "$ codex --full-auto", "shell", "$ go test ./...", "┌", "┬", "┐", "└", "┴", "┘"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "2 logs") {
		t.Fatalf("active-tab atlas rendered an inactive tab:\n%s", view)
	}
	if strings.Contains(view, "PREVIEW · 2 tabs") {
		t.Fatalf("wide preview duplicated layout counts:\n%s", view)
	}
	if got, want := lipgloss.Height(view), 30; got != want {
		t.Fatalf("view height=%d, want %d:\n%s", got, want, view)
	}
}

func TestPaneMapPreservesCollapsedPaneBorder(t *testing.T) {
	got := paneMap(model.Session{WorkspacePanes: []model.WorkspacePane{{ID: "w1:p1", Label: "Codex"}}}, herdr.PaneLayout{
		Area:          herdr.PaneRect{Width: 100, Height: 100},
		FocusedPaneID: "w1:p1",
		Panes:         []herdr.PaneLayoutPane{{ID: "w1:p1", Rect: herdr.PaneRect{Width: 100, Height: 1}}},
	}, 20, 3)
	lines := strings.Split(got, "\n")
	if got, want := lines[1], "└──────────────────┘"; got != want {
		t.Fatalf("collapsed pane border=%q, want %q\n%s", got, want, strings.Join(lines, "\n"))
	}
}

func TestPaneMapShowsAllPanesWhenZoomed(t *testing.T) {
	got := paneMap(model.Session{WorkspacePanes: []model.WorkspacePane{
		{ID: "w1:p1", Label: "Codex"},
		{ID: "w1:p2", Label: "shell"},
	}}, herdr.PaneLayout{
		Zoomed:        true,
		Area:          herdr.PaneRect{Width: 100, Height: 40},
		FocusedPaneID: "w1:p1",
		Panes: []herdr.PaneLayoutPane{
			{ID: "w1:p1", Rect: herdr.PaneRect{Width: 60, Height: 40}},
			{ID: "w1:p2", Rect: herdr.PaneRect{X: 60, Width: 40, Height: 40}},
		},
	}, 20, 4)

	if !strings.Contains(got, "shell") || !strings.Contains(got, "Codex") {
		t.Fatalf("zoomed atlas hid a pane:\n%s", got)
	}
	if first := strings.Split(got, "\n")[0]; !strings.HasPrefix(first, "┌▶ Active tab · 2") {
		t.Fatalf("zoomed atlas missing active-tab frame: %q\n%s", first, got)
	}
}

func TestPaneMapOmitsUnknownPaneStatus(t *testing.T) {
	got := paneMap(model.Session{WorkspacePanes: []model.WorkspacePane{{ID: "p1", AgentStatus: "unknown"}}}, herdr.PaneLayout{
		Area:  herdr.PaneRect{Width: 80, Height: 24},
		Panes: []herdr.PaneLayoutPane{{ID: "p1", Rect: herdr.PaneRect{Width: 80, Height: 24}}},
	}, 24, 4)

	if strings.Contains(got, "unknown") {
		t.Fatalf("atlas rendered unresolved pane status:\n%s", got)
	}
}

func TestPaneMapPreservesBorderWithWideLabel(t *testing.T) {
	got := paneMap(model.Session{WorkspacePanes: []model.WorkspacePane{{ID: "p1", Label: "界面"}}}, herdr.PaneLayout{
		Area:  herdr.PaneRect{Width: 100, Height: 40},
		Panes: []herdr.PaneLayoutPane{{ID: "p1", Rect: herdr.PaneRect{Width: 100, Height: 40}}},
	}, 12, 4)

	for i, line := range strings.Split(got, "\n") {
		if width := ansi.StringWidth(line); width != 12 {
			t.Fatalf("line %d width=%d, want 12: %q\n%s", i, width, line, got)
		}
	}
	if line := strings.Split(got, "\n")[1]; !strings.HasSuffix(line, "│") {
		t.Fatalf("wide label displaced right border: %q\n%s", line, got)
	}
}

func TestPaneMapKeepsCommandsInShortSplitPanes(t *testing.T) {
	got := paneMap(model.Session{WorkspacePanes: []model.WorkspacePane{
		{ID: "p1", Label: "tests"},
		{ID: "p2", Label: "server"},
	}}, herdr.PaneLayout{
		Area: herdr.PaneRect{Width: 100, Height: 100},
		Panes: []herdr.PaneLayoutPane{
			{ID: "p1", Rect: herdr.PaneRect{Width: 100, Height: 50}, Command: "go test ./..."},
			{ID: "p2", Rect: herdr.PaneRect{Y: 50, Width: 100, Height: 50}, Command: "go run ./cmd/api"},
		},
	}, 32, 5)

	for _, want := range []string{"tests · $ go test ./...", "server · $ go run ./cmd/api", "├", "┤"} {
		if !strings.Contains(got, want) {
			t.Fatalf("short split atlas missing %q:\n%s", want, got)
		}
	}
}

func TestWorkspaceLayoutRejectsInactiveTab(t *testing.T) {
	m := newTeaModel([]model.Session{{
		Source:        "herdr",
		WorkspaceID:   "w1",
		ActiveTabID:   "w1:t1",
		WorkspaceTabs: []model.WorkspaceTab{{ID: "w1:t1", Label: "main"}, {ID: "w1:t2", Label: "logs"}},
	}}, Options{})
	m.layout = herdr.PaneLayout{
		WorkspaceID: "w1",
		TabID:       "w1:t2",
		Zoomed:      true,
		Area:        herdr.PaneRect{Width: 80, Height: 24},
		Panes:       []herdr.PaneLayoutPane{{ID: "p2", Rect: herdr.PaneRect{Width: 80, Height: 24}}},
	}

	if got := ansi.Strip(m.workspaceLayoutView(40, 5)); !strings.Contains(got, "Layout unavailable") || strings.Contains(got, "▶ 2 logs") || strings.Contains(got, "zoomed") {
		t.Fatalf("inactive tab layout rendered as active:\n%s", got)
	}
}

func TestPaneMapSanitizesTerminalMetadata(t *testing.T) {
	got := paneMap(model.Session{
		ActiveTabID:    "t1",
		WorkspaceTabs:  []model.WorkspaceTab{{ID: "t1", Label: "\x1b[31mmain\x1b[0m\nforged"}},
		WorkspacePanes: []model.WorkspacePane{{ID: "p1", Label: "Codex\nforged\a"}},
	}, herdr.PaneLayout{
		TabID: "t1",
		Area:  herdr.PaneRect{Width: 80, Height: 24},
		Panes: []herdr.PaneLayoutPane{{ID: "p1", Rect: herdr.PaneRect{Width: 80, Height: 24}, Command: "go test\a\n./..."}},
	}, 32, 5)

	if strings.ContainsAny(got, "\x1b\a") || strings.Count(got, "\n") != 4 {
		t.Fatalf("atlas contains terminal controls or forged rows: %q", got)
	}
	for _, want := range []string{"main forged", "Codex forged", "$ go test ./..."} {
		if !strings.Contains(got, want) {
			t.Fatalf("sanitized atlas missing %q:\n%s", want, got)
		}
	}
}

func TestTeaModelLoadsActiveTabPaneLayout(t *testing.T) {
	calledPaneID := ""
	m := newTeaModel([]model.Session{{
		Source:         "herdr",
		Name:           "api",
		WorkspaceID:    "w1",
		ActiveTabID:    "w1:t1",
		WorkspaceTabs:  []model.WorkspaceTab{{ID: "w1:t1"}},
		WorkspacePanes: []model.WorkspacePane{{ID: "w1:p1", TabID: "w1:t1"}},
	}}, Options{LoadPaneLayout: func(paneID string) (herdr.PaneLayout, error) {
		calledPaneID = paneID
		return herdr.PaneLayout{WorkspaceID: "w1", TabID: "w1:t1"}, nil
	}})

	msg := m.Init()()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("init command=%T, want batch", msg)
	}
	var layout paneLayoutMsg
	found := false
	for _, cmd := range batch {
		if got, ok := cmd().(paneLayoutMsg); ok {
			layout = got
			found = true
		}
	}
	if !found || calledPaneID != "w1:p1" || layout.key != model.Key(m.list.Filtered[0]) || layout.layout.WorkspaceID != "w1" {
		t.Fatalf("found=%v pane=%q layout=%#v", found, calledPaneID, layout)
	}
}

func TestTeaModelRefreshesPaneLayoutWhenSelectionChanges(t *testing.T) {
	t.Setenv("HERDR_SESH_REDUCE_MOTION", "1")
	calledPaneID := ""
	m := newTeaModel([]model.Session{
		{Source: "herdr", Name: "api", WorkspaceID: "w1", ActiveTabID: "w1:t1", WorkspaceTabs: []model.WorkspaceTab{{ID: "w1:t1"}}, WorkspacePanes: []model.WorkspacePane{{ID: "w1:p1", TabID: "w1:t1"}}},
		{Source: "herdr", Name: "web", WorkspaceID: "w2", ActiveTabID: "w2:t1", WorkspaceTabs: []model.WorkspaceTab{{ID: "w2:t1"}}, WorkspacePanes: []model.WorkspacePane{{ID: "w2:p1", TabID: "w2:t1"}}},
	}, Options{LoadPaneLayout: func(paneID string) (herdr.PaneLayout, error) {
		calledPaneID = paneID
		return herdr.PaneLayout{WorkspaceID: "w2", TabID: "w2:t1"}, nil
	}})
	m.listFocused = true
	m.input.Blur()

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(teaModel)
	if cmd == nil || m.layoutKey != model.Key(m.list.Filtered[1]) {
		t.Fatalf("cmd=%v layoutKey=%q", cmd, m.layoutKey)
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("move command=%T, want batch", msg)
	}
	found := false
	for _, child := range batch {
		if layout, ok := child().(paneLayoutMsg); ok {
			updated, _ = m.Update(layout)
			m = updated.(teaModel)
			found = true
		}
	}
	if !found || calledPaneID != "w2:p1" || m.layout.WorkspaceID != "w2" {
		t.Fatalf("found=%v pane=%q layout=%#v", found, calledPaneID, m.layout)
	}
}

func TestTeaModelRefreshesPaneLayoutWhenFilteringChangesSelection(t *testing.T) {
	m := newTeaModel([]model.Session{
		{Source: "herdr", Name: "api", WorkspaceID: "w1", ActiveTabID: "w1:t1", WorkspaceTabs: []model.WorkspaceTab{{ID: "w1:t1"}}, WorkspacePanes: []model.WorkspacePane{{ID: "w1:p1", TabID: "w1:t1"}}},
		{Source: "herdr", Name: "web", WorkspaceID: "w2", ActiveTabID: "w2:t1", WorkspaceTabs: []model.WorkspaceTab{{ID: "w2:t1"}}, WorkspacePanes: []model.WorkspacePane{{ID: "w2:p1", TabID: "w2:t1"}}},
	}, Options{LoadPaneLayout: func(string) (herdr.PaneLayout, error) {
		return herdr.PaneLayout{}, nil
	}})

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'w', Text: "web"})
	m = updated.(teaModel)
	if cmd == nil || m.layoutKey != model.Key(m.list.Filtered[0]) || m.list.Filtered[0].Name != "web" {
		t.Fatalf("cmd=%v layoutKey=%q filtered=%#v", cmd, m.layoutKey, m.list.Filtered)
	}
}

func TestTeaModelShowsPaneLayoutLoadingState(t *testing.T) {
	m := newTeaModel([]model.Session{{
		Source:         "herdr",
		Name:           "api",
		WorkspaceID:    "w1",
		TabCount:       1,
		PaneCount:      2,
		ActiveTabID:    "w1:t1",
		WorkspaceTabs:  []model.WorkspaceTab{{ID: "w1:t1"}},
		WorkspacePanes: []model.WorkspacePane{{ID: "w1:p1", TabID: "w1:t1"}},
	}}, Options{LoadPaneLayout: func(string) (herdr.PaneLayout, error) {
		return herdr.PaneLayout{}, nil
	}})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = updated.(teaModel)

	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "Loading layout…") {
		t.Fatalf("view missing layout loading state:\n%s", view)
	}
}

func TestTeaModelIgnoresStalePaneLayoutResult(t *testing.T) {
	m := newTeaModel([]model.Session{
		{Source: "herdr", Name: "api", WorkspaceID: "w1", ActiveTabID: "w1:t1", WorkspaceTabs: []model.WorkspaceTab{{ID: "w1:t1"}}, WorkspacePanes: []model.WorkspacePane{{ID: "w1:p1", TabID: "w1:t1"}}},
		{Source: "herdr", Name: "web", WorkspaceID: "w2", ActiveTabID: "w2:t1", WorkspaceTabs: []model.WorkspaceTab{{ID: "w2:t1"}}, WorkspacePanes: []model.WorkspacePane{{ID: "w2:p1", TabID: "w2:t1"}}},
	}, Options{LoadPaneLayout: func(string) (herdr.PaneLayout, error) {
		return herdr.PaneLayout{}, nil
	}})
	staleKey := m.layoutKey
	staleRequest := m.layoutRequest
	m.list.Selected = 1
	m, _ = m.refreshWorkspaceLayout()
	updated, _ := m.Update(paneLayoutMsg{key: staleKey, request: staleRequest, layout: herdr.PaneLayout{WorkspaceID: "w1"}})
	m = updated.(teaModel)
	if m.layout.WorkspaceID != "" || !m.layoutLoading {
		t.Fatalf("stale layout applied: layout=%#v loading=%v", m.layout, m.layoutLoading)
	}
}

func TestTeaModelIgnoresOlderLayoutResultAfterReturningToWorkspace(t *testing.T) {
	aCalls := 0
	m := newTeaModel([]model.Session{
		{Source: "herdr", Name: "api", WorkspaceID: "w1", ActiveTabID: "w1:t1", WorkspaceTabs: []model.WorkspaceTab{{ID: "w1:t1"}}, WorkspacePanes: []model.WorkspacePane{{ID: "w1:p1", TabID: "w1:t1"}}},
		{Source: "herdr", Name: "web", WorkspaceID: "w2", ActiveTabID: "w2:t1", WorkspaceTabs: []model.WorkspaceTab{{ID: "w2:t1"}}, WorkspacePanes: []model.WorkspacePane{{ID: "w2:p1", TabID: "w2:t1"}}},
	}, Options{LoadPaneLayout: func(paneID string) (herdr.PaneLayout, error) {
		if paneID == "w1:p1" {
			aCalls++
			return herdr.PaneLayout{WorkspaceID: "w1", Area: herdr.PaneRect{X: aCalls}}, nil
		}
		return herdr.PaneLayout{WorkspaceID: "w2"}, nil
	}})

	batch, ok := m.Init()().(tea.BatchMsg)
	if !ok {
		t.Fatal("initial command was not a batch")
	}
	var olderA paneLayoutMsg
	for _, cmd := range batch {
		if msg, ok := cmd().(paneLayoutMsg); ok {
			olderA = msg
		}
	}

	m.list.Selected = 1
	m, bCmd := m.refreshWorkspaceLayout()
	_ = bCmd()
	m.list.Selected = 0
	m, newerACmd := m.refreshWorkspaceLayout()
	newerA := newerACmd().(paneLayoutMsg)

	updated, _ := m.Update(newerA)
	m = updated.(teaModel)
	updated, _ = m.Update(olderA)
	m = updated.(teaModel)
	if got, want := m.layout.Area.X, 2; got != want {
		t.Fatalf("older A result overwrote newer A result: x=%d, want %d", got, want)
	}
}

func TestTeaModelShowsPaneLayoutFailureWithoutClosing(t *testing.T) {
	m := newTeaModel([]model.Session{{
		Source:         "herdr",
		Name:           "api",
		WorkspaceID:    "w1",
		TabCount:       1,
		PaneCount:      1,
		ActiveTabID:    "w1:t1",
		WorkspaceTabs:  []model.WorkspaceTab{{ID: "w1:t1"}},
		WorkspacePanes: []model.WorkspacePane{{ID: "w1:p1", TabID: "w1:t1"}},
	}}, Options{LoadPaneLayout: func(string) (herdr.PaneLayout, error) {
		return herdr.PaneLayout{}, nil
	}})
	updated, _ := m.Update(paneLayoutMsg{key: m.layoutKey, request: m.layoutRequest, err: errors.New("pane vanished")})
	m = updated.(teaModel)
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = updated.(teaModel)
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "Layout unavailable") || !strings.Contains(view, "pane vanished") || m.chosen {
		t.Fatalf("layout failure disrupted picker:\n%s", view)
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
		{status: "working", glyph: "●", color: "38;2;158;206;106"},
		{status: "blocked", glyph: "◆", color: "38;2;224;175;104"},
		{status: "idle", glyph: "○", color: "38;2;86;95;137"},
		{status: "done", glyph: "✓", color: "38;2;187;154;247"},
	}
	for _, tt := range tests {
		got := row(model.Session{Source: "herdr", Name: "api", AgentStatus: tt.status}, false, 80, true, "")
		if !strings.Contains(ansi.Strip(got), tt.glyph) || !strings.Contains(got, tt.color) {
			t.Fatalf("row for status %q missing glyph/color:\n%q", tt.status, got)
		}
	}
	for _, status := range []string{"", "unknown", "future"} {
		got := ansi.Strip(row(model.Session{Source: "herdr", Name: "api", AgentStatus: status}, false, 80, true, ""))
		if strings.ContainsAny(got, "●◆○✓") {
			t.Fatalf("row for status %q unexpectedly contains indicator: %q", status, got)
		}
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
	layoutRow := -1
	for i, line := range lines {
		if strings.Contains(line, "LAYOUT") {
			layoutRow = i
			break
		}
	}
	if layoutRow < 0 || layoutRow < 18 || layoutRow > 22 {
		t.Fatalf("layout row=%d, want lower-right quadrant near midpoint:\n%s", layoutRow, view)
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
	for _, want := range []string{"38;2;125;207;255", "38;2;158;206;106", herdrSourceIcon + " herdr", "herdr-plugin-sesh", "/tmp/herdr-plugin-sesh"} {
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
	m := newTeaModel([]model.Session{{Source: "herdr", Name: "api", Path: "/tmp/api", WorkspaceID: "w1", TabCount: 2, PaneCount: 3, AgentStatus: "blocked"}}, Options{})
	m.preview = "preview content"
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 70, Height: 28})
	m = updated.(teaModel)
	view := ansi.Strip(m.View().Content)
	if got, want := lipgloss.Height(view), 28; got != want {
		t.Fatalf("view height=%d, want %d:\n%s", got, want, view)
	}
	if !strings.Contains(view, "WORKSPACES") || !strings.Contains(view, "PREVIEW · 2 tabs · 3 panes · api · blocked") {
		t.Fatalf("narrow view missing stacked sections:\n%s", view)
	}
	if strings.Contains(view, "LAYOUT") {
		t.Fatalf("narrow view should keep the existing stacked preview:\n%s", view)
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

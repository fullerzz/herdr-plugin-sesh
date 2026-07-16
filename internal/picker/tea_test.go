package picker

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"charm.land/bubbles/v2/cursor"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
)

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
	cur, ok := m.list.Current()
	if !ok || cur.Name != "web" {
		t.Fatalf("current = %#v ok=%v", cur, ok)
	}
}

func TestTeaModelSmearsRapidSelectionMoves(t *testing.T) {
	t.Setenv("HERDR_SESH_REDUCE_MOTION", "")
	m := newTeaModel([]model.Session{{Name: "api"}, {Name: "web"}, {Name: "docs"}}, Options{})
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
	for _, want := range []string{"herdr / sesh", "3 workspaces", "Find> ", "Search sessions", "WORKSPACES", "PREVIEW · workspace-api · working", herdrSourceIcon + " herdr", zoxideSourceIcon + " zoxide", configSourceIcon + " config", "api", "preview content", "enter select"} {
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

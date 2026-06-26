package picker

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/cursor"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"forgejo.local/fullerzz/herdr-plugin-sesh/internal/model"
)

func TestTeaModelFiltersMovesAndChooses(t *testing.T) {
	m := newTeaModel([]model.Session{
		{Name: "api-service", Path: "/tmp/api"},
		{Name: "web", Path: "/tmp/web"},
	}, Options{SeparatorAware: true})
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("api service")})
	m = updated.(teaModel)
	cur, ok := m.list.Current()
	if !ok || cur.Name != "api-service" {
		t.Fatalf("current = %#v ok=%v", cur, ok)
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(teaModel)
	if cmd == nil || !m.chosen || m.choice.Name != "api-service" {
		t.Fatalf("chosen=%v choice=%#v cmd=%v", m.chosen, m.choice, cmd)
	}
}

func TestTeaModelMovesSelection(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "api"}, {Name: "web"}}, Options{})
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(teaModel)
	cur, ok := m.list.Current()
	if !ok || cur.Name != "web" {
		t.Fatalf("current = %#v ok=%v", cur, ok)
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
		{Source: "herdr", Name: "workspace-api", Path: "/tmp/workspace-api"},
		{Source: "zoxide", Name: "tools", Path: "/tmp/tools"},
		{Source: "config", Name: "api", Path: "/tmp/api"},
	}, Options{
		Prompt:      "Find> ",
		Placeholder: "Search sessions",
	})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(teaModel)
	updated, _ = m.Update(previewCommand(m.previewKey, m.list.Filtered[m.list.Selected], m.defaultPreviewCommand)())
	m = updated.(teaModel)
	view := m.View()
	for _, want := range []string{"herdr workspace picker", "3/3 matches", "Find> ", "Search sessions", herdrSourceIcon + " herdr", zoxideSourceIcon + " zoxide", configSourceIcon + " config", "api", "preview", "preview content", "Enter select"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
}

func TestRowUsesSourceCategoryColors(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(prev)
	})

	tests := []struct {
		source string
		color  string
	}{
		{source: "herdr", color: "38;5;81"},
		{source: "config", color: "38;5;214"},
		{source: "zoxide", color: "38;5;114"},
		{source: "dir", color: "38;5;176"},
	}
	for _, tt := range tests {
		got := row(model.Session{Source: tt.source, Name: tt.source}, false, 80)
		if !strings.Contains(got, tt.color) {
			t.Fatalf("row for source %q missing color %s:\n%q", tt.source, tt.color, got)
		}
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
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(teaModel)
	if cmd == nil || !strings.Contains(m.preview, "Loading preview") {
		t.Fatalf("cmd=%v preview=%q", cmd, m.preview)
	}
	current, ok := m.list.Current()
	if !ok || current.Name != "web" || m.previewKey != model.Key(current) {
		t.Fatalf("current=%#v ok=%v previewKey=%q", current, ok, m.previewKey)
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
	if got, want := lipgloss.Height(short), 4+previewTitleRows+previewBorderRows; got != want {
		t.Fatalf("preview height=%d, want %d\n%s", got, want, short)
	}
	if !strings.Contains(long, "...") {
		t.Fatalf("long preview missing truncation marker:\n%s", long)
	}
	if last := strings.Split(long, "\n")[lipgloss.Height(long)-1]; !strings.HasPrefix(last, "+") || !strings.HasSuffix(last, "+") {
		t.Fatalf("preview bottom border was clipped:\n%s", long)
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
	view := m.View()
	if got, want := lipgloss.Height(view), 40; got != want {
		t.Fatalf("view height=%d, want %d", got, want)
	}
	lines := strings.Split(view, "\n")
	if lines[0] != "" {
		t.Fatalf("expected top padding row, got %q\n%s", lines[0], view)
	}
	if topBorder := lines[1]; !strings.HasPrefix(topBorder, "+") || !strings.HasSuffix(topBorder, "+") {
		t.Fatalf("expected parent top border after padding, got %q\n%s", topBorder, view)
	}
	if padding := lines[len(lines)-2]; strings.Contains(padding, "+") || strings.Contains(padding, "preview") || strings.Contains(padding, "Enter select") {
		t.Fatalf("expected one padding line before parent bottom border, got %q\n%s", padding, view)
	}
}

func TestSelectedRowHighlightDoesNotResetBeforeContent(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(prev)
	})

	got := row(model.Session{Source: "herdr", Name: "herdr-plugin-sesh", Path: "/tmp/herdr-plugin-sesh"}, true, 80)
	if !strings.Contains(got, "48;5;63") {
		t.Fatalf("selected row missing highlight background:\n%q", got)
	}
	for _, want := range []string{herdrSourceIcon + " herdr", "herdr-plugin-sesh", "/tmp/herdr-plugin-sesh"} {
		i := strings.Index(got, want)
		if i == -1 {
			t.Fatalf("selected row missing %q:\n%q", want, got)
		}
		prefix := got[:i]
		if bg, reset := strings.LastIndex(prefix, "48;5;63"), strings.LastIndex(prefix, "\x1b[0m"); bg == -1 || bg < reset {
			t.Fatalf("selected row highlight inactive before %q:\n%q", want, got)
		}
	}
}

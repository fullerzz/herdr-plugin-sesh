package picker

import (
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
	view := m.View()
	for _, want := range []string{"herdr workspace picker", "3/3 matches", "Find> ", "Search sessions", herdrSourceIcon + " herdr", zoxideSourceIcon + " zoxide", configSourceIcon + " config", "api", "Enter select"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
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

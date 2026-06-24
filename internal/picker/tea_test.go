package picker

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

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

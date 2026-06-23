package picker

import (
	"strings"

	"forgejo.local/fullerzz/herdr-plugin-sesh/internal/model"
)

type Model struct {
	All            []model.Session
	Filtered       []model.Session
	Query          string
	Selected       int
	SeparatorAware bool
}

func New(items []model.Session) Model { return Model{All: items, Filtered: items} }
func (m *Model) Filter(q string) {
	m.Query = q
	m.Filtered = m.Filtered[:0]
	for _, s := range m.All {
		if Match(s.Name, q, m.SeparatorAware) || Match(s.Path, q, m.SeparatorAware) {
			m.Filtered = append(m.Filtered, s)
		}
	}
	if m.Selected >= len(m.Filtered) {
		m.Selected = len(m.Filtered) - 1
	}
	if m.Selected < 0 {
		m.Selected = 0
	}
}
func (m *Model) Current() (model.Session, bool) {
	if len(m.Filtered) == 0 || m.Selected < 0 || m.Selected >= len(m.Filtered) {
		return model.Session{}, false
	}
	return m.Filtered[m.Selected], true
}
func Match(s, q string, sep bool) bool {
	s = strings.ToLower(s)
	q = strings.ToLower(q)
	if sep {
		repl := strings.NewReplacer("-", " ", "_", " ", "/", " ", ".", " ")
		s = repl.Replace(s)
	}
	return strings.Contains(s, q)
}

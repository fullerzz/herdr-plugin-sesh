package picker

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
)

type Model struct {
	All            []model.Session
	Filtered       []model.Session
	Query          string
	Selected       int
	SeparatorAware bool
}

func New(items []model.Session) Model {
	return Model{
		All:      append([]model.Session(nil), items...),
		Filtered: append([]model.Session(nil), items...),
	}
}
func (m *Model) Filter(q string) {
	queryChanged := q != m.Query
	m.Query = q
	m.Filtered = m.Filtered[:0]
	var homeMatches []model.Session
	var pathMatches []model.Session
	homeQuery := strings.EqualFold(q, "home")
	for _, s := range m.All {
		nameMatch := Match(s.Name, q, m.SeparatorAware)
		pathMatch := Match(s.Path, q, m.SeparatorAware) || homeQuery && isHomePath(s.Path)
		if !nameMatch && !pathMatch {
			continue
		}
		if homeQuery && isHomePath(s.Path) {
			homeMatches = append(homeMatches, s)
		} else if nameMatch {
			m.Filtered = append(m.Filtered, s)
		} else {
			pathMatches = append(pathMatches, s)
		}
	}
	m.Filtered = append(m.Filtered, pathMatches...)
	if len(homeMatches) > 0 {
		m.Filtered = append(homeMatches, m.Filtered...)
	}
	if queryChanged {
		m.Selected = 0
	}
	m.clampSelected()
}
func (m *Model) Move(delta int) {
	m.Selected += delta
	m.clampSelected()
}
func (m *Model) UpdateAgentStatuses(statuses map[string]string) {
	for i := range m.All {
		if m.All[i].WorkspaceID != "" {
			m.All[i].AgentStatus = statuses[m.All[i].WorkspaceID]
		}
	}
	m.Filter(m.Query)
}
func (m *Model) clampSelected() {
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
		q = repl.Replace(q)
	}
	return strings.Contains(s, q)
}
func isHomePath(p string) bool {
	if p == "" {
		return false
	}
	cleaned := filepath.Clean(p)
	if cleaned == "~" {
		return true
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return false
	}
	return strings.EqualFold(cleaned, filepath.Clean(home))
}
func isHomeSession(s model.Session) bool {
	return isHomePath(s.Name) || isHomePath(s.Path)
}

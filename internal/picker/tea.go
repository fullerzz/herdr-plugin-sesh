package picker

import (
	"fmt"
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	sessionmodel "forgejo.local/fullerzz/herdr-plugin-sesh/internal/model"
)

const maxVisibleRows = 12

type Options struct {
	Output         io.Writer
	Prompt         string
	Placeholder    string
	SeparatorAware bool
}

func Run(items []sessionmodel.Session, opts Options) (sessionmodel.Session, bool, error) {
	popts := []tea.ProgramOption{tea.WithAltScreen()}
	if opts.Output != nil {
		popts = append(popts, tea.WithOutput(opts.Output))
	}
	final, err := tea.NewProgram(newTeaModel(items, opts), popts...).Run()
	if err != nil {
		return sessionmodel.Session{}, false, err
	}
	m, ok := final.(teaModel)
	if !ok || !m.chosen {
		return sessionmodel.Session{}, false, nil
	}
	return m.choice, true, nil
}

type teaModel struct {
	list        Model
	prompt      string
	placeholder string
	choice      sessionmodel.Session
	chosen      bool
}

func newTeaModel(items []sessionmodel.Session, opts Options) teaModel {
	list := New(items)
	list.SeparatorAware = opts.SeparatorAware
	prompt := opts.Prompt
	if prompt == "" {
		prompt = "Sesh> "
	}
	return teaModel{list: list, prompt: prompt, placeholder: opts.Placeholder}
}

func (m teaModel) Init() tea.Cmd { return nil }

//nolint:ireturn // Bubble Tea's Model interface requires this return shape.
func (m teaModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.Type {
	case tea.KeyCtrlC, tea.KeyEsc:
		return m, tea.Quit
	case tea.KeyEnter:
		if choice, ok := m.list.Current(); ok {
			m.choice = choice
			m.chosen = true
		}
		return m, tea.Quit
	case tea.KeyUp, tea.KeyCtrlP:
		m.list.Move(-1)
	case tea.KeyDown, tea.KeyCtrlN:
		m.list.Move(1)
	case tea.KeyBackspace, tea.KeyCtrlH:
		query := []rune(m.list.Query)
		if len(query) > 0 {
			m.list.Filter(string(query[:len(query)-1]))
		}
	case tea.KeyCtrlU:
		m.list.Filter("")
	case tea.KeyRunes, tea.KeySpace:
		m.list.Filter(m.list.Query + string(key.Runes))
	}
	return m, nil
}

func (m teaModel) View() string {
	var b strings.Builder
	query := m.list.Query
	if query == "" && m.placeholder != "" {
		query = m.placeholder
	}
	fmt.Fprintf(&b, "%s%s\n\n", m.prompt, query)
	if len(m.list.Filtered) == 0 {
		b.WriteString("No matching workspaces\n")
	} else {
		start := 0
		if m.list.Selected >= maxVisibleRows {
			start = m.list.Selected - maxVisibleRows + 1
		}
		end := start + maxVisibleRows
		if end > len(m.list.Filtered) {
			end = len(m.list.Filtered)
		}
		if start > 0 {
			b.WriteString("  ...\n")
		}
		for i := start; i < end; i++ {
			b.WriteString(row(m.list.Filtered[i], i == m.list.Selected))
		}
		if end < len(m.list.Filtered) {
			b.WriteString("  ...\n")
		}
	}
	b.WriteString("\nEnter select  Esc cancel  Ctrl+U clear\n")
	return b.String()
}

func row(s sessionmodel.Session, selected bool) string {
	cursor := " "
	if selected {
		cursor = ">"
	}
	label := s.Name
	if label == "" {
		label = s.Path
	}
	if s.Path == "" || s.Path == label {
		return fmt.Sprintf("%s [%s] %s\n", cursor, s.Source, label)
	}
	return fmt.Sprintf("%s [%s] %s  %s\n", cursor, s.Source, label, s.Path)
}

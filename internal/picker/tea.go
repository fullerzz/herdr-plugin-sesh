package picker

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	sessionmodel "forgejo.local/fullerzz/herdr-plugin-sesh/internal/model"
)

const maxVisibleRows = 12

const (
	defaultPrompt      = "Sesh> "
	defaultPlaceholder = "Filter workspaces"
	defaultWidth       = 80
	minContentWidth    = 36
	maxContentWidth    = 84
	herdrSourceIcon    = "\U000f0cc6"
	zoxideSourceIcon   = "\uf114"
	configSourceIcon   = "\ue615"
)

var (
	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.Border{
			Top:         "-",
			Bottom:      "-",
			Left:        "|",
			Right:       "|",
			TopLeft:     "+",
			TopRight:    "+",
			BottomLeft:  "+",
			BottomRight: "+",
		}).
		BorderForeground(lipgloss.Color("63")).
		Padding(1, 2)

	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("63")).
			Bold(true)

	countStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244"))

	inputBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.Border{
			Top:         "-",
			Bottom:      "-",
			Left:        "|",
			Right:       "|",
			TopLeft:     "+",
			TopRight:    "+",
			BottomLeft:  "+",
			BottomRight: "+",
		}).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1)

	rowStyle = lipgloss.NewStyle().
			Padding(0, 1)

	selectedRowStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("230")).
				Background(lipgloss.Color("63")).
				Bold(true).
				Padding(0, 1)

	sourceStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("81")).
			Bold(true)

	pathStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244"))

	emptyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("203")).
			Padding(1, 1)

	moreStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244")).
			Padding(0, 1)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244"))
)

type Options struct {
	Output         io.Writer
	Prompt         string
	Placeholder    string
	SeparatorAware bool
	FZFCommand     string
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
	list   Model
	input  textinput.Model
	width  int
	choice sessionmodel.Session
	chosen bool
}

func newTeaModel(items []sessionmodel.Session, opts Options) teaModel {
	list := New(items)
	list.SeparatorAware = opts.SeparatorAware
	prompt := opts.Prompt
	if prompt == "" {
		prompt = defaultPrompt
	}
	placeholder := opts.Placeholder
	if placeholder == "" {
		placeholder = defaultPlaceholder
	}
	input := textinput.New()
	input.Prompt = prompt
	input.Placeholder = placeholder
	input.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("63")).Bold(true)
	input.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	input.PlaceholderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	input.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("63"))
	input.Focus()
	return teaModel{list: list, input: input}
}

func (m teaModel) Init() tea.Cmd { return m.input.Focus() }

//nolint:ireturn // Bubble Tea's Model interface requires this return shape.
func (m teaModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = size.Width
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		m.list.Filter(m.input.Value())
		return m, cmd
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
	case tea.KeyCtrlU:
		m.input.SetValue("")
		m.list.Filter("")
	default:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		m.list.Filter(m.input.Value())
		return m, cmd
	}
	return m, nil
}

func (m teaModel) View() string {
	width := m.contentWidth()
	var b strings.Builder
	b.WriteString(m.header(width))
	b.WriteString("\n\n")
	input := m.input
	input.Width = maxInt(8, width-lipgloss.Width(input.Prompt)-4)
	b.WriteString(inputBoxStyle.Width(width - 2).Render(input.View()))
	b.WriteString("\n\n")
	if len(m.list.Filtered) == 0 {
		b.WriteString(emptyStyle.Width(width - 2).Render("No matching workspaces"))
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
			b.WriteString(moreStyle.Render("...") + "\n")
		}
		for i := start; i < end; i++ {
			b.WriteString(row(m.list.Filtered[i], i == m.list.Selected, width))
		}
		if end < len(m.list.Filtered) {
			b.WriteString(moreStyle.Render("...") + "\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Width(width).Render("Enter select  Up/Down move  Ctrl+U clear  Esc cancel"))
	return panelStyle.Width(width + 4).Render(b.String())
}

func (m teaModel) contentWidth() int {
	width := m.width
	if width == 0 {
		width = defaultWidth
	}
	width -= 6
	if width < minContentWidth {
		width = minContentWidth
	}
	if width > maxContentWidth {
		width = maxContentWidth
	}
	return width
}

func (m teaModel) header(width int) string {
	title := titleStyle.Render("herdr workspace picker")
	count := countStyle.Render(fmt.Sprintf("%d/%d matches", len(m.list.Filtered), len(m.list.All)))
	gap := width - lipgloss.Width(title) - lipgloss.Width(count)
	if gap < 1 {
		gap = 1
	}
	return title + strings.Repeat(" ", gap) + count
}

func row(s sessionmodel.Session, selected bool, width int) string {
	cursor := " "
	if selected {
		cursor = ">"
	}
	label := s.Name
	if label == "" {
		label = s.Path
	}
	source := s.Source
	badgeText := sourceBadge(source)
	badge := sourceStyle.Render(badgeText)
	path := ""
	showPath := s.Path != "" && s.Path != label
	if showPath {
		path = pathStyle.Inline(true).MaxWidth(maxInt(8, width/2)).Render(s.Path)
	}
	line := rowText(cursor, badge, label, path)
	if selected {
		path = ""
		if showPath {
			path = lipgloss.NewStyle().Inline(true).MaxWidth(maxInt(8, width/2)).Render(s.Path)
		}
		line = rowText(cursor, badgeText, label, path)
		return selectedRowStyle.Width(width-2).Render(line) + "\n"
	}
	return rowStyle.Width(width-2).Render(line) + "\n"
}

func sourceBadge(source string) string {
	switch source {
	case "herdr":
		return herdrSourceIcon + " herdr"
	case "zoxide":
		return zoxideSourceIcon + " zoxide"
	case "config":
		return configSourceIcon + " config"
	case "":
		return "[session]"
	default:
		return "[" + source + "]"
	}
}

func rowText(cursor, badge, label, path string) string {
	if path == "" {
		return fmt.Sprintf("%s %s %s", cursor, badge, label)
	}
	return fmt.Sprintf("%s %s %s  %s", cursor, badge, label, path)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

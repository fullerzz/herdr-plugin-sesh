package picker

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	sessionmodel "github.com/fullerzz/herdr-plugin-sesh/internal/model"
	previewpkg "github.com/fullerzz/herdr-plugin-sesh/internal/preview"
)

const defaultVisibleRows = 12

const (
	defaultPrompt      = "Sesh> "
	defaultPlaceholder = "Filter workspaces"
	defaultWidth       = 80
	minContentWidth    = 36
	maxContentWidth    = 132
	previewSplitWidth  = 92
	minPreviewWidth    = 36
	maxPreviewWidth    = 52
	previewTitleRows   = 1
	previewBorderRows  = 2
	pickerTopPadding   = 1
	pickerChromeRows   = 14 + pickerTopPadding
	compactPreviewBody = 6
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

	previewBoxStyle = lipgloss.NewStyle().
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

var renderPreview = previewpkg.Render

type Options struct {
	Output                io.Writer
	Prompt                string
	Placeholder           string
	SeparatorAware        bool
	DefaultPreviewCommand string
	FZFCommand            string
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
	height int
	choice sessionmodel.Session
	chosen bool

	preview    string
	previewKey string

	defaultPreviewCommand string
}

type previewMsg struct {
	key  string
	text string
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
	m := teaModel{list: list, input: input, defaultPreviewCommand: opts.DefaultPreviewCommand}
	if current, ok := list.Current(); ok {
		m.previewKey = sessionmodel.Key(current)
		m.preview = "Loading preview..."
	}
	return m
}

func (m teaModel) Init() tea.Cmd {
	cmds := []tea.Cmd{m.input.Focus()}
	if current, ok := m.list.Current(); ok && m.previewKey != "" {
		cmds = append(cmds, previewCommand(m.previewKey, current, m.defaultPreviewCommand))
	}
	return tea.Batch(cmds...)
}

//nolint:ireturn // Bubble Tea's Model interface requires this return shape.
func (m teaModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if preview, ok := msg.(previewMsg); ok {
		if preview.key == m.previewKey {
			m.preview = preview.text
		}
		return m, nil
	}
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = size.Width
		m.height = size.Height
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		m.list.Filter(m.input.Value())
		m, previewCmd := m.refreshPreview()
		return m, tea.Batch(cmd, previewCmd)
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
		return m.refreshPreview()
	case tea.KeyDown, tea.KeyCtrlN:
		m.list.Move(1)
		return m.refreshPreview()
	case tea.KeyCtrlU:
		m.input.SetValue("")
		m.list.Filter("")
		return m.refreshPreview()
	default:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		m.list.Filter(m.input.Value())
		m, previewCmd := m.refreshPreview()
		return m, tea.Batch(cmd, previewCmd)
	}
}

func (m teaModel) View() string {
	width := m.contentWidth()
	listWidth, previewWidth := previewLayout(width)
	previewLines := m.previewBodyLines()
	listRows := previewLines
	if previewWidth == 0 {
		previewLines = compactPreviewBody
		listRows = defaultVisibleRows
	}
	var b strings.Builder
	b.WriteString(m.header(width))
	b.WriteString("\n\n")
	input := m.input
	input.Width = maxInt(8, width-lipgloss.Width(input.Prompt)-4)
	b.WriteString(inputBoxStyle.Width(width - 2).Render(input.View()))
	b.WriteString("\n\n")
	list := m.listView(listWidth, listRows)
	if previewWidth > 0 {
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, list, "  ", m.previewView(previewWidth, previewLines)))
	} else {
		b.WriteString(list)
		b.WriteString("\n")
		b.WriteString(m.previewView(width, previewLines))
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Width(width).Render("Enter select  Up/Down move  Ctrl+U clear  Esc cancel"))
	return strings.Repeat("\n", pickerTopPadding) + panelStyle.Width(width+4).Render(b.String())
}

func (m teaModel) listView(width, visibleRows int) string {
	var b strings.Builder
	if len(m.list.Filtered) == 0 {
		b.WriteString(emptyStyle.Width(width - 2).Render("No matching workspaces"))
	} else {
		start := 0
		if m.list.Selected >= visibleRows {
			start = m.list.Selected - visibleRows + 1
		}
		end := start + visibleRows
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
	return b.String()
}

func (m teaModel) previewBodyLines() int {
	if m.height == 0 {
		return defaultVisibleRows
	}
	lines := m.height - pickerChromeRows
	if lines < compactPreviewBody {
		return compactPreviewBody
	}
	return lines
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

func (m teaModel) previewView(width, maxLines int) string {
	text := strings.TrimRight(m.preview, "\n")
	if text == "" {
		text = "No preview available"
	}
	bodyWidth := maxInt(8, width-4)
	text = fixedVisualLines(text, bodyWidth, maxLines)
	height := maxLines + previewTitleRows
	return previewBoxStyle.
		Width(width - 2).
		Height(height).
		MaxWidth(width).
		Render(titleStyle.Render("preview") + "\n" + text)
}

func (m teaModel) refreshPreview() (teaModel, tea.Cmd) {
	current, ok := m.list.Current()
	if !ok {
		m.previewKey = ""
		m.preview = "No preview available"
		return m, nil
	}
	key := sessionmodel.Key(current)
	if key == m.previewKey {
		return m, nil
	}
	m.previewKey = key
	m.preview = "Loading preview..."
	return m, previewCommand(key, current, m.defaultPreviewCommand)
}

func previewCommand(key string, s sessionmodel.Session, defaultPreviewCommand string) tea.Cmd {
	return func() tea.Msg {
		text, err := renderPreview(context.Background(), s, defaultPreviewCommand)
		if err != nil {
			text = err.Error()
		}
		text = strings.TrimRight(text, "\n")
		if text == "" {
			text = "No preview available"
		}
		return previewMsg{key: key, text: text}
	}
}

func previewLayout(width int) (int, int) {
	if width < previewSplitWidth {
		return width, 0
	}
	previewWidth := width / 2
	if previewWidth > maxPreviewWidth {
		previewWidth = maxPreviewWidth
	}
	if previewWidth < minPreviewWidth {
		previewWidth = minPreviewWidth
	}
	return width - previewWidth - 2, previewWidth
}

func fixedVisualLines(text string, width, count int) string {
	if count < 1 {
		count = 1
	}
	lines := strings.Split(lipgloss.NewStyle().Width(width).MaxWidth(width).Render(text), "\n")
	if len(lines) > count {
		if count == 1 {
			lines = []string{"..."}
		} else {
			lines = append(lines[:count-1], "...")
		}
	}
	for len(lines) < count {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
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
	badge := sourceBadgeStyle(source).Render(badgeText)
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

func sourceBadgeStyle(source string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(sourceBadgeColor(source))).Bold(true)
}

func sourceBadgeColor(source string) string {
	color := "244"
	switch source {
	case "herdr":
		color = "81"
	case "config":
		color = "214"
	case "zoxide":
		color = "114"
	case "dir":
		color = "176"
	}
	return color
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

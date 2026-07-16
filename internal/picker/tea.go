package picker

import (
	"context"
	"fmt"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	sessionmodel "github.com/fullerzz/herdr-plugin-sesh/internal/model"
	previewpkg "github.com/fullerzz/herdr-plugin-sesh/internal/preview"
)

const (
	defaultVisibleRows    = 12
	statusRefreshInterval = time.Second
)

const (
	defaultPrompt      = "Sesh> "
	defaultPlaceholder = "Filter workspaces"
	defaultWidth       = 80
	previewSplitWidth  = 92
	minPreviewWidth    = 36
	maxPreviewWidth    = 52
	previewTitleRows   = 1
	pickerChromeRows   = 8
	compactPreviewBody = 6
	horizontalPadding  = 2
	rowPathMinWidth    = 60
	rowSourceWidth     = 10
	rowNameMinWidth    = 12
	rowNameMaxWidth    = 28
	smearMaxLength     = 3
	smearFrameInterval = 35 * time.Millisecond
	herdrSourceIcon    = "\U000f0cc6"
	zoxideSourceIcon   = "\uf114"
	configSourceIcon   = "\ue615"
)

var (
	skyColor    = lipgloss.Color("#7DCFFF")
	violetColor = lipgloss.Color("#BB9AF7")
	greenColor  = lipgloss.Color("#9ECE6A")
	amberColor  = lipgloss.Color("#E0AF68")
	textColor   = lipgloss.Color("#C0CAF5")
	mutedColor  = lipgloss.Color("#565F89")

	titleStyle = lipgloss.NewStyle().
			Foreground(violetColor).
			Bold(true)

	countStyle = lipgloss.NewStyle().
			Foreground(mutedColor)

	sectionStyle = lipgloss.NewStyle().
			Foreground(violetColor).
			Bold(true)

	ruleStyle = lipgloss.NewStyle().
			Foreground(mutedColor)

	rowLabelStyle = lipgloss.NewStyle().
			Foreground(textColor)

	selectedLabelStyle = rowLabelStyle.Bold(true)

	matchStyle = lipgloss.NewStyle().
			Foreground(violetColor).
			Bold(true)

	selectionRailStyle = lipgloss.NewStyle().
				Foreground(skyColor).
				Bold(true)

	smearTrailStyle = lipgloss.NewStyle().
			Foreground(violetColor)

	pathStyle = lipgloss.NewStyle().
			Foreground(mutedColor)

	emptyStyle = lipgloss.NewStyle().
			Foreground(amberColor)

	moreStyle = lipgloss.NewStyle().
			Foreground(mutedColor)

	helpStyle = lipgloss.NewStyle().
			Foreground(mutedColor)
)

var renderPreview = previewpkg.Render

type Options struct {
	Output                io.Writer
	Prompt                string
	Placeholder           string
	ShowIcons             bool
	SeparatorAware        bool
	DefaultPreviewCommand string
	FZFCommand            string
	RefreshAgentStatuses  func() (map[string]string, error)
}

func Run(items []sessionmodel.Session, opts Options) (sessionmodel.Session, bool, error) {
	var popts []tea.ProgramOption
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

	smearTail    int
	smearActive  bool
	reduceMotion bool

	preview    string
	previewKey string

	defaultPreviewCommand string
	showIcons             bool
	refreshAgentStatuses  func() (map[string]string, error)
}

type previewMsg struct {
	key  string
	text string
}

type statusRefreshTickMsg struct{}

type agentStatusesMsg struct {
	statuses map[string]string
	err      error
}

type smearTickMsg struct{}

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
	styles := input.Styles()
	styles.Focused.Prompt = lipgloss.NewStyle().Foreground(skyColor).Bold(true)
	styles.Focused.Text = lipgloss.NewStyle().Foreground(textColor)
	styles.Focused.Placeholder = lipgloss.NewStyle().Foreground(mutedColor)
	styles.Cursor.Color = skyColor
	input.SetStyles(styles)
	input.Focus()
	reduceMotion := os.Getenv("HERDR_SESH_REDUCE_MOTION")
	m := teaModel{
		list:                  list,
		input:                 input,
		defaultPreviewCommand: opts.DefaultPreviewCommand,
		showIcons:             opts.ShowIcons,
		refreshAgentStatuses:  opts.RefreshAgentStatuses,
		reduceMotion:          reduceMotion == "1" || strings.EqualFold(reduceMotion, "true"),
	}
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
	if m.refreshAgentStatuses != nil {
		cmds = append(cmds, scheduleStatusRefresh())
	}
	return tea.Batch(cmds...)
}

//nolint:ireturn // Bubble Tea's Model interface requires this return shape.
func (m teaModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(statusRefreshTickMsg); ok {
		return m, refreshAgentStatusesCommand(m.refreshAgentStatuses)
	}
	if statuses, ok := msg.(agentStatusesMsg); ok {
		if statuses.err == nil {
			m.list.UpdateAgentStatuses(statuses.statuses)
		}
		return m, scheduleStatusRefresh()
	}
	if _, ok := msg.(smearTickMsg); ok {
		if !m.smearActive {
			return m, nil
		}
		if m.smearTail < m.list.Selected {
			m.smearTail++
		} else if m.smearTail > m.list.Selected {
			m.smearTail--
		}
		if m.smearTail == m.list.Selected {
			m.smearActive = false
			return m, nil
		}
		return m, smearTick()
	}
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
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		m = m.filter(m.input.Value())
		m, previewCmd := m.refreshPreview()
		return m, tea.Batch(cmd, previewCmd)
	}
	switch key.String() {
	case "ctrl+c", "esc":
		return m, tea.Quit
	case "enter":
		if choice, ok := m.list.Current(); ok {
			m.choice = choice
			m.chosen = true
		}
		return m, tea.Quit
	case "up", "ctrl+p":
		return m.moveSelection(-1)
	case "down", "ctrl+n":
		return m.moveSelection(1)
	case "ctrl+u":
		m.input.SetValue("")
		m = m.filter("")
		return m.refreshPreview()
	default:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		m = m.filter(m.input.Value())
		m, previewCmd := m.refreshPreview()
		return m, tea.Batch(cmd, previewCmd)
	}
}

func scheduleStatusRefresh() tea.Cmd {
	return tea.Tick(statusRefreshInterval, func(time.Time) tea.Msg { return statusRefreshTickMsg{} })
}

func refreshAgentStatusesCommand(refresh func() (map[string]string, error)) tea.Cmd {
	return func() tea.Msg {
		statuses, err := refresh()
		return agentStatusesMsg{statuses: statuses, err: err}
	}
}

func (m teaModel) View() tea.View {
	width := m.contentWidth()
	listWidth, previewWidth := previewLayout(width)
	lines := []string{"", m.header(width), horizontalRule(width)}
	input := m.input
	input.SetWidth(maxInt(8, width-lipgloss.Width(input.Prompt)-1))
	lines = append(lines, fitLine(input.View(), width), horizontalRule(width))

	if previewWidth > 0 {
		previewLines := m.previewBodyLines()
		list := sectionStyle.Render("WORKSPACES") + "\n" + m.listView(listWidth, previewLines)
		preview := m.previewView(previewWidth, previewLines)
		lines = append(lines, strings.Split(joinPanels(list, preview, listWidth, previewWidth), "\n")...)
	} else {
		listRows, previewLines := m.stackedBodyLines()
		lines = append(lines, sectionStyle.Render("WORKSPACES"))
		lines = append(lines, strings.Split(strings.TrimSuffix(m.listView(listWidth, listRows), "\n"), "\n")...)
		lines = append(lines, strings.Split(m.previewView(width, previewLines), "\n")...)
	}
	lines = append(lines,
		horizontalRule(width),
		helpStyle.Render("enter select   ↑/↓ move   ctrl+u clear   esc close"),
		"",
	)
	framed := make([]string, len(lines))
	for i, line := range lines {
		if line == "" {
			continue
		}
		framed[i] = strings.Repeat(" ", horizontalPadding) + fitLine(line, width) + strings.Repeat(" ", horizontalPadding)
	}
	view := tea.NewView(strings.Join(framed, "\n"))
	view.AltScreen = true
	return view
}

func (m teaModel) listView(width, visibleRows int) string {
	if visibleRows < 1 {
		return ""
	}
	lines := make([]string, 0, visibleRows)
	if len(m.list.Filtered) == 0 {
		lines = append(lines, emptyStyle.Render("No matching workspaces"))
	} else {
		start, end, moreAbove, moreBelow := listWindow(len(m.list.Filtered), m.list.Selected, visibleRows)
		if moreAbove {
			lines = append(lines, moreStyle.Render(fmt.Sprintf("↑ %d more", start)))
		}
		for i := start; i < end; i++ {
			line := strings.TrimSuffix(row(m.list.Filtered[i], i == m.list.Selected, width, m.showIcons, m.list.Query), "\n")
			if rail := m.smearRail(i); rail != "" {
				line = smearTrailStyle.Render(rail+" ") + strings.TrimPrefix(line, "  ")
			}
			lines = append(lines, line)
		}
		if moreBelow {
			lines = append(lines, moreStyle.Render(fmt.Sprintf("↓ %d more", len(m.list.Filtered)-end)))
		}
	}
	for len(lines) < visibleRows {
		lines = append(lines, "")
	}
	if len(lines) > visibleRows {
		lines = lines[:visibleRows]
	}
	for i := range lines {
		lines[i] = fitLine(lines[i], width)
	}
	return strings.Join(lines, "\n") + "\n"
}

func (m teaModel) previewBodyLines() int {
	if m.height == 0 {
		return defaultVisibleRows
	}
	lines := m.height - pickerChromeRows - previewTitleRows
	if lines < compactPreviewBody {
		return compactPreviewBody
	}
	return lines
}

func (m teaModel) stackedBodyLines() (int, int) {
	if m.height == 0 {
		return defaultVisibleRows, compactPreviewBody
	}
	const stackedChromeRows = 10
	available := m.height - stackedChromeRows
	if available < 2 {
		return 1, 1
	}
	previewLines := min(compactPreviewBody, maxInt(1, available/3))
	return maxInt(1, available-previewLines), previewLines
}

func (m teaModel) contentWidth() int {
	width := m.width
	if width == 0 {
		width = defaultWidth
	}
	return maxInt(1, width-horizontalPadding*2)
}

func (m teaModel) header(width int) string {
	title := titleStyle.Render("herdr / sesh")
	countText := fmt.Sprintf("%d workspaces", len(m.list.All))
	if m.list.Query != "" {
		countText = fmt.Sprintf("%d/%d workspaces", len(m.list.Filtered), len(m.list.All))
	}
	count := countStyle.Render(countText)
	gap := width - lipgloss.Width(title) - lipgloss.Width(count)
	if gap < 1 {
		gap = 1
	}
	return fitLine(title+strings.Repeat(" ", gap)+count, width)
}

func (m teaModel) previewView(width, maxLines int) string {
	text := strings.TrimRight(m.preview, "\n")
	if text == "" {
		text = "No preview available"
	}
	text = fixedVisualLines(text, width, maxLines)
	lines := append([]string{m.previewTitle()}, strings.Split(text, "\n")...)
	for i := range lines {
		lines[i] = fitLine(lines[i], width)
	}
	return strings.Join(lines, "\n")
}

func (m teaModel) previewTitle() string {
	title := sectionStyle.Render("PREVIEW")
	current, ok := m.list.Current()
	if !ok {
		return title
	}
	label := current.Name
	if label == "" {
		label = compactHome(current.Path)
	}
	if label != "" {
		title += countStyle.Render(" · " + label)
	}
	if _, status := agentStatusIndicator(current.AgentStatus); status != "" {
		title += agentStatusStyle(current.AgentStatus).Render(" · " + status)
	}
	return title
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

func (m teaModel) moveSelection(delta int) (teaModel, tea.Cmd) {
	previous := m.list.Selected
	m.list.Move(delta)
	var tick tea.Cmd
	if m.list.Selected != previous && !m.reduceMotion && !m.smearActive {
		m.smearTail = previous
		m.smearActive = true
		tick = smearTick()
	}
	if m.smearActive {
		m.smearTail = min(maxInt(m.smearTail, m.list.Selected-smearMaxLength), m.list.Selected+smearMaxLength)
	}
	m, previewCmd := m.refreshPreview()
	return m, tea.Batch(previewCmd, tick)
}

func (m teaModel) filter(query string) teaModel {
	queryChanged := query != m.list.Query
	m.list.Filter(query)
	if queryChanged && m.smearActive {
		m.smearTail = m.list.Selected
	}
	return m
}

func (m teaModel) smearRail(index int) string {
	selected := m.list.Selected
	if !m.smearActive || m.smearTail == selected {
		return ""
	}
	if m.smearTail < selected {
		if index < m.smearTail || index >= selected {
			return ""
		}
		if index == m.smearTail {
			return "╷"
		}
		return "│"
	}
	if index <= selected || index > m.smearTail {
		return ""
	}
	if index == m.smearTail {
		return "╵"
	}
	return "│"
}

func smearTick() tea.Cmd {
	return tea.Tick(smearFrameInterval, func(time.Time) tea.Msg { return smearTickMsg{} })
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
	if width < previewSplitWidth-horizontalPadding*2 {
		return width, 0
	}
	previewWidth := width / 2
	if previewWidth > maxPreviewWidth {
		previewWidth = maxPreviewWidth
	}
	if previewWidth < minPreviewWidth {
		previewWidth = minPreviewWidth
	}
	return width - previewWidth - 3, previewWidth
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

func row(s sessionmodel.Session, selected bool, width int, showIcons bool, query string) string {
	rail := "  "
	if selected {
		rail = selectionRailStyle.Render("┃ ")
	}
	label := s.Name
	if label == "" {
		label = compactHome(s.Path)
	}
	statusGlyph, _ := agentStatusIndicator(s.AgentStatus)
	status := "  "
	if statusGlyph != "" {
		status = agentStatusStyle(s.AgentStatus).Render(statusGlyph + " ")
	}
	badge := sourceBadgeStyle(s.Source).Render(fitPlain(sourceBadge(s.Source, showIcons), rowSourceWidth))
	remaining := maxInt(1, width-lipgloss.Width(rail)-2-rowSourceWidth)
	path := compactHome(s.Path)
	showPath := width >= rowPathMinWidth && path != "" && path != label
	nameWidth := remaining
	pathWidth := 0
	if showPath {
		available := maxInt(1, remaining-2)
		nameWidth = min(rowNameMaxWidth, maxInt(rowNameMinWidth, available*2/5))
		if nameWidth >= available {
			showPath = false
			nameWidth = remaining
		} else {
			pathWidth = available - nameWidth
		}
	}
	labelStyle := rowLabelStyle
	if selected {
		labelStyle = selectedLabelStyle
	}
	line := rail + status + badge + highlightMatches(label, query, nameWidth, labelStyle)
	if showPath {
		line += "  " + highlightMatches(path, query, pathWidth, pathStyle)
	}
	return fitLine(line, width) + "\n"
}

func highlightMatches(text, query string, width int, baseStyle lipgloss.Style) string {
	text = fitPlain(text, width)
	if query == "" {
		return baseStyle.Render(text)
	}

	textRunes := []rune(text)
	queryRunes := []rune(query)
	var rendered strings.Builder
	plainStart := 0
	for index := 0; index+len(queryRunes) <= len(textRunes); {
		matchEnd := index + len(queryRunes)
		if !strings.EqualFold(string(textRunes[index:matchEnd]), query) {
			index++
			continue
		}
		rendered.WriteString(baseStyle.Render(string(textRunes[plainStart:index])))
		rendered.WriteString(matchStyle.Render(string(textRunes[index:matchEnd])))
		index = matchEnd
		plainStart = matchEnd
	}
	rendered.WriteString(baseStyle.Render(string(textRunes[plainStart:])))
	return rendered.String()
}

func sourceBadge(source string, showIcons bool) string {
	if !showIcons {
		if source == "" {
			return "[session]"
		}
		return "[" + source + "]"
	}
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
	return lipgloss.NewStyle().Foreground(sourceBadgeTerminalColor(source)).Bold(true)
}

func sourceBadgeTerminalColor(source string) color.Color {
	color := mutedColor
	switch source {
	case "herdr":
		color = skyColor
	case "config":
		color = amberColor
	case "zoxide":
		color = greenColor
	case "dir":
		color = violetColor
	}
	return color
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

func agentStatusIndicator(status string) (string, string) {
	switch status {
	case "working":
		return "●", "working"
	case "blocked":
		return "◆", "blocked"
	case "idle":
		return "○", "idle"
	case "done":
		return "✓", "done"
	default:
		return "", ""
	}
}

func agentStatusStyle(status string) lipgloss.Style {
	color := mutedColor
	switch status {
	case "working":
		color = greenColor
	case "blocked":
		color = amberColor
	case "done":
		color = violetColor
	}
	return lipgloss.NewStyle().Foreground(color).Bold(true)
}

func compactHome(path string) string {
	if path == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	cleanPath := filepath.Clean(path)
	cleanHome := filepath.Clean(home)
	if cleanPath == cleanHome {
		return "~"
	}
	prefix := cleanHome + string(filepath.Separator)
	if strings.HasPrefix(cleanPath, prefix) {
		return "~" + cleanPath[len(cleanHome):]
	}
	return path
}

func horizontalRule(width int) string {
	return ruleStyle.Render(strings.Repeat("─", maxInt(1, width)))
}

func fitLine(line string, width int) string {
	if width < 1 {
		return ""
	}
	line = ansi.Truncate(line, width, "…")
	return line + strings.Repeat(" ", maxInt(0, width-lipgloss.Width(line)))
}

func fitPlain(text string, width int) string {
	if width < 1 {
		return ""
	}
	text = ansi.Truncate(text, width, "…")
	return text + strings.Repeat(" ", maxInt(0, width-lipgloss.Width(text)))
}

func joinPanels(left, right string, leftWidth, rightWidth int) string {
	leftLines := strings.Split(strings.TrimSuffix(left, "\n"), "\n")
	rightLines := strings.Split(strings.TrimSuffix(right, "\n"), "\n")
	height := max(len(leftLines), len(rightLines))
	lines := make([]string, height)
	divider := ruleStyle.Render("│")
	for i := range height {
		var leftLine, rightLine string
		if i < len(leftLines) {
			leftLine = leftLines[i]
		}
		if i < len(rightLines) {
			rightLine = rightLines[i]
		}
		lines[i] = fitLine(leftLine, leftWidth) + " " + divider + " " + fitLine(rightLine, rightWidth)
	}
	return strings.Join(lines, "\n")
}

func listWindow(total, selected, height int) (start, end int, moreAbove, moreBelow bool) {
	if total <= 0 || height <= 0 {
		return 0, 0, false, false
	}
	if total <= height {
		return 0, total, false, false
	}
	if height == 1 {
		selected = min(maxInt(0, selected), total-1)
		return selected, selected + 1, false, false
	}
	if height == 2 {
		selected = min(maxInt(0, selected), total-1)
		if selected < total-1 {
			return selected, selected + 1, false, true
		}
		return selected, selected + 1, true, false
	}
	if selected < height-1 {
		return 0, height - 1, false, true
	}
	if selected >= total-(height-1) {
		return total - (height - 1), total, true, false
	}
	itemRows := height - 2
	start = selected - itemRows + 1
	return start, start + itemRows, true, true
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

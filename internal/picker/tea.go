package picker

import (
	"context"
	"fmt"
	"image/color"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
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
	filterLineIndex    = 3
	listFirstRowIndex  = 6
	herdrSourceIcon    = "\U000f0cc6"
	zoxideSourceIcon   = "\uf114"
	configSourceIcon   = "\ue615"

	defaultSkyColor    = "#7DCFFF"
	defaultVioletColor = "#BB9AF7"
	defaultGreenColor  = "#9ECE6A"
	defaultAmberColor  = "#E0AF68"
	defaultRedColor    = "#F7768E"
	defaultTextColor   = "#C0CAF5"
	defaultMutedColor  = "#565F89"
	defaultGhostColor  = "#737AA2"
)

var (
	agentStatusSpinner = spinner.Jump

	skyColor    = lipgloss.Color(defaultSkyColor)
	violetColor = lipgloss.Color(defaultVioletColor)
	greenColor  = lipgloss.Color(defaultGreenColor)
	amberColor  = lipgloss.Color(defaultAmberColor)
	redColor    = lipgloss.Color(defaultRedColor)
	textColor   = lipgloss.Color(defaultTextColor)
	mutedColor  = lipgloss.Color(defaultMutedColor)
	ghostColor  = lipgloss.Color(defaultGhostColor)

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

	worktreeMarkerStyle = lipgloss.NewStyle().
				Foreground(violetColor).
				Bold(true)

	worktreeRelationStyle = lipgloss.NewStyle().
				Foreground(ghostColor)

	emptyStyle = lipgloss.NewStyle().
			Foreground(amberColor)

	moreStyle = lipgloss.NewStyle().
			Foreground(mutedColor)

	helpStyle = lipgloss.NewStyle().
			Foreground(mutedColor)
)

var renderPreview = previewpkg.Render

type Options struct {
	Context                        context.Context
	Output                         io.Writer
	Prompt                         string
	Placeholder                    string
	ShowIcons                      bool
	HerdrThemeInherit              bool
	DisableWorktreeIconReplacement bool
	HideLastWorkspace              bool
	HideLastWorkspacePath          bool
	SeparatorAware                 bool
	DefaultPreviewCommand          string
	FZFCommand                     string
	RefreshAgentStatuses           func() (map[string]string, error)
	CloseWorkspace                 func(context.Context, string) error
	// ReloadPicker refreshes picker state after a workspace close. Its
	// ReloadResult is consumed even when it returns an error: the
	// last-workspace fields must always be valid (set LastWorkspaceUnknown
	// when unsure), and a nil HerdrWorkspaces means "keep the existing
	// metadata" while an empty slice means "no workspaces".
	ReloadPicker         func(context.Context) (ReloadResult, error)
	RecentWorkspaceIDs   []string
	RecentWorkspaceSort  bool
	LastWorkspaceID      string
	LastWorkspaceUnknown bool
	HerdrWorkspaces      []sessionmodel.Session
	ReportSelection      func(seq uint64, workspaceID string)
}

type ReloadResult struct {
	Sessions             []sessionmodel.Session
	HerdrWorkspaces      []sessionmodel.Session
	LastWorkspaceID      string
	LastWorkspaceUnknown bool
}

func Run(items []sessionmodel.Session, opts Options) (sessionmodel.Session, bool, error) {
	configureHerdrTheme(opts.HerdrThemeInherit)
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
	list         Model
	input        textinput.Model
	agentSpinner spinner.Model
	width        int
	height       int
	choice       sessionmodel.Session
	chosen       bool

	listFocused         bool
	smearTail           int
	smearActive         bool
	focusSmearStart     int
	focusSmearFrame     int
	focusSmearStep      int
	focusSmearSteps     int
	focusSmearDirection int
	focusSmearActive    bool
	reduceMotion        bool
	smear               smearPreset

	preview    string
	previewKey string

	defaultPreviewCommand   string
	showIcons               bool
	replaceWorktreeIcon     bool
	hideLastWorkspace       bool
	hideLastWorkspacePath   bool
	refreshAgentStatuses    func() (map[string]string, error)
	workspaceOrder          []string
	recentWorkspaceIDs      []string
	recentSort              bool
	lastWorkspaceID         string
	lastWorkspaceUnknown    bool
	herdrWorkspaces         map[string]sessionmodel.Session
	closeWorkspace          func(context.Context, string) error
	reloadPicker            func(context.Context) (ReloadResult, error)
	workspaceCloseContext   context.Context
	closingWorkspaceID      string
	cancelWorkspaceClose    context.CancelFunc
	quitAfterWorkspaceClose bool
	closeError              string
	reportSelection         func(seq uint64, workspaceID string)
	reportSeq               uint64
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

type workspaceCloseMsg struct {
	workspaceID string
	result      ReloadResult
	reloadRan   bool
	closeErr    error
	reloadErr   error
}

type smearTickMsg struct{}

type smearPreset struct {
	name          string
	frameInterval time.Duration
	maxLength     int
	headGlyph     string
}

func newTeaModel(items []sessionmodel.Session, opts Options) teaModel {
	closeContext := opts.Context
	if closeContext == nil {
		closeContext = context.Background()
	}
	workspaceOrder := herdrWorkspaceIDs(items)
	items = append([]sessionmodel.Session(nil), items...)
	initialOrder := workspaceOrder
	if opts.RecentWorkspaceSort {
		initialOrder = opts.RecentWorkspaceIDs
	}
	sortHerdrWorkspaces(items, initialOrder)
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
		agentSpinner:          spinner.New(spinner.WithSpinner(agentStatusSpinner)),
		defaultPreviewCommand: opts.DefaultPreviewCommand,
		showIcons:             opts.ShowIcons,
		replaceWorktreeIcon:   !opts.DisableWorktreeIconReplacement,
		hideLastWorkspace:     opts.HideLastWorkspace,
		hideLastWorkspacePath: opts.HideLastWorkspacePath,
		refreshAgentStatuses:  opts.RefreshAgentStatuses,
		workspaceOrder:        workspaceOrder,
		recentWorkspaceIDs:    append([]string(nil), opts.RecentWorkspaceIDs...),
		recentSort:            opts.RecentWorkspaceSort,
		lastWorkspaceID:       opts.LastWorkspaceID,
		lastWorkspaceUnknown:  opts.LastWorkspaceUnknown,
		herdrWorkspaces:       workspaceSessionsByID(opts.HerdrWorkspaces),
		closeWorkspace:        opts.CloseWorkspace,
		reloadPicker:          opts.ReloadPicker,
		workspaceCloseContext: closeContext,
		reduceMotion:          reduceMotion == "1" || strings.EqualFold(reduceMotion, "true"),
		smear:                 newSmearPreset(os.Getenv("HERDR_SESH_SMEAR_PRESET")),
		reportSelection:       opts.ReportSelection,
		reportSeq:             1,
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
		cmds = append(cmds, reportSelectionCommand(m.reportSelection, m.reportSeq, current.WorkspaceID))
	}
	if m.refreshAgentStatuses != nil {
		cmds = append(cmds, scheduleStatusRefresh(), m.agentSpinner.Tick)
	}
	return tea.Batch(cmds...)
}

func newSmearPreset(name string) smearPreset {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "gooey":
		return smearPreset{name: "gooey", frameInterval: 45 * time.Millisecond, maxLength: 4, headGlyph: "█"}
	case "ghost":
		return smearPreset{name: "ghost", frameInterval: 55 * time.Millisecond, maxLength: 3, headGlyph: "◆"}
	default:
		return smearPreset{name: "crisp", frameInterval: 22 * time.Millisecond, maxLength: 2, headGlyph: "┃"}
	}
}

func (p smearPreset) column(start, step, steps, direction int) int {
	remaining := steps - step
	switch p.name {
	case "gooey":
		if direction < 0 {
			return start * (steps*steps - step*step) / (steps * steps)
		}
		return start * remaining * remaining / (steps * steps)
	case "ghost":
		return start * (steps*steps*steps - 3*step*step*steps + 2*step*step*step) / (steps * steps * steps)
	default:
		return start * remaining / steps
	}
}

func (p smearPreset) headStyle() lipgloss.Style {
	if p.name == "ghost" {
		return lipgloss.NewStyle().Foreground(ghostColor)
	}
	return selectionRailStyle
}

func (p smearPreset) trailStyle(age int) lipgloss.Style {
	switch p.name {
	case "gooey":
		if age > 1 {
			return lipgloss.NewStyle().Foreground(mutedColor)
		}
		return smearTrailStyle
	case "ghost":
		return lipgloss.NewStyle().Foreground(ghostColor)
	default:
		return smearTrailStyle
	}
}

func (p smearPreset) trailGlyph(age int, diagonal bool) string {
	switch p.name {
	case "gooey":
		if age > 1 {
			return "▒"
		}
		return "▓"
	case "ghost":
		return "·"
	default:
		if diagonal {
			return "╱"
		}
		return "│"
	}
}

//nolint:gocyclo,ireturn // Bubble Tea's central event dispatcher requires this return shape.
func (m teaModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(spinner.TickMsg); ok {
		var cmd tea.Cmd
		m.agentSpinner, cmd = m.agentSpinner.Update(msg)
		return m, cmd
	}
	if _, ok := msg.(statusRefreshTickMsg); ok {
		return m, refreshAgentStatusesCommand(m.refreshAgentStatuses)
	}
	if statuses, ok := msg.(agentStatusesMsg); ok {
		if statuses.err == nil {
			m.list.UpdateAgentStatuses(statuses.statuses)
		}
		// Re-report the current selection so its metadata token outlives its TTL.
		m.reportSeq++
		return m, tea.Batch(scheduleStatusRefresh(), reportSelectionCommand(m.reportSelection, m.reportSeq, m.currentWorkspaceID()))
	}
	if _, ok := msg.(smearTickMsg); ok {
		if m.focusSmearActive {
			m.focusSmearFrame++
			traveled := acceleratedFocusSmearStep(m.focusSmearSteps, m.focusSmearFrame)
			m.focusSmearStep = traveled
			if m.focusSmearDirection < 0 {
				m.focusSmearStep = m.focusSmearSteps - traveled
			}
			if m.focusSmearFrame >= focusSmearFrameCount(m.focusSmearSteps) {
				m.focusSmearActive = false
				if m.focusSmearDirection < 0 {
					return m.focusInput()
				}
				return m, nil
			}
			return m, m.smearTick()
		}
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
		return m, m.smearTick()
	}
	if preview, ok := msg.(previewMsg); ok {
		if preview.key == m.previewKey {
			m.preview = preview.text
		}
		return m, nil
	}
	if closed, ok := msg.(workspaceCloseMsg); ok {
		if closed.workspaceID != m.closingWorkspaceID {
			return m, nil
		}
		m.closingWorkspaceID = ""
		m.cancelWorkspaceClose = nil
		if m.quitAfterWorkspaceClose {
			m.quitAfterWorkspaceClose = false
			return m, tea.Quit
		}
		if closed.closeErr != nil {
			m.closeError = fmt.Sprintf("Failed to close workspace: %v", closed.closeErr)
			return m.refreshPreview()
		}
		if closed.reloadRan {
			m.lastWorkspaceID = closed.result.LastWorkspaceID
			m.lastWorkspaceUnknown = closed.result.LastWorkspaceUnknown
		}
		if closed.result.HerdrWorkspaces != nil {
			m.herdrWorkspaces = workspaceSessionsByID(closed.result.HerdrWorkspaces)
		}
		selectedKey := ""
		if current, currentOK := m.list.Current(); currentOK {
			selectedKey = sessionmodel.Key(current)
		}
		if closed.reloadErr == nil && closed.result.Sessions != nil {
			m.workspaceOrder = herdrWorkspaceIDs(closed.result.Sessions)
			m.list.All = append(m.list.All[:0], closed.result.Sessions...)
		} else {
			remaining := m.list.All[:0]
			for _, item := range m.list.All {
				if item.Source == "herdr" && item.WorkspaceID == closed.workspaceID {
					continue
				}
				if item.Worktree.ParentWorkspaceID == closed.workspaceID {
					item.Worktree.ParentWorkspaceID = ""
					item.Worktree.ParentWorkspaceName = ""
				}
				remaining = append(remaining, item)
			}
			m.list.All = remaining
		}
		m.resortWorkspaces()
		m.list.Filter(m.list.Query)
		for i, item := range m.list.Filtered {
			if sessionmodel.Key(item) == selectedKey {
				m.list.Selected = i
				break
			}
		}
		if closed.reloadErr != nil {
			m.closeError = fmt.Sprintf("Workspace closed, but sessions could not be refreshed: %v", closed.reloadErr)
		}
		return m.refreshPreview()
	}
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = size.Width
		m.height = size.Height
		return m, nil
	}
	if _, ok := msg.(tea.PasteMsg); ok {
		return m.updateInput(msg)
	}
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		m = m.filter(m.input.Value())
		m, previewCmd := m.refreshPreview()
		return m, tea.Batch(cmd, previewCmd)
	}
	m.closeError = ""
	switch key.String() {
	case "ctrl+c", "esc":
		if m.closingWorkspaceID != "" {
			m.quitAfterWorkspaceClose = true
			m.cancelWorkspaceClose()
			m.preview = "Cancelling workspace close..."
			return m, nil
		}
		return m, tea.Quit
	case "enter":
		if m.closingWorkspaceID != "" {
			return m, nil
		}
		if choice, ok := m.list.Current(); ok {
			m.choice = choice
			m.chosen = true
		}
		return m, tea.Quit
	case "up", "ctrl+p", "ctrl+k":
		if !m.listFocused {
			if key.String() == "ctrl+k" {
				return m.updateInput(msg)
			}
			return m, nil
		}
		if m.list.Selected == 0 {
			return m.smearToInput()
		}
		m.focusSmearActive = false
		return m.moveSelection(-1)
	case "down", "ctrl+n", "ctrl+j":
		if !m.listFocused {
			return m.focusList()
		}
		m.focusSmearActive = false
		return m.moveSelection(1)
	case "ctrl+u":
		var focusCmd tea.Cmd
		if m.listFocused {
			m, focusCmd = m.focusInput()
		}
		m.input.SetValue("")
		m = m.filter("")
		m, previewCmd := m.refreshPreview()
		return m, tea.Batch(focusCmd, previewCmd)
	case "ctrl+r":
		return m.toggleWorkspaceSort()
	case "ctrl+x":
		return m.closeSelectedWorkspace()
	case "right":
		if m.listFocused {
			return m.smearToInput()
		}
		fallthrough
	default:
		return m.updateInput(msg)
	}
}

func (m teaModel) closeSelectedWorkspace() (teaModel, tea.Cmd) {
	current, ok := m.list.Current()
	if !ok || current.Source != "herdr" || current.WorkspaceID == "" || m.closeWorkspace == nil || m.closingWorkspaceID != "" {
		return m, nil
	}
	m.closingWorkspaceID = current.WorkspaceID
	m.closeError = ""
	m.previewKey = ""
	m.preview = "Closing workspace..."
	closeCtx, cancel := context.WithCancel(m.workspaceCloseContext)
	m.cancelWorkspaceClose = cancel
	return m, closeWorkspaceCommand(closeCtx, cancel, m.closeWorkspace, m.reloadPicker, current.WorkspaceID)
}

func closeWorkspaceCommand(
	ctx context.Context,
	cancel context.CancelFunc,
	closeWorkspace func(context.Context, string) error,
	reloadPicker func(context.Context) (ReloadResult, error),
	workspaceID string,
) tea.Cmd {
	return func() tea.Msg {
		defer cancel()
		msg := workspaceCloseMsg{workspaceID: workspaceID, closeErr: closeWorkspace(ctx, workspaceID)}
		if msg.closeErr == nil && reloadPicker != nil {
			msg.result, msg.reloadErr = reloadPicker(ctx)
			msg.reloadRan = true
		}
		return msg
	}
}

func (m teaModel) resortWorkspaces() {
	order := m.workspaceOrder
	if m.recentSort {
		order = m.recentWorkspaceIDs
	}
	sortHerdrWorkspaces(m.list.All, order)
}

func (m teaModel) toggleWorkspaceSort() (teaModel, tea.Cmd) {
	m.recentSort = !m.recentSort
	m.resortWorkspaces()
	m.list.Selected = 0
	m.list.Filter(m.list.Query)
	m.smearActive = false
	m.focusSmearActive = false
	return m.refreshPreview()
}

func (m teaModel) updateInput(msg tea.Msg) (teaModel, tea.Cmd) {
	var focusCmd tea.Cmd
	smearing := false
	if m.listFocused {
		m, focusCmd = m.smearToInput()
		smearing = m.focusSmearActive
		if smearing {
			m.input.Focus()
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if smearing {
		m.input.Blur()
		m.focusSmearStart = m.inputCursorColumn()
	}
	m = m.filter(m.input.Value())
	m, previewCmd := m.refreshPreview()
	return m, tea.Batch(focusCmd, cmd, previewCmd)
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
	sortMode := "workspace"
	if m.recentSort {
		sortMode = "recent"
	}
	footer := helpStyle.Render(fmt.Sprintf("enter select · ctrl+j/k · ctrl+r %s · ctrl+x close · esc exit", sortMode))
	if m.closeError != "" {
		footer = emptyStyle.Render(m.closeError)
	}
	lines = append(lines,
		horizontalRule(width),
		m.footerLine(footer, width),
		"",
	)
	for i, line := range lines {
		if line != "" {
			lines[i] = fitLine(line, width)
		}
	}
	m.renderFocusSmear(lines, width)
	framed := make([]string, len(lines))
	for i, line := range lines {
		if line == "" {
			continue
		}
		framed[i] = strings.Repeat(" ", horizontalPadding) + line + strings.Repeat(" ", horizontalPadding)
	}
	view := tea.NewView(strings.Join(framed, "\n"))
	view.AltScreen = true
	return view
}

func herdrWorkspaceIDs(items []sessionmodel.Session) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		if item.Source == "herdr" {
			ids = append(ids, item.WorkspaceID)
		}
	}
	return ids
}

func sortHerdrWorkspaces(items []sessionmodel.Session, order []string) {
	ranks := make(map[string]int, len(order))
	for i, id := range order {
		if _, exists := ranks[id]; id != "" && !exists {
			ranks[id] = i
		}
	}

	type rankedWorkspace struct {
		session sessionmodel.Session
		rank    int
		family  int
		parent  bool
	}
	workspaces := make([]rankedWorkspace, 0, len(items))
	byID := make(map[string]struct{})
	for _, item := range items {
		if item.Source != "herdr" {
			continue
		}
		// Unranked workspaces get unique ranks past the order list, preserving
		// their input order and sorting them after every ranked workspace.
		rank, ranked := ranks[item.WorkspaceID]
		if !ranked {
			rank = len(order) + len(workspaces)
		}
		workspaces = append(workspaces, rankedWorkspace{session: item, rank: rank})
		if item.WorkspaceID != "" {
			byID[item.WorkspaceID] = struct{}{}
		}
	}

	// A family is a parent workspace plus its linked worktrees; a family's rank
	// is its best member rank. Ranks are unique, so family ranks are too.
	familyOrdinals := make(map[string]int, len(workspaces))
	familyRanks := make([]int, 0, len(workspaces))
	for i := range workspaces {
		entry := &workspaces[i]
		familyID := entry.session.WorkspaceID
		parentID := entry.session.Worktree.ParentWorkspaceID
		if _, parentPresent := byID[parentID]; parentID != "" && parentPresent {
			familyID = parentID
		}
		entry.parent = entry.session.WorkspaceID == familyID
		ordinal, exists := familyOrdinals[familyID]
		if familyID == "" || !exists {
			ordinal = len(familyRanks)
			familyRanks = append(familyRanks, entry.rank)
			if familyID != "" {
				familyOrdinals[familyID] = ordinal
			}
		} else if entry.rank < familyRanks[ordinal] {
			familyRanks[ordinal] = entry.rank
		}
		entry.family = ordinal
	}

	sort.SliceStable(workspaces, func(i, j int) bool {
		a, b := workspaces[i], workspaces[j]
		if a.family != b.family {
			return familyRanks[a.family] < familyRanks[b.family]
		}
		if a.parent != b.parent {
			return a.parent
		}
		return a.rank < b.rank
	})

	workspace := 0
	for i := range items {
		if items[i].Source == "herdr" {
			items[i] = workspaces[workspace].session
			workspace++
		}
	}
}

func workspaceSessionsByID(items []sessionmodel.Session) map[string]sessionmodel.Session {
	workspaces := make(map[string]sessionmodel.Session, len(items))
	for _, item := range items {
		if item.Source == "herdr" && item.WorkspaceID != "" {
			workspaces[item.WorkspaceID] = item
		}
	}
	return workspaces
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
			selected := m.listFocused && !m.focusSmearActive && i == m.list.Selected
			selectedRail := m.smear.headStyle().Render(m.smear.headGlyph + " ")
			treePrefix := worktreeTreePrefix(m.list.Filtered, i)
			line := strings.TrimSuffix(rowWithRail(m.list.Filtered[i], selected, width, m.showIcons, m.replaceWorktreeIcon, m.list.Query, selectedRail, m.agentSpinner.View(), treePrefix), "\n")
			if rail, age := m.smearRail(i); rail != "" {
				line = m.smear.trailStyle(age).Render(rail+" ") + strings.TrimPrefix(line, "  ")
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

func (m teaModel) lastWorkspaceStatus() (label, path string) {
	label = "None recorded"
	if m.lastWorkspaceUnknown {
		label = "Unavailable"
	} else if m.lastWorkspaceID != "" {
		label = m.lastWorkspaceID
		if item, ok := m.herdrWorkspaces[m.lastWorkspaceID]; ok {
			if item.Name != "" {
				label = item.Name
			} else if item.Path != "" {
				label = compactHome(item.Path)
			}
			path = item.Path
		}
	}
	return label, path
}

func (m teaModel) lastWorkspaceText() string {
	label, path := m.lastWorkspaceStatus()
	line := sectionStyle.Render("LAST WORKSPACE") + countStyle.Render(" · ") + rowLabelStyle.Render(label)
	displayPath := compactHome(path)
	if !m.hideLastWorkspacePath && displayPath != "" && displayPath != label {
		line += pathStyle.Render("  " + displayPath)
	}
	return line
}

func (m teaModel) lastWorkspaceCompactText() string {
	label, _ := m.lastWorkspaceStatus()
	return sectionStyle.Render("last:") + rowLabelStyle.Render(" "+label)
}

// footerLine keeps the keybind help (or a close error, which gets the whole
// row) intact and fits the last-workspace status into the remaining width,
// compacting or dropping it rather than truncating the help.
func (m teaModel) footerLine(footer string, width int) string {
	if m.closeError != "" || m.hideLastWorkspace {
		return fitLine(footer, width)
	}
	available := width - lipgloss.Width(footer) - 2
	last := m.lastWorkspaceText()
	if lipgloss.Width(last) > available {
		last = m.lastWorkspaceCompactText()
	}
	if lipgloss.Width(last) > available {
		return fitLine(footer, width)
	}
	gap := maxInt(1, width-lipgloss.Width(footer)-lipgloss.Width(last))
	return fitLine(footer+strings.Repeat(" ", gap)+last, width)
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
	if relation := worktreeDescription(current); relation != "" {
		title += worktreeRelationStyle.Render(" · " + relation)
	}
	if _, status := agentStatusIndicator(current.AgentStatus, m.agentSpinner.View()); status != "" {
		title += agentStatusStyle(current.AgentStatus).Render(" · " + status)
	}
	return title
}

func (m teaModel) refreshPreview() (teaModel, tea.Cmd) {
	current, ok := m.list.Current()
	if !ok {
		hadSelection := m.previewKey != ""
		m.previewKey = ""
		m.preview = "No preview available"
		if hadSelection {
			m.reportSeq++
			return m, reportSelectionCommand(m.reportSelection, m.reportSeq, "")
		}
		return m, nil
	}
	key := sessionmodel.Key(current)
	if key == m.previewKey {
		return m, nil
	}
	m.previewKey = key
	m.preview = "Loading preview..."
	m.reportSeq++
	return m, tea.Batch(
		previewCommand(key, current, m.defaultPreviewCommand),
		reportSelectionCommand(m.reportSelection, m.reportSeq, current.WorkspaceID),
	)
}

func (m teaModel) currentWorkspaceID() string {
	if current, ok := m.list.Current(); ok {
		return current.WorkspaceID
	}
	return ""
}

// seq is assigned here, inside the single-threaded update loop, because the
// returned commands run on concurrent goroutines with no ordering guarantee;
// the receiver uses it to drop reports that arrive out of order.
func reportSelectionCommand(report func(uint64, string), seq uint64, workspaceID string) tea.Cmd {
	if report == nil {
		return nil
	}
	return func() tea.Msg {
		report(seq, workspaceID)
		return nil
	}
}

func (m teaModel) focusList() (teaModel, tea.Cmd) {
	if _, ok := m.list.Current(); !ok {
		return m, nil
	}
	m.listFocused = true
	m.focusSmearStart = m.inputCursorColumn()
	m.input.Blur()
	if m.reduceMotion {
		return m, nil
	}
	m.focusSmearFrame = 0
	m.focusSmearStep = 0
	m.focusSmearSteps = m.focusSmearDistance()
	m.focusSmearDirection = 1
	m.focusSmearActive = true
	return m, m.smearTick()
}

func (m teaModel) smearToInput() (teaModel, tea.Cmd) {
	if m.reduceMotion {
		return m.focusInput()
	}
	if m.focusSmearActive && m.focusSmearDirection < 0 {
		return m, nil
	}
	m.focusSmearStart = m.inputCursorColumn()
	m.focusSmearFrame = 0
	m.focusSmearSteps = m.focusSmearDistance()
	m.focusSmearStep = m.focusSmearSteps
	m.focusSmearDirection = -1
	m.focusSmearActive = true
	return m, m.smearTick()
}

func (m teaModel) focusSmearDistance() int {
	visibleRows := m.previewBodyLines()
	if _, previewWidth := previewLayout(m.contentWidth()); previewWidth == 0 {
		visibleRows, _ = m.stackedBodyLines()
	}
	start, _, moreAbove, _ := listWindow(len(m.list.Filtered), m.list.Selected, visibleRows)
	selectedLine := m.list.Selected - start
	if moreAbove {
		selectedLine++
	}
	return maxInt(1, listFirstRowIndex+selectedLine-filterLineIndex)
}

func focusSmearFrameCount(distance int) int {
	// Constant acceleration keeps travel time proportional to sqrt(distance).
	return min(distance, maxInt(1, int(math.Ceil(math.Sqrt(2*float64(distance))))))
}

func acceleratedFocusSmearStep(distance, frame int) int {
	frames := focusSmearFrameCount(distance)
	frame = min(frame, frames)
	denominator := frames * frames
	if frame*2 <= frames {
		return (2*distance*frame*frame + denominator/2) / denominator
	}
	remaining := frames - frame
	return distance - (2*distance*remaining*remaining+denominator/2)/denominator
}

func (m teaModel) focusInput() (teaModel, tea.Cmd) {
	m.listFocused = false
	m.smearActive = false
	m.focusSmearActive = false
	return m, m.input.Focus()
}

func (m teaModel) inputCursorColumn() int {
	value := []rune(m.input.Value())
	position := min(m.input.Position(), len(value))
	column := lipgloss.Width(m.input.Prompt) + lipgloss.Width(string(value[:position]))
	return min(column, m.contentWidth()-1)
}

func (m teaModel) renderFocusSmear(lines []string, width int) {
	if !m.focusSmearActive || m.focusSmearSteps < 1 {
		return
	}
	head := min(m.focusSmearStep, m.focusSmearSteps)
	start, end := maxInt(0, head-m.smear.maxLength+1), head
	if m.focusSmearDirection < 0 {
		start, end = head, min(m.focusSmearSteps, head+m.smear.maxLength-1)
	}
	for step := start; step <= end; step++ {
		lineIndex := filterLineIndex + step
		if lineIndex < 0 || lineIndex >= len(lines) {
			continue
		}
		column := m.smear.column(m.focusSmearStart, step, m.focusSmearSteps, m.focusSmearDirection)
		glyph := m.smear.headStyle().Render(m.smear.headGlyph)
		if step != head {
			nextStep := step + m.focusSmearDirection
			nextColumn := m.smear.column(m.focusSmearStart, nextStep, m.focusSmearSteps, m.focusSmearDirection)
			age := step - head
			if age < 0 {
				age = -age
			}
			glyph = m.smear.trailStyle(age).Render(m.smear.trailGlyph(age, nextColumn != column))
		}
		lines[lineIndex] = overlayCell(lines[lineIndex], column, glyph, width)
	}
}

func (m teaModel) moveSelection(delta int) (teaModel, tea.Cmd) {
	previous := m.list.Selected
	m.list.Move(delta)
	var tick tea.Cmd
	if m.list.Selected != previous && !m.reduceMotion && !m.smearActive {
		m.smearTail = previous
		m.smearActive = true
		tick = m.smearTick()
	}
	if m.smearActive {
		m.smearTail = min(maxInt(m.smearTail, m.list.Selected-m.smear.maxLength), m.list.Selected+m.smear.maxLength)
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

func (m teaModel) smearRail(index int) (string, int) {
	selected := m.list.Selected
	if !m.smearActive || m.smearTail == selected {
		return "", 0
	}
	age := selected - index
	if age < 0 {
		age = -age
	}
	if m.smearTail < selected {
		if index < m.smearTail || index >= selected {
			return "", 0
		}
		if m.smear.name != "crisp" {
			return m.smear.trailGlyph(age, false), age
		}
		if index == m.smearTail {
			return "╷", age
		}
		return "│", age
	}
	if index <= selected || index > m.smearTail {
		return "", 0
	}
	if m.smear.name != "crisp" {
		return m.smear.trailGlyph(age, false), age
	}
	if index == m.smearTail {
		return "╵", age
	}
	return "│", age
}

func (m teaModel) smearTick() tea.Cmd {
	return tea.Tick(m.smear.frameInterval, func(time.Time) tea.Msg { return smearTickMsg{} })
}

func overlayCell(line string, column int, cell string, width int) string {
	column = min(maxInt(0, column), width-1)
	line = fitLine(line, width)
	return fitLine(ansi.Cut(line, 0, column)+cell+ansi.Cut(line, column+1, width), width)
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
	return rowWithRail(s, selected, width, showIcons, true, query, selectionRailStyle.Render("┃ "), agentStatusSpinner.Frames[0], "")
}

func rowWithRail(s sessionmodel.Session, selected bool, width int, showIcons, replaceWorktreeIcon bool, query, selectedRail, workingGlyph, treePrefix string) string {
	rail := "  "
	if selected {
		rail = selectedRail
	}
	label := s.Name
	if label == "" {
		label = compactHome(s.Path)
	}
	statusGlyph, _ := agentStatusIndicator(s.AgentStatus, workingGlyph)
	status := "  "
	if statusGlyph != "" {
		status = agentStatusStyle(s.AgentStatus).Render(statusGlyph + " ")
	}
	// Status is always two cells: a glyph plus space, or two blanks.
	fixedWidth := lipgloss.Width(rail) + 2
	badgeText := sessionSourceBadge(s, showIcons, replaceWorktreeIcon)
	badgeWidth := rowSourceWidth
	if s.Worktree.Linked && replaceWorktreeIcon {
		if width <= fixedWidth+2 {
			return compactWorktreeRow(rail, status, width, fixedWidth)
		}
		badgeWidth = min(rowSourceWidth, maxInt(1, width-fixedWidth-1))
		if badgeWidth < lipgloss.Width(badgeText) {
			badgeText = "↳ herdr"
		}
	}
	badge := sessionSourceBadgeStyle(s).Render(fitPlain(badgeText, badgeWidth))
	remaining := maxInt(1, width-fixedWidth-badgeWidth)
	path := compactHome(s.Path)
	if path == label {
		path = ""
	}
	showSecondary := width >= rowPathMinWidth && path != ""
	nameWidth := remaining
	secondaryWidth := 0
	if showSecondary {
		available := maxInt(1, remaining-2)
		nameWidth = min(rowNameMaxWidth, maxInt(rowNameMinWidth, available*2/5))
		if nameWidth >= available {
			showSecondary = false
			nameWidth = remaining
		} else {
			secondaryWidth = available - nameWidth
		}
	}
	labelStyle := rowLabelStyle
	if selected {
		labelStyle = selectedLabelStyle
	}
	tree := worktreeMarkerStyle.Render(treePrefix)
	labelWidth := maxInt(1, nameWidth-lipgloss.Width(tree))
	line := rail + status + badge + tree + highlightMatches(label, query, labelWidth, labelStyle)
	if showSecondary {
		line += "  " + highlightMatches(path, query, secondaryWidth, pathStyle)
	}
	return fitLine(line, width) + "\n"
}

func worktreeTreePrefix(items []sessionmodel.Session, index int) string {
	if index < 0 || index >= len(items) {
		return ""
	}
	child := items[index]
	parentID := child.Worktree.ParentWorkspaceID
	if !child.Worktree.Linked || parentID == "" {
		return ""
	}
	parentIndex := -1
	for i, item := range items {
		if item.Source == "herdr" && item.WorkspaceID == parentID {
			parentIndex = i
			break
		}
	}
	if parentIndex < 0 || parentIndex >= index {
		return ""
	}
	for i := parentIndex + 1; i < index; i++ {
		if !items[i].Worktree.Linked || items[i].Worktree.ParentWorkspaceID != parentID {
			return ""
		}
	}
	if index+1 < len(items) && items[index+1].Worktree.Linked && items[index+1].Worktree.ParentWorkspaceID == parentID {
		return "├─ "
	}
	return "└─ "
}

func compactWorktreeRow(rail, status string, width, fixedWidth int) string {
	marker := worktreeMarkerStyle.Render("↳")
	if width <= fixedWidth {
		return fitLine(marker, width) + "\n"
	}
	return fitLine(rail+status, width-1) + marker + "\n"
}

func worktreeDescription(s sessionmodel.Session) string {
	if !s.Worktree.Linked {
		return ""
	}
	if s.Worktree.ParentWorkspaceName != "" {
		return "worktree of " + s.Worktree.ParentWorkspaceName
	}
	return "linked worktree"
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

func sessionSourceBadge(s sessionmodel.Session, showIcons, replaceWorktreeIcon bool) string {
	if s.Source == "herdr" && s.Worktree.Linked && replaceWorktreeIcon {
		if showIcons {
			return "↳ herdr"
		}
		return "[↳ herdr]"
	}
	return sourceBadge(s.Source, showIcons)
}

func sessionSourceBadgeStyle(s sessionmodel.Session) lipgloss.Style {
	if s.Source == "herdr" && s.Worktree.Linked {
		return lipgloss.NewStyle().Foreground(violetColor).Bold(true)
	}
	return sourceBadgeStyle(s.Source)
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

func agentStatusIndicator(status, workingGlyph string) (string, string) {
	switch status {
	case "working":
		return workingGlyph, "working"
	case "blocked":
		return "◉", "blocked"
	case "idle":
		return "✓", "idle"
	case "done":
		return "●", "done"
	default:
		return "", ""
	}
}

func agentStatusStyle(status string) lipgloss.Style {
	color := mutedColor
	switch status {
	case "working":
		color = amberColor
	case "blocked":
		color = redColor
	case "idle":
		color = greenColor
	case "done":
		color = skyColor
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

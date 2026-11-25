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

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/fullerzz/herdr-plugin-sesh/internal/config"
	sessionmodel "github.com/fullerzz/herdr-plugin-sesh/internal/model"
	previewpkg "github.com/fullerzz/herdr-plugin-sesh/internal/preview"
	"github.com/fullerzz/herdr-plugin-sesh/internal/settings"
	"github.com/fullerzz/herdr-plugin-sesh/internal/uitheme"
)

const (
	statusRefreshInterval      = time.Second
	panePreviewRefreshInterval = time.Second

	workspaceSortWorkspace = "workspace"
	workspaceSortRecent    = "recent"
	workspaceSortAgent     = "agent"
)

const (
	defaultPrompt      = "Sesh> "
	defaultPlaceholder = "Filter workspaces"
	defaultWidth       = 80
	defaultHeight      = 24
	previewSplitWidth  = 92
	minPreviewWidth    = 36
	minListWidth       = 36
	maxPreviewWidth    = 52
	compactPreviewBody = 6
	horizontalPadding  = 2
	rowPathMinWidth    = 60
	rowSourceWidth     = 10
	rowNameMinWidth    = 12
	rowNameMaxWidth    = 28
	herdrSourceIcon    = "\U000f0cc6"
	zoxideSourceIcon   = "\uf114"
	configSourceIcon   = "\ue615"

	defaultSkyColor    = uitheme.DefaultSkyColor
	defaultVioletColor = uitheme.DefaultVioletColor
	defaultGreenColor  = uitheme.DefaultGreenColor
	defaultAmberColor  = uitheme.DefaultAmberColor
	defaultRedColor    = uitheme.DefaultRedColor
	defaultTextColor   = uitheme.DefaultTextColor
	defaultMutedColor  = uitheme.DefaultMutedColor
	defaultGhostColor  = uitheme.DefaultGhostColor
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

	helpStyle = lipgloss.NewStyle().
			Foreground(mutedColor)
)

var renderPreview = previewpkg.Render
var renderPanePreview = previewpkg.RenderPane

type Backend interface {
	CloseWorkspace(context.Context, string) error
	ReloadPicker(context.Context) (ReloadResult, error)
	RefreshAgentStatuses() (map[string]string, error)
	OpenSettings() (settings.Model, error)
	ReloadSettings(context.Context, settings.Result) (DisplayOptions, ReloadResult, error)
}

type Options struct {
	DisplayOptions

	Backend Backend
}

type DisplayOptions struct {
	// nil uses the default binding; an empty string disables cycling.
	CyclePreviewModeKey            *string
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
	DisableHomePrioritization      bool
	HidePath                       bool
	HidePreview                    bool
	PreviewMode                    string
	DefaultPreviewCommand          string
	FZFCommand                     string
	RecentWorkspaceIDs             []string
	WorkspaceSort                  string
	LastWorkspaceID                string
	LastWorkspaceUnknown           bool
	HerdrWorkspaces                []sessionmodel.Session
}

// Nil metadata slices preserve existing picker metadata; empty slices clear it.
type ReloadResult struct {
	RecentWorkspaceIDs   []string
	Sessions             []sessionmodel.Session
	HerdrWorkspaces      []sessionmodel.Session
	LastWorkspaceID      string
	LastWorkspaceUnknown bool
}

func Run(items []sessionmodel.Session, opts Options) (sessionmodel.Session, bool, error) {
	uitheme.DetectHostAppearance()
	configureHerdrTheme(opts.HerdrThemeInherit)
	configureMotion()
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
	ui                *bloom.Root
	backend           Backend
	settings          *settings.Model
	settingsBusy      bool
	settingsCancel    context.CancelFunc
	quitAfterSettings bool
	refreshGeneration uint64
	list              Model
	input             textinput.Model
	agentSpinner      spinner.Model
	width             int
	height            int
	choice            sessionmodel.Session
	chosen            bool

	listFocused         bool
	smearTail           int
	smearActive         bool
	focusSmearStart     int
	focusSmearFrame     int
	focusSmearStep      int
	focusSmearSteps     int
	focusSmearInputRow  int
	focusSmearDirection int
	focusSmearActive    bool
	reduceMotion        bool
	smear               smearPreset

	preview              string
	previewLoading       bool
	previewWidth         int
	pointer              *pointerInput
	previewScroll        *previewViewport
	listOffset           int
	previewKey           string
	previewParentContext context.Context
	previewContext       context.Context
	cancelPreview        context.CancelFunc
	previewRequestID     uint64
	panePreview          bool
	cyclePreviewModeKey  string

	defaultPreviewCommand   string
	hidePath                bool
	hidePreview             bool
	showIcons               bool
	replaceWorktreeIcon     bool
	hideLastWorkspace       bool
	hideLastWorkspacePath   bool
	workspaceOrder          []string
	recentWorkspaceIDs      []string
	workspaceSort           string
	lastWorkspaceID         string
	lastWorkspaceUnknown    bool
	herdrWorkspaces         map[string]sessionmodel.Session
	workspaceCloseContext   context.Context
	closingWorkspaceID      string
	cancelWorkspaceClose    context.CancelFunc
	quitAfterWorkspaceClose bool
	closeError              string
	// toastText outlives closeError so the toast keeps its text while hiding.
	toast     anim.Presence
	toastText string
	shimmer   anim.Sequence

	// previewIn fades in text for another workspace or preview mode.
	previewIn anim.Value
	// countRoll turns the workspace count over from countFrom.
	countRoll anim.Value
	countFrom string
	// shake wobbles the list when a key is refused.
	shake anim.Value
}

const (
	previewInDuration    = 220 * time.Millisecond
	countRollDuration    = 300 * time.Millisecond
	refusedShakeDuration = 360 * time.Millisecond
)

// oneShot is an effect's progress, at rest at 1 until it plays from 0.
func oneShot(d time.Duration) anim.Value {
	return anim.NewValue(1, anim.Duration(d), anim.Ease(anim.Linear))
}

type previewMsg struct {
	key       string
	requestID uint64
	text      string
}

type panePreviewTickMsg struct{ requestID uint64 }

type previewLoadingMsg struct{ requestID uint64 }

type statusRefreshTickMsg struct{ generation uint64 }

type agentStatusesMsg struct {
	generation uint64
	statuses   map[string]string
	err        error
}

type workspaceClickedMsg struct{ key string }

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
	workspaceSort := normalizeWorkspaceSort(opts.WorkspaceSort)
	initialOrder := workspaceOrder
	switch workspaceSort {
	case workspaceSortRecent:
		initialOrder = opts.RecentWorkspaceIDs
	case workspaceSortAgent:
		initialOrder = agentWorkspaceOrder(items, workspaceOrder)
	}
	sortHerdrWorkspaces(items, initialOrder)
	list := New(items)
	list.SeparatorAware = opts.SeparatorAware
	list.DisableHomePrioritization = opts.DisableHomePrioritization
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
	cyclePreviewModeKey := config.Default().Keys.CyclePreviewMode
	if opts.CyclePreviewModeKey != nil {
		cyclePreviewModeKey = *opts.CyclePreviewModeKey
	}
	m := teaModel{
		ui:                    bloom.New(),
		toast:                 anim.NewPresence(),
		shimmer:               anim.NewLoop(shimmerPeriod),
		previewIn:             anim.NewValue(1, anim.Duration(previewInDuration), anim.Ease(anim.OutCubic)),
		countRoll:             oneShot(countRollDuration),
		shake:                 oneShot(refusedShakeDuration),
		pointer:               &pointerInput{},
		previewScroll:         &previewViewport{},
		backend:               opts.Backend,
		list:                  list,
		input:                 input,
		agentSpinner:          spinner.New(spinner.WithSpinner(agentStatusSpinner)),
		defaultPreviewCommand: opts.DefaultPreviewCommand,
		hidePath:              opts.HidePath,
		hidePreview:           opts.HidePreview,
		panePreview:           opts.PreviewMode == "pane",
		cyclePreviewModeKey:   cyclePreviewModeKey,
		showIcons:             opts.ShowIcons,
		replaceWorktreeIcon:   !opts.DisableWorktreeIconReplacement,
		hideLastWorkspace:     opts.HideLastWorkspace,
		hideLastWorkspacePath: opts.HideLastWorkspacePath,
		workspaceOrder:        workspaceOrder,
		recentWorkspaceIDs:    append([]string(nil), opts.RecentWorkspaceIDs...),
		workspaceSort:         workspaceSort,
		lastWorkspaceID:       opts.LastWorkspaceID,
		lastWorkspaceUnknown:  opts.LastWorkspaceUnknown,
		herdrWorkspaces:       workspaceSessionsByID(opts.HerdrWorkspaces),
		workspaceCloseContext: closeContext,
		previewParentContext:  closeContext,
		reduceMotion:          reduceMotionRequested(),
		smear:                 newSmearPreset(os.Getenv("HERDR_SESH_SMEAR_PRESET")),
	}
	if current, ok := list.Current(); ok && !m.hidePreview {
		m, _ = m.startPreview(current)
	}
	return m
}

// configureMotion applies HERDR_SESH_REDUCE_MOTION to bloom's animations.
// Motion is process-wide in bloom, so it is set both ways on every run.
func configureMotion() {
	anim.SetMotion(!reduceMotionRequested())
}

func reduceMotionRequested() bool {
	value := os.Getenv("HERDR_SESH_REDUCE_MOTION")
	return value == "1" || strings.EqualFold(value, "true")
}

func (m teaModel) Init() tea.Cmd {
	cmds := []tea.Cmd{m.input.Focus(), m.ui.Init()}
	if current, ok := m.list.Current(); ok && m.previewKey != "" {
		cmds = append(cmds, previewCommand(m.previewContext, m.previewKey, m.previewRequestID, current, m.defaultPreviewCommand, m.panePreview), previewLoadingCommand(m.previewContext, m.previewRequestID))
	}
	if m.backend != nil {
		cmds = append(cmds, scheduleStatusRefreshFor(m.refreshGeneration))
		if !m.reduceMotion {
			// Reduced motion keeps the working spinner on its first frame.
			cmds = append(cmds, m.agentSpinner.Tick)
		}
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

//nolint:ireturn // Bubble Tea's Model interface requires this return shape.
func (m teaModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var uiCmd tea.Cmd
	if _, mouse := msg.(tea.MouseMsg); !mouse || m.settings == nil {
		// The root's last frame is the picker, not settings, so its handlers
		// must not see clicks made in settings.
		uiCmd = m.ui.Update(msg)
	}
	animCmd := anim.Update(msg, &m.toast, &m.shimmer, &m.previewIn, &m.countRoll, &m.shake)
	count := m.countText()
	if _, ok := msg.(anim.FrameMsg); ok {
		// Animation frames only redraw; the picker's catch-all would refilter on each one.
		return m, tea.Batch(uiCmd, animCmd)
	}
	next, cmd := m.update(msg)
	if _, mouse := msg.(tea.MouseMsg); mouse {
		// update applies what the handlers recorded or, when it ignores the
		// mouse, such as while settings load, drops it here so it cannot
		// replay on a later event.
		m.pointer.wheel, m.pointer.listScrolled, m.pointer.dividerMoved = 0, false, false
	}
	if picker, ok := next.(teaModel); ok {
		var syncCmd tea.Cmd
		picker, syncCmd = picker.syncToast()
		var effectsCmd tea.Cmd
		picker, effectsCmd = picker.syncEffects(count)
		next, cmd = picker, tea.Batch(cmd, syncCmd, effectsCmd)
	}
	return next, tea.Batch(cmd, uiCmd, animCmd)
}

// syncEffects rolls the workspace count over when an update changes it.
// count is the count shown before the update.
func (m teaModel) syncEffects(count string) (teaModel, tea.Cmd) {
	if m.countText() == count || m.reduceMotion {
		return m, nil
	}
	m.countFrom = count
	return m, m.countRoll.Play(0, 1)
}

// refuse shakes the list, for a key that does nothing right now.
func (m teaModel) refuse() (teaModel, tea.Cmd) {
	if m.reduceMotion {
		return m, nil
	}
	return m, m.shake.Play(0, 1)
}

// syncToast shows the toast while closeError is set, so every path that sets
// or clears the error drives it.
func (m teaModel) syncToast() (teaModel, tea.Cmd) {
	switch {
	case m.closeError != "":
		m.toastText = m.closeError
		if !m.toast.Shown() {
			return m, m.toast.Show()
		}
	case m.toast.Shown():
		return m, m.toast.Hide()
	}
	return m, nil
}

//nolint:gocyclo,ireturn // Bubble Tea's central event dispatcher requires this return shape.
func (m teaModel) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if next, cmd, handled := m.updateSettings(msg); handled {
		return next, cmd
	}
	if _, ok := msg.(spinner.TickMsg); ok {
		if m.reduceMotion {
			return m, nil
		}
		var cmd tea.Cmd
		m.agentSpinner, cmd = m.agentSpinner.Update(msg)
		return m, cmd
	}
	if tick, ok := msg.(statusRefreshTickMsg); ok {
		if tick.generation != m.refreshGeneration || m.backend == nil {
			return m, nil
		}
		return m, refreshAgentStatusesCommand(m.backend.RefreshAgentStatuses, m.refreshGeneration)
	}
	if statuses, ok := msg.(agentStatusesMsg); ok {
		if statuses.generation != m.refreshGeneration {
			return m, nil
		}
		if statuses.err != nil {
			return m, scheduleStatusRefreshFor(m.refreshGeneration)
		}
		selectedKey := ""
		if current, currentOK := m.list.Current(); currentOK {
			selectedKey = sessionmodel.Key(current)
		}
		m.list.UpdateAgentStatuses(statuses.statuses)
		if m.workspaceSort != workspaceSortAgent {
			return m, scheduleStatusRefreshFor(m.refreshGeneration)
		}
		m.resortWorkspaces()
		m.list.Filter(m.list.Query)
		for i, item := range m.list.Filtered {
			if sessionmodel.Key(item) == selectedKey {
				m.list.Selected = i
				break
			}
		}
		m.smearActive = false
		m.focusSmearActive = false
		m, previewCmd := m.refreshPreview()
		if previewCmd == nil {
			return m, scheduleStatusRefreshFor(m.refreshGeneration)
		}
		return m, tea.Batch(previewCmd, scheduleStatusRefreshFor(m.refreshGeneration))
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
		return m.receivePreview(preview)
	}
	if loading, ok := msg.(previewLoadingMsg); ok {
		if loading.requestID == m.previewRequestID && m.cancelPreview != nil && m.previewContext.Err() == nil {
			m.previewLoading = true
			return m, m.shimmer.Loop()
		}
		return m, nil
	}
	if tick, ok := msg.(panePreviewTickMsg); ok {
		return m.refreshPanePreview(tick)
	}
	if closed, ok := msg.(workspaceCloseMsg); ok {
		if closed.workspaceID != m.closingWorkspaceID {
			return m, nil
		}
		m.closingWorkspaceID = ""
		m.cancelWorkspaceClose = nil
		if m.quitAfterWorkspaceClose {
			m.quitAfterWorkspaceClose = false
			m = m.cancelActivePreview()
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
		if closed.result.RecentWorkspaceIDs != nil {
			m.recentWorkspaceIDs = append([]string(nil), closed.result.RecentWorkspaceIDs...)
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
		m.pointer.dividerActive = false
		if m.focusSmearActive {
			m.focusSmearInputRow, _ = m.smearEnds()
		}
		return m, nil
	}
	if clicked, ok := msg.(workspaceClickedMsg); ok {
		if m.closingWorkspaceID != "" {
			return m, nil
		}
		for i, item := range m.list.Filtered {
			if sessionmodel.Key(item) == clicked.key {
				m.list.Selected = i
				return m.chooseCurrent()
			}
		}
		return m, nil
	}
	if _, ok := msg.(tea.MouseWheelMsg); ok {
		// The wheel steps the selection as the arrow keys do. A handler from
		// a frame drawn before a close started may have recorded it.
		dy := m.pointer.wheel
		m.pointer.wheel = 0
		switch {
		case !m.selectable():
			return m, nil
		case dy < 0:
			msg = tea.KeyPressMsg{Code: tea.KeyUp}
		case dy > 0:
			msg = tea.KeyPressMsg{Code: tea.KeyDown}
		default:
			return m, nil
		}
	}
	if _, ok := msg.(tea.MouseMsg); ok {
		if m.pointer.dividerMoved {
			m.previewWidth, m.pointer.dividerMoved = m.pointer.dividerWidth, false
		}
		if m.pointer.listScrolled {
			m.pointer.listScrolled = false
			if m.selectable() && m.pointer.listQuery == m.list.Query {
				return m.scrollListTo(m.pointer.listFirst)
			}
		}
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
	if m.cyclePreviewModeKey != "" && key.String() == m.cyclePreviewModeKey {
		if m.hidePreview || m.closingWorkspaceID != "" {
			return m, nil
		}
		m.panePreview = !m.panePreview
		m.previewKey = ""
		return m.refreshPreview()
	}
	switch key.String() {
	case "ctrl+c", "esc":
		if m.closingWorkspaceID != "" {
			m.quitAfterWorkspaceClose = true
			m.cancelWorkspaceClose()
			return m, nil
		}
		m = m.cancelActivePreview()
		return m, tea.Quit
	case "enter":
		return m.chooseCurrent()
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

func (m teaModel) chooseCurrent() (teaModel, tea.Cmd) {
	if m.closingWorkspaceID != "" {
		return m.refuse()
	}
	if choice, ok := m.list.Current(); ok {
		m.choice = choice
		m.chosen = true
	}
	m = m.cancelActivePreview()
	return m, tea.Quit
}

func (m teaModel) closeSelectedWorkspace() (teaModel, tea.Cmd) {
	current, ok := m.list.Current()
	if !ok || current.Source != "herdr" || current.WorkspaceID == "" || m.backend == nil || m.closingWorkspaceID != "" {
		return m.refuse()
	}
	m.closingWorkspaceID = current.WorkspaceID
	m.closeError = ""
	m = m.cancelActivePreview()
	m.previewKey = ""
	closeCtx, cancel := context.WithCancel(m.workspaceCloseContext)
	m.cancelWorkspaceClose = cancel
	return m, closeWorkspaceCommand(closeCtx, cancel, m.backend, current.WorkspaceID)
}

func closeWorkspaceCommand(
	ctx context.Context,
	cancel context.CancelFunc,
	backend Backend,
	workspaceID string,
) tea.Cmd {
	return func() tea.Msg {
		defer cancel()
		msg := workspaceCloseMsg{workspaceID: workspaceID, closeErr: backend.CloseWorkspace(ctx, workspaceID)}
		if msg.closeErr == nil {
			msg.result, msg.reloadErr = backend.ReloadPicker(ctx)
			msg.reloadRan = true
		}
		return msg
	}
}

func (m teaModel) resortWorkspaces() {
	order := m.workspaceOrder
	switch m.workspaceSort {
	case workspaceSortRecent:
		order = m.recentWorkspaceIDs
	case workspaceSortAgent:
		order = agentWorkspaceOrder(m.list.All, m.workspaceOrder)
	}
	sortHerdrWorkspaces(m.list.All, order)
}

func (m teaModel) toggleWorkspaceSort() (teaModel, tea.Cmd) {
	switch m.workspaceSort {
	case workspaceSortWorkspace:
		m.workspaceSort = workspaceSortRecent
	case workspaceSortRecent:
		m.workspaceSort = workspaceSortAgent
	default:
		m.workspaceSort = workspaceSortWorkspace
	}
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
		// The edit can change the filter's height, as when a multiline
		// placeholder goes, which moves the input's cursor line.
		m.focusSmearInputRow, _ = m.smearEnds()
	}
	m = m.filter(m.input.Value())
	m, previewCmd := m.refreshPreview()
	return m, tea.Batch(focusCmd, cmd, previewCmd)
}

func scheduleStatusRefreshFor(generation uint64) tea.Cmd {
	return tea.Tick(statusRefreshInterval, func(time.Time) tea.Msg { return statusRefreshTickMsg{generation: generation} })
}

func refreshAgentStatusesCommand(refresh func() (map[string]string, error), generation ...uint64) tea.Cmd {
	var epoch uint64
	if len(generation) > 0 {
		epoch = generation[0]
	}
	return func() tea.Msg {
		statuses, err := refresh()
		return agentStatusesMsg{statuses: statuses, err: err, generation: epoch}
	}
}

func (m teaModel) View() tea.View {
	if m.settings != nil {
		if m.settingsBusy {
			message := "Reloading settings…"
			if m.quitAfterSettings {
				message = "Cancelling settings reload…"
			}
			view := tea.NewView(sectionStyle.Render(message) + "\n\n" + helpStyle.Render("ctrl+c exit"))
			view.AltScreen = true
			return view
		}
		return m.settings.View()
	}
	width := m.contentWidth() + horizontalPadding*2
	view := m.ui.View(width, m.windowHeight(), bloom.Overlay(m.pickerTree(), m.pickerLayers(width)...))
	m.previewScroll.drawn = m.previewScroll.shown
	view.AltScreen = true
	// The wheel moves the selection from Update, which needs reports even
	// when no node has a handler. Bloom asks for all motion while the
	// divider's grip follows the pointer; that is kept.
	if view.MouseMode == tea.MouseModeNone {
		view.MouseMode = tea.MouseModeCellMotion
	}
	return view
}

func (m teaModel) footerText() string {
	if !m.previewShown() && m.closingWorkspaceID != "" {
		status := "Closing workspace..."
		if m.quitAfterWorkspaceClose {
			status = "Cancelling workspace close..."
		}
		return emptyStyle.Render(status)
	}
	if m.settingsBusy {
		return helpStyle.Render("Loading settings…")
	}
	help := fmt.Sprintf("enter select · ctrl+j/k · ctrl+r %s · ctrl+x close · esc exit", m.workspaceSort)
	if m.backend != nil {
		help = m.settingsKey() + " settings · " + help
	}
	return helpStyle.Render(help)
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

func normalizeWorkspaceSort(sortMode string) string {
	switch sortMode {
	case workspaceSortWorkspace, workspaceSortRecent, workspaceSortAgent:
		return sortMode
	default:
		return workspaceSortWorkspace
	}
}

func agentStatusRank(status string) int {
	switch status {
	case "blocked":
		return 0
	case "done":
		return 1
	case "working":
		return 2
	case "idle":
		return 3
	default:
		return 4
	}
}

func agentWorkspaceOrder(items []sessionmodel.Session, workspaceOrder []string) []string {
	statuses := make(map[string]string, len(items))
	for _, item := range items {
		if item.Source == "herdr" && item.WorkspaceID != "" {
			statuses[item.WorkspaceID] = item.AgentStatus
		}
	}
	order := append([]string(nil), workspaceOrder...)
	sort.SliceStable(order, func(i, j int) bool {
		return agentStatusRank(statuses[order[i]]) < agentStatusRank(statuses[order[j]])
	})
	return order
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

// stackedPreviewRows is how many preview lines show under the list when the
// terminal is too narrow to put them side by side.
func (m teaModel) stackedPreviewRows() int {
	const stackedChromeRows = 10
	available := m.windowHeight() - stackedChromeRows
	if available < 2 {
		return 1
	}
	return min(compactPreviewBody, maxInt(1, available/3))
}

// windowHeight is the terminal's height, or a standard one before Bubble
// Tea reports it.
func (m teaModel) windowHeight() int {
	if m.height == 0 {
		return defaultHeight
	}
	return m.height
}

func (m teaModel) contentWidth() int {
	width := m.width
	if width == 0 {
		width = defaultWidth
	}
	return maxInt(1, width-horizontalPadding*2)
}

// countText is how many workspaces there are, and how many match while the
// filter has text.
func (m teaModel) countText() string {
	if m.list.Query != "" {
		return fmt.Sprintf("%d/%d workspaces", len(m.list.Filtered), len(m.list.All))
	}
	return fmt.Sprintf("%d workspaces", len(m.list.All))
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
			if item.Machine != "" {
				label = item.DisplayName()
			}
		}
	}
	return label, path
}

func (m teaModel) lastWorkspaceText() string {
	label, path := m.lastWorkspaceStatus()
	line := sectionStyle.Render("LAST WORKSPACE") + countStyle.Render(" · ") + rowLabelStyle.Render(label)
	shownPath := compactHome(path)
	if item, ok := m.herdrWorkspaces[m.lastWorkspaceID]; ok && item.Machine != "" {
		shownPath = path
	}
	if !m.hideLastWorkspacePath && shownPath != "" && shownPath != label {
		line += pathStyle.Render("  " + shownPath)
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
	if m.hideLastWorkspace {
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

func (m teaModel) previewTitle() string {
	title := sectionStyle.Render("PREVIEW")
	if m.panePreview {
		title = sectionStyle.Render("PANE")
	}
	if m.cyclePreviewModeKey != "" {
		title += countStyle.Render(" [" + m.cyclePreviewModeKey + "]")
	}
	current, ok := m.list.Current()
	if !ok {
		return title
	}
	label := current.Name
	if current.Machine != "" {
		label = current.DisplayName()
	}
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

func (m teaModel) receivePreview(preview previewMsg) (teaModel, tea.Cmd) {
	if preview.requestID != m.previewRequestID || preview.key != m.previewKey {
		return m, nil
	}
	m.preview = preview.text
	// Text for another workspace or preview mode starts at the top; a pane
	// refresh of the same one keeps where it was scrolled.
	var fadeIn tea.Cmd
	if shown := fmt.Sprintf("%t:%s", m.panePreview, preview.key); shown != m.previewScroll.shown {
		m.previewScroll.shown, m.previewScroll.pos = shown, bloom.ViewportPos{}
		if !m.reduceMotion {
			fadeIn = m.previewIn.Play(0, 1)
		}
	}
	m.previewLoading = false
	m.shimmer.Stop()
	if m.cancelPreview != nil {
		m.cancelPreview()
		m.cancelPreview = nil
	}
	if current, ok := m.list.Current(); ok && m.panePreview && !m.hidePreview && current.Source == "herdr" && current.WorkspaceID != "" {
		return m, tea.Batch(fadeIn, tea.Tick(panePreviewRefreshInterval, func(time.Time) tea.Msg {
			return panePreviewTickMsg{requestID: preview.requestID}
		}))
	}
	return m, fadeIn
}

func (m teaModel) refreshPanePreview(tick panePreviewTickMsg) (teaModel, tea.Cmd) {
	if tick.requestID != m.previewRequestID || !m.panePreview || m.hidePreview {
		return m, nil
	}
	if current, ok := m.list.Current(); ok {
		// Keep the last snapshot visible while the next read is in flight.
		return m.requestPreview(current)
	}
	return m, nil
}

func (m teaModel) refreshPreview() (teaModel, tea.Cmd) {
	if m.hidePreview {
		m = m.cancelActivePreview()
		m.previewKey = ""
		m.preview = ""
		return m, nil
	}
	current, ok := m.list.Current()
	if !ok {
		m = m.cancelActivePreview()
		m.previewKey = ""
		m.preview = "No preview available"
		return m, nil
	}
	key := sessionmodel.Key(current)
	if key == m.previewKey {
		return m, nil
	}
	return m.startPreview(current)
}

func (m teaModel) cancelActivePreview() teaModel {
	m.previewLoading = false
	m.shimmer.Stop()
	if m.cancelPreview != nil {
		m.cancelPreview()
		m.cancelPreview = nil
	}
	m.previewRequestID++
	return m
}

func (m teaModel) startPreview(s sessionmodel.Session) (teaModel, tea.Cmd) {
	m, cmd := m.requestPreview(s)
	return m, tea.Batch(cmd, previewLoadingCommand(m.previewContext, m.previewRequestID))
}

func previewLoadingCommand(ctx context.Context, requestID uint64) tea.Cmd {
	return func() tea.Msg {
		timer := time.NewTimer(500 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
			return previewLoadingMsg{requestID: requestID}
		}
	}
}

//nolint:contextcheck // Bubble Tea persists the caller context on the model between messages.
func (m teaModel) requestPreview(s sessionmodel.Session) (teaModel, tea.Cmd) {
	m = m.cancelActivePreview()
	ctx, cancel := context.WithCancel(m.previewParentContext)
	m.previewContext = ctx
	m.cancelPreview = cancel
	m.previewKey = sessionmodel.Key(s)
	return m, previewCommand(ctx, m.previewKey, m.previewRequestID, s, m.defaultPreviewCommand, m.panePreview)
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
	m = m.measureFocusSmear()
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
	m = m.measureFocusSmear()
	m.focusSmearStep = m.focusSmearSteps
	m.focusSmearDirection = -1
	m.focusSmearActive = true
	return m, m.smearTick()
}

// measureFocusSmear solves the layout once as the smear starts, rather than
// on every frame, for the row it starts from, the input's cursor line, and
// how many rows it travels down to the selected row.
func (m teaModel) measureFocusSmear() teaModel {
	inputRow, selectedRow := m.smearEnds()
	m.focusSmearInputRow, m.focusSmearSteps = inputRow, maxInt(1, selectedRow-inputRow)
	return m
}

// smearEnds solves the layout for the screen rows of the input's cursor line
// and of the selected workspace.
func (m teaModel) smearEnds() (inputRow, selectedRow int) {
	frame := bloom.Solve(m.contentWidth()+horizontalPadding*2, m.windowHeight(), m.pickerTree())
	if input, ok := frame.Rect(inputID); ok {
		inputRow = input.Max.Y - 1
	}
	selectedRow = inputRow + 1
	list, listOK := frame.Rect(listID)
	shown, shownOK := frame.List(listID)
	if listOK && shownOK {
		selectedRow = list.Min.Y + m.list.Selected - shown.First
	}
	return inputRow, selectedRow
}

// scrollListTo shows the list from item first, as the scrollbar was dragged
// to, and keeps the selection among the items shown.
func (m teaModel) scrollListTo(first int) (teaModel, tea.Cmd) {
	if len(m.list.Filtered) == 0 {
		return m, nil
	}
	m.listOffset = first
	visible := 1
	if shown, ok := m.ui.Frame().List(listID); ok {
		visible = max(shown.Visible, 1)
	}
	m.list.Selected = min(max(m.list.Selected, first), first+visible-1, len(m.list.Filtered)-1)
	// Focus the list so its rail marks what enter or ctrl+x would act on,
	// and drop any trail: the selection may have jumped far.
	m.listFocused = true
	m.input.Blur()
	m.smearActive, m.focusSmearActive = false, false
	return m.refreshPreview()
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
		lineIndex := step
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
	// Keep scrolls from where the list was on screen.
	if shown, ok := m.ui.Frame().List(listID); ok {
		m.listOffset = shown.First
	}
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

func previewCommand(ctx context.Context, key string, requestID uint64, s sessionmodel.Session, defaultPreviewCommand string, pane bool) tea.Cmd {
	return func() tea.Msg {
		var text string
		var err error
		if pane {
			text, err = renderPanePreview(ctx, s)
		} else {
			text, err = renderPreview(ctx, s, defaultPreviewCommand)
		}
		if err != nil {
			text = err.Error()
		}
		text = strings.TrimRight(text, "\n")
		if text == "" {
			text = "No preview available"
		}
		return previewMsg{key: key, requestID: requestID, text: text}
	}
}

func (m teaModel) previewLayout() (int, int) {
	width := m.contentWidth()
	if m.hidePreview || width < previewSplitWidth-horizontalPadding*2 {
		return width, 0
	}
	previewWidth := m.previewWidth
	if previewWidth == 0 {
		previewWidth = width / 2
		if !m.hidePath {
			previewWidth = min(previewWidth, maxPreviewWidth)
		}
	}
	previewWidth = min(max(previewWidth, minPreviewWidth), width-minListWidth-3)
	return width - previewWidth - 3, previewWidth
}

func row(s sessionmodel.Session, selected bool, width int, showIcons bool, query string) string {
	return rowWithRail(s, selected, width, showIcons, true, false, query, selectionRailStyle.Render("┃ "), agentStatusSpinner.Frames[0], "")
}

func rowWithRail(s sessionmodel.Session, selected bool, width int, showIcons, replaceWorktreeIcon, hidePath bool, query, selectedRail, workingGlyph, treePrefix string) string {
	rail := "  "
	if selected {
		rail = selectedRail
	}
	label := s.Name
	if s.Machine != "" {
		label = s.DisplayName()
	}
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
	path := displayPath(s)
	if path == label {
		path = ""
	}
	showSecondary := !hidePath && width >= rowPathMinWidth && path != ""
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

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

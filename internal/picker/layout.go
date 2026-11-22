package picker

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	sessionmodel "github.com/fullerzz/herdr-plugin-sesh/internal/model"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/fx"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	toastMaxWidth     = 56
	toastFooterOffset = 3
	shimmerPeriod     = 1400 * time.Millisecond
)

// pointerInput is shared by every copy of the model, as mouse handlers run
// on the copy that drew the frame. They record what happened here and
// Update applies it.
type pointerInput struct {
	dividerActive bool
	dividerMoved  bool
	dividerWidth  int

	wheel        int
	listFirst    int
	listQuery    string
	listScrolled bool
}

// nodeID names the nodes that Update finds in a solved or drawn frame.
type nodeID string

const (
	inputID   nodeID = "input"
	listID    nodeID = "list"
	previewID nodeID = "preview"
	// previewBoxID is the preview's box, which a short stacked layout drops.
	previewBoxID nodeID = "preview box"
)

// pickerTree lays out the picker screen: the filter in a box with the
// title and count on its border, the list and preview boxes below it, and
// the footer. Widths still come from previewLayout: rows are drawn as
// strings at a known width, and a dragged preview width must be clamped,
// which a Fixed slot does not do.
func (m teaModel) pickerTree() bloom.BoxNode {
	width := m.contentWidth()
	listWidth, previewWidth := m.previewLayout()
	var body bloom.Child
	switch {
	case m.bodyRows() < panelFrameRows+1:
		// A box would leave no row inside its border, so the rows go bare
		// where the box's text would start, and the preview is dropped.
		body = bloom.Box(lipgloss.NewStyle().PaddingLeft(smearInset), m.listBody(width-panelInset))
	case m.hidePreview:
		body = m.listBox(width)
	case previewWidth > 0:
		// The list box takes both blank columns that used to flank the
		// divider, so its rows keep their width, and the path its column.
		body = bloom.Row(
			bloom.Fixed(listWidth+2, m.listBox(listWidth+2)),
			bloom.Fixed(1, dividerGrip{drag: on.Drag(m.dragDivider)}),
			bloom.Fixed(previewWidth, m.previewBox(previewWidth)),
		)
	default:
		// The preview goes first when the body is too short for both, so a
		// workspace row stays on screen.
		body = bloom.Column(
			bloom.Fill(m.listBox(listWidth)).Min(panelFrameRows+1),
			bloom.Fixed(panelFrameRows+m.stackedPreviewRows(), m.previewBox(width)).Collapsible(1),
		)
	}
	screen := bloom.Column(
		textLine("", width),
		m.filterBox(width),
		bloom.Fill(body),
		textLine(m.footerLine(m.footerText(), width), width),
		textLine("", width),
	)
	box := bloom.Box(lipgloss.NewStyle().Padding(0, horizontalPadding), screen)
	if m.selectable() {
		// On the box, not the column, so the side margins take the wheel too.
		box = box.On(on.Scroll(m.scrollWheel))
	}
	return box
}

// panelFrameRows is the rows a panel's border takes, top and bottom.
const panelFrameRows = 2

// panelInset is the columns a panel's border and left padding take from
// the text inside it.
const panelInset = 3

// panel is the rounded border of every box, lit while it has the focus.
func panel(focused bool) lipgloss.Style {
	border := mutedColor
	if focused {
		border = skyColor
	}
	return panelColored(border)
}

func panelColored(border color.Color) lipgloss.Style {
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(border)
}

// closingPanel marks the box that says a workspace is closing.
func closingPanel() lipgloss.Style { return panelColored(redColor) }

// edgeText is a title or counter drawn on a border, a space either side.
func edgeText(text string) bloom.TextNode {
	return bloom.Text(" " + text + " ").Truncate()
}

// edgeTitle is a title on the top border of a box width wide. It drops the
// spaces either side before it would be cut short: the border keeps a cell
// free at each corner, so the title has width-4 cells.
func edgeTitle(text string, width int) bloom.TextNode {
	if lipgloss.Width(text)+2 > width-4 {
		return bloom.Text(text).Truncate()
	}
	return edgeText(text)
}

// filterBox is the filter input, with the picker's title and workspace
// count on the border above it. The count rolls to a new number as the
// filter narrows the list.
func (m teaModel) filterBox(width int) bloom.SlotNode {
	input := m.input
	inner := width - panelInset - 1
	input.SetWidth(maxInt(8, inner-lipgloss.Width(input.Prompt)-1))
	line, rows := fitText(input.View(), inner)
	count := bloom.Child(edgeText(countStyle.Render(m.countText())))
	if m.countRoll.Animating() {
		count = bloom.Transition(m.countRoll.Float(), count, fx.Roll{From: m.countFrom})
	}
	box := bloom.Box(panel(!m.listFocused).Padding(0, 1), bloom.Column(line.ID(inputID))).
		Edge(bloom.Top, bloom.Start, edgeText(titleStyle.Render("herdr / sesh"))).
		Edge(bloom.Top, bloom.End, count)
	return bloom.Fixed(rows+panelFrameRows, box)
}

// dividerGrip is the column between the list and the preview, which drags
// to resize them. A grip at its middle shows where to take hold.
type dividerGrip struct{ drag on.Handler }

func (g dividerGrip) View() string { return "" }

//nolint:ireturn // bloom.Layouter returns a Child.
func (g dividerGrip) Layout(ctx bloom.Context) bloom.Child {
	lines := make([]string, max(ctx.Height, 1))
	lines[len(lines)/2] = "⋮"
	return bloom.Text(strings.Join(lines, "\n")).
		Style(ruleStyle).
		HoverStyle(selectionRailStyle).
		On(g.drag)
}

// previewShown reports whether the preview is on screen. A stacked preview
// collapses when the body is too short for it and a workspace row, and any
// preview when the body is too short for a box with a row inside it; the
// footer and the list's border then carry its status.
// TestPreviewShownMatchesTheLayout holds this to bloom's layout.
func (m teaModel) previewShown() bool {
	body := m.bodyRows()
	if m.hidePreview || body < panelFrameRows+1 {
		return false
	}
	if _, previewWidth := m.previewLayout(); previewWidth > 0 {
		return true
	}
	return body >= (panelFrameRows+1)+(panelFrameRows+m.stackedPreviewRows())
}

// bodyRows is how many rows the list and preview share: the window less a
// blank line, the filter box, the footer and a blank line.
func (m teaModel) bodyRows() int {
	return m.windowHeight() - (m.filterRows() + panelFrameRows) - 3
}

// filterRows is how many rows the filter's prompt and text take.
func (m teaModel) filterRows() int {
	return strings.Count(m.input.View(), "\n") + 1
}

// selectable reports whether the mouse may move or choose the selection. It
// may not while a workspace closes or settings reload.
func (m teaModel) selectable() bool {
	return m.closingWorkspaceID == "" && !m.settingsBusy
}

func (m teaModel) pickerLayers(width int) []bloom.LayerNode {
	layers := []bloom.LayerNode{m.toastLayer(width, m.windowHeight())}
	if smear, ok := m.smearLayer(); ok {
		layers = append(layers, smear)
	}
	return layers
}

func textLine(line string, width int) bloom.SlotNode {
	slot, _ := fitText(line, width)
	return slot
}

// fitText fits each line of text to width and returns its slot and the rows
// it takes. Lines are fitted one by one: truncating the whole text would
// count the width of every line before the last against it.
func fitText(text string, width int) (bloom.SlotNode, int) {
	lines := strings.Split(text, "\n")
	if text != "" {
		for i, line := range lines {
			lines[i] = fitLine(line, width)
		}
	}
	return bloom.Fixed(len(lines), bloom.Text(strings.Join(lines, "\n"))), len(lines)
}

// listBox is the list in a box, with its title and the selected row's
// place on the border above it, and a scrollbar on its right border while
// some workspaces are out of view. It shakes when a key is refused, and its
// border turns red while a workspace closes with no preview on screen.
//
//nolint:ireturn // A shaking box is drawn through a transition.
func (m teaModel) listBox(width int) bloom.Child {
	style := panel(m.listFocused)
	if m.closingWorkspaceID != "" && !m.previewShown() {
		style = closingPanel()
	}
	box := bloom.Box(style.PaddingLeft(1), m.listBody(width-panelInset)).
		Edge(bloom.Top, bloom.Start, edgeText(sectionStyle.Render("WORKSPACES"))).
		Edge(bloom.Right, bloom.Stretch, listScrollbar{bar: m.listScrollbar(), border: style})
	if n := len(m.list.Filtered); n > 0 {
		box = box.Edge(bloom.Top, bloom.End, edgeText(countStyle.Render(fmt.Sprintf("%d/%d", m.list.Selected+1, n))))
	}
	if m.shake.Animating() {
		return bloom.Transition(m.shake.Float(), box, fx.Shake{Amount: 2, Times: 3})
	}
	return box
}

// listScrollbar leaves the scrollbar out when the list is one row tall, as
// its thumb would fill the track and could not move, and draws the box's
// own border there instead.
type listScrollbar struct {
	bar    bloom.ScrollbarNode
	border lipgloss.Style
}

func (s listScrollbar) View() string { return s.bar.View() }

//nolint:ireturn // bloom.Layouter returns a Child.
func (s listScrollbar) Layout(ctx bloom.Context) bloom.Child {
	if ctx.Height < 2 {
		edge := strings.TrimSuffix(strings.Repeat(s.border.GetBorderStyle().Right+"\n", max(ctx.Height, 1)), "\n")
		return bloom.Text(edge).Style(lipgloss.NewStyle().Foreground(s.border.GetBorderRightForeground()))
	}
	return s.bar
}

func (m teaModel) listScrollbar() bloom.ScrollbarNode {
	bar := bloom.ScrollbarOf(listID)
	if m.selectable() {
		bar = bar.OnScroll(m.scrollList(m.list.Query))
	}
	return bar
}

func (m teaModel) listBody(width int) bloom.FlexNode {
	if len(m.list.Filtered) == 0 {
		return bloom.Column(bloom.Text(fitLine(emptyStyle.Render("No matching workspaces"), width)))
	}
	clickable := m.selectable()
	return bloom.Column(bloom.Fill(bloom.List(len(m.list.Filtered), func(i int) bloom.Child {
		row := bloom.Text(fitLine(m.listRow(i, width), width))
		if clickable {
			row = row.On(workspaceClick(sessionmodel.Key(m.list.Filtered[i])))
		}
		return row
	}).Offset(m.listOffset).Keep(m.list.Selected).ID(listID)))
}

// listRow draws workspace i of the filtered list, with the selection rail
// and any smear trail beside it.
func (m teaModel) listRow(i, width int) string {
	selected := m.listFocused && !m.focusSmearActive && i == m.list.Selected
	selectedRail := m.smear.headStyle().Render(m.smear.headGlyph + " ")
	treePrefix := worktreeTreePrefix(m.list.Filtered, i)
	line := strings.TrimSuffix(rowWithRail(m.list.Filtered[i], selected, width, m.showIcons, m.replaceWorktreeIcon, m.hidePath, m.list.Query, selectedRail, m.agentSpinner.View(), treePrefix), "\n")
	if rail, age := m.smearRail(i); rail != "" {
		line = m.smear.trailStyle(age).Render(rail+" ") + strings.TrimPrefix(line, "  ")
	}
	return line
}

// scrollWheel records wheel steps over the picker for Update, which moves the
// selection by them. The preview's viewport takes the wheel over itself.
func (m teaModel) scrollWheel(_, dy int) tea.Cmd {
	m.pointer.wheel += dy
	return nil
}

// scrollList records where the scrollbar was dragged for Update, with the
// filter of the results it scrolled, so Update can drop a scroll of results
// the filter has since replaced.
func (m teaModel) scrollList(query string) func(int) tea.Cmd {
	return func(first int) tea.Cmd {
		p := m.pointer
		p.listFirst, p.listQuery, p.listScrolled = first, query, true
		return nil
	}
}

// workspaceClick chooses on the press, as the picker always has; on.Click
// would wait for the release.
func workspaceClick(key string) on.Handler {
	return on.Mouse(func(e on.Event) tea.Cmd {
		click, ok := e.Msg.(tea.MouseClickMsg)
		if !ok || click.Button != tea.MouseLeft {
			return nil
		}
		return func() tea.Msg { return workspaceClickedMsg{key: key} }
	})
}

func (m teaModel) dragDivider(e on.Event) tea.Cmd {
	p := m.pointer
	switch msg := e.Msg.(type) {
	case tea.MouseClickMsg:
		p.dividerActive = msg.Button == tea.MouseLeft
	case tea.MouseMotionMsg:
		// A motion that ends the drag has no button down: the release was
		// lost, and the pointer has moved on without the divider.
		if p.dividerActive && !e.Done {
			width := m.contentWidth()
			// The preview box starts in the column after the divider.
			p.dividerWidth = min(max(width+horizontalPadding-e.Screen.X-1, minPreviewWidth), width-minListWidth-3)
			p.dividerMoved = true
		}
	}
	if e.Done {
		p.dividerActive = false
	}
	return nil
}

// previewBox is the preview in a box: a status line, a placeholder while it
// loads, or the preview text in a viewport that scrolls, with its title on
// the border above it and a scrollbar on its right border. The wheel
// anywhere over the box belongs to the preview: the viewport scrolls under
// it, and over the border, the title and the scrollbar the box scrolls it
// too, so the wheel never moves the selection there. The border turns red
// while a workspace closes.
func (m teaModel) previewBox(width int) bloom.BoxNode {
	style := panel(false)
	if m.closingWorkspaceID != "" {
		style = closingPanel()
	}
	// Padded both sides: preview text fills its lines, where list rows end
	// in a path cut short.
	return bloom.Box(style.Padding(0, 1), m.previewBody(width-panelInset-1)).ID(previewBoxID).
		Edge(bloom.Top, bloom.Start, edgeTitle(m.previewTitle(), width)).
		Edge(bloom.Right, bloom.Stretch, bloom.ScrollbarOf(previewID)).
		On(on.Scroll(m.scrollPreview(m.previewScroll.scrollFor(m.previewScroll.shown))))
}

// previewWheelStep is how many rows a wheel step scrolls the preview from
// its border, as bloom's viewport scrolls under the wheel.
const previewWheelStep = 3

// scrollPreview scrolls the preview viewport from the last frame drawn by
// the wheel steps over the box around it, through scroll, which drops the
// steps once another workspace's text is on screen. With no viewport, a
// status or a placeholder, the wheel does nothing.
func (m teaModel) scrollPreview(scroll func(bloom.ViewportPos) tea.Cmd) func(int, int) tea.Cmd {
	return func(_, dy int) tea.Cmd {
		view, ok := m.ui.Frame().Viewport(previewID)
		if !ok || dy == 0 {
			return nil
		}
		return scroll(view.Down(dy * previewWheelStep))
	}
}

func (m teaModel) previewBody(width int) bloom.FlexNode {
	text := strings.TrimRight(m.preview, "\n")
	switch {
	case m.closingWorkspaceID != "" && (m.previewKey == "" || m.quitAfterWorkspaceClose):
		status := "Closing workspace..."
		if m.quitAfterWorkspaceClose {
			status = "Cancelling workspace close..."
		}
		return bloom.Column(bloom.Text(fitLine(status, width)))
	case m.previewLoading:
		bars := make([]int, m.windowHeight())
		for i := range bars {
			// Uneven bars read as text; every fourth line is blank like a paragraph break.
			if i%4 != 3 {
				bars[i] = width * (9 - i%3*2) / 10
			}
		}
		return bloom.Column(bloom.Skeleton(bars...).Shimmer(m.shimmer.Float()))
	case text == "" && m.cancelPreview == nil:
		return bloom.Column(bloom.Text(fitLine("No preview available", width)))
	}
	view := m.previewScroll
	viewport := bloom.Child(bloom.Viewport(view.buffer(text)).Wrap(bloom.WrapWords).At(view.pos).OnScroll(view.scrollFor(view.shown)).ID(previewID))
	if m.previewIn.Animating() {
		// New text fades in a row at a time from the top, keeping its glyphs.
		viewport = bloom.Transition(m.previewIn.Float(), viewport, fx.Stagger{Effect: fx.Reverse{Effect: previewFade{}}, From: bloom.Top})
	}
	return bloom.Column(bloom.Fill(viewport))
}

// scrollFor keeps where the preview of shown, a workspace and preview mode,
// was scrolled to. It needs no Update: only View reads it, and
// receivePreview resets it when the text on screen changes to another
// workspace or mode. A frame drawn before that change may still be on
// screen; its scrolls belong to the old text and are dropped.
func (v *previewViewport) scrollFor(shown string) func(bloom.ViewportPos) tea.Cmd {
	return func(pos bloom.ViewportPos) tea.Cmd {
		if v.shown == shown {
			v.pos = pos
		}
		return nil
	}
}

// previewViewport is shared by every copy of the model, as pointerInput is.
// It keeps one buffer per preview text, so a frame that draws the same text
// reuses what bloom measured, and where the preview was scrolled. View swaps
// the buffer, not Update, because tests and benchmarks set preview directly;
// a swap only replaces the pointer, which no handler holds.
type previewViewport struct {
	text  string
	buf   *bloom.Buffer
	pos   bloom.ViewportPos
	shown string // the workspace and mode whose text pos scrolls
}

func (v *previewViewport) buffer(text string) *bloom.Buffer {
	if v.buf == nil || v.text != text {
		v.buf, v.text = bloom.NewBuffer(text), text
	}
	return v.buf
}

// smearInset is where the filter text and the list rows start inside their
// boxes, past the border and one column of padding. The smear runs between
// them, so it is drawn from there.
const smearInset = 2

func (m teaModel) smearLayer() (bloom.LayerNode, bool) {
	if !m.focusSmearActive || m.focusSmearSteps < 1 {
		return bloom.LayerNode{}, false
	}
	width := m.contentWidth()
	lines := make([]string, m.focusSmearSteps+1)
	m.renderFocusSmear(lines, width)
	inputRow, _ := m.smearEnds()
	return bloom.Layer(bloom.Text(strings.Join(lines, "\n"))).
		Transparent().
		Anchor(bloom.Start, bloom.Start).
		Offset(horizontalPadding+smearInset, inputRow), true
}

// toastLayer anchors the toast above the footer. In a pane too short for the
// whole message it is cut to the rows above the footer, keeping the start of
// the message, which says what failed. A click on the toast stops there, so
// it never chooses the row the toast covers; the wheel still moves the
// selection.
func (m teaModel) toastLayer(width, height int) bloom.LayerNode {
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(amberColor).Padding(0, 1)
	textStyle := lipgloss.NewStyle().Foreground(amberColor)
	text := bloom.Text(m.toastText).Style(textStyle)
	toastWidth := min(toastMaxWidth, width-horizontalPadding*2)
	toastHeight := 0
	if available := height - toastFooterOffset; height > 0 && m.toast.Visible() {
		wrapped := textStyle.Width(max(1, toastWidth-box.GetHorizontalFrameSize())).Render(m.toastText)
		if lipgloss.Height(wrapped)+box.GetVerticalFrameSize() > available {
			toastHeight = max(available, box.GetVerticalFrameSize()+1)
		}
	}
	handlers := []on.Handler{on.Mouse(func(on.Event) tea.Cmd { return nil })}
	if m.selectable() {
		handlers = append(handlers, on.Scroll(m.scrollWheel))
	}
	return bloom.Layer(bloom.Box(box, text).On(handlers...)).
		Size(toastWidth, toastHeight).
		Anchor(bloom.End, bloom.End).
		Offset(-horizontalPadding, -toastFooterOffset).
		Presence(m.toast, fx.Slide{From: bloom.Right}, paletteDistinct{Effect: fx.Fade{}})
}

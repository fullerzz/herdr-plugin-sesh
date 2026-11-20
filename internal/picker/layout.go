package picker

import (
	"strings"
	"time"

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

// dividerDrag is shared by every copy of the model, as the divider's drag
// handler runs on the copy that drew the frame. Update copies a moved width
// into previewWidth.
type dividerDrag struct {
	active bool
	moved  bool
	width  int
}

// pickerTree lays out the picker screen. Sizes still come from previewLayout
// and the body-line counts; bloom places the nodes and routes the mouse.
func (m teaModel) pickerTree() bloom.BoxNode {
	width := m.contentWidth()
	listWidth, previewWidth := m.previewLayout()
	var rows []bloom.Child
	for _, line := range m.headerLines(width) {
		rows = append(rows, textLine(line, width))
	}
	switch {
	case m.hidePreview:
		list, _ := m.listNode(width, m.listOnlyBodyLines())
		rows = append(rows, list)
	case previewWidth > 0:
		visibleRows := m.previewBodyLines()
		list, listRows := m.listNode(listWidth, visibleRows)
		preview, previewRows := m.previewNode(previewWidth, visibleRows)
		rows = append(rows, bloom.Row(
			bloom.Fixed(listWidth, list),
			bloom.Fixed(1, nil),
			bloom.Fixed(1, m.dividerNode(max(listRows, previewRows))),
			bloom.Fixed(1, nil),
			bloom.Fixed(previewWidth, preview),
		))
	default:
		visibleRows, previewLines := m.stackedBodyLines()
		list, _ := m.listNode(listWidth, visibleRows)
		preview, _ := m.previewNode(width, previewLines)
		rows = append(rows, list, preview)
	}
	rows = append(rows,
		textLine(horizontalRule(width), width),
		textLine(m.footerLine(m.footerText(), width), width),
		textLine("", width),
	)
	return bloom.Box(lipgloss.NewStyle().Padding(0, horizontalPadding), bloom.Column(rows...))
}

func (m teaModel) pickerLayers(width int) []bloom.LayerNode {
	layers := []bloom.LayerNode{m.toastLayer(width, m.height)}
	if smear, ok := m.smearLayer(); ok {
		layers = append(layers, smear)
	}
	return layers
}

func textLine(line string, width int) bloom.SlotNode {
	slot, _ := fitText(line, width)
	return slot
}

// fitText fits line to width and returns its slot and the rows it takes.
func fitText(line string, width int) (bloom.SlotNode, int) {
	if line != "" {
		line = fitLine(line, width)
	}
	rows := max(1, lipgloss.Height(line))
	return bloom.Fixed(rows, bloom.Text(line)), rows
}

// listNode returns the list and the rows it draws.
func (m teaModel) listNode(width, visibleRows int) (bloom.FlexNode, int) {
	view := m.listView(width, visibleRows)
	title, rows := fitText(sectionStyle.Render("WORKSPACES"), width)
	children := make([]bloom.Child, 0, 1+len(view.lines))
	children = append(children, title)
	clickable := m.closingWorkspaceID == "" && !m.settingsBusy
	for i, line := range view.lines {
		slot, lineRows := fitText(line, width)
		if key := view.keys[i]; clickable && key != "" {
			slot = slot.On(workspaceClick(key))
		}
		children = append(children, slot)
		rows += lineRows
	}
	return bloom.Column(children...), rows
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

func (m teaModel) dividerNode(height int) bloom.TextNode {
	divider := ruleStyle.Render("│")
	lines := make([]string, height)
	for i := range lines {
		lines[i] = divider
	}
	return bloom.Text(strings.Join(lines, "\n")).On(on.Drag(m.dragDivider))
}

func (m teaModel) dragDivider(e on.Event) tea.Cmd {
	drag := m.divider
	switch msg := e.Msg.(type) {
	case tea.MouseClickMsg:
		drag.active = msg.Button == tea.MouseLeft
	case tea.MouseMotionMsg:
		if drag.active {
			width := m.contentWidth()
			drag.width = min(max(width+horizontalPadding-e.Screen.X-2, minPreviewWidth), width-minListWidth-3)
			drag.moved = true
		}
	}
	if e.Done {
		drag.active = false
	}
	return nil
}

// previewNode returns the preview pane and the rows it draws.
func (m teaModel) previewNode(width, maxLines int) (bloom.FlexNode, int) {
	if !m.previewLoading || m.closingWorkspaceID != "" {
		text := m.previewView(width, maxLines)
		return bloom.Column(bloom.Text(text)), lipgloss.Height(text)
	}
	bars := make([]int, maxLines)
	for i := range bars {
		// Uneven bars read as text; every fourth line is blank like a paragraph break.
		if i%4 != 3 {
			bars[i] = width * (9 - i%3*2) / 10
		}
	}
	title, titleRows := fitText(m.previewTitle(), width)
	return bloom.Column(
		title,
		bloom.Fixed(maxLines, bloom.Skeleton(bars...).Shimmer(m.shimmer.Float())),
	), titleRows + maxLines
}

func (m teaModel) smearLayer() (bloom.LayerNode, bool) {
	if !m.focusSmearActive || m.focusSmearSteps < 1 {
		return bloom.LayerNode{}, false
	}
	width := m.contentWidth()
	lines := make([]string, filterLineIndex+m.focusSmearSteps+1)
	m.renderFocusSmear(lines, width)
	return bloom.Layer(bloom.Text(strings.Join(lines, "\n"))).
		Transparent().
		Anchor(bloom.Start, bloom.Start).
		Offset(horizontalPadding, 0), true
}

// toastLayer anchors the toast above the footer. In a pane too short for the
// whole message it is cut to the rows above the footer, keeping the start of
// the message, which says what failed.
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
	return bloom.Layer(bloom.Box(box, text)).
		Size(toastWidth, toastHeight).
		Anchor(bloom.End, bloom.End).
		Offset(-horizontalPadding, -toastFooterOffset).
		Presence(m.toast, fx.Slide{From: bloom.Right}, fx.Fade{})
}

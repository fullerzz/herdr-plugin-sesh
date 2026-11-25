package picker

import (
	"fmt"

	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bloomfx.dev/bloom"

	"bloomfx.dev/bloom/anim"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fullerzz/herdr-plugin-sesh/internal/config"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/fullerzz/herdr-plugin-sesh/internal/settings"
)

// The frames were written while the bloom tree still matched, case for
// case, the string renderer it replaced, so they pin the screen it drew.
// Write them again with UPDATE_PICKER_FRAMES=1.
func TestPickerFramesMatchGolden(t *testing.T) {
	sessions := []model.Session{
		{Name: "api", Source: "herdr", WorkspaceID: "w1", Path: "/src/api", AgentStatus: "working"},
		{Name: "web", Source: "zoxide", Path: "/src/web"},
		{Name: "dots", Source: "config", Path: "/src/dots"},
	}
	many := make([]model.Session, 30)
	for i := range many {
		many[i] = model.Session{Name: fmt.Sprintf("workspace-%02d", i), Source: "herdr", WorkspaceID: fmt.Sprintf("w%d", i)}
	}
	longPreview := func(m *teaModel) {
		lines := make([]string, 40)
		for i := range lines {
			lines[i] = fmt.Sprintf("preview line %02d", i)
		}
		m.preview = strings.Join(lines, "\n")
	}
	for _, tc := range []struct {
		name   string
		width  int
		height int
		opts   DisplayOptions
		items  []model.Session
		setup  func(*teaModel)
	}{
		{name: "long list top", width: 120, height: 20, items: many, setup: func(m *teaModel) { m.listFocused = true }},
		{name: "long list middle", width: 120, height: 20, items: many, setup: func(m *teaModel) {
			m.listFocused = true
			m.list.Selected = 15
		}},
		{name: "long list end", width: 120, height: 20, items: many, setup: func(m *teaModel) {
			m.listFocused = true
			m.list.Selected = 29
		}},
		{name: "long list stacked", width: 80, height: 20, items: many, setup: func(m *teaModel) { m.list.Selected = 15 }},
		{name: "long list only", width: 100, height: 20, items: many, opts: DisplayOptions{HidePreview: true}, setup: func(m *teaModel) { m.list.Selected = 15 }},
		{name: "long preview split", width: 120, height: 20, setup: longPreview},
		{name: "long preview stacked", width: 80, height: 20, setup: longPreview},
		{name: "short split", width: 120, height: 10, items: many},
		{name: "short stacked", width: 80, height: 10, items: many},
		{name: "short list only", width: 100, height: 10, items: many, opts: DisplayOptions{HidePreview: true}},
		{name: "split preview", width: 120, height: 28},
		{name: "stacked preview", width: 70, height: 28},
		{name: "list only", width: 100, height: 28, opts: DisplayOptions{HidePreview: true}},
		{name: "icons", width: 120, height: 28, opts: DisplayOptions{ShowIcons: true}},
		{name: "no window size", opts: DisplayOptions{ShowIcons: true}},
		{name: "list focused", width: 120, height: 28, setup: func(m *teaModel) {
			m.listFocused = true
			m.list.Selected = 1
		}},
		{name: "focus smear mid-flight", width: 120, height: 28, setup: func(m *teaModel) {
			m.input.SetValue("we")
			m.list.Selected = 1
			m.listFocused = true
			m.focusSmearStart = m.inputCursorColumn()
			*m = m.measureFocusSmear()
			m.focusSmearStep = 2
			m.focusSmearDirection = 1
			m.focusSmearActive = true
		}},
		{name: "loading split", width: 120, height: 20, setup: func(m *teaModel) { m.previewLoading = true }},
		{name: "loading stacked", width: 80, height: 20, setup: func(m *teaModel) { m.previewLoading = true }},
		{name: "closing", width: 120, height: 28, setup: func(m *teaModel) {
			m.closingWorkspaceID = "w1"
			m.previewKey = ""
		}},
		{name: "closing list only", width: 120, height: 28, opts: DisplayOptions{HidePreview: true}, setup: func(m *teaModel) {
			m.closingWorkspaceID = "w1"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			items := sessions
			if tc.items != nil {
				items = tc.items
			}
			m := newTeaModel(items, Options{DisplayOptions: tc.opts})
			t.Cleanup(func() { m.cancelActivePreview() })
			m.width, m.height = tc.width, tc.height
			m.preview = "line one\nline two"
			if tc.setup != nil {
				tc.setup(&m)
			}

			got := strings.Join(strippedLines(m.View().Content), "\n") + "\n"
			path := filepath.Join("testdata", "frames", strings.ReplaceAll(tc.name, " ", "-")+".txt")
			if os.Getenv("UPDATE_PICKER_FRAMES") != "" {
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
				require.NoError(t, os.WriteFile(path, []byte(got), 0o600))
			}
			want, err := os.ReadFile(path) //nolint:gosec // The path is built from a test case name under testdata.
			require.NoError(t, err, "run with UPDATE_PICKER_FRAMES=1 to write %s", path)
			assert.Equal(t, string(want), got)
		})
	}
}

// Typing with the list focused starts the smear before the edit lands; a
// multiline placeholder then goes and the filter shrinks under it.
func TestFocusSmearFollowsTheFilterAfterAnEdit(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "api"}, {Name: "web"}}, Options{DisplayOptions: DisplayOptions{
		HidePreview: true,
		Placeholder: "Filter\nworkspaces",
	}})
	m.width, m.height, m.reduceMotion = 120, 20, false
	m.listFocused = true
	m.input.Blur()
	require.Equal(t, 2, m.filterRows(), "the placeholder must take two rows")

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = updated.(teaModel)
	require.True(t, m.focusSmearActive)
	require.Equal(t, 1, m.filterRows(), "the edit must drop the placeholder")
	m.View()
	input, ok := m.ui.Frame().Rect(inputID)
	require.True(t, ok)

	assert.Equal(t, input.Max.Y-1, m.focusSmearInputRow, "the smear starts from the input's cursor line")
}

func TestMultilinePromptKeepsFramePadding(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "api"}}, Options{DisplayOptions: DisplayOptions{Prompt: "Choose a workspace\nSesh> "}})
	t.Cleanup(func() { m.cancelActivePreview() })
	m.width, m.height = 120, 28

	lines := strippedLines(m.View().Content)

	assert.True(t, strings.HasPrefix(lines[2], "  │ Choose a workspace "), "first prompt line: %q", lines[2])
	assert.True(t, strings.HasPrefix(lines[3], "  │ Sesh>"), "second prompt line: %q", lines[3])
	assert.NotContains(t, lines[3], "…", "the first prompt line does not count against the second")
}

func TestPreviewShowsSkeletonWhileLoading(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "api"}}, Options{})
	t.Cleanup(func() { m.cancelActivePreview() })
	m.width, m.height = 120, 28
	updated, cmd := m.Update(previewLoadingMsg{requestID: m.previewRequestID})
	m = updated.(teaModel)

	require.True(t, m.previewLoading)
	require.NotNil(t, cmd, "loading starts the shimmer")
	view := ansi.Strip(m.View().Content)
	assert.Contains(t, view, "PREVIEW")
	assert.Contains(t, view, "█")

	updated, _ = m.Update(previewMsg{key: m.previewKey, requestID: m.previewRequestID, text: "ready"})
	m = updated.(teaModel)
	assert.False(t, m.shimmer.Animating())
	assert.NotContains(t, ansi.Strip(m.View().Content), "█")
}

func TestClickWhileSettingsOpenDoesNotChooseWorkspace(t *testing.T) {
	m := newTeaModel([]model.Session{{Source: "herdr", Name: "api", WorkspaceID: "w1"}}, Options{DisplayOptions: DisplayOptions{HidePreview: true}})
	m.width, m.height = 120, 28
	m.View()
	editor, err := settings.Open(config.LoadOptions{Path: filepath.Join(t.TempDir(), "config.toml")}, nil)
	require.NoError(t, err)
	m.settings = &editor

	updated, cmd := m.Update(tea.MouseClickMsg{X: 20, Y: 6, Button: tea.MouseLeft})

	assert.False(t, updated.(teaModel).chosen)
	assert.Nil(t, cmd, "the picker's row handlers must not run under settings")
}

func TestBloomKeepsPreviewColors(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "api"}}, Options{})
	t.Cleanup(func() { m.cancelActivePreview() })
	m.width, m.height = 120, 12
	m.preview = "\x1b[38;2;10;20;30mtrue\x1b[0m \x1b[31mred\x1b[0m \x1b[38;5;141mpalette\x1b[0m"

	view := m.View().Content

	for _, code := range []string{"\x1b[38;2;10;20;30mtrue", "\x1b[31mred", "\x1b[38;5;141mpalette"} {
		assert.Contains(t, view, code)
	}
}

func TestRowClickPassesThroughLayers(t *testing.T) {
	items := []model.Session{
		{Source: "herdr", Name: "api", WorkspaceID: "w1"},
		{Source: "herdr", Name: "web", WorkspaceID: "w2"},
	}
	for _, tc := range []struct {
		name  string
		setup func(*teaModel)
		x, y  int
	}{
		{name: "focus smear", x: 5, y: 6, setup: func(m *teaModel) {
			m.input.SetValue("w")
			m.listFocused = true
			m.focusSmearStart = m.inputCursorColumn()
			*m = m.measureFocusSmear()
			m.focusSmearStep = 2
			m.focusSmearDirection = 1
			m.focusSmearActive = true
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newTeaModel(items, Options{DisplayOptions: DisplayOptions{HidePreview: true}})
			m.width, m.height = 120, 12
			tc.setup(&m)

			hit := m.View().OnMouse(tea.MouseClickMsg{X: tc.x, Y: tc.y, Button: tea.MouseLeft})

			require.NotNil(t, hit)
			assert.Equal(t, workspaceClickedMsg{key: model.Key(items[tc.y-listFirstRowIndex])}, previewResult(hit))
		})
	}
}

func TestToastTakesClicksOnItself(t *testing.T) {
	items := []model.Session{
		{Source: "herdr", Name: "api", WorkspaceID: "w1"},
		{Source: "herdr", Name: "web", WorkspaceID: "w2"},
	}
	m := newTeaModel(items, Options{DisplayOptions: DisplayOptions{HidePreview: true}})
	m.width, m.height = 120, 12
	m.closeError = "Failed to close workspace: boom"
	m.toastText = m.closeError
	m.toast.Show()
	m.toast.Advance(time.Now().Add(time.Hour))
	view := m.View()
	row := 6
	// The toast's top border covers the end of the second row.
	require.Contains(t, strippedLines(view.Content)[row], "web")
	require.Contains(t, strippedLines(view.Content)[row], "╭")

	onToast := view.OnMouse(tea.MouseClickMsg{X: 100, Y: row, Button: tea.MouseLeft})
	beside := view.OnMouse(tea.MouseClickMsg{X: 5, Y: row, Button: tea.MouseLeft})
	view.OnMouse(tea.MouseWheelMsg{X: 100, Y: row, Button: tea.MouseWheelDown})

	if onToast != nil {
		assert.NotEqual(t, workspaceClickedMsg{key: model.Key(items[1])}, previewResult(onToast))
	}
	require.NotNil(t, beside)
	assert.Equal(t, workspaceClickedMsg{key: model.Key(items[1])}, previewResult(beside))
	assert.Equal(t, 1, m.pointer.wheel)
}

func TestWheelOverToastOnPreviewScrollsPreview(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "api"}, {Name: "web"}}, Options{})
	t.Cleanup(func() { m.cancelActivePreview() })
	m.width, m.height, m.reduceMotion = 120, 20, true
	m.listFocused = true
	m.input.Blur()
	lines := make([]string, 40)
	for i := range lines {
		lines[i] = fmt.Sprintf("preview line %02d", i)
	}
	deliver := func(m teaModel) teaModel {
		updated, _ := m.Update(previewMsg{key: m.previewKey, requestID: m.previewRequestID, text: strings.Join(lines, "\n")})
		return updated.(teaModel)
	}
	m = deliver(m)
	m.closeError = "Failed to close workspace: boom"
	m.toastText = m.closeError
	m.toast.Show()
	m.toast.Advance(time.Now().Add(time.Hour))
	screen := strippedLines(m.View().Content)
	require.Contains(t, strings.Join(screen, "\n"), lines[0])
	x, y := -1, -1
	for row, line := range screen {
		if i := strings.Index(line, "boom"); i >= 0 {
			x, y = ansi.StringWidth(line[:i]), row
		}
	}
	require.GreaterOrEqual(t, y, 0, "the toast must be on screen")
	box, ok := m.ui.Frame().Rect(previewBoxID)
	require.True(t, ok)
	require.True(t, x >= box.Min.X && x < box.Max.X && y >= box.Min.Y && y < box.Max.Y, "the toast must cover the preview")

	wheel := tea.MouseWheelMsg{X: x, Y: y, Button: tea.MouseWheelDown}

	updated, _ := m.Update(wheel)
	m = updated.(teaModel)

	assert.Equal(t, 0, m.list.Selected, "the wheel over the toast on the preview leaves the selection")
	assert.NotContains(t, ansi.Strip(m.View().Content), lines[0], "the wheel scrolls the preview beneath the toast")

	// The frame on screen can predate the next workspace's text; its wheel
	// scrolls the old text, so it leaves the new text at the top.
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = deliver(updated.(teaModel))
	require.Equal(t, 1, m.list.Selected)
	updated, _ = m.Update(wheel)
	m = updated.(teaModel)
	assert.Contains(t, ansi.Strip(m.View().Content), lines[0], "a wheel over the old text leaves the new at the top")
}

// A wheel can arrive for a frame older than the last one drawn, such as
// after the divider moves; the toast routes it by the frame it was found in.
func TestToastWheelUsesTheFrameItWasFoundIn(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "api"}, {Name: "web"}}, Options{})
	t.Cleanup(func() { m.cancelActivePreview() })
	m.width, m.height, m.reduceMotion = 120, 20, true
	lines := make([]string, 40)
	for i := range lines {
		lines[i] = fmt.Sprintf("preview line %02d", i)
	}
	updated, _ := m.Update(previewMsg{key: m.previewKey, requestID: m.previewRequestID, text: strings.Join(lines, "\n")})
	m = updated.(teaModel)
	m.closeError = "Failed to close workspace: boom"
	m.toastText = m.closeError
	m.toast.Show()
	m.toast.Advance(time.Now().Add(time.Hour))
	old := m.View()
	before, ok := m.ui.Frame().Rect(previewBoxID)
	require.True(t, ok)
	x, y := before.Min.X+1, -1
	for row, line := range strippedLines(old.Content) {
		if i := strings.Index(line, "Failed"); i >= 0 {
			require.Greater(t, x, ansi.StringWidth(line[:i])-2, "the toast must cover the preview's left edge")
			y = row
		}
	}
	require.GreaterOrEqual(t, y, 0, "the toast must be on screen")

	m.previewWidth = minPreviewWidth
	m.View()
	after, ok := m.ui.Frame().Rect(previewBoxID)
	require.True(t, ok)
	require.Greater(t, after.Min.X, x, "the narrower preview must leave the point over the list")

	old.OnMouse(tea.MouseWheelMsg{X: x, Y: y, Button: tea.MouseWheelDown})

	assert.Zero(t, m.pointer.wheel, "the wheel over the old frame's preview leaves the selection")
	assert.NotContains(t, ansi.Strip(m.View().Content), lines[0], "the wheel scrolls the preview the old frame showed")
}

func TestReduceMotionShowsToastWithoutAnimating(t *testing.T) {
	t.Setenv("HERDR_SESH_REDUCE_MOTION", "1")
	configureMotion()
	t.Cleanup(func() { anim.SetMotion(true) })
	m := newTeaModel([]model.Session{{Name: "api"}}, Options{})
	t.Cleanup(func() { m.cancelActivePreview() })
	m.width, m.height = 120, 28
	m.closeError = "Failed to close workspace: herdr workspace close w1: boom"

	updated, _ := m.Update(tea.FocusMsg{})
	m = updated.(teaModel)

	assert.Contains(t, ansi.Strip(m.View().Content), "herdr workspace close w1")
}

func TestToastKeepsErrorSummaryInShortPane(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "api"}}, Options{DisplayOptions: DisplayOptions{HidePreview: true}})
	m.width, m.height = 120, 12
	m.closeError = "Workspace closed, but sessions could not be refreshed: " + strings.Repeat("stderr line\n", 8)
	updated, _ := m.Update(tea.FocusMsg{})
	m = updated.(teaModel)

	lines := strings.Split(settledView(m), "\n")

	assert.Len(t, lines, 12)
	assert.Contains(t, strings.Join(lines, "\n"), "Workspace closed, but sessions")
	assert.Contains(t, lines[len(lines)-2], "esc exit", "the toast must stay above the footer")
}

func TestSplitDividerSpansBody(t *testing.T) {
	long := strings.Repeat("x", 70)
	for _, tc := range []struct {
		name     string
		sessions []model.Session
		// fits is set when every panel line fits one row, so the frame
		// keeps to the window height.
		fits bool
	}{
		{name: "multiline preview title", sessions: []model.Session{{Name: "first\nsecond"}, {Name: "web"}}},
		{name: "truncated multiline title", sessions: []model.Session{{Name: long + "\nsecond"}, {Name: "web"}}, fits: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newTeaModel(tc.sessions, Options{})
			t.Cleanup(func() { m.cancelActivePreview() })
			// 20 rows leaves the preview above its minimum height.
			m.width, m.height = 120, 20
			m.preview = "body"
			_, previewWidth := m.previewLayout()
			require.Positive(t, previewWidth)

			lines := strippedLines(m.View().Content)

			// Rows 5 up to the rule above the footer hold the two panels.
			column := m.contentWidth() + horizontalPadding - previewWidth - 2
			for y := 5; y < len(lines)-2; y++ {
				assert.Equal(t, "│", ansi.Cut(lines[y], column, column+1), "row %d:\n%s", y, strings.Join(lines, "\n"))
			}
			if tc.fits {
				// The last window row is blank, so strippedLines drops it.
				assert.Len(t, lines, m.height-1, "no row beyond the panels:\n%s", strings.Join(lines, "\n"))
			}
		})
	}
}

func TestSettingsReloadKeepsTerminalColors(t *testing.T) {
	items := []model.Session{{Name: "api"}}
	loadingLine := func(m teaModel) string {
		m.width, m.height = 120, 14
		m.previewLoading = true
		for _, line := range strings.Split(m.View().Content, "\n") {
			if strings.Contains(line, "█") {
				return line
			}
		}
		return ""
	}
	m := newTeaModel(items, Options{})
	t.Cleanup(func() { m.cancelActivePreview() })
	updated, _ := m.Update(tea.BackgroundColorMsg{Color: color.White})
	m = updated.(teaModel)
	want := loadingLine(m)
	require.NotEmpty(t, want)

	m.settings = &settings.Model{}
	updated, _ = m.Update(settingsReloadedMsg{result: ReloadResult{Sessions: items}})
	reloaded := updated.(teaModel)
	t.Cleanup(func() { reloaded.cancelActivePreview() })

	assert.Equal(t, want, loadingLine(reloaded))
}

func TestWheelOverHeaderMovesSelection(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "api"}, {Name: "web"}}, Options{})
	t.Cleanup(func() { m.cancelActivePreview() })
	m.width, m.height, m.reduceMotion = 120, 20, true
	m.listFocused = true
	m.View()

	updated, _ := m.Update(tea.MouseWheelMsg{X: 20, Y: 1, Button: tea.MouseWheelDown})

	assert.Equal(t, 1, updated.(teaModel).list.Selected)
}

func TestScrollbarClickScrollsListAndSelection(t *testing.T) {
	items := make([]model.Session, 30)
	for i := range items {
		items[i] = model.Session{Name: fmt.Sprintf("workspace-%02d", i)}
	}
	m := newTeaModel(items, Options{DisplayOptions: DisplayOptions{HidePreview: true}})
	t.Cleanup(func() { m.cancelActivePreview() })
	m.width, m.height = 120, 12
	lines := strippedLines(m.View().Content)
	// The scrollbar is on the list box's right border, just inside the
	// right padding.
	scrollbarX := m.contentWidth() + horizontalPadding - 1
	require.Equal(t, "┃", ansi.Cut(lines[listFirstRowIndex], scrollbarX, scrollbarX+1))

	updated, _ := m.Update(tea.MouseClickMsg{X: scrollbarX, Y: m.height - 4, Button: tea.MouseLeft})
	m = updated.(teaModel)

	require.Positive(t, m.listOffset)
	view := ansi.Strip(m.View().Content)
	assert.Contains(t, view, "┃   [session] "+items[m.list.Selected].Name, "the rail marks the new selection")
	assert.NotContains(t, view, "workspace-00")
	assert.True(t, m.listFocused)
	assert.False(t, m.smearActive, "no trail across the jump")
}

func TestWheelInSideMarginMovesSelection(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "api"}, {Name: "web"}}, Options{})
	t.Cleanup(func() { m.cancelActivePreview() })
	m.width, m.height, m.reduceMotion = 120, 20, true
	m.listFocused = true
	m.View()

	updated, _ := m.Update(tea.MouseWheelMsg{X: 0, Y: 7, Button: tea.MouseWheelDown})

	assert.Equal(t, 1, updated.(teaModel).list.Selected)
}

func TestShortStackedPaneKeepsWorkspaceRow(t *testing.T) {
	for _, height := range []int{10, 11, 12} {
		m := newTeaModel([]model.Session{{Name: "api"}, {Name: "web"}}, Options{})
		m.cancelActivePreview()
		m.width, m.height = 80, height
		view := ansi.Strip(m.View().Content)
		assert.Contains(t, view, "[session] api", "height %d", height)
		if height == 11 {
			// Too short for a preview line: drop the pane rather than show a bare title.
			assert.NotContains(t, view, "PREVIEW", "height %d", height)
		}
	}
}

func TestWheelOverPreviewScrollsPreview(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "api"}, {Name: "web"}}, Options{})
	t.Cleanup(func() { m.cancelActivePreview() })
	m.width, m.height, m.reduceMotion = 120, 16, true
	m.listFocused = true
	m.input.Blur()
	lines := make([]string, 40)
	for i := range lines {
		lines[i] = fmt.Sprintf("preview line %02d", i)
	}
	long := strings.Join(lines, "\n")
	deliver := func(m teaModel) teaModel {
		updated, _ := m.Update(previewMsg{key: m.previewKey, requestID: m.previewRequestID, text: long})
		return updated.(teaModel)
	}
	wheel := func(m teaModel, x, y int) teaModel {
		m.View()
		updated, _ := m.Update(tea.MouseWheelMsg{X: x, Y: y, Button: tea.MouseWheelDown})
		return updated.(teaModel)
	}
	m = deliver(m)
	require.Contains(t, ansi.Strip(m.View().Content), "preview line 00")

	m = wheel(m, 100, 8)
	assert.NotContains(t, ansi.Strip(m.View().Content), "preview line 00")
	// The preview title and its scrollbar take the wheel too.
	m = wheel(m, 100, 5)
	m = wheel(m, m.contentWidth()+horizontalPadding-1, 8)
	require.Equal(t, 0, m.list.Selected, "the wheel over the preview leaves the selection")

	m = deliver(m)
	assert.NotContains(t, ansi.Strip(m.View().Content), "preview line 00", "a refresh of the same preview keeps its scroll")

	// Scrolling the old text while the next preview loads does not carry over.
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(teaModel)
	require.Equal(t, 1, m.list.Selected)
	m = wheel(m, 100, 8)
	m = deliver(m)
	assert.Contains(t, ansi.Strip(m.View().Content), "preview line 00", "another workspace starts at the top")

	m = wheel(m, 100, 8)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	m = deliver(updated.(teaModel))
	assert.Contains(t, ansi.Strip(m.View().Content), "preview line 00", "another preview mode starts at the top")

	// The frame on screen can predate the next workspace's text; its wheel
	// scrolls the old text, so it leaves the new text at the top.
	m.View()
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m = deliver(updated.(teaModel))
	updated, _ = m.Update(tea.MouseWheelMsg{X: 100, Y: 8, Button: tea.MouseWheelDown})
	m = updated.(teaModel)
	assert.Contains(t, ansi.Strip(m.View().Content), "preview line 00", "a wheel over the old text leaves the new at the top")
}

// bloom's viewport passes a wheel step outward when it can't move that way;
// the preview box must take it, so the selection stays.
func TestWheelAtPreviewEdgeLeavesSelection(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "api"}, {Name: "web"}, {Name: "dots"}}, Options{})
	t.Cleanup(func() { m.cancelActivePreview() })
	m.width, m.height, m.reduceMotion = 120, 16, true
	m.listFocused = true
	m.input.Blur()
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(teaModel)
	require.Equal(t, 1, m.list.Selected)
	deliver := func(m teaModel, text string) teaModel {
		updated, _ := m.Update(previewMsg{key: m.previewKey, requestID: m.previewRequestID, text: text})
		return updated.(teaModel)
	}
	wheel := func(m teaModel, button tea.MouseButton) teaModel {
		m.View()
		updated, _ := m.Update(tea.MouseWheelMsg{X: 100, Y: 8, Button: button})
		return updated.(teaModel)
	}

	m = deliver(m, "line one\nline two")
	m = wheel(m, tea.MouseWheelDown)
	assert.Equal(t, 1, m.list.Selected, "wheel down over a preview whose text all fits")
	m = wheel(m, tea.MouseWheelUp)
	assert.Equal(t, 1, m.list.Selected, "wheel up over a preview whose text all fits")

	lines := make([]string, 40)
	for i := range lines {
		lines[i] = fmt.Sprintf("preview line %02d", i)
	}
	m = deliver(m, strings.Join(lines, "\n"))
	m = wheel(m, tea.MouseWheelUp)
	assert.Equal(t, 1, m.list.Selected, "a preview at its top")
	for range 20 {
		m = wheel(m, tea.MouseWheelDown)
	}
	require.Contains(t, ansi.Strip(m.View().Content), "preview line 39")
	m = wheel(m, tea.MouseWheelDown)
	assert.Equal(t, 1, m.list.Selected, "a preview at its bottom")
}

func TestScrollbarIgnoredAfterFilterChanges(t *testing.T) {
	items := make([]model.Session, 30)
	for i := range items {
		items[i] = model.Session{Name: fmt.Sprintf("workspace-%02d", i)}
	}
	for _, tc := range []struct{ name, query string }{
		{name: "other matches", query: "workspace-2"},
		{name: "no matches", query: "zzz"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newTeaModel(items, Options{DisplayOptions: DisplayOptions{HidePreview: true}})
			t.Cleanup(func() { m.cancelActivePreview() })
			m.width, m.height, m.reduceMotion = 120, 12, true
			// The frame on screen is drawn before the filter changes.
			m.View()
			m.input.SetValue(tc.query)
			m = m.filter(tc.query)
			selected := m.list.Selected

			updated, _ := m.Update(tea.MouseClickMsg{X: m.contentWidth() + horizontalPadding - 1, Y: m.height - 4, Button: tea.MouseLeft})
			m = updated.(teaModel)

			assert.Zero(t, m.listOffset)
			assert.Equal(t, selected, m.list.Selected)
			assert.False(t, m.listFocused, "the filter keeps the focus")
		})
	}
}

func TestDividerGripAsksForPointerMotion(t *testing.T) {
	for _, tc := range []struct {
		name   string
		width  int
		hidden bool
		want   tea.MouseMode
	}{
		{name: "split", width: 120, want: tea.MouseModeAllMotion},
		{name: "stacked", width: 80, want: tea.MouseModeCellMotion},
		{name: "hidden", width: 120, hidden: true, want: tea.MouseModeCellMotion},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newTeaModel([]model.Session{{Name: "api"}}, Options{DisplayOptions: DisplayOptions{HidePreview: tc.hidden}})
			t.Cleanup(func() { m.cancelActivePreview() })
			m.width, m.height = tc.width, 20

			assert.Equal(t, tc.want, m.View().MouseMode)
		})
	}
}

func TestMouseScrollIgnoredWhileWorkspaceCloses(t *testing.T) {
	items := make([]model.Session, 30)
	for i := range items {
		items[i] = model.Session{Name: fmt.Sprintf("workspace-%02d", i), WorkspaceID: fmt.Sprintf("w%d", i)}
	}
	m := newTeaModel(items, Options{DisplayOptions: DisplayOptions{HidePreview: true}})
	t.Cleanup(func() { m.cancelActivePreview() })
	m.width, m.height, m.reduceMotion = 120, 12, true
	m.listFocused = true
	// The frame on screen is still the one drawn before the close started,
	// so its handlers are the ones that run.
	m.View()
	m.closingWorkspaceID = "w0"
	scrollbarX := m.contentWidth() + horizontalPadding - 1

	for _, msg := range []tea.Msg{
		tea.MouseWheelMsg{X: 20, Y: 7, Button: tea.MouseWheelUp},
		tea.MouseClickMsg{X: scrollbarX, Y: m.height - 4, Button: tea.MouseLeft},
		tea.MouseReleaseMsg{X: scrollbarX, Y: m.height - 4, Button: tea.MouseLeft},
	} {
		updated, _ := m.Update(msg)
		m = updated.(teaModel)
	}
	require.Equal(t, 0, m.list.Selected)
	require.Zero(t, m.listOffset)

	m.closingWorkspaceID = ""
	m.View()
	updated, _ := m.Update(tea.MouseWheelMsg{X: 20, Y: 7, Button: tea.MouseWheelDown})

	assert.Equal(t, 1, updated.(teaModel).list.Selected, "no wheel step left over from the close")
}

func strippedLines(s string) []string {
	lines := strings.Split(ansi.Strip(s), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// renderedPreview draws the preview pane alone, title included, in a
// width×rows pane, without styles.
func renderedPreview(m teaModel, width, rows int) string {
	return ansi.Strip(bloom.Render(width, rows, m.previewBox(width)))
}

// renderedList draws the workspace list alone in a width×rows pane.
func renderedList(m teaModel, width, rows int) string {
	return bloom.Render(width, rows, m.listBody(width))
}

// Screen rows of the picker with a one-line prompt.
const (
	filterLineIndex   = 2
	listFirstRowIndex = 5
)

// fgSequence is the SGR parameters that draw text in c.
func fgSequence(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("38;2;%d;%d;%d", r>>8, g>>8, b>>8)
}

func TestFocusedBoxBorderIsLit(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "api"}, {Name: "web"}}, Options{})
	t.Cleanup(func() { m.cancelActivePreview() })
	m.width, m.height = 120, 20
	lit := fgSequence(skyColor)
	borders := func(m teaModel) (filter, list string) {
		lines := strings.Split(m.View().Content, "\n")
		return lines[filterLineIndex-1], lines[listFirstRowIndex-1]
	}

	filter, list := borders(m)
	assert.Contains(t, filter, lit, "the filter has the focus")
	assert.NotContains(t, list, lit)

	m.listFocused = true
	filter, list = borders(m)
	assert.NotContains(t, filter, lit)
	assert.Contains(t, list, lit, "the list has the focus")
}

func TestClosingWorkspaceTurnsItsBoxRed(t *testing.T) {
	for _, hidden := range []bool{false, true} {
		t.Run(fmt.Sprintf("hide preview %t", hidden), func(t *testing.T) {
			m := newTeaModel([]model.Session{{Source: "herdr", Name: "api", WorkspaceID: "w1"}}, Options{DisplayOptions: DisplayOptions{HidePreview: hidden}})
			t.Cleanup(func() { m.cancelActivePreview() })
			m.width, m.height = 120, 20
			red := fgSequence(redColor)
			require.NotContains(t, m.View().Content, red)

			m.closingWorkspaceID = "w1"
			assert.Contains(t, strings.Split(m.View().Content, "\n")[listFirstRowIndex-1], red)
		})
	}
}

func TestPreviewFadesInTextForAnotherWorkspaceOnly(t *testing.T) {
	m := newTeaModel([]model.Session{{Source: "herdr", Name: "api", WorkspaceID: "w1"}}, Options{})
	t.Cleanup(func() { m.cancelActivePreview() })
	m.width, m.height, m.reduceMotion = 120, 20, false
	settle := func() { m.previewIn.Advance(time.Now().Add(time.Hour)) }
	deliver := func(text string) {
		updated, _ := m.Update(previewMsg{key: m.previewKey, requestID: m.previewRequestID, text: text})
		m = updated.(teaModel)
	}

	deliver("first text")
	require.True(t, m.previewIn.Animating(), "new text fades in")
	assert.Contains(t, renderedPreview(m, 60, 6), "first text", "the fade keeps the glyphs")

	settle()
	deliver("refreshed text")
	assert.False(t, m.previewIn.Animating(), "a refresh of the same workspace does not fade again")

	m.panePreview = !m.panePreview
	deliver("another mode")
	assert.True(t, m.previewIn.Animating(), "another preview mode fades in")

	settle()
	m.reduceMotion = true
	m.panePreview = !m.panePreview
	deliver("still")
	assert.False(t, m.previewIn.Animating(), "reduced motion shows text at once")
}

func TestWorkspaceCountRollsWhenTheFilterChangesIt(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "api"}, {Name: "web"}}, Options{})
	t.Cleanup(func() { m.cancelActivePreview() })
	m.width, m.height, m.reduceMotion = 120, 20, false

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = updated.(teaModel)
	require.True(t, m.countRoll.Animating())
	assert.Equal(t, "2 workspaces", m.countFrom)

	m.countRoll.Advance(time.Now().Add(time.Hour))
	require.Contains(t, strings.Split(ansi.Strip(m.View().Content), "\n")[filterLineIndex-1], "1/2 workspaces")
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(teaModel)
	assert.False(t, m.countRoll.Animating(), "the count did not change")
}

// TestEffectsChangeTheFrameMidway draws each effect partway through and
// checks it changes what is on screen, not only the model's state.
func TestEffectsChangeTheFrameMidway(t *testing.T) {
	press := func(key tea.KeyPressMsg) func(*teaModel) {
		return func(m *teaModel) {
			updated, _ := m.Update(key)
			*m = updated.(teaModel)
		}
	}
	for _, tc := range []struct {
		name   string
		before func(*teaModel)
		start  func(*teaModel)
		value  func(*teaModel) *anim.Value
		midway time.Duration
		// glyphs reports whether the effect moves or changes glyphs, not
		// only their colors.
		glyphs bool
	}{
		{name: "preview fade", start: func(m *teaModel) {
			updated, _ := m.Update(previewMsg{key: m.previewKey, requestID: m.previewRequestID, text: "plain preview text"})
			*m = updated.(teaModel)
		}, value: func(m *teaModel) *anim.Value { return &m.previewIn }, midway: previewInDuration / 2},
		// The first key adds the "13/" in front, which has no digits to roll
		// from; the next one turns 13 over to 10, which shows by halfway.
		{name: "count roll", before: press(tea.KeyPressMsg{Code: 'w', Text: "w"}), start: press(tea.KeyPressMsg{Code: '0', Text: "0"}),
			value: func(m *teaModel) *anim.Value { return &m.countRoll }, midway: countRollDuration / 4, glyphs: true},
		// A quarter of the way in, the wobble is at its widest.
		{name: "shake", start: press(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl}),
			value: func(m *teaModel) *anim.Value { return &m.shake }, midway: refusedShakeDuration / 4, glyphs: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			items := make([]model.Session, 14)
			items[0] = model.Session{Name: "api"}
			for i := range 13 {
				items[i+1] = model.Session{Name: fmt.Sprintf("w%02d", i)}
			}
			m := newTeaModel(items, Options{})
			t.Cleanup(func() { m.cancelActivePreview() })
			m.width, m.height, m.reduceMotion = 120, 20, false
			if tc.before != nil {
				tc.before(&m)
			}
			m.countRoll.Advance(time.Now().Add(time.Hour))
			start := time.Now()
			tc.start(&m)
			v := tc.value(&m)
			require.True(t, v.Animating())

			v.Advance(start.Add(tc.midway))
			midway := m.View().Content
			v.Advance(start.Add(time.Hour))
			settled := m.View().Content

			if tc.glyphs {
				settled, midway = ansi.Strip(settled), ansi.Strip(midway)
			}
			assert.NotEqual(t, settled, midway)
		})
	}
}

func TestPreviewShownMatchesTheLayout(t *testing.T) {
	for _, prompt := range []string{"", "Choose a workspace\nSesh> "} {
		for _, width := range []int{80, 120} {
			for height := 6; height <= 24; height++ {
				m := newTeaModel([]model.Session{{Name: "api"}, {Name: "web"}}, Options{DisplayOptions: DisplayOptions{Prompt: prompt}})
				m.width, m.height = width, height
				_, shown := bloom.Solve(width, height, m.pickerTree()).Rect(previewBoxID)
				assert.Equal(t, shown, m.previewShown(), "%dx%d, prompt %q", width, height, prompt)
				m.cancelActivePreview()
			}
		}
	}
}

func TestCollapsedPreviewMovesCloseStatusToTheFooter(t *testing.T) {
	m := newTeaModel([]model.Session{{Source: "herdr", Name: "api", WorkspaceID: "w1"}}, Options{})
	t.Cleanup(func() { m.cancelActivePreview() })
	m.width, m.height = 80, 11
	require.False(t, m.previewShown(), "the stacked preview collapses at this height")
	m.closingWorkspaceID = "w1"

	view := m.View().Content
	assert.Contains(t, ansi.Strip(view), "Closing workspace...")
	assert.Contains(t, strings.Split(view, "\n")[listFirstRowIndex-1], fgSequence(redColor), "the list's border turns red")

	m.quitAfterWorkspaceClose = true
	assert.Contains(t, ansi.Strip(m.View().Content), "Cancelling workspace close...")
}

func shortPaneItems() []model.Session {
	return []model.Session{
		{Source: "herdr", Name: "workspace-00", WorkspaceID: "w0"},
		{Source: "herdr", Name: "workspace-01", WorkspaceID: "w1"},
	}
}

func TestShortPaneKeepsWorkspaceRowAndCloseStatus(t *testing.T) {
	for _, tc := range []struct {
		width, height int
		hide          bool
	}{
		{120, 7, false}, {120, 8, false}, {80, 7, false}, {80, 8, false}, {100, 7, true}, {100, 8, true},
	} {
		t.Run(fmt.Sprintf("%dx%d hide=%t", tc.width, tc.height, tc.hide), func(t *testing.T) {
			m := newTeaModel(shortPaneItems(), Options{DisplayOptions: DisplayOptions{HidePreview: tc.hide}, Backend: &fakeBackend{}})
			t.Cleanup(func() { m.cancelActivePreview() })
			m.width, m.height, m.reduceMotion = tc.width, tc.height, true

			assert.Contains(t, ansi.Strip(m.View().Content), "[herdr]   workspace-00")

			updated, closing := m.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
			m = updated.(teaModel)
			require.NotNil(t, closing)
			require.Equal(t, "w0", m.closingWorkspaceID)
			assert.Contains(t, ansi.Strip(m.View().Content), "Closing workspace...")
		})
	}
}

func TestOneRowListKeepsItsRightBorder(t *testing.T) {
	for _, tc := range []struct {
		name  string
		width int
		hide  bool
	}{
		{"stacked", 80, false}, {"split", 120, false}, {"list only", 100, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newTeaModel(shortPaneItems(), Options{DisplayOptions: DisplayOptions{HidePreview: tc.hide}})
			t.Cleanup(func() { m.cancelActivePreview() })
			m.width, m.height = tc.width, 9
			lines := strippedLines(m.View().Content)
			right := ansi.StringWidth(strings.SplitN(lines[listFirstRowIndex-1], "╮", 2)[0])

			assert.Contains(t, lines[listFirstRowIndex], "workspace-00")
			assert.Equal(t, "│", ansi.Cut(lines[listFirstRowIndex], right, right+1))
		})
	}
}

func TestDividerIgnoresMotionAfterLostRelease(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "api"}, {Name: "web"}}, Options{})
	t.Cleanup(func() { m.cancelActivePreview() })
	m.width, m.height, m.reduceMotion = 120, 20, true
	listWidth, _ := m.previewLayout()
	grip := horizontalPadding + listWidth + 2
	send := func(msg tea.MouseMsg) {
		m.View().OnMouse(msg)
		updated, _ := m.Update(msg)
		m = updated.(teaModel)
	}

	send(tea.MouseClickMsg{X: grip, Y: 8, Button: tea.MouseLeft})
	send(tea.MouseMotionMsg{X: grip - 10, Y: 8, Button: tea.MouseLeft})
	_, held := m.previewLayout()
	send(tea.MouseMotionMsg{X: grip - 25, Y: 8})
	_, afterLost := m.previewLayout()
	send(tea.MouseMotionMsg{X: grip - 5, Y: 8})
	_, later := m.previewLayout()

	assert.Equal(t, held, afterLost)
	assert.Equal(t, held, later)
	assert.False(t, m.pointer.dividerActive)
}

func TestDividerDragWhileSettingsLoadIsDropped(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "api"}, {Name: "web"}}, Options{Backend: &fakeBackend{}})
	t.Cleanup(func() { m.cancelActivePreview() })
	m.width, m.height, m.reduceMotion = 120, 20, true
	listWidth, before := m.previewLayout()
	grip := horizontalPadding + listWidth + 2
	send := func(msg tea.MouseMsg) {
		m.View().OnMouse(msg)
		updated, _ := m.Update(msg)
		m = updated.(teaModel)
	}
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyF2})
	m = updated.(teaModel)
	require.True(t, m.settingsBusy)

	send(tea.MouseClickMsg{X: grip, Y: 8, Button: tea.MouseLeft})
	send(tea.MouseMotionMsg{X: grip - 15, Y: 8, Button: tea.MouseLeft})
	send(tea.MouseReleaseMsg{X: grip - 15, Y: 8, Button: tea.MouseLeft})
	updated, _ = m.Update(settingsOpenedMsg{err: fmt.Errorf("boom")})
	m = updated.(teaModel)
	require.False(t, m.settingsBusy)
	send(tea.MouseClickMsg{X: 20, Y: filterLineIndex, Button: tea.MouseLeft})
	send(tea.MouseReleaseMsg{X: 20, Y: filterLineIndex, Button: tea.MouseLeft})

	_, after := m.previewLayout()
	assert.Equal(t, before, after)
}

func TestLoadingPlaceholderShowsWhileWorkspaceCloses(t *testing.T) {
	items := []model.Session{
		{Source: "herdr", Name: "api", WorkspaceID: "w1"},
		{Source: "herdr", Name: "web", WorkspaceID: "w2"},
	}
	m := newTeaModel(items, Options{Backend: &fakeBackend{}})
	t.Cleanup(func() { m.cancelActivePreview() })
	m.width, m.height, m.reduceMotion = 120, 20, true
	updated, _ := m.Update(previewMsg{key: m.previewKey, requestID: m.previewRequestID, text: "API PREVIEW TEXT"})
	m = updated.(teaModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	m = updated.(teaModel)
	require.Equal(t, "w1", m.closingWorkspaceID)
	for range 2 { // focus the list, then select web
		updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		m = updated.(teaModel)
	}
	require.Equal(t, 1, m.list.Selected)
	updated, _ = m.Update(previewLoadingMsg{requestID: m.previewRequestID})
	m = updated.(teaModel)
	require.True(t, m.previewLoading)

	view := ansi.Strip(m.View().Content)
	assert.NotContains(t, view, "API PREVIEW TEXT")
	assert.Contains(t, view, "█")
}

func TestWheelOverPreviewBorderScrollsPreview(t *testing.T) {
	for _, size := range []struct{ width, height int }{{120, 18}, {80, 23}} {
		for _, region := range []string{"title", "scrollbar", "left border"} {
			t.Run(fmt.Sprintf("%dx%d %s", size.width, size.height, region), func(t *testing.T) {
				m := newTeaModel([]model.Session{{Name: "api"}, {Name: "web"}}, Options{})
				t.Cleanup(func() { m.cancelActivePreview() })
				m.width, m.height, m.reduceMotion = size.width, size.height, true
				lines := make([]string, 40)
				for i := range lines {
					lines[i] = fmt.Sprintf("preview line %02d", i)
				}
				updated, _ := m.Update(previewMsg{key: m.previewKey, requestID: m.previewRequestID, text: strings.Join(lines, "\n")})
				m = updated.(teaModel)
				view := m.View()
				box, ok := m.ui.Frame().Rect(previewBoxID)
				require.True(t, ok)
				require.Contains(t, ansi.Strip(view.Content), lines[0])
				x, y := box.Min.X+4, box.Min.Y
				switch region {
				case "scrollbar":
					x, y = box.Max.X-1, box.Min.Y+1
				case "left border":
					x, y = box.Min.X, box.Min.Y+1
				}

				wheel := tea.MouseWheelMsg{X: x, Y: y, Button: tea.MouseWheelDown}
				view.OnMouse(wheel)
				updated, _ = m.Update(wheel)
				m = updated.(teaModel)

				assert.NotContains(t, ansi.Strip(m.View().Content), lines[0])
				assert.Equal(t, 0, m.list.Selected)
			})
		}
	}
}

// A terminal can send several wheel events before the screen is redrawn;
// each steps on from the last, not from the frame they all reach.
func TestWheelStepsOverPreviewBorderAddUp(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "api"}, {Name: "web"}}, Options{})
	t.Cleanup(func() { m.cancelActivePreview() })
	m.width, m.height, m.reduceMotion = 120, 20, true
	m.listFocused = true
	m.input.Blur()
	lines := make([]string, 40)
	for i := range lines {
		lines[i] = fmt.Sprintf("preview line %02d", i)
	}
	deliver := func(m teaModel) teaModel {
		updated, _ := m.Update(previewMsg{key: m.previewKey, requestID: m.previewRequestID, text: strings.Join(lines, "\n")})
		return updated.(teaModel)
	}
	key := func(m teaModel, code rune) teaModel {
		updated, _ := m.Update(tea.KeyPressMsg{Code: code})
		return deliver(updated.(teaModel))
	}
	m = deliver(m)
	painted := m.View()
	box, ok := m.ui.Frame().Rect(previewBoxID)
	require.True(t, ok)
	wheel := tea.MouseWheelMsg{X: box.Min.X + 4, Y: box.Min.Y, Button: tea.MouseWheelDown}
	top := func(m teaModel) string { return strippedLines(m.View().Content)[box.Min.Y+1] }

	painted.OnMouse(wheel)
	m.View()
	painted.OnMouse(wheel)

	assert.Contains(t, top(m), lines[2*previewWheelStep], "both steps scroll the preview")

	// The frame drawn last may show another workspace's preview, scrolled
	// elsewhere; a step for this one then goes from the frame it reached.
	painted = m.View()
	m = key(m, tea.KeyDown)
	m.View().OnMouse(wheel)
	m.View()
	m = key(m, tea.KeyUp)
	painted.OnMouse(wheel)

	assert.Contains(t, top(m), lines[3*previewWheelStep], "the step goes from where the frame it reached had the preview")
}

func TestReduceMotionStillsAgentSpinner(t *testing.T) {
	t.Setenv("HERDR_SESH_REDUCE_MOTION", "1")
	m := newTeaModel([]model.Session{{Name: "api", Source: "herdr", WorkspaceID: "w1", AgentStatus: "working"}}, Options{})
	t.Cleanup(func() { m.cancelActivePreview() })
	m.width, m.height = 120, 12
	before := strippedLines(m.View().Content)[listFirstRowIndex]

	updated, next := m.Update(m.agentSpinner.Tick())
	m = updated.(teaModel)

	assert.Equal(t, before, strippedLines(m.View().Content)[listFirstRowIndex])
	assert.Nil(t, next)
}

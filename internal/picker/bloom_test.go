package picker

import (
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	for _, tc := range []struct {
		name   string
		width  int
		height int
		opts   DisplayOptions
		setup  func(*teaModel)
	}{
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
			m.focusSmearSteps = m.focusSmearDistance()
			m.focusSmearStep = 2
			m.focusSmearDirection = 1
			m.focusSmearActive = true
		}},
		{name: "closing", width: 120, height: 28, setup: func(m *teaModel) {
			m.closingWorkspaceID = "w1"
			m.previewKey = ""
		}},
		{name: "closing list only", width: 120, height: 28, opts: DisplayOptions{HidePreview: true}, setup: func(m *teaModel) {
			m.closingWorkspaceID = "w1"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newTeaModel(sessions, Options{DisplayOptions: tc.opts})
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

func TestMultilinePromptKeepsFramePadding(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "api"}}, Options{DisplayOptions: DisplayOptions{Prompt: "Choose a workspace\nSesh> "}})
	t.Cleanup(func() { m.cancelActivePreview() })
	m.width, m.height = 120, 28

	lines := strippedLines(m.View().Content)

	assert.Equal(t, "  Choose a workspace", lines[3])
	assert.True(t, strings.HasPrefix(lines[4], "  Sesh>"), "second prompt line: %q", lines[4])
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
		{name: "focus smear", x: 3, y: 7, setup: func(m *teaModel) {
			m.input.SetValue("w")
			m.listFocused = true
			m.focusSmearStart = m.inputCursorColumn()
			m.focusSmearSteps = m.focusSmearDistance()
			m.focusSmearStep = 2
			m.focusSmearDirection = 1
			m.focusSmearActive = true
		}},
		{name: "toast", x: 100, y: 6, setup: func(m *teaModel) {
			m.closeError = "Failed to close workspace: boom"
			m.toastText = m.closeError
			m.toast.Show()
			m.toast.Advance(time.Now().Add(time.Hour))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newTeaModel(items, Options{DisplayOptions: DisplayOptions{HidePreview: true}})
			m.width, m.height = 120, 12
			tc.setup(&m)

			hit := m.View().OnMouse(tea.MouseClickMsg{X: tc.x, Y: tc.y, Button: tea.MouseLeft})

			require.NotNil(t, hit)
			assert.Equal(t, workspaceClickedMsg{key: model.Key(items[tc.y-6])}, previewResult(hit))
		})
	}
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

func listText(v workspaceListView) string {
	return strings.Join(v.lines, "\n")
}

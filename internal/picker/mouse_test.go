package picker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/fullerzz/herdr-plugin-sesh/internal/config"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/fullerzz/herdr-plugin-sesh/internal/settings"
	"github.com/fullerzz/herdr-plugin-sesh/internal/sources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mouseTestOutput struct {
	mu           sync.Mutex
	buffer       bytes.Buffer
	marker       string
	initial      chan struct{}
	once         sync.Once
	initialBytes int
}

func (w *mouseTestOutput) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.buffer.Write(p)
	if strings.Contains(w.buffer.String(), w.marker) {
		w.once.Do(func() {
			w.initialBytes = w.buffer.Len()
			close(w.initial)
		})
	}
	return n, err
}

// Mouse input must arrive after a frame is flushed, as it does in a terminal.
func runMouseInput(t *testing.T, m teaModel, input string) (teaModel, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	reader, writer := io.Pipe()
	defer func() { _ = reader.Close() }()
	output := &mouseTestOutput{marker: "WORKSPACES", initial: make(chan struct{})}
	inputErr := make(chan error, 1)
	go func() {
		defer func() { _ = writer.Close() }()
		select {
		case <-output.initial:
			_, err := io.WriteString(writer, input)
			inputErr <- err
		case <-ctx.Done():
			inputErr <- ctx.Err()
		}
	}()
	final, err := tea.NewProgram(m, tea.WithInput(reader), tea.WithOutput(output),
		tea.WithWindowSize(m.width, m.height), tea.WithContext(ctx)).Run()
	require.NoError(t, err)
	require.NoError(t, <-inputErr)
	return final.(teaModel), output.buffer.String()
}

type statusClickTestModel struct {
	teaModel

	output                *mouseTestOutput
	statusApplied         chan struct{}
	clickedOnInitialFrame bool
}

//nolint:ireturn // Bubble Tea models return the framework interface.
func (m statusClickTestModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(tea.MouseClickMsg); ok {
		m.output.mu.Lock()
		m.clickedOnInitialFrame = m.output.buffer.Len() == m.output.initialBytes
		m.output.mu.Unlock()
	}
	next, cmd := m.teaModel.Update(msg)
	m.teaModel = next.(teaModel)
	if _, ok := msg.(agentStatusesMsg); ok {
		select {
		case <-m.statusApplied:
		default:
			close(m.statusApplied)
		}
	}
	return m, cmd
}

func TestTeaModelClickUsesLastRenderedFrame(t *testing.T) {
	items := []model.Session{
		{Source: "herdr", Name: "api-workspace", WorkspaceID: "w1"},
		{Source: "herdr", Name: "web-workspace", WorkspaceID: "w2"},
	}
	output := &mouseTestOutput{marker: "web-workspace", initial: make(chan struct{})}
	statusApplied := make(chan struct{})
	m := statusClickTestModel{
		teaModel: newTeaModel(items, Options{DisplayOptions: DisplayOptions{HidePreview: true, WorkspaceSort: "agent"}, Backend: &fakeBackend{refreshAgentStatuses: func() (map[string]string, error) {
			return map[string]string{"w1": "idle", "w2": "blocked"}, nil
		}}}),
		output: output, statusApplied: statusApplied,
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	p := tea.NewProgram(m, tea.WithInput(nil), tea.WithOutput(output), tea.WithWindowSize(120, 28), tea.WithContext(ctx))
	go func() {
		select {
		case <-output.initial:
		case <-ctx.Done():
			return
		}
		p.Send(statusRefreshTickMsg{})
		select {
		case <-statusApplied:
		case <-ctx.Done():
			return
		}
		p.Send(tea.MouseClickMsg{X: 20, Y: 6, Button: tea.MouseLeft})
	}()
	final, err := p.Run()
	require.NoError(t, err)
	result := final.(statusClickTestModel)
	require.True(t, result.clickedOnInitialFrame, "the original row must still be on screen when clicked")
	assert.Equal(t, "api-workspace", result.choice.Name)
}

func TestTeaModelClickWithMultilinePrompt(t *testing.T) {
	for _, tc := range []struct {
		name   string
		width  int
		hidden bool
	}{
		{name: "hidden", width: 120, hidden: true},
		{name: "split", width: 120},
		{name: "stacked", width: 80},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "config.toml")
			data := fmt.Sprintf("version = 1\n[picker]\nprompt = \"Choose a workspace\\nSesh> \"\nshow_preview = %t\n", !tc.hidden)
			require.NoError(t, os.WriteFile(configPath, []byte(data), 0o600))
			cfg, _, err := config.Load(config.LoadOptions{Path: configPath})
			require.NoError(t, err)
			m := newTeaModel([]model.Session{
				{Source: "herdr", Name: "api", WorkspaceID: "w1"},
				{Source: "herdr", Name: "web", WorkspaceID: "w2"},
				{Source: "herdr", Name: "cli", WorkspaceID: "w3"},
			}, Options{DisplayOptions: DisplayOptions{HidePreview: !cfg.TUI.ShowPreview, Prompt: cfg.TUI.Prompt}})
			t.Cleanup(func() { m.cancelActivePreview() })
			m.width, m.height = tc.width, 28
			lines := strings.Split(ansi.Strip(m.View().Content), "\n")
			row := -1
			for y, line := range lines {
				if strings.Contains(line, "web") {
					row = y
					break
				}
			}
			require.NotEqual(t, -1, row, "web must be rendered")
			chosen, output := runMouseInput(t, m, fmt.Sprintf("\x1b[<0;21;%dM", row+1))
			require.Contains(t, ansi.Strip(output), "web")
			require.True(t, chosen.chosen)
			assert.Equal(t, "w2", chosen.choice.WorkspaceID)
		})
	}
}

func TestTeaModelIgnoresStaleWorkspaceClick(t *testing.T) {
	for _, state := range []string{"filtered out", "closing", "settings loading", "settings open"} {
		t.Run(state, func(t *testing.T) {
			m := newTeaModel([]model.Session{
				{Source: "herdr", Name: "api", WorkspaceID: "w1"},
				{Source: "herdr", Name: "web", WorkspaceID: "w2"},
			}, Options{DisplayOptions: DisplayOptions{HidePreview: true}})
			m.width, m.height = 120, 28
			click := m.View().OnMouse(tea.MouseClickMsg{X: 20, Y: 6, Button: tea.MouseLeft})
			require.NotNil(t, click)
			switch state {
			case "filtered out":
				m.list.Filter("web")
			case "closing":
				m.closingWorkspaceID = "w1"
			case "settings loading":
				m.settingsBusy = true
			case "settings open":
				editor, err := settings.Open(config.LoadOptions{Path: filepath.Join(t.TempDir(), "config.toml")}, nil)
				require.NoError(t, err)
				m.settings = &editor
			}
			updated, _ := m.Update(click())
			assert.False(t, updated.(teaModel).chosen)
		})
	}
}

func TestTeaModelClickAfterMultilineSessionName(t *testing.T) {
	for _, tc := range []struct {
		name   string
		width  int
		hidden bool
	}{
		{name: "hidden", width: 120, hidden: true},
		{name: "split", width: 120},
		{name: "stacked", width: 80},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			data := fmt.Sprintf("version = 1\n[picker]\nshow_preview = %t\n[[workspace]]\nname = \"api\\nextra\"\npath = \"/tmp/api\"\n[[workspace]]\nname = \"web\"\npath = \"/tmp/web\"\n[[workspace]]\nname = \"cli\"\npath = \"/tmp/cli\"\n", !tc.hidden)
			require.NoError(t, os.WriteFile(path, []byte(data), 0o600))
			cfg, _, err := config.Load(config.LoadOptions{Path: path})
			require.NoError(t, err)
			sessions, err := (sources.ConfigSessions{Config: cfg}).List(t.Context())
			require.NoError(t, err)
			m := newTeaModel(sessions.Ordered(), Options{DisplayOptions: DisplayOptions{HidePreview: !cfg.TUI.ShowPreview}})
			t.Cleanup(func() { m.cancelActivePreview() })
			m.width, m.height = tc.width, 28
			lines := strings.Split(ansi.Strip(m.View().Content), "\n")
			for y, line := range lines {
				if !strings.Contains(line, "web") {
					continue
				}
				chosen, output := runMouseInput(t, m, fmt.Sprintf("\x1b[<0;21;%dM", y+1))
				require.Contains(t, ansi.Strip(output), "web")
				require.True(t, chosen.chosen)
				assert.Equal(t, "web", chosen.choice.Name)
				return
			}
			t.Fatal("web was not rendered")
		})
	}
}

func TestTeaModelMouseWheelNavigates(t *testing.T) {
	for _, tc := range []struct {
		name   string
		width  int
		hidden bool
	}{
		{name: "hidden", width: 120, hidden: true},
		{name: "split", width: 120},
		{name: "stacked", width: 80},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newTeaModel([]model.Session{{Name: "api"}, {Name: "web"}}, Options{DisplayOptions: DisplayOptions{HidePreview: tc.hidden}})
			t.Cleanup(func() { m.cancelActivePreview() })
			m.width, m.height, m.reduceMotion = tc.width, 28, true
			for i, button := range []tea.MouseButton{tea.MouseWheelDown, tea.MouseWheelDown, tea.MouseWheelUp, tea.MouseWheelUp} {
				updated, _ := m.Update(tea.MouseWheelMsg{X: 20, Y: 7, Button: button})
				m = updated.(teaModel)
				assert.Equal(t, []int{0, 1, 0, 0}[i], m.list.Selected)
				assert.Equal(t, i < 3, m.listFocused)
				assert.False(t, m.chosen)
			}
		})
	}
}

func TestTeaModelDividerFollowsMultilinePrompt(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "api"}}, Options{DisplayOptions: DisplayOptions{Prompt: "Choose a workspace\nSesh> "}})
	t.Cleanup(func() { m.cancelActivePreview() })
	m.width, m.height = 120, 28
	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	top, bottom := -1, -1
	for y, line := range lines {
		if ansi.Cut(line, 64, 65) == "│" {
			if top == -1 {
				top = y
			}
			bottom = y
		}
	}
	require.NotEqual(t, -1, top)
	for _, tc := range []struct {
		y    int
		drag bool
	}{{top - 1, false}, {top, true}, {bottom, true}} {
		updated, _ := m.Update(tea.MouseClickMsg{X: 64, Y: tc.y, Button: tea.MouseLeft})
		assert.Equal(t, tc.drag, updated.(teaModel).draggingPreview, "screen row %d", tc.y)
	}
}

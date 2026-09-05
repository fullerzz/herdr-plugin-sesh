package picker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/cursor"
	"charm.land/bubbles/v2/spinner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
)

func TestMain(m *testing.M) {
	if err := os.Setenv("HERDR_SESH_SMEAR_PRESET", "crisp"); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func TestTeaModelHomePrioritizationOption(t *testing.T) {
	for _, tc := range []struct {
		name         string
		opts         Options
		wantDisabled bool
	}{
		{name: "zero options keep prioritization enabled"},
		{name: "explicit disable turns prioritization off", opts: Options{DisableHomePrioritization: true}, wantDisabled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newTeaModel(nil, tc.opts)
			assert.Equal(t, tc.wantDisabled, m.list.DisableHomePrioritization)
		})
	}
}

func TestTeaModelDragsPreviewDivider(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "api"}, {Name: "web"}}, Options{})
	t.Cleanup(func() { m.cancelActivePreview() })
	m.width, m.height = 120, 28
	m.list.Selected = 1
	m.preview = strings.Repeat("preview ", 30)
	previewID := m.previewRequestID
	require.Equal(t, tea.MouseModeCellMotion, m.View().MouseMode)
	for _, step := range []struct {
		name string
		msg  tea.Msg
		x    int
	}{
		{"press", tea.MouseClickMsg{X: 64, Y: 5, Button: tea.MouseLeft}, 64},
		{"widen", tea.MouseMotionMsg{X: 54, Y: 6, Button: tea.MouseLeft}, 54},
		{"minimum list", tea.MouseMotionMsg{X: 0, Y: 0, Button: tea.MouseLeft}, 39},
		{"minimum preview", tea.MouseMotionMsg{X: 200, Y: 40, Button: tea.MouseLeft}, 80},
		{"drag back", tea.MouseMotionMsg{X: 54, Y: 6, Button: tea.MouseLeft}, 54},
		{"release outside", tea.MouseReleaseMsg{X: 54, Y: 40, Button: tea.MouseLeft}, 54},
		{"motion after release", tea.MouseMotionMsg{X: 70, Y: 6, Button: tea.MouseLeft}, 54},
		{"terminal shrinks", tea.WindowSizeMsg{Width: 92, Height: 28}, 39},
		{"terminal expands", tea.WindowSizeMsg{Width: 120, Height: 28}, 54},
	} {
		t.Run(step.name, func(t *testing.T) {
			updated, cmd := m.Update(step.msg)
			m = updated.(teaModel)
			require.Nil(t, cmd)
			require.Equal(t, previewID, m.previewRequestID)
			require.Equal(t, 1, m.list.Selected)
			require.Empty(t, m.input.Value())
			lines := strings.Split(ansi.Strip(m.View().Content), "\n")
			for y := 5; y < 5+previewTitleRows+m.previewBodyLines(); y++ {
				require.Equal(t, "│", ansi.Cut(lines[y], step.x, step.x+1))
				width := lipgloss.Width(lines[y])
				require.Equal(t, m.width, width)
			}
		})
	}
}

func TestTeaModelIgnoresMouseOutsidePreviewDivider(t *testing.T) {
	for _, tc := range []struct {
		name   string
		width  int
		hidden bool
		click  tea.MouseClickMsg
		after  tea.Msg
	}{
		{name: "list", width: 120, click: tea.MouseClickMsg{X: 63, Y: 6, Button: tea.MouseLeft}},
		{name: "preview", width: 120, click: tea.MouseClickMsg{X: 65, Y: 6, Button: tea.MouseLeft}},
		{name: "header", width: 120, click: tea.MouseClickMsg{X: 64, Y: 4, Button: tea.MouseLeft}},
		{name: "footer", width: 120, click: tea.MouseClickMsg{X: 64, Y: 25, Button: tea.MouseLeft}},
		{name: "right button", width: 120, click: tea.MouseClickMsg{X: 64, Y: 6, Button: tea.MouseRight}},
		{name: "hidden", width: 120, hidden: true, click: tea.MouseClickMsg{X: 64, Y: 6, Button: tea.MouseLeft}},
		{name: "stacked", width: 80, click: tea.MouseClickMsg{X: 64, Y: 6, Button: tea.MouseLeft}},
		{name: "resize cancels drag", width: 120, click: tea.MouseClickMsg{X: 64, Y: 6, Button: tea.MouseLeft}, after: tea.WindowSizeMsg{Width: 120, Height: 28}},
		{name: "buttonless motion cancels drag", width: 120, click: tea.MouseClickMsg{X: 64, Y: 6, Button: tea.MouseLeft}, after: tea.MouseMotionMsg{X: 64, Y: 6}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newTeaModel(nil, Options{HidePreview: tc.hidden})
			m.width, m.height = tc.width, 28
			before := m.View()
			if tc.hidden || tc.width < previewSplitWidth {
				require.Equal(t, tea.MouseModeNone, before.MouseMode, "mouse reporting enabled without a divider")
			}
			updated, _ := m.Update(tc.click)
			m = updated.(teaModel)
			if tc.after != nil {
				updated, _ = m.Update(tc.after)
				m = updated.(teaModel)
			}
			updated, _ = m.Update(tea.MouseMotionMsg{X: 54, Y: 6, Button: tea.MouseLeft})
			m = updated.(teaModel)
			assert.Equal(t, before.Content, m.View().Content)
		})
	}
}

func TestTeaModelFiltersMovesAndChooses(t *testing.T) {
	m := newTeaModel([]model.Session{
		{Name: "api-service", Path: "/tmp/api"},
		{Name: "web", Path: "/tmp/web"},
	}, Options{SeparatorAware: true})
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "api service"})
	m = updated.(teaModel)
	cur, ok := m.list.Current()
	require.True(t, ok)
	require.Equal(t, "api-service", cur.Name)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(teaModel)
	require.NotNil(t, cmd)
	require.True(t, m.chosen)
	assert.Equal(t, "api-service", m.choice.Name)
}

func TestTeaModelMovesSelection(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "api"}, {Name: "web"}}, Options{})
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(teaModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(teaModel)
	cur, ok := m.list.Current()
	require.True(t, ok)
	assert.Equal(t, "web", cur.Name)
}

func TestTeaModelCtrlJKMovesSelection(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "api"}, {Name: "web"}}, Options{})

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl})
	m = updated.(teaModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl})
	m = updated.(teaModel)
	current, ok := m.list.Current()
	require.True(t, ok)
	require.Equal(t, "web", current.Name)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})
	m = updated.(teaModel)
	current, ok = m.list.Current()
	require.True(t, ok)
	require.Equal(t, "api", current.Name)
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 28})
	m = updated.(teaModel)
	view := ansi.Strip(m.View().Content)
	require.Contains(t, view, "enter select · ctrl+j/k · ctrl+r workspace · ctrl+x close · esc exit")
	assert.Contains(t, view, "LAST WORKSPACE · None recorded")
}

func TestTeaModelCtrlKDeletesAfterFilterCursor(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "api-web"}}, Options{})
	m.input.SetValue("api-web")
	m.input.SetCursor(3)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})
	m = updated.(teaModel)

	assert.Equal(t, "api", m.input.Value())
}

func TestTeaModelCtrlXClosesSelectedHerdrWorkspace(t *testing.T) {
	var closed string
	m := newTeaModel([]model.Session{
		{Source: "herdr", Name: "api-close", WorkspaceID: "w1"},
		{Source: "config", Name: "api-selected"},
		{Source: "herdr", Name: "api-other", WorkspaceID: "w2"},
	}, Options{CloseWorkspace: func(_ context.Context, id string) error {
		closed = id
		return nil
	}})
	m.list.Filter("api")
	m, _ = m.refreshPreview()

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	m = updated.(teaModel)
	require.NotNil(t, cmd)
	updated, enterCmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(teaModel)
	require.Nil(t, enterCmd)
	require.False(t, m.chosen)
	m.list.Move(1)
	m, _ = m.refreshPreview()
	updated, _ = m.Update(cmd())
	m = updated.(teaModel)

	require.Equal(t, "w1", closed)
	require.Equal(t, []string{"api-selected", "api-other"}, sessionNames(m.list.All))
	current, ok := m.list.Current()
	require.True(t, ok)
	assert.Equal(t, "api-selected", current.Name)
	assert.Equal(t, model.Key(current), m.previewKey)
}

func TestTeaModelCtrlXUpdatesLastWorkspace(t *testing.T) {
	m := newTeaModel([]model.Session{
		{Source: "herdr", Name: "previous", WorkspaceID: "w1"},
		{Source: "herdr", Name: "older", WorkspaceID: "w2"},
	}, Options{
		RecentWorkspaceIDs: []string{"current", "current", "w1", "w2"},
		LastWorkspaceID:    "w1",
		HerdrWorkspaces: []model.Session{
			{Source: "herdr", Name: "previous", WorkspaceID: "w1"},
			{Source: "herdr", Name: "older", WorkspaceID: "w2"},
		},
		CloseWorkspace: func(context.Context, string) error { return nil },
		ReloadPicker: func(context.Context) (ReloadResult, error) {
			return ReloadResult{
				Sessions:        []model.Session{{Source: "herdr", Name: "older", WorkspaceID: "w2"}},
				HerdrWorkspaces: []model.Session{{Source: "herdr", Name: "older", WorkspaceID: "w2"}},
				LastWorkspaceID: "w2",
			}, nil
		},
	})

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 28})
	m = updated.(teaModel)
	updated, closeCmd := m.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	m = updated.(teaModel)
	updated, _ = m.Update(closeCmd())
	m = updated.(teaModel)

	require.Equal(t, "w2", m.lastWorkspaceID)
	assert.Contains(t, ansi.Strip(m.View().Content), "LAST WORKSPACE · older")
}

func TestTeaModelDoesNotQuitWhileWorkspaceCloseIsPending(t *testing.T) {
	tests := []struct {
		name string
		key  tea.KeyPressMsg
		move bool
	}{
		{name: "enter another workspace", key: tea.KeyPressMsg{Code: tea.KeyEnter}, move: true},
		{name: "escape", key: tea.KeyPressMsg{Code: tea.KeyEscape}},
		{name: "control c", key: tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTeaModel([]model.Session{
				{Source: "herdr", Name: "api", WorkspaceID: "w1"},
				{Source: "herdr", Name: "web", WorkspaceID: "w2"},
			}, Options{CloseWorkspace: func(context.Context, string) error { return nil }})
			updated, _ := m.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
			m = updated.(teaModel)
			if tt.move {
				m.list.Move(1)
			}

			updated, cmd := m.Update(tt.key)
			m = updated.(teaModel)

			require.Nil(t, cmd)
			require.False(t, m.chosen)
			assert.Equal(t, "w1", m.closingWorkspaceID)
		})
	}
}

func TestTeaModelCtrlCCancelsWorkspaceCloseAndQuitsAfterResult(t *testing.T) {
	tests := []struct {
		name string
		opts Options
	}{
		{name: "close", opts: Options{CloseWorkspace: func(ctx context.Context, _ string) error {
			<-ctx.Done()
			return ctx.Err()
		}}},
		{name: "reload", opts: Options{
			CloseWorkspace: func(context.Context, string) error { return nil },
			ReloadPicker: func(ctx context.Context) (ReloadResult, error) {
				<-ctx.Done()
				return ReloadResult{}, ctx.Err()
			},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTeaModel([]model.Session{
				{Source: "herdr", Name: "api", WorkspaceID: "w1"},
				{Source: "herdr", Name: "web", WorkspaceID: "w2"},
			}, tt.opts)
			updated, closeCmd := m.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
			m = updated.(teaModel)
			updated, _ = m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
			m = updated.(teaModel)

			updated, quitCmd := m.Update(closeCmd())
			_ = updated.(teaModel)
			require.NotNil(t, quitCmd)
			msg := quitCmd()
			_, ok := msg.(tea.QuitMsg)
			assert.True(t, ok)
		})
	}
}

func TestTeaModelCtrlXRestoresDeduplicatedSessionAfterClose(t *testing.T) {
	m := newTeaModel([]model.Session{
		{Source: "herdr", Name: "api", WorkspaceID: "w1"},
		{Source: "herdr", Name: "web", WorkspaceID: "w2"},
	}, Options{
		CloseWorkspace: func(context.Context, string) error { return nil },
		ReloadPicker: func(context.Context) (ReloadResult, error) {
			return ReloadResult{Sessions: []model.Session{
				{Source: "config", Name: "api", Path: "/configured/api"},
				{Source: "herdr", Name: "web", WorkspaceID: "w2"},
			}}, nil
		},
	})

	updated, closeCmd := m.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	m = updated.(teaModel)
	m.list.Move(1)
	updated, _ = m.Update(closeCmd())
	m = updated.(teaModel)

	require.Equal(t, []string{"api", "web"}, sessionNames(m.list.All))
	current, ok := m.list.Current()
	require.True(t, ok)
	assert.Equal(t, "web", current.Name)
}

func TestTeaModelCtrlXRetainsActiveWorkspacesWhenReloadFails(t *testing.T) {
	m := newTeaModel([]model.Session{
		{Source: "herdr", Name: "api", WorkspaceID: "w1"},
		{Source: "herdr", Name: "web", WorkspaceID: "w2"},
	}, Options{
		CloseWorkspace: func(context.Context, string) error { return nil },
		ReloadPicker: func(context.Context) (ReloadResult, error) {
			return ReloadResult{LastWorkspaceUnknown: true}, errors.New("workspace list failed")
		},
	})

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 28})
	m = updated.(teaModel)
	updated, closeCmd := m.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	m = updated.(teaModel)
	updated, _ = m.Update(closeCmd())
	m = updated.(teaModel)

	require.Equal(t, []string{"web"}, sessionNames(m.list.All))
	require.Contains(t, ansi.Strip(m.View().Content), "Workspace closed, but sessions could not be refreshed: workspace list failed")
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})
	m = updated.(teaModel)
	assert.Contains(t, ansi.Strip(m.View().Content), "LAST WORKSPACE · Unavailable")
}

func TestTeaModelCtrlXClearsClosedParentWhenReloadFails(t *testing.T) {
	m := newTeaModel([]model.Session{
		{Source: "herdr", Name: "parent", WorkspaceID: "w-parent"},
		{
			Source:      "herdr",
			Name:        "child",
			WorkspaceID: "w-child",
			Worktree: model.WorktreeRelation{
				Linked:              true,
				ParentWorkspaceID:   "w-parent",
				ParentWorkspaceName: "parent",
			},
		},
	}, Options{
		CloseWorkspace: func(context.Context, string) error { return nil },
		ReloadPicker: func(context.Context) (ReloadResult, error) {
			return ReloadResult{LastWorkspaceUnknown: true}, errors.New("workspace list failed")
		},
	})

	updated, closeCmd := m.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	m = updated.(teaModel)
	updated, _ = m.Update(closeCmd())
	m = updated.(teaModel)

	child, ok := m.list.Current()
	require.True(t, ok)
	require.True(t, child.Worktree.Linked)
	require.Empty(t, child.Worktree.ParentWorkspaceID)
	require.Empty(t, child.Worktree.ParentWorkspaceName)
	rowText := ansi.Strip(row(child, false, 80, false, ""))
	require.Contains(t, rowText, "[↳ herdr]")
	require.NotContains(t, rowText, "worktree of parent")
	assert.NotContains(t, rowText, "linked worktree")
}

func TestTeaModelCtrlXRefreshesHerdrMetadataWhenSessionReloadFails(t *testing.T) {
	m := newTeaModel([]model.Session{
		{Source: "herdr", Name: "closing", WorkspaceID: "w1"},
		{Source: "herdr", Name: "old label", WorkspaceID: "w2"},
	}, Options{
		LastWorkspaceID: "w2",
		HerdrWorkspaces: []model.Session{{Source: "herdr", Name: "old label", WorkspaceID: "w2"}},
		CloseWorkspace:  func(context.Context, string) error { return nil },
		ReloadPicker: func(context.Context) (ReloadResult, error) {
			return ReloadResult{
				HerdrWorkspaces: []model.Session{{Source: "herdr", Name: "new label", WorkspaceID: "w2"}},
				LastWorkspaceID: "w2",
			}, errors.New("config refresh failed")
		},
	})

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 28})
	m = updated.(teaModel)
	updated, closeCmd := m.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	m = updated.(teaModel)
	updated, _ = m.Update(closeCmd())
	m = updated.(teaModel)

	require.Equal(t, []string{"old label"}, sessionNames(m.list.All))
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})
	m = updated.(teaModel)
	assert.Contains(t, ansi.Strip(m.View().Content), "LAST WORKSPACE · new label")
}

func TestTeaModelCtrlXGroupsReloadedWorktreeFamily(t *testing.T) {
	m := newTeaModel([]model.Session{{Source: "herdr", Name: "closing", WorkspaceID: "w-closing"}}, Options{
		CloseWorkspace: func(context.Context, string) error { return nil },
		ReloadPicker: func(context.Context) (ReloadResult, error) {
			return ReloadResult{Sessions: []model.Session{
				{Source: "herdr", Name: "child", WorkspaceID: "w-child", Worktree: model.WorktreeRelation{Linked: true, ParentWorkspaceID: "w-parent"}},
				{Source: "herdr", Name: "parent", WorkspaceID: "w-parent"},
			}}, nil
		},
	})

	updated, closeCmd := m.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	m = updated.(teaModel)
	updated, _ = m.Update(closeCmd())
	m = updated.(teaModel)

	assert.Equal(t, []string{"parent", "child"}, sessionNames(m.list.All))
}

func TestTeaModelCtrlXKeepsWorkspaceWhenCloseFails(t *testing.T) {
	m := newTeaModel([]model.Session{
		{Source: "herdr", Name: "api", WorkspaceID: "w1"},
		{Source: "herdr", Name: "web", WorkspaceID: "w2"},
	}, Options{
		CloseWorkspace: func(context.Context, string) error { return errors.New("close failed") },
	})

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	m = updated.(teaModel)
	m.list.Move(1)
	m, _ = m.refreshPreview()
	stalePreview := previewMsg{key: m.previewKey, text: "stale preview"}
	updated, _ = m.Update(cmd())
	m = updated.(teaModel)
	require.NotEqual(t, "Closing workspace...", m.preview)
	updated, _ = m.Update(stalePreview)
	m = updated.(teaModel)

	require.Len(t, m.list.All, 2)
	assert.Contains(t, ansi.Strip(m.View().Content), "close failed")
}

func TestTeaModelCtrlXRefreshesPreviewWhenCloseFails(t *testing.T) {
	m := newTeaModel([]model.Session{{Source: "herdr", Name: "api", WorkspaceID: "w1"}}, Options{
		CloseWorkspace: func(context.Context, string) error { return errors.New("close failed") },
	})

	updated, closeCmd := m.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	m = updated.(teaModel)
	updated, previewCmd := m.Update(closeCmd())
	m = updated.(teaModel)

	require.NotNil(t, previewCmd)
	assert.NotEqual(t, "Closing workspace...", m.preview)
}

func TestTeaModelCtrlXIgnoresNonHerdrSession(t *testing.T) {
	called := false
	m := newTeaModel([]model.Session{{Source: "config", Name: "api"}}, Options{
		CloseWorkspace: func(context.Context, string) error {
			called = true
			return nil
		},
	})

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	_ = updated.(teaModel)
	require.Nil(t, cmd)
	assert.False(t, called)
}

func TestTeaModelDownTransfersCursorFromFilterToList(t *testing.T) {
	t.Setenv("HERDR_SESH_REDUCE_MOTION", "")
	m := newTeaModel([]model.Session{{Name: "workspace-api"}, {Name: "workspace-web"}}, Options{})
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'w', Text: "work"})
	m = updated.(teaModel)
	require.NotContains(t, ansi.Strip(m.listView(40, 2)), "┃")

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(teaModel)
	require.False(t, m.input.Focused(), "filter remained focused after moving into the list")
	require.Equal(t, 0, m.list.Selected)
	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	startColumn := visualColumn(lines[3], "┃")
	wantStartColumn := horizontalPadding + lipgloss.Width(defaultPrompt+"work")
	require.Equal(t, wantStartColumn, startColumn)

	updated, _ = m.Update(smearTickMsg{})
	m = updated.(teaModel)
	lines = strings.Split(ansi.Strip(m.View().Content), "\n")
	nextColumn := visualColumn(lines[4], "┃")
	require.GreaterOrEqual(t, nextColumn, horizontalPadding)
	require.Less(t, nextColumn, startColumn)

	for range 10 {
		updated, _ = m.Update(smearTickMsg{})
		m = updated.(teaModel)
	}
	view := ansi.Strip(m.View().Content)
	require.Equal(t, 1, strings.Count(view, "┃"))
	assert.Contains(t, ansi.Strip(m.listView(40, 2)), "┃")
}

func TestTeaModelUpTransfersCursorFromListToFilter(t *testing.T) {
	t.Setenv("HERDR_SESH_REDUCE_MOTION", "")
	m := newTeaModel([]model.Session{{Name: "workspace-api"}, {Name: "workspace-web"}}, Options{})
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'w', Text: "work"})
	m = updated.(teaModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(teaModel)
	for range 10 {
		updated, _ = m.Update(smearTickMsg{})
		m = updated.(teaModel)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m = updated.(teaModel)
	require.False(t, m.input.Focused(), "filter refocused before the reverse smear completed")
	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	startColumn := visualColumn(lines[listFirstRowIndex], "┃")
	require.Equal(t, horizontalPadding, startColumn)

	updated, _ = m.Update(smearTickMsg{})
	m = updated.(teaModel)
	lines = strings.Split(ansi.Strip(m.View().Content), "\n")
	nextColumn := visualColumn(lines[listFirstRowIndex-1], "┃")
	require.Greater(t, nextColumn, startColumn)

	for range 10 {
		updated, _ = m.Update(smearTickMsg{})
		m = updated.(teaModel)
	}
	require.True(t, m.input.Focused())
	assert.NotContains(t, ansi.Strip(m.listView(40, 2)), "┃")
}

func TestTeaModelRightTransfersCursorFromListToFilter(t *testing.T) {
	t.Setenv("HERDR_SESH_REDUCE_MOTION", "")
	m := newTeaModel([]model.Session{{Name: "workspace-api"}, {Name: "workspace-web"}}, Options{})
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'w', Text: "work"})
	m = updated.(teaModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(teaModel)
	for range 10 {
		updated, _ = m.Update(smearTickMsg{})
		m = updated.(teaModel)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	m = updated.(teaModel)
	require.False(t, m.input.Focused())
	require.True(t, m.focusSmearActive)
	require.Equal(t, -1, m.focusSmearDirection)

	updated, _ = m.Update(smearTickMsg{})
	m = updated.(teaModel)
	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	assert.Greater(t, visualColumn(lines[listFirstRowIndex-1], "┃"), horizontalPadding)
}

func TestTeaModelTypingTransfersCursorFromListToFilter(t *testing.T) {
	t.Setenv("HERDR_SESH_REDUCE_MOTION", "")
	m := newTeaModel([]model.Session{{Name: "workspace-api"}, {Name: "workspace-web"}}, Options{})
	m.list.Selected = 1
	m.listFocused = true
	m.input.Blur()

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
	m = updated.(teaModel)
	require.Equal(t, "w", m.input.Value())
	require.Equal(t, "w", m.list.Query)
	require.False(t, m.input.Focused())
	require.True(t, m.focusSmearActive)
	require.Equal(t, -1, m.focusSmearDirection)

	updated, _ = m.Update(smearTickMsg{})
	m = updated.(teaModel)
	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	assert.Greater(t, visualColumn(lines[listFirstRowIndex], "┃"), horizontalPadding)
}

func TestTeaModelPasteTransfersCursorFromListToFilter(t *testing.T) {
	t.Setenv("HERDR_SESH_REDUCE_MOTION", "")
	m := newTeaModel([]model.Session{{Name: "workspace-api"}, {Name: "workspace-web"}}, Options{})
	m.listFocused = true
	m.input.Blur()

	updated, _ := m.Update(tea.PasteMsg{Content: "workspace"})
	m = updated.(teaModel)
	require.Equal(t, "workspace", m.input.Value())
	require.Equal(t, "workspace", m.list.Query)
	require.False(t, m.input.Focused())
	require.True(t, m.focusSmearActive)
	assert.Equal(t, -1, m.focusSmearDirection)
}

func TestTeaModelAcceleratesLongFocusTransfers(t *testing.T) {
	t.Setenv("HERDR_SESH_REDUCE_MOTION", "")
	items := make([]model.Session, 40)
	for i := range items {
		items[i] = model.Session{Name: "workspace"}
	}
	m := newTeaModel(items, Options{})
	m.width = 100
	m.height = 100
	m.list.Selected = 30
	m.listFocused = true
	m.input.Blur()

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	m = updated.(teaModel)
	distance := m.focusSmearSteps
	wantDistance := listFirstRowIndex + m.list.Selected - filterLineIndex
	require.Equal(t, wantDistance, distance)
	previousStep := m.focusSmearStep
	ticks := 0
	largestAdvance := 0
	for m.focusSmearActive {
		updated, _ = m.Update(smearTickMsg{})
		m = updated.(teaModel)
		ticks++
		largestAdvance = max(largestAdvance, previousStep-m.focusSmearStep)
		previousStep = m.focusSmearStep
	}

	require.Less(t, ticks, distance)
	assert.Greater(t, largestAdvance, 1)
}

func TestTeaModelGooeyReverseTransferEasesOut(t *testing.T) {
	t.Setenv("HERDR_SESH_REDUCE_MOTION", "")
	t.Setenv("HERDR_SESH_SMEAR_PRESET", "gooey")
	m := newTeaModel([]model.Session{{Name: "workspace-api"}, {Name: "workspace-web"}}, Options{})
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'w', Text: "work"})
	m = updated.(teaModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(teaModel)
	for range 10 {
		updated, _ = m.Update(smearTickMsg{})
		m = updated.(teaModel)
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m = updated.(teaModel)

	columns := make([]int, 0, 3)
	for frame := range 3 {
		lines := strings.Split(ansi.Strip(m.View().Content), "\n")
		column := visualColumn(lines[listFirstRowIndex-frame], "█")
		require.GreaterOrEqual(t, column, 0)
		columns = append(columns, column)
		if frame < 2 {
			updated, _ = m.Update(smearTickMsg{})
			m = updated.(teaModel)
		}
	}

	firstMove := columns[1] - columns[0]
	secondMove := columns[2] - columns[1]
	assert.Greater(t, firstMove, secondMove)
}

func TestTeaModelSmearPresets(t *testing.T) {
	tests := []struct {
		name          string
		head          string
		transferTrail []string
		rowTrail      string
		rowTrailCells int
	}{
		{name: "crisp", head: "┃", transferTrail: []string{"╱"}, rowTrail: "╷│╵", rowTrailCells: 2},
		{name: "gooey", head: "█", transferTrail: []string{"▓", "▒"}, rowTrail: "▓▒", rowTrailCells: 4},
		{name: "ghost", head: "◆", transferTrail: []string{"·"}, rowTrail: "·", rowTrailCells: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HERDR_SESH_SMEAR_PRESET", tt.name)
			items := make([]model.Session, 6)
			for i := range items {
				items[i] = model.Session{Name: "workspace"}
			}
			m := newTeaModel(items, Options{})
			updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
			m = updated.(teaModel)
			for range 2 {
				updated, _ = m.Update(smearTickMsg{})
				m = updated.(teaModel)
			}
			transfer := ansi.Strip(m.View().Content)
			for _, glyph := range append([]string{tt.head}, tt.transferTrail...) {
				require.Contains(t, transfer, glyph)
			}

			for range 10 {
				updated, _ = m.Update(smearTickMsg{})
				m = updated.(teaModel)
			}
			require.Contains(t, ansi.Strip(m.listView(40, 6)), tt.head)
			for range len(items) - 1 {
				updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
				m = updated.(teaModel)
			}
			rowView := ansi.Strip(m.listView(40, 6))
			trailCells := 0
			for _, glyph := range rowView {
				if strings.ContainsRune(tt.rowTrail, glyph) {
					trailCells++
				}
			}
			assert.Equal(t, tt.rowTrailCells, trailCells)
		})
	}
}

func TestTeaModelSmearsRapidSelectionMoves(t *testing.T) {
	t.Setenv("HERDR_SESH_REDUCE_MOTION", "")
	m := newTeaModel([]model.Session{{Name: "api"}, {Name: "web"}, {Name: "docs"}}, Options{})
	m.listFocused = true
	m.input.Blur()
	for range 2 {
		updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		m = updated.(teaModel)
	}

	lines := strings.Split(strings.TrimSuffix(ansi.Strip(m.listView(40, 3)), "\n"), "\n")
	for i, want := range []string{"╷ ", "│ ", "┃ "} {
		require.True(t, strings.HasPrefix(lines[i], want))
	}
}

func TestTeaModelSmearRetracts(t *testing.T) {
	t.Setenv("HERDR_SESH_REDUCE_MOTION", "")
	oldPreview := renderPreview
	renderPreview = func(context.Context, model.Session, string) (string, error) { return "preview", nil }
	t.Cleanup(func() { renderPreview = oldPreview })

	m := newTeaModel([]model.Session{{Name: "api"}, {Name: "web"}}, Options{})
	m.listFocused = true
	m.input.Blur()
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(teaModel)
	require.True(t, strings.HasPrefix(ansi.Strip(m.listView(40, 2)), "╷ "))

	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	require.True(t, ok)
	tickHandled := false
	for _, child := range batch {
		msg := child()
		if _, ok := msg.(previewMsg); ok {
			continue
		}
		updated, _ = m.Update(msg)
		m = updated.(teaModel)
		tickHandled = true
	}
	require.True(t, tickHandled, "move batch did not contain an animation tick")
	assert.False(t, strings.HasPrefix(ansi.Strip(m.listView(40, 2)), "╷ "))
}

func TestTeaModelCapsSmearSettleTime(t *testing.T) {
	t.Setenv("HERDR_SESH_REDUCE_MOTION", "")
	items := make([]model.Session, 100)
	for i := range items {
		items[i] = model.Session{Name: "workspace"}
	}
	m := newTeaModel(items, Options{})
	m.listFocused = true
	m.input.Blur()
	for range len(items) - 1 {
		updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		m = updated.(teaModel)
	}

	for ticks := 1; m.smearActive; ticks++ {
		require.LessOrEqual(t, ticks, 3)
		updated, _ := m.Update(smearTickMsg{})
		m = updated.(teaModel)
	}
}

func TestTeaModelQueryChangeClearsSmear(t *testing.T) {
	t.Setenv("HERDR_SESH_REDUCE_MOTION", "")
	m := newTeaModel([]model.Session{
		{Name: "workspace-0"},
		{Name: "workspace-1"},
		{Name: "workspace-2"},
		{Name: "workspace-3"},
	}, Options{})
	m.list.Selected = 2
	m.listFocused = true
	m.input.Blur()
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(teaModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'w', Text: "workspace"})
	m = updated.(teaModel)

	view := ansi.Strip(m.listView(40, 4))
	require.NotContains(t, view, "╵ ")
	assert.NotContains(t, view, "│ ")
}

func TestTeaModelReducedMotionSkipsSmear(t *testing.T) {
	t.Setenv("HERDR_SESH_REDUCE_MOTION", "1")
	m := newTeaModel([]model.Session{{Name: "api"}, {Name: "web"}}, Options{})
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(teaModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(teaModel)

	current, ok := m.list.Current()
	require.True(t, ok)
	require.Equal(t, "web", current.Name)
	view := ansi.Strip(m.listView(40, 2))
	require.NotContains(t, view, "╷ ")
	assert.NotContains(t, view, "│ ")
}

func TestTeaModelForwardsTextInputNonKeyMessages(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "api"}}, Options{})
	updated, cmd := m.Update(cursor.Blink())
	m = updated.(teaModel)
	require.NotNil(t, cmd)
	assert.Equal(t, m.input.Value(), m.list.Query)
}

func TestTeaModelViewRendersStyledShell(t *testing.T) {
	oldPreview := renderPreview
	renderPreview = func(context.Context, model.Session, string) (string, error) {
		return "preview content", nil
	}
	t.Cleanup(func() { renderPreview = oldPreview })

	m := newTeaModel([]model.Session{
		{Source: "herdr", Name: "workspace-api", Path: "/tmp/workspace-api", WorkspaceID: "ws-api", AgentStatus: "working"},
		{Source: "zoxide", Name: "tools", Path: "/tmp/tools"},
		{Source: "config", Name: "api", Path: "/tmp/api"},
	}, Options{
		Prompt:          "Find> ",
		Placeholder:     "Search sessions",
		ShowIcons:       true,
		LastWorkspaceID: "ws-api",
		HerdrWorkspaces: []model.Session{{Source: "herdr", Name: "workspace-api", Path: "/tmp/workspace-api", WorkspaceID: "ws-api"}},
	})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
	m = updated.(teaModel)
	updated, _ = m.Update(previewCommand(m.previewContext, m.previewKey, m.previewRequestID, m.list.Filtered[m.list.Selected], m.defaultPreviewCommand, false)())
	m = updated.(teaModel)
	view := ansi.Strip(m.View().Content)
	for _, want := range []string{"herdr / sesh", "3 workspaces", "Find> ", "Search sessions", "LAST WORKSPACE · workspace-api  /tmp/workspace-api", "WORKSPACES", "PREVIEW [ctrl+o] · workspace-api", herdrSourceIcon + " herdr", zoxideSourceIcon + " zoxide", configSourceIcon + " config", "api", "preview content", "enter select"} {
		require.Contains(t, view, want)
	}
	require.NotContains(t, view, "+-")
	require.NotContains(t, view, "| ")
	assert.Equal(t, 160, maxLineWidth(view))
}

func TestTeaModelViewGroupsAndDescribesWorktreeFamily(t *testing.T) {
	m := newTeaModel([]model.Session{
		{
			Source:      "herdr",
			Name:        "feature",
			Path:        "/tmp/project-feature",
			WorkspaceID: "w-child-a",
			Worktree: model.WorktreeRelation{
				Linked:              true,
				ParentWorkspaceID:   "w-parent",
				ParentWorkspaceName: "project",
			},
		},
		{Source: "herdr", Name: "project", Path: "/tmp/project", WorkspaceID: "w-parent"},
		{
			Source:      "herdr",
			Name:        "docs",
			Path:        "/tmp/project-docs",
			WorkspaceID: "w-child-b",
			Worktree: model.WorktreeRelation{
				Linked:              true,
				ParentWorkspaceID:   "w-parent",
				ParentWorkspaceName: "project",
			},
		},
	}, Options{})
	m.list.Selected = 1
	m.preview = "preview content"
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 28})
	m = updated.(teaModel)

	view := ansi.Strip(m.View().Content)
	parentLine, firstChildLine, lastChildLine := -1, -1, -1
	for i, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "[herdr]") && strings.Contains(line, "project") && !strings.Contains(line, "↳") {
			parentLine = i
		}
		if strings.Contains(line, "[↳ herdr]") && strings.Contains(line, "├─ feature") {
			firstChildLine = i
		}
		if strings.Contains(line, "[↳ herdr]") && strings.Contains(line, "└─ docs") {
			lastChildLine = i
		}
	}
	require.GreaterOrEqual(t, parentLine, 0)
	require.Equal(t, parentLine+1, firstChildLine)
	require.Equal(t, firstChildLine+1, lastChildLine)
	require.Contains(t, view, "PREVIEW [ctrl+o] · feature · worktree of project")
	require.Equal(t, 160, maxLineWidth(view))
	assert.Equal(t, 28, lipgloss.Height(view))
}

func TestWorktreeTreePrefixRequiresContiguousVisibleFamily(t *testing.T) {
	items := []model.Session{
		{Source: "herdr", Name: "parent", WorkspaceID: "w-parent"},
		{Source: "zoxide", Name: "unrelated"},
		{Source: "herdr", Name: "child", WorkspaceID: "w-child", Worktree: model.WorktreeRelation{Linked: true, ParentWorkspaceID: "w-parent"}},
	}

	assert.Empty(t, worktreeTreePrefix(items, 2))
}

func TestListViewKeepsWorktreeBranchWhenParentIsOffScreen(t *testing.T) {
	m := newTeaModel([]model.Session{
		{Source: "herdr", Name: "parent", WorkspaceID: "w-parent"},
		{Source: "herdr", Name: "child", WorkspaceID: "w-child", Worktree: model.WorktreeRelation{Linked: true, ParentWorkspaceID: "w-parent"}},
	}, Options{})
	m.list.Selected = 1

	view := ansi.Strip(m.listView(80, 1))
	assert.Contains(t, view, "└─ child")
}

func TestTeaModelLastWorkspaceFallsBackToRecordedID(t *testing.T) {
	m := newTeaModel([]model.Session{{Source: "herdr", Name: "current", WorkspaceID: "current"}}, Options{LastWorkspaceID: "unavailable-workspace"})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 28})
	m = updated.(teaModel)

	view := ansi.Strip(m.View().Content)
	assert.Contains(t, view, "LAST WORKSPACE · unavailable-workspace")
}

func TestTeaModelLastWorkspaceUsesRawHerdrMetadata(t *testing.T) {
	m := newTeaModel([]model.Session{{Source: "config", Name: "api", Path: "/configured/api"}}, Options{
		LastWorkspaceID: "ws-api",
		HerdrWorkspaces: []model.Session{{Source: "herdr", Name: "api", Path: "/live/api", WorkspaceID: "ws-api"}},
	})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 28})
	m = updated.(teaModel)

	view := ansi.Strip(m.View().Content)
	assert.Contains(t, view, "LAST WORKSPACE · api  /live/api")
}

func TestTeaModelCanHideLastWorkspacePath(t *testing.T) {
	m := newTeaModel(nil, Options{
		HideLastWorkspacePath: true,
		LastWorkspaceID:       "ws-api",
		HerdrWorkspaces:       []model.Session{{Source: "herdr", Name: "api", Path: "/live/api", WorkspaceID: "ws-api"}},
	})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 24})
	m = updated.(teaModel)

	view := ansi.Strip(m.View().Content)
	require.Contains(t, view, "LAST WORKSPACE · api")
	assert.NotContains(t, view, "/live/api")
}

func TestTeaModelCanHideLastWorkspace(t *testing.T) {
	m := newTeaModel(nil, Options{
		HideLastWorkspace: true,
		LastWorkspaceID:   "ws-api",
		HerdrWorkspaces:   []model.Session{{Source: "herdr", Name: "api", Path: "/live/api", WorkspaceID: "ws-api"}},
	})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 24})
	m = updated.(teaModel)

	view := ansi.Strip(m.View().Content)
	require.NotContains(t, view, "LAST WORKSPACE")
	require.NotContains(t, view, "last: api")
	assert.NotContains(t, view, "/live/api")
}

func TestTeaModelLastWorkspaceSharesFooterWithKeybinds(t *testing.T) {
	m := newTeaModel(nil, Options{LastWorkspaceID: "ws-api"})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	m = updated.(teaModel)

	view := ansi.Strip(m.View().Content)
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "LAST WORKSPACE") {
			require.Contains(t, line, "enter select")
			require.True(t, strings.HasSuffix(strings.TrimSpace(line), "LAST WORKSPACE · ws-api"))
			return
		}
	}
	require.FailNow(t, fmt.Sprintf("view missing last workspace footer:\n%s", view))
}

func TestFooterLinePrioritizesKeybindHelpAtNarrowWidths(t *testing.T) {
	m := newTeaModel(nil, Options{
		LastWorkspaceID: "ws-api",
		HerdrWorkspaces: []model.Session{{Source: "herdr", Name: "a-rather-long-workspace-name", Path: "/home/test/some/deep/project/path", WorkspaceID: "ws-api"}},
	})
	help := helpStyle.Render("enter select · ctrl+j/k · ctrl+r workspace · ctrl+x close · esc exit")
	for _, width := range []int{10, 20, 40, 80, 120} {
		line := m.footerLine(help, width)
		require.Equal(t, width, lipgloss.Width(line))
		if width >= lipgloss.Width(help) {
			assert.Contains(t, ansi.Strip(line), "esc exit")
		}
	}
}

func TestFooterLineCompactsLastWorkspaceBeforeDroppingIt(t *testing.T) {
	m := newTeaModel(nil, Options{
		LastWorkspaceID: "api",
		HerdrWorkspaces: []model.Session{{Source: "herdr", Name: "api", Path: "/some/long/path/to/the/project", WorkspaceID: "api"}},
	})
	help := helpStyle.Render("enter select · ctrl+j/k · ctrl+r workspace · ctrl+x close · esc exit")
	line := ansi.Strip(m.footerLine(help, 80))
	require.Contains(t, line, "last: api")
	require.NotContains(t, line, "LAST WORKSPACE")
	assert.Contains(t, line, "esc exit")
}

func TestFooterLineGivesCloseErrorWholeRow(t *testing.T) {
	m := newTeaModel(nil, Options{LastWorkspaceID: "ws-api"})
	m.closeError = "Failed to close workspace: herdr workspace close w1: boom"
	line := ansi.Strip(m.footerLine(emptyStyle.Render(m.closeError), 80))
	require.Contains(t, line, "close w1: boom")
	require.NotContains(t, line, "LAST WORKSPACE")
	assert.NotContains(t, line, "last:")
}

func TestTeaModelLastWorkspaceUnavailable(t *testing.T) {
	m := newTeaModel(nil, Options{LastWorkspaceUnknown: true})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 28})
	m = updated.(teaModel)

	assert.Contains(t, ansi.Strip(m.View().Content), "LAST WORKSPACE · Unavailable")
}

func TestTeaModelUnnamedLastWorkspaceDoesNotRepeatCompactedPath(t *testing.T) {
	t.Setenv("HOME", "/home/test")
	m := newTeaModel(nil, Options{
		LastWorkspaceID: "ws-api",
		HerdrWorkspaces: []model.Session{{Source: "herdr", Path: "/home/test/api", WorkspaceID: "ws-api"}},
	})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 28})
	m = updated.(teaModel)

	view := ansi.Strip(m.View().Content)
	assert.Equal(t, 1, strings.Count(view, "~/api"))
}

func TestTeaModelShowIconsControlsSourceIcons(t *testing.T) {
	items := []model.Session{{Source: "herdr", Name: "api"}}
	withoutIcons := ansi.Strip(newTeaModel(items, Options{}).View().Content)
	require.NotContains(t, withoutIcons, herdrSourceIcon)

	withIcons := ansi.Strip(newTeaModel(items, Options{ShowIcons: true}).View().Content)
	assert.Contains(t, withIcons, herdrSourceIcon+" herdr")
}

func TestRowUsesSourceCategoryColors(t *testing.T) {
	tests := []struct {
		source string
		color  string
	}{
		{source: "herdr", color: "38;2;125;207;255"},
		{source: "config", color: "38;2;224;175;104"},
		{source: "zoxide", color: "38;2;158;206;106"},
		{source: "dir", color: "38;2;187;154;247"},
	}
	for _, tt := range tests {
		got := row(model.Session{Source: tt.source, Name: tt.source}, false, 80, true, "")
		require.Contains(t, got, tt.color)
	}
}

func TestChildWorktreeUsesPurpleTypeBadge(t *testing.T) {
	s := model.Session{Source: "herdr", Name: "feature", Worktree: model.WorktreeRelation{Linked: true}}

	withoutIcons := row(s, false, 80, false, "")
	plain := ansi.Strip(withoutIcons)
	require.Contains(t, plain, "[↳ herdr]")
	require.NotContains(t, plain, "↳ feature")
	require.Contains(t, withoutIcons, "38;2;187;154;247")
	require.NotContains(t, withoutIcons, "38;2;125;207;255")

	withIcons := ansi.Strip(row(s, false, 80, true, ""))
	require.Contains(t, withIcons, "↳ herdr")
	require.NotContains(t, withIcons, herdrSourceIcon)
	assert.NotContains(t, withIcons, "↳ feature")
}

func TestChildWorktreeIconReplacementCanBeDisabled(t *testing.T) {
	items := []model.Session{
		{Source: "herdr", Name: "project", WorkspaceID: "w-parent"},
		{Source: "herdr", Name: "feature", WorkspaceID: "w-child", Worktree: model.WorktreeRelation{Linked: true, ParentWorkspaceID: "w-parent"}},
	}
	m := newTeaModel(items, Options{ShowIcons: true, DisableWorktreeIconReplacement: true})
	withIcons := m.listView(80, 2)
	plain := ansi.Strip(withIcons)
	require.Contains(t, plain, herdrSourceIcon+" herdr")
	require.Contains(t, plain, "└─ feature")
	require.NotContains(t, plain, "↳")
	require.Contains(t, withIcons, "38;2;187;154;247")

	m = newTeaModel(items, Options{DisableWorktreeIconReplacement: true})
	withoutIcons := ansi.Strip(m.listView(80, 2))
	require.Contains(t, withoutIcons, "[herdr]")
	assert.NotContains(t, withoutIcons, "↳")
}

func TestRowUsesAgentStatusIndicators(t *testing.T) {
	tests := []struct {
		status string
		glyph  string
		color  string
	}{
		{status: "working", glyph: "⢄", color: "38;2;224;175;104"},
		{status: "blocked", glyph: "◉", color: "38;2;247;118;142"},
		{status: "idle", glyph: "✓", color: "38;2;158;206;106"},
		{status: "done", glyph: "●", color: "38;2;125;207;255"},
	}
	for _, tt := range tests {
		got := row(model.Session{Source: "herdr", Name: "api", AgentStatus: tt.status}, false, 80, true, "")
		require.Contains(t, ansi.Strip(got), tt.glyph)
		require.Contains(t, got, tt.color)
	}
	for _, status := range []string{"", "unknown", "future"} {
		got := ansi.Strip(row(model.Session{Source: "herdr", Name: "api", AgentStatus: status}, false, 80, true, ""))
		require.False(t, strings.ContainsAny(got, "⢄◉✓●"))
	}
}

func TestTeaModelAnimatesWorkingAgentStatusIndicator(t *testing.T) {
	m := newTeaModel([]model.Session{{Source: "herdr", Name: "api", AgentStatus: "working"}}, Options{})
	before := ansi.Strip(m.listView(80, 1))

	updated, cmd := m.Update(spinner.TickMsg{})
	m = updated.(teaModel)
	after := ansi.Strip(m.listView(80, 1))

	require.Contains(t, before, "⢄")
	require.Contains(t, after, "⢂")
	require.NotNil(t, cmd)
}

func TestTeaModelStartsAgentStatusSpinner(t *testing.T) {
	m := newTeaModel(nil, Options{RefreshAgentStatuses: func() (map[string]string, error) { return nil, nil }})
	batch, ok := m.Init()().(tea.BatchMsg)
	require.True(t, ok)
	require.NotEmpty(t, batch)
	assert.Equal(t, reflect.TypeOf(spinner.TickMsg{}), reflect.TypeOf(batch[len(batch)-1]()))
}

func TestRowCompactsHomeAndNeverWraps(t *testing.T) {
	t.Setenv("HOME", "/Users/picker")
	s := model.Session{
		Source:      "herdr",
		Name:        "workspace-with-a-name-that-is-longer-than-the-column",
		Path:        "/Users/picker/Code/Go/workspace-with-a-path-that-is-longer-than-the-row",
		AgentStatus: "working",
	}
	wide := ansi.Strip(strings.TrimSuffix(row(s, true, 76, true, ""), "\n"))
	require.NotContains(t, wide, "\n")
	require.Equal(t, 76, lipgloss.Width(wide))
	require.Contains(t, wide, "~/Code/Go/")
	require.NotContains(t, wide, "/Users/picker")
	narrow := ansi.Strip(strings.TrimSuffix(row(s, false, 48, true, ""), "\n"))
	require.NotContains(t, narrow, "~/")
	require.NotContains(t, narrow, "/Users/picker")
	require.NotContains(t, narrow, "\n")
	assert.Equal(t, 48, lipgloss.Width(narrow))
}

func TestRowShowsWorktreePathWithoutParentDescription(t *testing.T) {
	s := model.Session{
		Source:      "herdr",
		Name:        "feature",
		Path:        "/tmp/project-feature",
		AgentStatus: "blocked",
		Worktree: model.WorktreeRelation{
			Linked:              true,
			ParentWorkspaceID:   "w-parent",
			ParentWorkspaceName: "project",
		},
	}

	wide := row(s, true, 100, true, "")
	widePlain := ansi.Strip(strings.TrimSuffix(wide, "\n"))
	for _, want := range []string{"┃", "◉", "↳ herdr", "feature", "/tmp/project-feature"} {
		require.Contains(t, widePlain, want)
	}
	require.NotContains(t, widePlain, "worktree of")
	require.NotContains(t, widePlain, herdrSourceIcon)
	require.Equal(t, 100, lipgloss.Width(widePlain))
	require.NotContains(t, widePlain, "\n")

	narrow := row(s, false, 48, false, "")
	narrowPlain := ansi.Strip(strings.TrimSuffix(narrow, "\n"))
	require.Contains(t, narrowPlain, "[↳ herdr]")
	require.Contains(t, narrowPlain, "feature")
	require.NotContains(t, narrowPlain, "↳ feature")
	require.NotContains(t, narrowPlain, "worktree of")
	require.NotContains(t, narrowPlain, "/tmp/project-feature")
	require.Equal(t, 48, lipgloss.Width(narrowPlain))
	assert.NotContains(t, narrowPlain, "\n")
}

func TestRowPreservesWorktreeMarkerAtCompactBoundary(t *testing.T) {
	s := model.Session{
		Source:      "herdr",
		Name:        "feature",
		AgentStatus: "working",
		Worktree:    model.WorktreeRelation{Linked: true, ParentWorkspaceName: "project"},
	}
	for _, showIcons := range []bool{false, true} {
		for _, width := range []int{1, 4, 5, 6, 10, 14, 15, 16} {
			got := ansi.Strip(strings.TrimSuffix(row(s, true, width, showIcons, ""), "\n"))
			require.Contains(t, got, "↳")
			require.Equal(t, width, lipgloss.Width(got))
			require.NotContains(t, got, "\n")
		}
	}
}

func TestRowUsesBadgeWithoutDescriptionWhenWorktreeParentIsUnresolved(t *testing.T) {
	got := ansi.Strip(row(model.Session{
		Source:   "herdr",
		Name:     "feature",
		Worktree: model.WorktreeRelation{Linked: true},
	}, false, 80, false, ""))
	require.Contains(t, got, "[↳ herdr]")
	require.Contains(t, got, "feature")
	require.NotContains(t, got, "linked worktree")
	require.NotContains(t, got, "worktree of")
	assert.NotContains(t, got, "↳ feature")
}

func TestRowDoesNotMarkNormalWorkspace(t *testing.T) {
	got := ansi.Strip(row(model.Session{Source: "herdr", Name: "project", Path: "/tmp/project"}, false, 80, false, ""))
	require.NotContains(t, got, "↳")
	assert.NotContains(t, got, "worktree")
}

func TestPreviewTitleShowsUnresolvedLinkedWorktree(t *testing.T) {
	m := newTeaModel([]model.Session{{
		Source:   "herdr",
		Name:     "feature",
		Worktree: model.WorktreeRelation{Linked: true},
	}}, Options{})

	got := ansi.Strip(m.previewTitle())
	require.Contains(t, got, "PREVIEW [ctrl+o] · feature · linked worktree")
	assert.NotContains(t, got, "worktree of")
}

func TestPreviewTitleShowsWorktreeParentBeforeAgentStatus(t *testing.T) {
	m := newTeaModel([]model.Session{
		{Source: "herdr", Name: "parent", WorkspaceID: "w-parent"},
		{
			Source:      "herdr",
			Name:        "feature",
			WorkspaceID: "w-child",
			AgentStatus: "working",
			Worktree: model.WorktreeRelation{
				Linked:              true,
				ParentWorkspaceID:   "w-parent",
				ParentWorkspaceName: "parent",
			},
		},
	}, Options{})
	m.list.Selected = 1

	got := ansi.Strip(m.previewTitle())
	assert.Contains(t, got, "PREVIEW [ctrl+o] · feature · worktree of parent · working")
}

func TestTeaModelPreviewUsesConfiguredCommand(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "api", Path: "/tmp/api"}}, Options{DefaultPreviewCommand: "printf preview:%s {}"})
	msg := previewCommand(m.previewContext, m.previewKey, m.previewRequestID, m.list.Filtered[m.list.Selected], m.defaultPreviewCommand, false)()
	preview := msg.(previewMsg)
	assert.Equal(t, "preview:/tmp/api", strings.TrimSpace(preview.text))
}

func TestTeaModelHidePreviewDoesNotRunInitialPreview(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "preview-ran")
	m := newTeaModel([]model.Session{{Name: "api", Path: "/tmp/api"}}, Options{
		HidePreview:           true,
		DefaultPreviewCommand: fmt.Sprintf("touch %q", marker),
	})

	executeTeaCommand(m.Init())
	_, err := os.Stat(marker)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestTeaModelHidePreviewDoesNotRefreshAfterSelection(t *testing.T) {
	t.Setenv("HERDR_SESH_REDUCE_MOTION", "1")
	marker := filepath.Join(t.TempDir(), "preview-ran")
	m := newTeaModel([]model.Session{{Name: "api", Path: "/tmp/api"}, {Name: "web", Path: "/tmp/web"}}, Options{
		HidePreview:           true,
		DefaultPreviewCommand: fmt.Sprintf("touch %q", marker),
	})
	m.listFocused = true

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(teaModel)
	executeTeaCommand(cmd)
	_, err := os.Stat(marker)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func executeTeaCommand(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return
	}
	for _, child := range batch {
		if child != nil {
			_ = child()
		}
	}
}

func TestTeaModelRefreshesPreviewWhenSelectionChanges(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "api", Path: "/tmp/api"}, {Name: "web", Path: "/tmp/web"}}, Options{})
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(teaModel)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(teaModel)
	require.NotNil(t, cmd)
	require.Contains(t, m.preview, "Loading preview")
	current, ok := m.list.Current()
	require.True(t, ok)
	require.Equal(t, "web", current.Name)
	assert.Equal(t, model.Key(current), m.previewKey)
}

func TestTeaModelCancelsSupersededPreview(t *testing.T) {
	firstStarted := make(chan struct{})
	firstCanceled := make(chan struct{})
	secondStarted := make(chan struct{})
	releaseSecond := make(chan struct{})
	previewErr := make(chan error, 1)
	oldPreview := renderPreview
	renderPreview = func(ctx context.Context, s model.Session, _ string) (string, error) {
		switch s.Name {
		case "api":
			close(firstStarted)
			<-ctx.Done()
			close(firstCanceled)
			return "", ctx.Err()
		case "web":
			close(secondStarted)
			<-releaseSecond
			return "web preview", nil
		default:
			err := fmt.Errorf("unexpected preview for %q", s.Name)
			previewErr <- err
			return "", err
		}
	}
	t.Cleanup(func() { renderPreview = oldPreview })

	m := newTeaModel([]model.Session{{Name: "api"}, {Name: "web"}}, Options{Context: context.Background()})
	m.previewKey = ""
	m, firstCmd := m.refreshPreview()
	firstResult := make(chan tea.Msg, 1)
	go func() { firstResult <- firstCmd() }()
	select {
	case <-firstStarted:
	case err := <-previewErr:
		require.FailNow(t, fmt.Sprint(err))
	case <-time.After(time.Second):
		require.FailNow(t, "first preview did not start")
	}

	m.list.Move(1)
	m, secondCmd := m.refreshPreview()
	secondResult := make(chan tea.Msg, 1)
	go func() { secondResult <- secondCmd() }()
	select {
	case <-secondStarted:
	case err := <-previewErr:
		require.FailNow(t, fmt.Sprint(err))
	case <-time.After(time.Second):
		require.FailNow(t, "replacement preview did not start")
	}
	select {
	case <-firstCanceled:
	case <-time.After(time.Second):
		require.FailNow(t, "superseded preview did not observe cancellation")
	}

	close(releaseSecond)
	updated, _ := m.Update(<-secondResult)
	m = updated.(teaModel)
	require.Equal(t, "web preview", m.preview)
	<-firstResult
}

func TestTeaModelRejectsObsoleteSameKeyPreview(t *testing.T) {
	oldPreview := renderPreview
	renderPreview = func(_ context.Context, s model.Session, _ string) (string, error) {
		return s.Name + " preview", nil
	}
	t.Cleanup(func() { renderPreview = oldPreview })

	m := newTeaModel([]model.Session{{Name: "api"}, {Name: "web"}}, Options{})
	m.previewKey = ""
	m, firstCmd := m.refreshPreview()
	m.list.Move(1)
	m, _ = m.refreshPreview()
	m.list.Move(-1)
	m, _ = m.refreshPreview()

	updated, _ := m.Update(firstCmd())
	m = updated.(teaModel)
	assert.Equal(t, "Loading preview...", m.preview)
}

func TestTeaModelCancelsPreviewOnQuitOrNoPreview(t *testing.T) {
	for _, tt := range []struct {
		name    string
		advance func(teaModel) (teaModel, tea.Cmd)
	}{
		{
			name: "quit",
			advance: func(m teaModel) (teaModel, tea.Cmd) {
				updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
				return updated.(teaModel), cmd
			},
		},
		{
			name: "no preview",
			advance: func(m teaModel) (teaModel, tea.Cmd) {
				m = m.filter("missing")
				return m.refreshPreview()
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			started := make(chan struct{})
			canceled := make(chan struct{})
			oldPreview := renderPreview
			renderPreview = func(ctx context.Context, _ model.Session, _ string) (string, error) {
				close(started)
				<-ctx.Done()
				close(canceled)
				return "", ctx.Err()
			}
			t.Cleanup(func() { renderPreview = oldPreview })

			m := newTeaModel([]model.Session{{Name: "api"}}, Options{Context: context.Background()})
			m.previewKey = ""
			m, previewCmd := m.refreshPreview()
			go previewCmd()
			select {
			case <-started:
			case <-time.After(time.Second):
				require.FailNow(t, "preview did not start")
			}

			_, _ = tt.advance(m)
			select {
			case <-canceled:
			case <-time.After(time.Second):
				require.FailNow(t, "preview did not observe cancellation")
			}
		})
	}
}

func TestTeaModelCancelsRestartedPreviewWhenQuittingPendingClose(t *testing.T) {
	t.Setenv("HERDR_SESH_REDUCE_MOTION", "1")
	closeStarted := make(chan struct{})
	previewStarted := make(chan struct{})
	previewCanceled := make(chan struct{})
	oldPreview := renderPreview
	renderPreview = func(ctx context.Context, _ model.Session, _ string) (string, error) {
		close(previewStarted)
		<-ctx.Done()
		close(previewCanceled)
		return "", ctx.Err()
	}
	t.Cleanup(func() { renderPreview = oldPreview })

	m := newTeaModel([]model.Session{
		{Source: "herdr", Name: "api", WorkspaceID: "w1"},
		{Source: "herdr", Name: "web", WorkspaceID: "w2"},
	}, Options{
		Context: context.Background(),
		CloseWorkspace: func(ctx context.Context, _ string) error {
			close(closeStarted)
			<-ctx.Done()
			return ctx.Err()
		},
	})
	m.listFocused = true
	updated, closeCmd := m.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	m = updated.(teaModel)
	closeResult := make(chan tea.Msg, 1)
	go func() { closeResult <- closeCmd() }()
	select {
	case <-closeStarted:
	case <-time.After(time.Second):
		require.FailNow(t, "workspace close did not start")
	}

	updated, moveCmd := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(teaModel)
	go executeTeaCommand(moveCmd)
	select {
	case <-previewStarted:
	case <-time.After(time.Second):
		require.FailNow(t, "navigation did not restart preview during close")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(teaModel)
	updated, quitCmd := m.Update(<-closeResult)
	m = updated.(teaModel)
	require.NotNil(t, quitCmd)
	if quitMsg := quitCmd(); quitMsg == nil {
		require.FailNow(t, "quit command returned nil")
	} else {
		_, ok := quitMsg.(tea.QuitMsg)
		require.True(t, ok)
	}
	select {
	case <-previewCanceled:
	case <-time.After(time.Second):
		require.FailNow(t, "restarted preview did not observe cancellation before quit")
	}
}

func TestTeaModelRefreshesAgentStatuses(t *testing.T) {
	m := newTeaModel([]model.Session{
		{Source: "herdr", Name: "api", WorkspaceID: "w1", AgentStatus: "working"},
		{Source: "config", Name: "local", Path: "/tmp/local"},
	}, Options{RefreshAgentStatuses: func() (map[string]string, error) {
		return map[string]string{"w1": "blocked"}, nil
	}})
	m.list.Filter("api")

	updated, cmd := m.Update(statusRefreshTickMsg{})
	m = updated.(teaModel)
	require.NotNil(t, cmd)

	updated, next := m.Update(cmd())
	m = updated.(teaModel)
	current, ok := m.list.Current()
	require.True(t, ok)
	require.Equal(t, "blocked", current.AgentStatus)
	require.Equal(t, "api", m.list.Query)
	require.Len(t, m.list.Filtered, 1)
	require.NotNil(t, next)
}

func TestTeaModelAgentSortRefreshPreservesSelectionAndPreview(t *testing.T) {
	m := newTeaModel([]model.Session{
		{Source: "herdr", Name: "api-one", WorkspaceID: "w1", AgentStatus: "working"},
		{Source: "herdr", Name: "api-two", WorkspaceID: "w2", AgentStatus: "idle"},
		{Source: "herdr", Name: "api-three", WorkspaceID: "w3", AgentStatus: "blocked"},
	}, Options{WorkspaceSort: "agent"})
	m.list.Filter("api")
	m.list.Selected = 1
	selected, ok := m.list.Current()
	require.True(t, ok)
	require.Equal(t, "w1", selected.WorkspaceID)
	selectedKey := model.Key(selected)
	m.previewKey = selectedKey
	previewRequestID := m.previewRequestID
	m.smearActive = true
	m.focusSmearActive = true

	updated, next := m.Update(agentStatusesMsg{statuses: map[string]string{
		"w1": "idle",
		"w2": "blocked",
		"w3": "done",
	}})
	m = updated.(teaModel)

	require.Equal(t, []string{"api-two", "api-three", "api-one"}, sessionNames(m.list.All))
	require.Equal(t, []string{"api-two", "api-three", "api-one"}, sessionNames(m.list.Filtered))
	current, ok := m.list.Current()
	require.True(t, ok)
	require.Equal(t, selectedKey, model.Key(current))
	require.Equal(t, "api", m.list.Query)
	require.Equal(t, selectedKey, m.previewKey)
	require.Equal(t, previewRequestID, m.previewRequestID)
	require.False(t, m.smearActive)
	require.False(t, m.focusSmearActive)
	require.NotNil(t, next)
}

func TestTeaModelAgentSortRefreshDemotesMissingStatus(t *testing.T) {
	m := newTeaModel([]model.Session{
		{Source: "herdr", Name: "blocked", WorkspaceID: "w1", AgentStatus: "blocked"},
		{Source: "herdr", Name: "idle", WorkspaceID: "w2", AgentStatus: "idle"},
	}, Options{WorkspaceSort: "agent"})

	updated, _ := m.Update(agentStatusesMsg{statuses: map[string]string{"w2": "idle"}})
	m = updated.(teaModel)

	require.Equal(t, []string{"idle", "blocked"}, sessionNames(m.list.All))
	assert.Empty(t, m.list.All[1].AgentStatus)
}

func TestTeaModelAgentSortRefreshErrorKeepsState(t *testing.T) {
	m := newTeaModel([]model.Session{
		{Source: "herdr", Name: "blocked", WorkspaceID: "w1", AgentStatus: "blocked"},
		{Source: "herdr", Name: "idle", WorkspaceID: "w2", AgentStatus: "idle"},
	}, Options{WorkspaceSort: "agent"})
	m.list.Selected = 1
	selected, _ := m.list.Current()
	selectedKey := model.Key(selected)
	m.previewKey = selectedKey
	previewRequestID := m.previewRequestID

	updated, next := m.Update(agentStatusesMsg{statuses: map[string]string{"w1": "idle", "w2": "blocked"}, err: errors.New("offline")})
	m = updated.(teaModel)

	require.Equal(t, []string{"blocked", "idle"}, sessionNames(m.list.All))
	require.Equal(t, "blocked", m.list.All[0].AgentStatus)
	require.Equal(t, "idle", m.list.All[1].AgentStatus)
	current, ok := m.list.Current()
	require.True(t, ok)
	require.Equal(t, selectedKey, model.Key(current))
	require.Equal(t, selectedKey, m.previewKey)
	require.Equal(t, previewRequestID, m.previewRequestID)
	require.NotNil(t, next)
}

func TestTeaModelAgentStatusRefreshKeepsNonAgentSortOrder(t *testing.T) {
	for _, tc := range []struct {
		mode string
		want []string
	}{
		{mode: "workspace", want: []string{"first", "second"}},
		{mode: "recent", want: []string{"second", "first"}},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			m := newTeaModel([]model.Session{
				{Source: "herdr", Name: "first", WorkspaceID: "w1", AgentStatus: "working"},
				{Source: "herdr", Name: "second", WorkspaceID: "w2", AgentStatus: "idle"},
			}, Options{WorkspaceSort: tc.mode, RecentWorkspaceIDs: []string{"w2", "w1"}})

			updated, _ := m.Update(agentStatusesMsg{statuses: map[string]string{"w1": "idle", "w2": "blocked"}})
			m = updated.(teaModel)

			require.Equal(t, tc.want, sessionNames(m.list.All))
			statuses := map[string]string{}
			for _, item := range m.list.All {
				statuses[item.WorkspaceID] = item.AgentStatus
			}
			assert.Equal(t, map[string]string{"w1": "idle", "w2": "blocked"}, statuses)
		})
	}
}

func TestPreviewViewUsesConstantHeight(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "api"}}, Options{})
	m.preview = "one line"
	short := m.previewView(40, 4)
	m.preview = strings.Repeat("wrapped preview content ", 20)
	long := m.previewView(40, 4)
	require.Equal(t, lipgloss.Height(long), lipgloss.Height(short))
	require.Equal(t, 4+previewTitleRows, lipgloss.Height(short))
	require.Contains(t, long, "...")
	assert.NotContains(t, ansi.Strip(long), "+-")
}

func TestTeaModelUsesAvailableWindowHeight(t *testing.T) {
	items := make([]model.Session, 30)
	for i := range items {
		items[i] = model.Session{Name: "workspace"}
	}
	m := newTeaModel(items, Options{})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(teaModel)
	require.Greater(t, m.previewBodyLines(), defaultVisibleRows)
	view := ansi.Strip(m.View().Content)
	require.Equal(t, 40, lipgloss.Height(view))
	lines := strings.Split(view, "\n")
	require.Empty(t, lines[0])
	require.Contains(t, lines[1], "herdr / sesh")
	require.Equal(t, 120, maxLineWidth(view))
	assert.Empty(t, strings.TrimSpace(lines[len(lines)-1]))
}

func TestSelectedRowUsesRailAndPreservesSourceColor(t *testing.T) {
	got := row(model.Session{Source: "herdr", Name: "herdr-plugin-sesh", Path: "/tmp/herdr-plugin-sesh", AgentStatus: "working"}, true, 80, true, "")
	plain := ansi.Strip(got)
	require.Contains(t, plain, "┃")
	for _, want := range []string{"38;2;125;207;255", "38;2;224;175;104", herdrSourceIcon + " herdr", "herdr-plugin-sesh", "/tmp/herdr-plugin-sesh"} {
		require.Contains(t, got, want)
	}
	require.NotContains(t, got, "48;2;")
	assert.NotContains(t, got, "48;5;")
}

func TestListViewHighlightsCaseInsensitiveQueryMatches(t *testing.T) {
	m := newTeaModel([]model.Session{{
		Name:     "workspace-API",
		Path:     "/tmp/workspace-API",
		Worktree: model.WorktreeRelation{Linked: true, ParentWorkspaceName: "root"},
	}}, Options{})
	m.list.Filter("api")

	got := m.listView(80, 1)
	want := lipgloss.NewStyle().Foreground(violetColor).Bold(true).Render("API")
	assert.Equal(t, 2, strings.Count(got, want))
}

func TestListViewPreservesUnicodeWhenHighlightingFoldedMatch(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "workspace-Ⱥ", Path: "/tmp/workspace-Ⱥ"}}, Options{})
	m.list.Filter("ⱥ")

	got := m.listView(80, 1)
	want := matchStyle.Render("Ⱥ")
	require.True(t, utf8.ValidString(got))
	assert.Equal(t, 2, strings.Count(got, want))
}

func TestTeaModelStacksPreviewAtNarrowWidth(t *testing.T) {
	m := newTeaModel([]model.Session{{Source: "herdr", Name: "api", Path: "/tmp/api", AgentStatus: "blocked"}}, Options{})
	m.preview = "preview content"
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 70, Height: 28})
	m = updated.(teaModel)
	view := ansi.Strip(m.View().Content)
	require.Equal(t, 28, lipgloss.Height(view))
	require.Contains(t, view, "WORKSPACES")
	require.Contains(t, view, "PREVIEW [ctrl+o] · api · blocked")
	assert.NotContains(t, view, "│")
}

func TestTeaModelHidePreviewUsesAllAvailableSpace(t *testing.T) {
	items := make([]model.Session, 18)
	for i := range items {
		items[i] = model.Session{
			Name: fmt.Sprintf("workspace-%02d", i),
			Path: "/tmp/path-that-only-fits-after-preview-space-is-reclaimed",
		}
	}

	for _, width := range []int{70, 120} {
		t.Run(fmt.Sprintf("width_%d", width), func(t *testing.T) {
			m := newTeaModel(items, Options{HidePreview: true})
			updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 28})
			m = updated.(teaModel)
			view := ansi.Strip(m.View().Content)

			require.NotContains(t, view, "PREVIEW")
			require.NotContains(t, view, "│")
			require.Equal(t, 28, lipgloss.Height(view))
			require.Equal(t, width, maxLineWidth(view))
			if width == 70 {
				assert.Contains(t, view, "workspace-17")
			}
			if width == 120 {
				assert.Contains(t, view, items[0].Path)
			}
		})
	}
}

func TestTeaModelHidePreviewFitsShortNarrowTerminal(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "api", Path: "/tmp/api"}}, Options{HidePreview: true})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 70, Height: 14})
	m = updated.(teaModel)
	view := ansi.Strip(m.View().Content)

	assert.Equal(t, 14, lipgloss.Height(view))
}

func TestTeaModelHidePreviewShowsWorkspaceCloseProgressInFooter(t *testing.T) {
	m := newTeaModel([]model.Session{{Source: "herdr", Name: "api", WorkspaceID: "w1"}}, Options{
		HidePreview:    true,
		CloseWorkspace: func(context.Context, string) error { return nil },
	})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 28})
	m = updated.(teaModel)
	m, _ = m.closeSelectedWorkspace()
	t.Cleanup(func() {
		if m.cancelWorkspaceClose != nil {
			m.cancelWorkspaceClose()
		}
	})

	require.Contains(t, renderedFooter(m), "Closing workspace...")
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(teaModel)
	assert.Contains(t, renderedFooter(m), "Cancelling workspace close...")
}

func renderedFooter(m teaModel) string {
	lines := strings.Split(strings.TrimSuffix(ansi.Strip(m.View().Content), "\n"), "\n")
	return lines[len(lines)-1]
}

func TestTeaModelSplitsPreviewAtTerminalThreshold(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "api"}}, Options{})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: previewSplitWidth, Height: 28})
	m = updated.(teaModel)
	assert.Contains(t, ansi.Strip(m.View().Content), "│")
}

func TestTeaModelHeaderShowsFilteredCountWhenAllRowsMatch(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "workspace-api"}, {Name: "workspace-web"}}, Options{})
	m.list.Filter("workspace")
	assert.Contains(t, ansi.Strip(m.header(80)), "2/2 workspaces")
}

func TestTeaModelCyclesHerdrWorkspaceSortModes(t *testing.T) {
	items := []model.Session{
		{Source: "config", Name: "configured"},
		{Source: "herdr", Name: "first", WorkspaceID: "w1", AgentStatus: "working"},
		{Source: "zoxide", Name: "recent-directory"},
		{Source: "herdr", Name: "second", WorkspaceID: "w2", AgentStatus: "blocked"},
		{Source: "herdr", Name: "third", WorkspaceID: "w3", AgentStatus: "idle"},
	}
	m := newTeaModel(items, Options{RecentWorkspaceIDs: []string{"w3", "w1"}})
	require.Equal(t, []string{"configured", "first", "recent-directory", "second", "third"}, sessionNames(m.list.All))

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	m = updated.(teaModel)
	require.Equal(t, []string{"configured", "third", "recent-directory", "first", "second"}, sessionNames(m.list.All))
	require.Contains(t, ansi.Strip(m.View().Content), "ctrl+r recent")

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	m = updated.(teaModel)
	require.Equal(t, []string{"configured", "second", "recent-directory", "first", "third"}, sessionNames(m.list.All))
	require.Contains(t, ansi.Strip(m.View().Content), "ctrl+r agent")

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	m = updated.(teaModel)
	require.Equal(t, []string{"configured", "first", "recent-directory", "second", "third"}, sessionNames(m.list.All))
	assert.Contains(t, ansi.Strip(m.View().Content), "ctrl+r workspace")
}

func TestTeaModelStartsWithConfiguredWorkspaceSort(t *testing.T) {
	items := []model.Session{
		{Source: "herdr", Name: "first", WorkspaceID: "w1", AgentStatus: "idle"},
		{Source: "herdr", Name: "second", WorkspaceID: "w2"},
		{Source: "herdr", Name: "third", WorkspaceID: "w3", AgentStatus: "blocked"},
	}
	tests := []struct {
		name string
		mode string
		want []string
	}{
		{name: "default", want: []string{"first", "second", "third"}},
		{name: "workspace", mode: "workspace", want: []string{"first", "second", "third"}},
		{name: "recent", mode: "recent", want: []string{"second", "first", "third"}},
		{name: "agent", mode: "agent", want: []string{"third", "first", "second"}},
		{name: "unknown falls back", mode: "future", want: []string{"first", "second", "third"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newTeaModel(items, Options{RecentWorkspaceIDs: []string{"w2", "w1", "w3"}, WorkspaceSort: tc.mode})
			require.Equal(t, tc.want, sessionNames(m.list.All))
			wantMode := tc.mode
			if wantMode == "" || wantMode == "future" {
				wantMode = "workspace"
			}
			assert.Contains(t, ansi.Strip(m.View().Content), "ctrl+r "+wantMode)
		})
	}
}

func TestTeaModelAgentSortRanksStatusesAndPreservesSourceSlots(t *testing.T) {
	items := []model.Session{
		{Source: "config", Name: "configured"},
		{Source: "herdr", Name: "unknown", WorkspaceID: "w-unknown", AgentStatus: "unknown"},
		{Source: "zoxide", Name: "directory"},
		{Source: "herdr", Name: "working-a", WorkspaceID: "w-working-a", AgentStatus: "working"},
		{Source: "herdr", Name: "agentless", WorkspaceID: "w-agentless"},
		{Source: "herdr", Name: "blocked", WorkspaceID: "w-blocked", AgentStatus: "blocked"},
		{Source: "herdr", Name: "done", WorkspaceID: "w-done", AgentStatus: "done"},
		{Source: "herdr", Name: "working-b", WorkspaceID: "w-working-b", AgentStatus: "working"},
		{Source: "herdr", Name: "idle", WorkspaceID: "w-idle", AgentStatus: "idle"},
		{Source: "herdr", Name: "future", WorkspaceID: "w-future", AgentStatus: "waiting"},
	}

	m := newTeaModel(items, Options{WorkspaceSort: "agent"})

	want := []string{"configured", "blocked", "directory", "done", "working-a", "working-b", "idle", "unknown", "agentless", "future"}
	assert.Equal(t, want, sessionNames(m.list.All))
}

func TestTeaModelAgentSortPromotesWorktreeFamilyByBestMember(t *testing.T) {
	items := []model.Session{
		{Source: "herdr", Name: "unresolved", WorkspaceID: "w-unresolved", AgentStatus: "working", Worktree: model.WorktreeRelation{Linked: true, ParentWorkspaceID: "w-missing"}},
		{Source: "herdr", Name: "child-idle", WorkspaceID: "w-child-idle", AgentStatus: "idle", Worktree: model.WorktreeRelation{Linked: true, ParentWorkspaceID: "w-parent"}},
		{Source: "herdr", Name: "other", WorkspaceID: "w-other", AgentStatus: "done"},
		{Source: "herdr", Name: "parent", WorkspaceID: "w-parent"},
		{Source: "herdr", Name: "child-blocked", WorkspaceID: "w-child-blocked", AgentStatus: "blocked", Worktree: model.WorktreeRelation{Linked: true, ParentWorkspaceID: "w-parent"}},
	}

	m := newTeaModel(items, Options{WorkspaceSort: "agent"})

	want := []string{"parent", "child-blocked", "child-idle", "other", "unresolved"}
	assert.Equal(t, want, sessionNames(m.list.All))
}

func TestSortHerdrWorkspacesGroupsChildrenBelowParent(t *testing.T) {
	items := []model.Session{
		{Source: "config", Name: "configured"},
		{Source: "herdr", Name: "child-b", WorkspaceID: "w-child-b", Worktree: model.WorktreeRelation{Linked: true, ParentWorkspaceID: "w-parent", ParentWorkspaceName: "parent"}},
		{Source: "zoxide", Name: "directory"},
		{Source: "herdr", Name: "unrelated", WorkspaceID: "w-unrelated"},
		{Source: "herdr", Name: "parent", WorkspaceID: "w-parent"},
		{Source: "herdr", Name: "child-a", WorkspaceID: "w-child-a", Worktree: model.WorktreeRelation{Linked: true, ParentWorkspaceID: "w-parent", ParentWorkspaceName: "parent"}},
	}

	sortHerdrWorkspaces(items, []string{"w-child-b", "w-unrelated", "w-parent", "w-child-a"})

	assert.Equal(t, []string{"configured", "parent", "directory", "child-b", "child-a", "unrelated"}, sessionNames(items))
}

func TestSortHerdrWorkspacesRanksFamilyByMostRecentMember(t *testing.T) {
	items := []model.Session{
		{Source: "herdr", Name: "parent", WorkspaceID: "w-parent"},
		{Source: "herdr", Name: "child-a", WorkspaceID: "w-child-a", Worktree: model.WorktreeRelation{Linked: true, ParentWorkspaceID: "w-parent"}},
		{Source: "herdr", Name: "child-b", WorkspaceID: "w-child-b", Worktree: model.WorktreeRelation{Linked: true, ParentWorkspaceID: "w-parent"}},
		{Source: "herdr", Name: "unrelated", WorkspaceID: "w-unrelated"},
	}

	sortHerdrWorkspaces(items, []string{"w-child-b", "w-unrelated", "w-parent"})

	assert.Equal(t, []string{"parent", "child-b", "child-a", "unrelated"}, sessionNames(items))
}

func TestSortHerdrWorkspacesLeavesUnresolvedChildIndependent(t *testing.T) {
	items := []model.Session{
		{Source: "herdr", Name: "unresolved", WorkspaceID: "w-child", Worktree: model.WorktreeRelation{Linked: true, ParentWorkspaceID: "w-missing"}},
		{Source: "herdr", Name: "other", WorkspaceID: "w-other"},
	}

	sortHerdrWorkspaces(items, []string{"w-other", "w-child"})

	assert.Equal(t, []string{"other", "unresolved"}, sessionNames(items))
}

func TestTeaModelGroupsWorktreeFamilyWithoutMutatingInput(t *testing.T) {
	items := []model.Session{
		{Source: "herdr", Name: "child", WorkspaceID: "w-child", Worktree: model.WorktreeRelation{Linked: true, ParentWorkspaceID: "w-parent"}},
		{Source: "herdr", Name: "parent", WorkspaceID: "w-parent"},
	}

	m := newTeaModel(items, Options{})

	require.Equal(t, []string{"parent", "child"}, sessionNames(m.list.All))
	assert.Equal(t, []string{"child", "parent"}, sessionNames(items))
}

func TestTeaModelKeepsWorktreeFamilyAcrossSortModes(t *testing.T) {
	items := []model.Session{
		{Source: "herdr", Name: "child", WorkspaceID: "w-child", AgentStatus: "blocked", Worktree: model.WorktreeRelation{Linked: true, ParentWorkspaceID: "w-parent"}},
		{Source: "herdr", Name: "parent", WorkspaceID: "w-parent"},
		{Source: "herdr", Name: "other", WorkspaceID: "w-other", AgentStatus: "done"},
	}
	m := newTeaModel(items, Options{RecentWorkspaceIDs: []string{"w-other", "w-child"}})
	require.Equal(t, []string{"parent", "child", "other"}, sessionNames(m.list.All))

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	m = updated.(teaModel)
	require.Equal(t, []string{"other", "parent", "child"}, sessionNames(m.list.All))

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	m = updated.(teaModel)
	require.Equal(t, []string{"parent", "child", "other"}, sessionNames(m.list.All))

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	m = updated.(teaModel)
	assert.Equal(t, []string{"parent", "child", "other"}, sessionNames(m.list.All))
}

func TestTeaModelFilterDoesNotInjectWorktreeParent(t *testing.T) {
	m := newTeaModel([]model.Session{
		{Source: "herdr", Name: "parent", WorkspaceID: "w-parent"},
		{Source: "herdr", Name: "feature", Path: "/tmp/feature", WorkspaceID: "w-child", Worktree: model.WorktreeRelation{Linked: true, ParentWorkspaceID: "w-parent", ParentWorkspaceName: "parent"}},
	}, Options{})

	m.list.Filter("feature")

	require.Equal(t, []string{"feature"}, sessionNames(m.list.Filtered))
	rowText := ansi.Strip(m.listView(80, 1))
	require.Contains(t, rowText, "[↳ herdr]")
	require.Contains(t, rowText, "feature")
	require.Contains(t, rowText, "/tmp/feature")
	require.NotContains(t, rowText, "worktree of parent")
	require.NotContains(t, rowText, "├─")
	assert.NotContains(t, rowText, "└─")
}

func TestTeaModelSearchRailDoesNotTruncateAtWindowEdge(t *testing.T) {
	m := newTeaModel([]model.Session{{Name: "api"}}, Options{})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 28})
	m = updated.(teaModel)
	for _, line := range strings.Split(ansi.Strip(m.View().Content), "\n") {
		if strings.Contains(line, defaultPrompt) {
			assert.NotContains(t, line, "…")
		}
	}
}

func TestListViewUsesDirectionalOverflowMarkers(t *testing.T) {
	items := make([]model.Session, 20)
	for i := range items {
		items[i] = model.Session{Name: "workspace"}
	}
	m := newTeaModel(items, Options{})
	m.list.Selected = 10
	view := ansi.Strip(m.listView(60, 6))
	require.Contains(t, view, "↑ 7 more")
	require.Contains(t, view, "↓ 9 more")
	assert.NotContains(t, view, "...")
}

func TestListViewKeepsSelectionVisibleWithTwoRows(t *testing.T) {
	items := []model.Session{{Name: "workspace-0"}, {Name: "workspace-1"}, {Name: "workspace-2"}, {Name: "workspace-3"}, {Name: "workspace-4"}}
	m := newTeaModel(items, Options{})
	m.list.Selected = 2
	assert.Contains(t, ansi.Strip(m.listView(60, 2)), "workspace-2")
}

func maxLineWidth(s string) int {
	maxWidth := 0
	for _, line := range strings.Split(s, "\n") {
		maxWidth = max(maxWidth, lipgloss.Width(line))
	}
	return maxWidth
}

func visualColumn(line, marker string) int {
	index := strings.Index(line, marker)
	if index < 0 {
		return -1
	}
	return lipgloss.Width(line[:index])
}

func sessionNames(items []model.Session) []string {
	names := make([]string, len(items))
	for i := range items {
		names[i] = items[i].Name
	}
	return names
}

package sources

import (
	"context"
	"testing"

	"github.com/fullerzz/herdr-plugin-sesh/internal/config"
	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoteWorkspacesSurviveNameCollisionsAndSkipLocalConfig(t *testing.T) {
	client := &countingHerdrClient{FakeClient: &herdr.FakeClient{Workspaces: []herdr.Workspace{
		{ID: "sp1", Label: "api", CWD: "/src/api"},
		{ID: "buntu26/sp1", Machine: "buntu26", Label: "api", CWD: "/src/api"},
		{ID: "ser8/sp1", Machine: "ser8", Label: "api"},
	}}}
	got, err := Merge(context.Background(), []Source{HerdrWorkspaces{Client: client}, staticSource{
		name: "config", sessions: []model.Session{{Source: "config", Name: "api", Path: "/src/api"}},
	}}, []string{"herdr", "config"}, nil, false, true)
	require.NoError(t, err)
	assert.Zero(t, client.paneListCalls)
	items := got.Ordered()
	require.Len(t, items, 3)
	assert.Equal(t, "buntu26", items[1].Machine)
	assert.Equal(t, "buntu26/sp1", items[1].WorkspaceID)
	assert.Equal(t, "/src/api", items[1].Path)
	assert.Empty(t, items[2].Path)
	ApplyConfig(&got, config.Config{
		DefaultSessionConfig: config.DefaultSessionConfig{StartupCommand: "local-startup"},
		WildcardConfigs:      []config.WildcardConfig{{Pattern: "/src/**", PreviewCommand: "local-preview", Windows: []string{"dev"}}},
		WindowConfigs:        []model.WindowConfig{{Name: "dev", Path: "local-dir"}},
	}, "")
	items = got.Ordered()
	assert.Equal(t, "local-startup", items[0].StartupCommand)
	assert.Empty(t, items[1].StartupCommand)
	assert.Empty(t, items[2].StartupCommand)
	assert.Equal(t, "local-preview", items[0].PreviewCommand)
	assert.Empty(t, items[1].PreviewCommand)
	assert.Empty(t, items[1].WindowConfigs)
}

func TestRemoteWorktreeDoesNotRelateToLocalRepository(t *testing.T) {
	got, err := (HerdrWorkspaces{Client: &herdr.FakeClient{Workspaces: []herdr.Workspace{
		{ID: "local", Worktree: &herdr.Worktree{RepoKey: "/src/.git"}},
		{ID: "buntu26/child", Machine: "buntu26", Worktree: &herdr.Worktree{RepoKey: "/src/.git", IsLinkedWorktree: true}},
	}}}).List(context.Background())
	require.NoError(t, err)
	items := got.Ordered()
	require.Len(t, items, 2)
	assert.Empty(t, items[1].Worktree.ParentWorkspaceID)
}

func TestRemoteConfigSessionKeepsPathsForItsMachine(t *testing.T) {
	cfg := config.Config{
		SessionConfigs: []config.SessionConfig{
			{Name: "api", Machine: "buntu26", Path: "~/src/api", Windows: []string{"dev"}},
			{Name: "local", Path: "~/src/local", Windows: []string{"dev"}},
		},
		WindowConfigs: []model.WindowConfig{{Name: "dev", Path: "web", Panes: []model.PaneConfig{
			{Name: "shell"}, {Name: "logs", Path: "~/logs"},
		}}},
		WildcardConfigs: []config.WildcardConfig{{Pattern: "~/src/**", StartupCommand: "local-rule"}},
	}
	got, err := ConfigSessions{Config: cfg, Home: "/Users/me"}.List(context.Background())
	require.NoError(t, err)
	ApplyConfig(&got, cfg, "/Users/me")
	items := got.Ordered()
	require.Len(t, items, 2)

	remote := items[0]
	assert.Equal(t, "buntu26", remote.Machine)
	assert.Equal(t, "~/src/api", remote.Path, "no local home expansion")
	assert.Empty(t, remote.StartupCommand, "no local rule")
	require.Len(t, remote.WindowConfigs, 1)
	assert.Equal(t, "~/src/api/web", remote.WindowConfigs[0].Path)
	assert.Equal(t, "~/src/api/web", remote.WindowConfigs[0].Panes[0].Path)
	assert.Equal(t, "~/logs", remote.WindowConfigs[0].Panes[1].Path, "a ~ path is not joined to its parent")

	local := items[1]
	assert.Equal(t, "/Users/me/src/local", local.Path)
	assert.Equal(t, "local-rule", local.StartupCommand)
	assert.Equal(t, "/Users/me/logs", local.WindowConfigs[0].Panes[1].Path)
}

func TestRemoteConfigRelativePathsKeepHomeAnchor(t *testing.T) {
	cfg := config.Config{
		SessionConfigs: []config.SessionConfig{
			{Name: "home", Machine: "buntu26", Path: "~", Windows: []string{"up", "down"}},
			{Name: "src", Machine: "buntu26", Path: "~/src", Windows: []string{"up2"}},
		},
		WindowConfigs: []model.WindowConfig{
			{Name: "up", Path: ".."},
			{Name: "down", Path: "web", Panes: []model.PaneConfig{{Name: "back", Path: ".."}}},
			{Name: "up2", Path: "../.."},
		},
	}
	got, err := ConfigSessions{Config: cfg, Home: "/Users/me"}.List(context.Background())
	require.NoError(t, err)
	items := got.Ordered()
	require.Len(t, items, 2)
	require.Len(t, items[0].WindowConfigs, 2)
	assert.Equal(t, "~/..", items[0].WindowConfigs[0].Path)
	assert.Equal(t, "~/web", items[0].WindowConfigs[1].Path)
	assert.Equal(t, "~", items[0].WindowConfigs[1].Panes[0].Path)
	require.Len(t, items[1].WindowConfigs, 1)
	assert.Equal(t, "~/..", items[1].WindowConfigs[0].Path)
}

func TestOpenRemoteSpaceStandsInForItsConfiguredWorkspace(t *testing.T) {
	got, err := Merge(context.Background(), []Source{
		HerdrWorkspaces{Client: &herdr.FakeClient{Workspaces: []herdr.Workspace{
			{ID: "buntu26/sp1", Machine: "buntu26", Label: "api", CWD: "/home/zach/src/api"},
			{ID: "ser8/sp2", Machine: "ser8", Label: "api"},
			{ID: "sp3", Label: "web", CWD: "/src/web"},
		}}},
		staticSource{name: "config", sessions: []model.Session{
			{Source: "config", Machine: "buntu26", Name: "api", Path: "~/src/api"},
			{Source: "config", Machine: "buntu26", Name: "docs", Path: "/srv/docs"},
			{Source: "config", Name: "web", Path: "/src/web"},
		}},
	}, []string{"herdr", "config"}, nil, false, true)
	require.NoError(t, err)
	items := got.Ordered()
	require.Len(t, items, 4)
	assert.Equal(t, "buntu26/sp1", items[0].WorkspaceID)
	assert.True(t, items[0].Configured)
	assert.False(t, items[1].Configured, "same name on another machine")
	assert.False(t, items[2].Configured, "local namesakes merge as before")
	assert.Equal(t, "config", items[3].Source)
	assert.Equal(t, "docs", items[3].Name)
}

// Source order is display order only: a configured remote entry listed first
// still resolves to the open space with the same machine and name.
func TestOpenRemoteSpaceStandsInForItsConfiguredWorkspaceInEitherOrder(t *testing.T) {
	for _, order := range [][]string{{"herdr", "config"}, {"config", "herdr"}} {
		got, err := Merge(context.Background(), []Source{
			HerdrWorkspaces{Client: &herdr.FakeClient{Workspaces: []herdr.Workspace{
				{ID: "buntu26/sp1", Machine: "buntu26", Label: "api", CWD: "/home/zach/src/api"},
				{ID: "sp3", Label: "web", CWD: "/src/web"},
			}}},
			staticSource{name: "config", sessions: []model.Session{
				{Source: "config", Machine: "buntu26", Name: "api", Path: "~/src/api"},
				{Source: "config", Name: "web", Path: "/src/web"},
			}},
		}, order, nil, false, true)
		require.NoError(t, err, order)
		items := got.Ordered()
		require.Len(t, items, 2, order)
		var api, web model.Session
		for _, s := range items {
			switch s.Name {
			case "api":
				api = s
			case "web":
				web = s
			}
		}
		assert.Equal(t, "buntu26/sp1", api.WorkspaceID, order)
		assert.True(t, api.Configured, order)
		assert.Equal(t, order[0], web.Source, "local namesakes keep the first source, %v", order)
		assert.Len(t, got.Directory, 2, order)
		for key, s := range got.Directory {
			assert.Equal(t, model.Key(s), key, order)
		}
	}
}

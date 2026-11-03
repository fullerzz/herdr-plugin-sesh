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

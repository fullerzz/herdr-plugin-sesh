package sources

import (
	"context"
	"testing"

	"github.com/fullerzz/herdr-plugin-sesh/internal/config"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigSessionsExpandsHomePaths(t *testing.T) {
	cfg := config.Config{
		SessionConfigs: []config.SessionConfig{
			{Name: "api", Path: "~/projects/api", Windows: []string{"logs"}},
		},
		WindowConfigs: []model.WindowConfig{
			{Name: "logs", Path: "~/projects/api/logs"},
		},
	}

	got, err := ConfigSessions{Config: cfg, Home: "/home/zach"}.List(context.Background())
	require.NoError(t, err)
	sessions := got.Ordered()
	require.Len(t, sessions, 1)
	require.Equal(t, "/home/zach/projects/api", sessions[0].Path)
	require.Len(t, sessions[0].WindowConfigs, 1)
	assert.Equal(t, "/home/zach/projects/api/logs", sessions[0].WindowConfigs[0].Path)
}

func TestApplyConfigAttachesWildcardWindows(t *testing.T) {
	cfg := config.Config{
		WildcardConfigs: []config.WildcardConfig{{Pattern: "~/projects/*", Windows: []string{"logs"}}},
		WindowConfigs:   []model.WindowConfig{{Name: "logs", Path: "~/projects/api/logs"}},
	}
	sessions := model.NewSessions()
	sessions.Add(model.Session{Source: "zoxide", Name: "api", Path: "/home/zach/projects/api"})

	ApplyConfig(&sessions, cfg, "/home/zach")

	got := sessions.Ordered()[0]
	require.Len(t, got.WindowConfigs, 1)
	require.Equal(t, "logs", got.WindowConfigs[0].Name)
	assert.Equal(t, "/home/zach/projects/api/logs", got.WindowConfigs[0].Path)
}

func TestApplyConfigWildcardDisablePrecedesStartupFallback(t *testing.T) {
	cfg := config.Config{
		DefaultSessionConfig: config.DefaultSessionConfig{StartupCommand: "default"},
		WildcardConfigs: []config.WildcardConfig{{
			Pattern:             "/projects/**",
			StartupCommand:      "wildcard",
			DisableStartCommand: true,
		}},
	}
	sessions := model.NewSessions()
	sessions.Add(model.Session{Source: "config", Name: "explicit", Path: "/projects/explicit", StartupCommand: "configured"})
	sessions.Add(model.Session{Source: "config", Name: "fallback", Path: "/projects/fallback"})

	ApplyConfig(&sessions, cfg, "")

	got := sessions.Ordered()
	require.True(t, got[0].DisableStartupCommand)
	require.Equal(t, "configured", got[0].StartupCommand)
	require.True(t, got[1].DisableStartupCommand)
	assert.Empty(t, got[1].StartupCommand)
}

func TestApplyConfigCarriesPaneLayoutsForWorkspacesAndRules(t *testing.T) {
	panes := []model.PaneConfig{{Name: "a"}, {Name: "b", SplitFrom: "a", Split: "right", Ratio: 0.3}}
	cfg := config.Config{
		WindowConfigs:   []model.WindowConfig{{Name: "dev", Panes: panes}},
		SessionConfigs:  []config.SessionConfig{{Name: "api", Path: "/tmp/api", Windows: []string{"dev"}}},
		WildcardConfigs: []config.WildcardConfig{{Pattern: "/tmp/projects/**", Windows: []string{"dev"}}},
	}
	sessions, err := ConfigSessions{Config: cfg}.List(context.Background())
	require.NoError(t, err)
	sessions.Add(model.Session{Source: "dir", Name: "web", Path: "/tmp/projects/web"})
	ApplyConfig(&sessions, cfg, "")
	got := sessions.Ordered()
	require.Len(t, got, 2)
	for _, s := range got {
		require.Len(t, s.WindowConfigs, 1, s.Name)
		want := []model.PaneConfig{{Name: "a", Path: s.Path}, {Name: "b", SplitFrom: "a", Split: "right", Ratio: 0.3, Path: s.Path}}
		assert.Equal(t, want, s.WindowConfigs[0].Panes, s.Name)
	}
}

func TestConfigResolvesLayoutPathsPerSessionWithoutMutatingTemplates(t *testing.T) {
	t.Setenv("HOME", "/wrong-home")
	cfg := config.Config{
		WindowConfigs: []model.WindowConfig{{Name: "dev", Path: "web", Panes: []model.PaneConfig{
			{Name: "root", Path: "src"},
			{Name: "inherited"},
			{Name: "home", Path: "~/logs"},
			{Name: "absolute", Path: "/var/log"},
		}}},
		SessionConfigs: []config.SessionConfig{
			{Name: "a", Path: "~/projects/a", Windows: []string{"dev"}},
			{Name: "b", Path: "~/projects/b", Windows: []string{"dev"}},
		},
		WildcardConfigs: []config.WildcardConfig{{Pattern: "/configured/projects/**", Windows: []string{"dev"}}},
	}
	sessions, err := (ConfigSessions{Config: cfg, Home: "/configured"}).List(context.Background())
	require.NoError(t, err)
	check := func(s model.Session) {
		t.Helper()
		require.Len(t, s.WindowConfigs, 1)
		w := s.WindowConfigs[0]
		assert.Equal(t, s.Path+"/web", w.Path)
		require.Len(t, w.Panes, 4)
		assert.Equal(t, s.Path+"/web/src", w.Panes[0].Path)
		assert.Equal(t, s.Path+"/web", w.Panes[1].Path)
		assert.Equal(t, "/configured/logs", w.Panes[2].Path)
		assert.Equal(t, "/var/log", w.Panes[3].Path)
	}
	for _, s := range sessions.Ordered() {
		check(s)
	}
	sessions.Add(model.Session{Source: "dir", Name: "c", Path: "/configured/projects/c"})
	for range 2 {
		ApplyConfig(&sessions, cfg, "/configured")
		for _, s := range sessions.Ordered() {
			check(s)
		}
	}
	assert.Equal(t, "web", cfg.WindowConfigs[0].Path)
	assert.Equal(t, "src", cfg.WindowConfigs[0].Panes[0].Path)
	assert.Empty(t, cfg.WindowConfigs[0].Panes[1].Path)
}

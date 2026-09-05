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

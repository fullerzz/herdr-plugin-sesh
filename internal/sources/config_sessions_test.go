package sources

import (
	"context"
	"testing"

	"github.com/fullerzz/herdr-plugin-sesh/internal/config"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
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
	if err != nil {
		t.Fatal(err)
	}
	sessions := got.Ordered()
	if len(sessions) != 1 {
		t.Fatalf("got %d sessions", len(sessions))
	}
	if sessions[0].Path != "/home/zach/projects/api" {
		t.Fatalf("session path = %q", sessions[0].Path)
	}
	if len(sessions[0].WindowConfigs) != 1 || sessions[0].WindowConfigs[0].Path != "/home/zach/projects/api/logs" {
		t.Fatalf("window configs = %#v", sessions[0].WindowConfigs)
	}
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
	if len(got.WindowConfigs) != 1 || got.WindowConfigs[0].Name != "logs" || got.WindowConfigs[0].Path != "/home/zach/projects/api/logs" {
		t.Fatalf("window configs = %#v", got.WindowConfigs)
	}
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
	if !got[0].DisableStartupCommand || got[0].StartupCommand != "configured" {
		t.Fatalf("explicit session = %#v", got[0])
	}
	if !got[1].DisableStartupCommand || got[1].StartupCommand != "" {
		t.Fatalf("fallback session = %#v", got[1])
	}
}

package sources

import (
	"context"
	"testing"

	"forgejo.local/fullerzz/herdr-plugin-sesh/internal/config"
	"forgejo.local/fullerzz/herdr-plugin-sesh/internal/model"
)

func TestConfigSessionsExpandsHomePaths(t *testing.T) {
	cfg := config.Config{
		SessionConfigs: []config.SessionConfig{
			{Name: "api", Path: "~/projects/api", DefaultSessionConfig: config.DefaultSessionConfig{Windows: []string{"logs"}}},
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

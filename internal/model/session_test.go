package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSessionJSONOmitsInternalPickerFields(t *testing.T) {
	s := Session{
		Source:         "config",
		Name:           "api",
		Path:           "/tmp/api",
		AgentStatus:    "working",
		ActiveTabID:    "secret-tab",
		WorkspaceTabs:  []WorkspaceTab{{ID: "secret-tab"}},
		WorkspacePanes: []WorkspacePane{{ID: "secret-pane"}},
		WindowConfigs:  []WindowConfig{{Name: "dev"}},
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if !strings.Contains(got, `"source":"config"`) || !strings.Contains(got, `"name":"api"`) {
		t.Fatalf("json missing public fields: %s", got)
	}
	if strings.Contains(got, "WindowConfigs") || strings.Contains(got, "window_configs") {
		t.Fatalf("json leaked internal window configs: %s", got)
	}
	if strings.Contains(got, "AgentStatus") || strings.Contains(got, "agent_status") {
		t.Fatalf("json leaked internal agent status: %s", got)
	}
	if strings.Contains(got, "secret-tab") || strings.Contains(got, "secret-pane") {
		t.Fatalf("json leaked internal workspace layout metadata: %s", got)
	}
}

func TestKeyIsStableAndSourceScoped(t *testing.T) {
	a := Session{Source: "config", Name: "api", Path: "/tmp/api"}
	b := Session{Source: "config", Name: "api", Path: "/tmp/api"}
	c := Session{Source: "zoxide", Name: "api", Path: "/tmp/api"}
	if Key(a) != Key(b) {
		t.Fatalf("expected stable key, got %q and %q", Key(a), Key(b))
	}
	if Key(a) == Key(c) {
		t.Fatalf("expected source-scoped key, got %q", Key(a))
	}
	b.AgentStatus = "working"
	if Key(a) != Key(b) {
		t.Fatalf("expected status-independent key, got %q and %q", Key(a), Key(b))
	}
}

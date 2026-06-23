package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSessionJSONOmitsInternalWindowConfigs(t *testing.T) {
	s := Session{Source: "config", Name: "api", Path: "/tmp/api", WindowConfigs: []WindowConfig{{Name: "dev"}}}
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
}

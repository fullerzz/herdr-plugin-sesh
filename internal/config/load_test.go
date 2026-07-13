package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadExplicitSessionAndWindows(t *testing.T) {
	d := t.TempDir()
	cfgp := filepath.Join(d, "sesh.toml")
	mustWrite(t, cfgp, `dir_length = 2
blacklist = ["scratch"]
[default_session]
startup_command = "nvim"
[[session]]
name = "API"
path = "~/projects/api"
windows = ["git"]
[[window]]
name = "git"
startup_script = "git status"
`)
	cfg, path, err := Load(LoadOptions{Path: cfgp, Home: "/home/zach"})
	if err != nil {
		t.Fatal(err)
	}
	if path != cfgp || cfg.DirLength != 2 || len(cfg.SessionConfigs) != 1 || cfg.SessionConfigs[0].Path != "~/projects/api" {
		t.Fatalf("unexpected cfg %#v path %s", cfg, path)
	}
	if len(cfg.WindowConfigs) != 1 || cfg.WindowConfigs[0].Name != "git" {
		t.Fatalf("missing window %#v", cfg.WindowConfigs)
	}
	if cfg.DefaultSessionConfig.PreviewCommand != DefaultPreviewCommand {
		t.Fatalf("preview command = %q", cfg.DefaultSessionConfig.PreviewCommand)
	}
}

func TestLoadStrictRejectsUnknown(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "sesh.toml")
	mustWrite(t, p, "strict_mode = true\nwat = 1\n")
	_, _, err := Load(LoadOptions{Path: p})
	if err == nil {
		t.Fatal("expected strict error")
	}
}
func TestLoadImportOrder(t *testing.T) {
	d := t.TempDir()
	mustWrite(t, filepath.Join(d, "extra.toml"), `[[session]]
name="extra"
path="/extra"
`)
	p := filepath.Join(d, "sesh.toml")
	mustWrite(t, p, "import=[\"extra.toml\"]\n[[session]]\nname=\"main\"\npath=\"/main\"\n")
	cfg, _, err := Load(LoadOptions{Path: p})
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{cfg.SessionConfigs[0].Name, cfg.SessionConfigs[1].Name}; got[0] != "extra" || got[1] != "main" {
		t.Fatalf("bad order %#v", got)
	}
}

func TestLoadMergesNestedConfigTablesFieldByField(t *testing.T) {
	d := t.TempDir()
	mustWrite(t, filepath.Join(d, "extra.toml"), `[default_session]
startup_command = "git status"
preview_command = "printf extra {}"
windows = ["git"]

[tui]
prompt = "Extra> "
placeholder = "Extra search"
`)
	p := filepath.Join(d, "sesh.toml")
	mustWrite(t, p, `import = ["extra.toml"]

[default_session]
startup_command = "make test"

[tui]
placeholder = "Search workspaces"
`)
	cfg, _, err := Load(LoadOptions{Path: p})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultSessionConfig.StartupCommand != "make test" {
		t.Fatalf("startup command = %q", cfg.DefaultSessionConfig.StartupCommand)
	}
	if cfg.DefaultSessionConfig.PreviewCommand != "printf extra {}" {
		t.Fatalf("preview command = %q", cfg.DefaultSessionConfig.PreviewCommand)
	}
	if want := []string{"git"}; !reflect.DeepEqual(cfg.DefaultSessionConfig.Windows, want) {
		t.Fatalf("windows = %#v, want %#v", cfg.DefaultSessionConfig.Windows, want)
	}
	if cfg.TUI.Prompt != "Extra> " {
		t.Fatalf("prompt = %q", cfg.TUI.Prompt)
	}
	if cfg.TUI.Placeholder != "Search workspaces" {
		t.Fatalf("placeholder = %q", cfg.TUI.Placeholder)
	}
}

func TestLoadExplicitEmptyPreviewCommandRestoresDefault(t *testing.T) {
	d := t.TempDir()
	mustWrite(t, filepath.Join(d, "extra.toml"), `[default_session]
preview_command = "printf extra {}"
`)
	p := filepath.Join(d, "sesh.toml")
	mustWrite(t, p, `import = ["extra.toml"]

[default_session]
preview_command = ""
`)
	cfg, _, err := Load(LoadOptions{Path: p})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultSessionConfig.PreviewCommand != DefaultPreviewCommand {
		t.Fatalf("preview command = %q", cfg.DefaultSessionConfig.PreviewCommand)
	}
}

func TestDefaultPreviewCommandUsesEzaIcons(t *testing.T) {
	cfg := Default()
	if cfg.DefaultSessionConfig.PreviewCommand != DefaultPreviewCommand {
		t.Fatalf("preview command = %q", cfg.DefaultSessionConfig.PreviewCommand)
	}
	if DefaultPreviewCommand != "eza --icons=always -la {}" {
		t.Fatalf("default preview command = %q", DefaultPreviewCommand)
	}
}

func mustWrite(t *testing.T, p, s string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), 0600); err != nil {
		t.Fatal(err)
	}
}

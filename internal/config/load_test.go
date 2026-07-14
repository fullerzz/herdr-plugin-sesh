package config

import (
	"os"
	"path/filepath"
	"strings"
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

func TestLoadStrictRejectsUnsupportedSeshKeys(t *testing.T) {
	tests := map[string]string{
		"tmux command":               "tmux_command = \"psmux\"\n",
		"default session windows":    "[default_session]\nwindows = [\"git\"]\n",
		"default session tmuxp":      "[default_session]\ntmuxp = \"project.yaml\"\n",
		"default session tmuxinator": "[default_session]\ntmuxinator = \"project\"\n",
		"session tmuxp":              "[[session]]\nname = \"api\"\npath = \"/tmp/api\"\ntmuxp = \"project.yaml\"\n",
		"session tmuxinator":         "[[session]]\nname = \"api\"\npath = \"/tmp/api\"\ntmuxinator = \"project\"\n",
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "sesh.toml")
			mustWrite(t, p, "strict_mode = true\n"+body)
			if _, _, err := Load(LoadOptions{Path: p}); err == nil {
				t.Fatal("expected strict error")
			}
		})
	}
}

func TestLoadStrictRejectsUnsupportedSeshKeysInImports(t *testing.T) {
	d := t.TempDir()
	mustWrite(t, filepath.Join(d, "extra.toml"), "tmux_command = \"psmux\"\n")
	p := filepath.Join(d, "sesh.toml")
	mustWrite(t, p, "strict_mode = true\nimport = [\"extra.toml\"]\n")
	if _, _, err := Load(LoadOptions{Path: p}); err == nil {
		t.Fatal("expected strict error from imported config")
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

func TestLoadExplicitEmptyTUITextOverridesImportedValues(t *testing.T) {
	d := t.TempDir()
	mustWrite(t, filepath.Join(d, "extra.toml"), `[tui]
prompt = "Extra> "
placeholder = "Extra search"
`)
	p := filepath.Join(d, "sesh.toml")
	mustWrite(t, p, `import = ["extra.toml"]

[tui]
prompt = ""
placeholder = ""
`)
	cfg, _, err := Load(LoadOptions{Path: p})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TUI.Prompt != "" || cfg.TUI.Placeholder != "" {
		t.Fatalf("TUI text = prompt %q, placeholder %q", cfg.TUI.Prompt, cfg.TUI.Placeholder)
	}
}

func TestLoadExplicitFalseShowIconsOverridesImportedValue(t *testing.T) {
	d := t.TempDir()
	mustWrite(t, filepath.Join(d, "extra.toml"), "[tui]\nshow_icons = true\n")
	p := filepath.Join(d, "sesh.toml")
	mustWrite(t, p, "import = [\"extra.toml\"]\n[tui]\nshow_icons = false\n")
	cfg, _, err := Load(LoadOptions{Path: p})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TUI.ShowIcons {
		t.Fatal("show_icons = true, want false")
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

func TestInitConfigDoesNotAdvertiseUnsupportedSeshSchema(t *testing.T) {
	p, err := InitConfig(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // p is returned from InitConfig using a test-owned temporary directory.
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "sesh.schema.json") {
		t.Fatalf("starter config advertises the full Sesh schema:\n%s", data)
	}
}

func mustWrite(t *testing.T, p, s string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), 0600); err != nil {
		t.Fatal(err)
	}
}

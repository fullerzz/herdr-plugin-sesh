package config

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const migrateLegacyBody = `cache = true
sort_order = ["config", "zoxide"]
dir_length = 2
separator_aware = true
blacklist = ["^scratch$"]

[tui]
show_icons = true
prompt = "P> "
default_sort = "recent"

[default_session]
startup_command = "make dev"
preview_command = "ls {}"

[[window]]
name = "git"
startup_script = "git status"

[[session]]
name = "brain"
path = "~/brain"
disable_startup_command = true
windows = ["git"]

[[wildcard]]
pattern = "~/projects/**"
startup_command = "git status"
windows = ["git"]
`

func TestMigrateProducesEquivalentNativeConfig(t *testing.T) {
	d := t.TempDir()
	legacy := filepath.Join(d, LegacyFileName)
	mustWrite(t, legacy, migrateLegacyBody)

	gotLegacy, gotNative, err := Migrate(LoadOptions{Path: legacy}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if gotLegacy != legacy || gotNative != filepath.Join(d, NativeFileName) {
		t.Fatalf("paths = %q -> %q", gotLegacy, gotNative)
	}

	legacyCfg, _, err := Load(LoadOptions{Path: legacy, Warn: &bytes.Buffer{}})
	if err != nil {
		t.Fatal(err)
	}
	nativeCfg, _, err := Load(LoadOptions{Path: gotNative, Warn: &bytes.Buffer{}})
	if err != nil {
		t.Fatal(err)
	}
	legacyCfg.ImportPaths = nil
	if !configsEqual(legacyCfg, nativeCfg) {
		t.Fatalf("migrated config differs:\nlegacy %#v\nnative %#v", legacyCfg, nativeCfg)
	}
}

func TestMigrateFlattensImports(t *testing.T) {
	d := t.TempDir()
	mustWrite(t, filepath.Join(d, "extra.toml"), "[[session]]\nname = \"extra\"\npath = \"/extra\"\n")
	legacy := filepath.Join(d, LegacyFileName)
	mustWrite(t, legacy, "import = [\"extra.toml\"]\n[[session]]\nname = \"main\"\npath = \"/main\"\n")

	_, native, err := Migrate(LoadOptions{Path: legacy}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _, err := Load(LoadOptions{Path: native, Warn: &bytes.Buffer{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.SessionConfigs) != 2 || cfg.SessionConfigs[0].Name != "extra" || cfg.SessionConfigs[1].Name != "main" {
		t.Fatalf("flattened workspaces = %#v", cfg.SessionConfigs)
	}
}

func TestMigrateSharedSeshDirUsesFallback(t *testing.T) {
	home := t.TempDir()
	seshDir := filepath.Join(home, ".config", "sesh")
	if err := os.MkdirAll(seshDir, 0700); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(seshDir, LegacyFileName), "cache = true\n")
	fallback := filepath.Join(home, ".config", "herdr-sesh")

	_, native, err := Migrate(LoadOptions{Home: home, Env: map[string]string{}}, fallback, false)
	if err != nil {
		t.Fatal(err)
	}
	if native != filepath.Join(fallback, NativeFileName) {
		t.Fatalf("native = %q, want inside %q", native, fallback)
	}
	if _, err := os.Stat(filepath.Join(seshDir, NativeFileName)); !os.IsNotExist(err) {
		t.Fatal("wrote native file into undiscovered ~/.config/sesh")
	}
}

func migrateAndRead(t *testing.T, legacyBody string) (string, Config) {
	t.Helper()
	d := t.TempDir()
	legacy := filepath.Join(d, LegacyFileName)
	mustWrite(t, legacy, legacyBody)
	_, native, err := Migrate(LoadOptions{Path: legacy}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // native is a test-owned temporary path returned by Migrate.
	data, err := os.ReadFile(native)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _, err := Load(LoadOptions{Path: native, Warn: &bytes.Buffer{}})
	if err != nil {
		t.Fatal(err)
	}
	return string(data), cfg
}

func TestMigrateEmitsShowIconsExplicitly(t *testing.T) {
	t.Run("explicit true survives", func(t *testing.T) {
		text, cfg := migrateAndRead(t, "[tui]\nshow_icons = true\n")
		if !strings.Contains(text, "show_icons = true") || !cfg.TUI.ShowIcons {
			t.Fatalf("show_icons=true lost:\n%s", text)
		}
	})
	t.Run("explicit false survives as text", func(t *testing.T) {
		text, cfg := migrateAndRead(t, "[tui]\nshow_icons = false\n")
		if !strings.Contains(text, "show_icons = false") || cfg.TUI.ShowIcons {
			t.Fatalf("explicit show_icons=false dropped from output:\n%s", text)
		}
	})
	t.Run("absent enables icons", func(t *testing.T) {
		text, cfg := migrateAndRead(t, "cache = true\n")
		if !strings.Contains(text, "show_icons = true") || !cfg.TUI.ShowIcons {
			t.Fatalf("absent show_icons did not enable icons:\n%s", text)
		}
	})
	t.Run("explicit false in import survives", func(t *testing.T) {
		d := t.TempDir()
		mustWrite(t, filepath.Join(d, "extra.toml"), "[tui]\nshow_icons = false\n")
		legacy := filepath.Join(d, LegacyFileName)
		mustWrite(t, legacy, "import = [\"extra.toml\"]\n")
		_, native, err := Migrate(LoadOptions{Path: legacy}, "", false)
		if err != nil {
			t.Fatal(err)
		}
		cfg, _, err := Load(LoadOptions{Path: native, Warn: &bytes.Buffer{}})
		if err != nil {
			t.Fatal(err)
		}
		if cfg.TUI.ShowIcons {
			t.Fatal("imported explicit show_icons=false was overridden")
		}
	})
}

// The exact shape older releases generated via `config init`: no [tui] table
// and the former colorless default preview baked in as an explicit value.
func TestMigrateFormerGeneratedStarterShape(t *testing.T) {
	text, cfg := migrateAndRead(t, "[default_session]\npreview_command = \"eza --icons=always -la {}\"\n")
	if !strings.Contains(text, "show_icons = true") || !cfg.TUI.ShowIcons {
		t.Fatalf("icons not enabled for legacy file without show_icons:\n%s", text)
	}
	if strings.Contains(text, "preview") {
		t.Fatalf("former default preview literal was kept as custom config:\n%s", text)
	}
	if cfg.DefaultSessionConfig.PreviewCommand != DefaultPreviewCommand {
		t.Fatalf("preview = %q, want color-forced runtime default", cfg.DefaultSessionConfig.PreviewCommand)
	}
}

func TestMigrateUpgradesFormerDefaultPreviewInWorkspacesAndRules(t *testing.T) {
	_, cfg := migrateAndRead(t, `[[session]]
name = "api"
path = "/tmp/api"
preview_command = "eza --icons=always -la {}"

[[wildcard]]
pattern = "~/projects/**"
preview_command = "eza --icons=always -la {}"
`)
	if got := cfg.SessionConfigs[0].PreviewCommand; got != DefaultPreviewCommand {
		t.Fatalf("workspace preview = %q, want upgraded default", got)
	}
	if got := cfg.WildcardConfigs[0].PreviewCommand; got != DefaultPreviewCommand {
		t.Fatalf("rule preview = %q, want upgraded default", got)
	}
}

func TestMigrateLeavesDefaultPreviewToRuntime(t *testing.T) {
	text, cfg := migrateAndRead(t, "cache = true\n")
	if strings.Contains(text, "preview") {
		t.Fatalf("unset preview was baked into migrated file:\n%s", text)
	}
	if cfg.DefaultSessionConfig.PreviewCommand != DefaultPreviewCommand {
		t.Fatalf("preview = %q, want runtime default", cfg.DefaultSessionConfig.PreviewCommand)
	}
}

func TestMigrateKeepsCustomPreviewVerbatim(t *testing.T) {
	text, cfg := migrateAndRead(t, "[default_session]\npreview_command = \"printf custom {}\"\n")
	if !strings.Contains(text, "printf custom {}") || cfg.DefaultSessionConfig.PreviewCommand != "printf custom {}" {
		t.Fatalf("custom preview rewritten:\n%s", text)
	}
}

func TestMigrateErrors(t *testing.T) {
	t.Run("no config", func(t *testing.T) {
		_, _, err := Migrate(LoadOptions{Home: t.TempDir(), Env: map[string]string{}}, "", false)
		if err == nil || !strings.Contains(err.Error(), "no config file") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("already native", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), NativeFileName)
		mustWrite(t, p, "version = 1\n")
		_, _, err := Migrate(LoadOptions{Path: p}, "", false)
		if err == nil || !strings.Contains(err.Error(), "already a native config") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("target exists", func(t *testing.T) {
		d := t.TempDir()
		legacy := filepath.Join(d, LegacyFileName)
		mustWrite(t, legacy, "cache = true\n")
		mustWrite(t, filepath.Join(d, NativeFileName), "version = 1\n")
		_, _, err := Migrate(LoadOptions{Path: legacy}, "", false)
		if err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("invalid legacy value fails native validation without writing", func(t *testing.T) {
		d := t.TempDir()
		legacy := filepath.Join(d, LegacyFileName)
		mustWrite(t, legacy, "blacklist = [\"[\"]\n")
		_, _, err := Migrate(LoadOptions{Path: legacy}, "", false)
		if err == nil || !strings.Contains(err.Error(), "native validation") {
			t.Fatalf("err = %v", err)
		}
		if _, err := os.Stat(filepath.Join(d, NativeFileName)); !os.IsNotExist(err) {
			t.Fatal("wrote native file despite validation failure")
		}
	})
}

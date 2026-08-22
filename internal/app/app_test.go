package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"

	"github.com/fullerzz/herdr-plugin-sesh/internal/config"
	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/fullerzz/herdr-plugin-sesh/internal/state"
)

func TestVersionCommand(t *testing.T) {
	var out bytes.Buffer
	a := &App{Out: &out, Err: &bytes.Buffer{}}
	if err := a.Run(context.Background(), []string{"--version"}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "herdr-sesh dev" {
		t.Fatalf("got %q", out.String())
	}
}

func TestConfigPathCommand(t *testing.T) {
	d := t.TempDir()
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", d)
	t.Setenv("HERDR_SESH_CONFIG", "")
	t.Setenv("HOME", d)

	var out bytes.Buffer
	a := &App{Out: &out, Err: &bytes.Buffer{}}
	if err := a.Run(context.Background(), []string{"config", "path"}); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(d, "config.toml")
	if strings.TrimSpace(out.String()) != want {
		t.Fatalf("no-candidate path = %q, want %q", out.String(), want)
	}

	legacy := filepath.Join(d, "sesh.toml")
	if err := os.WriteFile(legacy, []byte("cache = true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := a.Run(context.Background(), []string{"config", "path"}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != legacy {
		t.Fatalf("legacy path = %q, want %q", out.String(), legacy)
	}
}

func TestConfigValidatePrintsResolvedNativePath(t *testing.T) {
	d := t.TempDir()
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", d)
	t.Setenv("HERDR_SESH_CONFIG", "")
	t.Setenv("HOME", d)
	native := filepath.Join(d, config.NativeFileName)
	if err := os.WriteFile(native, []byte("version = 1\n"), 0600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	a := &App{Out: &out, Err: &bytes.Buffer{}}
	if err := a.Run(context.Background(), []string{"config", "validate"}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != native {
		t.Fatalf("stdout = %q, want %q", out.String(), native)
	}
}

func TestConfigValidateWarnsForLegacyConfig(t *testing.T) {
	legacy := filepath.Join(t.TempDir(), config.LegacyFileName)
	if err := os.WriteFile(legacy, []byte("cache = true\n"), 0600); err != nil {
		t.Fatal(err)
	}

	var out, errb bytes.Buffer
	a := &App{Out: &out, Err: &errb}
	if err := a.Run(context.Background(), []string{"config", "validate", legacy}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != legacy {
		t.Fatalf("stdout = %q, want %q", out.String(), legacy)
	}
	if !strings.Contains(errb.String(), "deprecated Sesh-compatible schema") {
		t.Fatalf("stderr = %q, want legacy deprecation warning", errb.String())
	}
}

func TestConfigValidateRejectsUnknownLegacyKey(t *testing.T) {
	legacy := filepath.Join(t.TempDir(), config.LegacyFileName)
	if err := os.WriteFile(legacy, []byte("cahe = true\n"), 0600); err != nil {
		t.Fatal(err)
	}

	a := &App{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}
	err := a.Run(context.Background(), []string{"config", "validate", legacy})
	if err == nil || !strings.Contains(err.Error(), "cahe") {
		t.Fatalf("err = %v, want unknown legacy key", err)
	}
}

func TestConfigValidateRejectsUnknownLegacyKeyInImport(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "extra.toml"), []byte("cahe = true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(d, config.LegacyFileName)
	if err := os.WriteFile(legacy, []byte("import = [\"extra.toml\"]\n"), 0600); err != nil {
		t.Fatal(err)
	}

	a := &App{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}
	err := a.Run(context.Background(), []string{"config", "validate", legacy})
	if err == nil || !strings.Contains(err.Error(), "cahe") {
		t.Fatalf("err = %v, want unknown imported legacy key", err)
	}
}

func TestConfigValidateRejectsInvalidNativeConfig(t *testing.T) {
	native := filepath.Join(t.TempDir(), config.NativeFileName)
	if err := os.WriteFile(native, []byte("version = 1\nunknown = true\n"), 0600); err != nil {
		t.Fatal(err)
	}

	a := &App{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}
	err := a.Run(context.Background(), []string{"config", "validate", native})
	if err == nil || !strings.Contains(err.Error(), "unknown keys") {
		t.Fatalf("err = %v, want native validation error", err)
	}
}

func TestConfigValidateRequiresExistingConfig(t *testing.T) {
	d := t.TempDir()
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", d)
	t.Setenv("HERDR_SESH_CONFIG", "")
	t.Setenv("HOME", d)

	a := &App{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}
	err := a.Run(context.Background(), []string{"config", "validate"})
	if err == nil || !strings.Contains(err.Error(), "no config file found") {
		t.Fatalf("err = %v, want missing config error", err)
	}
}

func TestConfigValidateAcceptsAtMostOnePath(t *testing.T) {
	a := &App{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}
	err := a.Run(context.Background(), []string{"config", "validate", "first.toml", "second.toml"})
	if err == nil || !strings.Contains(err.Error(), "accepts at most one path") {
		t.Fatalf("err = %v, want extra path error", err)
	}
}

func TestConfigValidateRejectsEmptyPath(t *testing.T) {
	d := t.TempDir()
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", d)
	t.Setenv("HERDR_SESH_CONFIG", "")
	t.Setenv("HOME", d)

	a := &App{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}
	err := a.Run(context.Background(), []string{"config", "validate", ""})
	if err == nil || !strings.Contains(err.Error(), "path must not be empty") {
		t.Fatalf("err = %v, want empty path error", err)
	}
}

func TestConfigInitDoesNotShadowActiveLegacyConfig(t *testing.T) {
	d := t.TempDir()
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", d)
	t.Setenv("HERDR_SESH_CONFIG", "")
	t.Setenv("HOME", d)
	legacy := filepath.Join(d, "sesh.toml")
	if err := os.WriteFile(legacy, []byte("cache = true\n"), 0600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	a := &App{Out: &out, Err: &bytes.Buffer{}}
	if err := a.Run(context.Background(), []string{"config", "init"}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != legacy {
		t.Fatalf("init printed %q, want existing legacy %q", out.String(), legacy)
	}
	if _, err := os.Stat(filepath.Join(d, "config.toml")); !os.IsNotExist(err) {
		t.Fatalf("init created shadowing config.toml: %v", err)
	}
}

func TestConfigInitHonorsEnvTarget(t *testing.T) {
	d := t.TempDir()
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", d)
	t.Setenv("HOME", d)
	target := filepath.Join(d, "custom", "mine.toml")
	t.Setenv("HERDR_SESH_CONFIG", target)

	var out bytes.Buffer
	a := &App{Out: &out, Err: &bytes.Buffer{}}
	if err := a.Run(context.Background(), []string{"config", "init"}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != target {
		t.Fatalf("init printed %q, want env target %q", out.String(), target)
	}
	cfg, _, err := config.Load(config.LoadOptions{Path: target, Warn: &bytes.Buffer{}})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultSessionConfig.PreviewCommand != config.DefaultPreviewCommand {
		t.Fatalf("starter preview = %q", cfg.DefaultSessionConfig.PreviewCommand)
	}
}

func TestConfigInitRejectsDirectoryCandidate(t *testing.T) {
	d := t.TempDir()
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", d)
	t.Setenv("HERDR_SESH_CONFIG", "")
	t.Setenv("HOME", d)
	if err := os.Mkdir(filepath.Join(d, config.NativeFileName), 0700); err != nil {
		t.Fatal(err)
	}

	a := &App{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}
	err := a.Run(context.Background(), []string{"config", "init"})
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("err = %v, want non-regular config path error", err)
	}
}

func TestConfigInitPropagatesResolveError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", "")
	t.Setenv("HERDR_SESH_CONFIG", "")
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".config"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".config", "sesh"), []byte("blocked\n"), 0600); err != nil {
		t.Fatal(err)
	}

	a := &App{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}
	err := a.Run(context.Background(), []string{"config", "init"})
	if !errors.Is(err, syscall.ENOTDIR) {
		t.Fatalf("err = %v, want higher-priority filesystem error", err)
	}
	if _, statErr := os.Stat(filepath.Join(home, ".config", "herdr-sesh", config.NativeFileName)); !os.IsNotExist(statErr) {
		t.Fatalf("init created config after resolver error: %v", statErr)
	}
}

func TestConfigMigrateCommand(t *testing.T) {
	d := t.TempDir()
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", d)
	t.Setenv("HERDR_SESH_CONFIG", "")
	t.Setenv("HOME", d)
	legacy := filepath.Join(d, "sesh.toml")
	if err := os.WriteFile(legacy, []byte("cache = true\n[[session]]\nname = \"api\"\npath = \"/tmp/api\"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	var out, errb bytes.Buffer
	a := &App{Out: &out, Err: &errb}
	if err := a.Run(context.Background(), []string{"config", "migrate"}); err != nil {
		t.Fatal(err)
	}
	native := filepath.Join(d, "config.toml")
	if strings.TrimSpace(out.String()) != native {
		t.Fatalf("stdout = %q, want %q", out.String(), native)
	}
	if !strings.Contains(errb.String(), "delete the legacy file") {
		t.Fatalf("stderr = %q", errb.String())
	}
	cfg, _, err := config.Load(config.LoadOptions{Path: native, Warn: &bytes.Buffer{}})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Cache || len(cfg.SessionConfigs) != 1 || cfg.SessionConfigs[0].Name != "api" {
		t.Fatalf("migrated cfg = %#v", cfg)
	}
	// The native file now wins discovery over the untouched legacy file.
	got, err := config.ResolvePath(config.LoadOptions{})
	if err != nil || got != native {
		t.Fatalf("resolved %q err %v, want %q", got, err, native)
	}
}

func TestConfigMigrateForceOverwritesExistingNativeConfig(t *testing.T) {
	d := t.TempDir()
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", d)
	t.Setenv("HERDR_SESH_CONFIG", "")
	t.Setenv("HOME", d)
	legacy := filepath.Join(d, "sesh.toml")
	if err := os.WriteFile(legacy, []byte("cache = true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	native := filepath.Join(d, config.NativeFileName)
	if err := os.WriteFile(native, []byte("version = 1\n[list]\ncache = false\n"), 0600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	a := &App{Out: &out, Err: &bytes.Buffer{}}
	if err := a.Run(context.Background(), []string{"config", "migrate", legacy, "--force"}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != native {
		t.Fatalf("stdout = %q, want %q", out.String(), native)
	}
	cfg, _, err := config.Load(config.LoadOptions{Path: native, Warn: &bytes.Buffer{}})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Cache {
		t.Fatal("forced migration did not replace existing native config")
	}
}

func TestConfigMigrateWarnsToRepointEnvBeforeDeletingLegacy(t *testing.T) {
	d := t.TempDir()
	legacy := filepath.Join(d, config.LegacyFileName)
	native := filepath.Join(d, config.NativeFileName)
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", d)
	t.Setenv("HERDR_SESH_CONFIG", legacy)
	t.Setenv("HOME", d)
	if err := os.WriteFile(legacy, []byte("cache = true\n"), 0600); err != nil {
		t.Fatal(err)
	}

	var errb bytes.Buffer
	a := &App{Out: &bytes.Buffer{}, Err: &errb}
	if err := a.Run(context.Background(), []string{"config", "migrate"}); err != nil {
		t.Fatal(err)
	}
	warning := errb.String()
	if !strings.Contains(warning, "set HERDR_SESH_CONFIG="+native+" before deleting "+legacy) {
		t.Fatalf("stderr = %q", warning)
	}
}

func TestConfigMigrateKeepsUnrelatedEnvSelection(t *testing.T) {
	d := t.TempDir()
	active := filepath.Join(d, "active", config.LegacyFileName)
	legacy := filepath.Join(d, "other", config.LegacyFileName)
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", d)
	t.Setenv("HERDR_SESH_CONFIG", active)
	t.Setenv("HOME", d)
	for _, p := range []string{active, legacy} {
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("cache = true\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	var errb bytes.Buffer
	a := &App{Out: &bytes.Buffer{}, Err: &errb}
	if err := a.Run(context.Background(), []string{"config", "migrate", "--config", legacy}); err != nil {
		t.Fatal(err)
	}
	warning := errb.String()
	if strings.Contains(warning, "set HERDR_SESH_CONFIG=") || !strings.Contains(warning, "HERDR_SESH_CONFIG continues to select "+active) {
		t.Fatalf("stderr = %q", warning)
	}
}

func TestConfigMigrateExplicitPathWarnsWhenNativeIsNotDiscovered(t *testing.T) {
	home := t.TempDir()
	legacyDir := t.TempDir()
	legacy := filepath.Join(legacyDir, config.LegacyFileName)
	native := filepath.Join(legacyDir, config.NativeFileName)
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", filepath.Join(home, "plugin-config"))
	t.Setenv("HERDR_SESH_CONFIG", "")
	t.Setenv("HOME", home)
	if err := os.WriteFile(legacy, []byte("cache = true\n"), 0600); err != nil {
		t.Fatal(err)
	}

	var errb bytes.Buffer
	a := &App{Out: &bytes.Buffer{}, Err: &errb}
	if err := a.Run(context.Background(), []string{"config", "migrate", legacy}); err != nil {
		t.Fatal(err)
	}
	warning := errb.String()
	if strings.Contains(warning, "now takes precedence") || !strings.Contains(warning, "set HERDR_SESH_CONFIG="+native+" before deleting "+legacy) {
		t.Fatalf("stderr = %q", warning)
	}
}

func TestConfigMigrateLegacySymlinkDoesNotClaimNativePrecedence(t *testing.T) {
	home := t.TempDir()
	pluginDir := filepath.Join(home, "plugin-config")
	legacyDir := t.TempDir()
	legacy := filepath.Join(legacyDir, config.LegacyFileName)
	native := filepath.Join(legacyDir, config.NativeFileName)
	legacyLink := filepath.Join(pluginDir, config.LegacyFileName)
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", pluginDir)
	t.Setenv("HERDR_SESH_CONFIG", "")
	t.Setenv("HOME", home)
	if err := os.MkdirAll(pluginDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte("cache = true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(native, legacyLink); err != nil {
		t.Fatal(err)
	}

	var errb bytes.Buffer
	a := &App{Out: &bytes.Buffer{}, Err: &errb}
	if err := a.Run(context.Background(), []string{"config", "migrate", legacy}); err != nil {
		t.Fatal(err)
	}
	cfg, resolved, err := config.Load(config.LoadOptions{Warn: &bytes.Buffer{}})
	if err != nil {
		t.Fatal(err)
	}
	if resolved != legacyLink || cfg.Cache {
		t.Fatalf("resolved %q with cache=%t, want legacy decoding through %q", resolved, cfg.Cache, legacyLink)
	}
	warning := errb.String()
	if strings.Contains(warning, "now takes precedence") || !strings.Contains(warning, "set HERDR_SESH_CONFIG="+native) {
		t.Fatalf("stderr = %q", warning)
	}
}

func TestListIgnoresCorruptSessionCache(t *testing.T) {
	d := t.TempDir()
	cfgPath := filepath.Join(d, "sesh.toml")
	if err := os.WriteFile(cfgPath, []byte("cache = true\n[[session]]\nname = \"api\"\npath = \"/tmp/api\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(d, "state")
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "sessions.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_PLUGIN_STATE_DIR", stateDir)

	var out, errb bytes.Buffer
	a := &App{Out: &out, Err: &errb}
	if err := a.Run(context.Background(), []string{"list", "--json", "--config", cfgPath}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"name": "api"`) {
		t.Fatalf("output = %q", out.String())
	}
	if !strings.Contains(errb.String(), "warning: ignoring session cache") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestListWarnsWhenSessionCacheCannotBeSaved(t *testing.T) {
	d := t.TempDir()
	cfgPath := filepath.Join(d, "sesh.toml")
	if err := os.WriteFile(cfgPath, []byte("cache = true\n[[session]]\nname = \"api\"\npath = \"/tmp/api\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(d, "state-file")
	if err := os.WriteFile(statePath, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_PLUGIN_STATE_DIR", statePath)

	var out, errb bytes.Buffer
	a := &App{Out: &out, Err: &errb}
	if err := a.Run(context.Background(), []string{"list", "--json", "--config", cfgPath}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"name": "api"`) {
		t.Fatalf("output = %q", out.String())
	}
	if !strings.Contains(errb.String(), "warning: ignoring session cache") || !strings.Contains(errb.String(), "warning: could not save session cache") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestListCacheDoesNotMaskBlacklistedResults(t *testing.T) {
	d := t.TempDir()
	cfgPath := filepath.Join(d, "sesh.toml")
	if err := os.WriteFile(cfgPath, []byte(`cache = true
blacklist = ["^scratch$"]

[[session]]
name = "api"
path = "/tmp/api"

[[session]]
name = "scratch"
path = "/tmp/scratch"
`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_PLUGIN_STATE_DIR", filepath.Join(d, "state"))

	if got := runListJSON(t, cfgPath, ""); len(got) != 1 || got[0].Name != "api" {
		t.Fatalf("normal sessions = %#v", got)
	}
	if got := runListJSON(t, cfgPath, "", "--blacklisted"); len(got) != 1 || got[0].Name != "scratch" {
		t.Fatalf("blacklisted sessions = %#v", got)
	}
}

func TestListCacheDoesNotMaskDuplicateResults(t *testing.T) {
	d := t.TempDir()
	cfgPath := filepath.Join(d, "sesh.toml")
	if err := os.WriteFile(cfgPath, []byte(`cache = true
sort_order = ["config", "zoxide"]

[[session]]
name = "api"
path = "/configured/api"
`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_PLUGIN_STATE_DIR", filepath.Join(d, "state"))
	zoxideOutput := "42 /discovered/api\n"

	if got := runListJSON(t, cfgPath, zoxideOutput); len(got) != 1 {
		t.Fatalf("deduplicated sessions = %#v", got)
	}
	if got := runListJSON(t, cfgPath, zoxideOutput, "--hide-duplicates=false"); len(got) != 2 {
		t.Fatalf("duplicate sessions = %#v", got)
	}
}

func TestListCacheDoesNotCrossConfigFiles(t *testing.T) {
	d := t.TempDir()
	firstConfig := filepath.Join(d, "first.toml")
	secondConfig := filepath.Join(d, "second.toml")
	if err := os.WriteFile(firstConfig, []byte("cache = true\n[[session]]\nname = \"api\"\npath = \"/tmp/api\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondConfig, []byte("cache = true\n[[session]]\nname = \"web\"\npath = \"/tmp/web\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_PLUGIN_STATE_DIR", filepath.Join(d, "state"))

	if got := runListJSON(t, firstConfig, ""); len(got) != 1 || got[0].Name != "api" {
		t.Fatalf("first config sessions = %#v", got)
	}
	if got := runListJSON(t, secondConfig, ""); len(got) != 1 || got[0].Name != "web" {
		t.Fatalf("second config sessions = %#v", got)
	}
}

func TestListCacheDistinguishesRelativeConfigsAcrossWorkingDirectories(t *testing.T) {
	d := t.TempDir()
	firstDir := filepath.Join(d, "first")
	secondDir := filepath.Join(d, "second")
	for dir, name := range map[string]string{firstDir: "api", secondDir: "web"} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		body := fmt.Sprintf("cache = true\n[[session]]\nname = %q\npath = %q\n", name, filepath.Join("/tmp", name))
		if err := os.WriteFile(filepath.Join(dir, "sesh.toml"), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HERDR_PLUGIN_STATE_DIR", filepath.Join(d, "state"))

	t.Chdir(firstDir)
	if got := runListJSON(t, "sesh.toml", ""); len(got) != 1 || got[0].Name != "api" {
		t.Fatalf("first config sessions = %#v", got)
	}
	t.Chdir(secondDir)
	if got := runListJSON(t, "sesh.toml", ""); len(got) != 1 || got[0].Name != "web" {
		t.Fatalf("second config sessions = %#v", got)
	}
}

func TestPickerJSONCommand(t *testing.T) {
	var out bytes.Buffer
	a := &App{Out: &out, Err: &bytes.Buffer{}}
	if err := a.Run(context.Background(), []string{"picker", "--json", "--config", filepath.Join("..", "..", "testdata", "sesh.toml")}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"name": "sesh"`) {
		t.Fatalf("output = %q", out.String())
	}
}

func TestPickerJSONAppliesDefaultStartupCommand(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "sesh.toml")
	if err := os.WriteFile(cfgPath, []byte(`[default_session]
startup_command = "printf default:{}"

[[session]]
name = "api"
path = "/tmp/api"
`), 0600); err != nil {
		t.Fatal(err)
	}

	sessions := runPickerJSON(t, cfgPath, "")
	if len(sessions) != 1 || sessions[0].StartupCommand != "printf default:{}" {
		t.Fatalf("sessions = %#v", sessions)
	}
}

func TestPickerJSONAppliesWildcardSettings(t *testing.T) {
	project := filepath.Join(t.TempDir(), "project")
	if err := os.Mkdir(project, 0700); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(t.TempDir(), "sesh.toml")
	if err := os.WriteFile(cfgPath, []byte(`strict_mode = true

[[wildcard]]
pattern = "`+project+`"
startup_command = "printf wildcard:{}"
preview_command = "printf preview:{}"
disable_startup_command = true
windows = ["git"]

[[window]]
name = "git"
startup_script = "git status"
`), 0600); err != nil {
		t.Fatal(err)
	}

	sessions := runPickerJSON(t, cfgPath, "42 "+project+"\n")
	if len(sessions) != 1 {
		t.Fatalf("sessions = %#v", sessions)
	}
	s := sessions[0]
	if s.StartupCommand != "" || s.PreviewCommand != "printf preview:{}" || !s.DisableStartupCommand || !reflect.DeepEqual(s.WindowNames, []string{"git"}) {
		t.Fatalf("wildcard session = %#v", s)
	}
	if len(s.WindowConfigs) != 0 {
		t.Fatalf("window configs leaked into JSON: %#v", s.WindowConfigs)
	}
}

func TestPickerJSONExplicitFalseOverridesWildcardDisable(t *testing.T) {
	project := filepath.Join(t.TempDir(), "project")
	if err := os.Mkdir(project, 0700); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(t.TempDir(), "sesh.toml")
	if err := os.WriteFile(cfgPath, []byte(`[default_session]
startup_command = "printf default:{}"

[[session]]
name = "project"
path = "`+project+`"
disable_startup_command = false

[[wildcard]]
pattern = "`+project+`"
startup_command = "printf wildcard:{}"
disable_startup_command = true
`), 0600); err != nil {
		t.Fatal(err)
	}

	sessions := runPickerJSON(t, cfgPath, "")
	if len(sessions) != 1 {
		t.Fatalf("sessions = %#v", sessions)
	}
	if sessions[0].DisableStartupCommand || sessions[0].StartupCommand != "printf wildcard:{}" {
		t.Fatalf("session = %#v", sessions[0])
	}
}

func TestCollectDirectPathUsesConfiguredDirLength(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "parent")
	target := filepath.Join(parent, "child")
	if err := os.MkdirAll(target, 0700); err != nil {
		t.Fatal(err)
	}
	configureFakeSources(t, "")
	cfg := config.Default()
	cfg.DirLength = 2

	sessions, err := (&App{}).collectAllowUnavailableHerdr(context.Background(), cfg, target)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].Name != filepath.Join("parent", "child") {
		t.Fatalf("sessions = %#v", sessions)
	}
}

func TestCollectPropagatesHerdrErrors(t *testing.T) {
	configureFakeSources(t, "")

	if _, err := (&App{}).collect(context.Background(), config.Default(), ""); err == nil {
		t.Fatal("collect succeeded when Herdr workspace listing failed")
	}
}

func TestCollectPickerPreservesHerdrError(t *testing.T) {
	configureFakeSources(t, "")

	col, err := (&App{}).collectPicker(context.Background(), config.Default())
	if err != nil {
		t.Fatalf("collect picker sources: %v", err)
	}
	if col.HerdrErr == nil {
		t.Fatal("collectPicker discarded Herdr workspace listing error")
	}
	if len(col.Sessions) != 0 || col.HerdrWorkspaces != nil {
		t.Fatalf("sessions=%#v workspaces=%#v", col.Sessions, col.HerdrWorkspaces)
	}
}

func TestCollectPickerReturnsRawHerdrWorkspaces(t *testing.T) {
	configureHerdrScript(t, `#!/bin/sh
case "$1 $2" in
"workspace list") printf '[{"id":"w1","label":"api","cwd":"/live/api"}]\n' ;;
"pane list") printf '[]\n' ;;
*) exit 1 ;;
esac
`)

	col, err := (&App{}).collectPicker(context.Background(), config.Default())
	if err != nil || col.HerdrErr != nil {
		t.Fatalf("collect picker: err=%v herdrErr=%v", err, col.HerdrErr)
	}
	want := []model.Session{{Source: "herdr", Name: "api", Path: "/live/api", WorkspaceID: "w1"}}
	if !reflect.DeepEqual(col.HerdrWorkspaces, want) {
		t.Fatalf("herdr workspaces=%#v want %#v", col.HerdrWorkspaces, want)
	}
}

func TestReloadPickerStateResolvesFocusedWorkspace(t *testing.T) {
	tests := []struct {
		name        string
		paneCurrent string
		wantID      string
		wantLast    string
		wantUnknown bool
		wantWarning string
	}{
		{
			name:        "records focused workspace",
			paneCurrent: `printf '{"id":"p1","workspace_id":"focused"}\n'`,
			wantID:      "focused",
			wantLast:    "previous",
		},
		{
			name:        "clears workspace when focus fails",
			paneCurrent: "exit 1",
			wantUnknown: true,
			wantWarning: "find focused workspace after close",
		},
		{
			name:        "clears workspace when focused pane has no workspace",
			paneCurrent: `printf '{"id":"p1"}\n'`,
			wantUnknown: true,
			wantWarning: "focused pane has no workspace ID",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configureHerdrScript(t, fmt.Sprintf(`#!/bin/sh
case "$1 $2" in
"workspace list") printf '[]\n' ;;
"pane list") printf '[]\n' ;;
"pane current") %s ;;
*) exit 1 ;;
esac
`, tt.paneCurrent))
			stateDir := filepath.Join(t.TempDir(), "state")
			if err := state.SaveHistory(stateDir, state.History{Workspaces: []string{"focused", "previous"}}); err != nil {
				t.Fatal(err)
			}
			t.Setenv("HERDR_PLUGIN_STATE_DIR", stateDir)
			pickerWorkspaceID := "closed-workspace"
			var warnings []string
			warn := func(format string, args ...any) {
				warnings = append(warnings, fmt.Sprintf(format, args...))
			}

			result, err := (&App{}).reloadPickerState(context.Background(), config.Default(), herdr.NewCLIClient(), &pickerWorkspaceID, warn)
			if err != nil {
				t.Fatalf("reload picker state: %v", err)
			}
			if pickerWorkspaceID != tt.wantID {
				t.Fatalf("picker workspace=%q, want %q", pickerWorkspaceID, tt.wantID)
			}
			if result.LastWorkspaceUnknown != tt.wantUnknown {
				t.Fatalf("last workspace unknown=%v, want %v", result.LastWorkspaceUnknown, tt.wantUnknown)
			}
			if !tt.wantUnknown && result.LastWorkspaceID != tt.wantLast {
				t.Fatalf("last workspace=%q, want %q", result.LastWorkspaceID, tt.wantLast)
			}
			if tt.wantWarning == "" && len(warnings) > 0 {
				t.Fatalf("unexpected warnings: %v", warnings)
			}
			if tt.wantWarning != "" && (len(warnings) != 1 || !strings.Contains(warnings[0], tt.wantWarning)) {
				t.Fatalf("warnings=%v, want one containing %q", warnings, tt.wantWarning)
			}
		})
	}
}

func TestCollectPickerLoadsHerdrAndZoxideConcurrently(t *testing.T) {
	d := t.TempDir()
	marker := filepath.Join(d, "zoxide-started")
	herdrScript := `#!/bin/sh
if [ "$1 $2" = "workspace list" ]; then
  i=0
  while [ ! -f "$CONCURRENT_SOURCE_MARKER" ] && [ "$i" -lt 100 ]; do
    sleep 0.01
    i=$((i + 1))
  done
  [ -f "$CONCURRENT_SOURCE_MARKER" ] || exit 1
fi
printf '[]\n'
`
	zoxideScript := `#!/bin/sh
: > "$CONCURRENT_SOURCE_MARKER"
`
	for name, script := range map[string]string{"herdr": herdrScript, "zoxide": zoxideScript} {
		//nolint:gosec // test creates local executable fixtures.
		if err := os.WriteFile(filepath.Join(d, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HERDR_BIN_PATH", filepath.Join(d, "herdr"))
	t.Setenv("CONCURRENT_SOURCE_MARKER", marker)
	t.Setenv("PATH", d+string(os.PathListSeparator)+os.Getenv("PATH"))

	col, err := (&App{}).collectPicker(context.Background(), config.Default())
	if err != nil {
		t.Fatalf("collect picker sources: %v", err)
	}
	if col.HerdrErr != nil {
		t.Fatalf("Herdr ran before zoxide: %v", col.HerdrErr)
	}
}

func TestPreviewCommandUsesExplicitConfig(t *testing.T) {
	d := t.TempDir()
	targetDir := filepath.Join(d, "target")
	if err := os.Mkdir(targetDir, 0700); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(d, "sesh.toml")
	if err := os.WriteFile(cfgPath, []byte("[default_session]\npreview_command = \"printf configured:%s {}\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	fakeBin := filepath.Join(d, "bin")
	if err := os.MkdirAll(fakeBin, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"herdr", "zoxide"} {
		//nolint:gosec // test creates local executable fixtures.
		if err := os.WriteFile(filepath.Join(fakeBin, name), []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	//nolint:gosec // test creates a local executable fixture.
	if err := os.WriteFile(filepath.Join(fakeBin, "eza"), []byte("#!/bin/sh\nprintf 'default:%s\\n' \"$*\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_BIN_PATH", filepath.Join(fakeBin, "herdr"))
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	var out bytes.Buffer
	a := &App{Out: &out, Err: &bytes.Buffer{}}
	if err := a.Run(context.Background(), []string{"preview", "--config", cfgPath, targetDir}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "configured:") || !strings.Contains(out.String(), targetDir) {
		t.Fatalf("output = %q", out.String())
	}
}

func TestLastFocusesPreviousWorkspaceAndRotatesHistory(t *testing.T) {
	d := t.TempDir()
	stateDir := filepath.Join(d, "state")
	if err := state.SaveHistory(stateDir, state.History{Workspaces: []string{"current", "previous", "older"}}); err != nil {
		t.Fatal(err)
	}
	fakeHerdr := filepath.Join(d, "herdr")
	logPath := filepath.Join(d, "herdr.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" > \"$HERDR_FAKE_LOG\"\n"
	//nolint:gosec // test creates a local executable fixture.
	if err := os.WriteFile(fakeHerdr, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_BIN_PATH", fakeHerdr)
	t.Setenv("HERDR_FAKE_LOG", logPath)
	t.Setenv("HERDR_PLUGIN_STATE_DIR", stateDir)
	t.Setenv("HERDR_WORKSPACE_ID", "current")

	a := &App{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}
	if err := a.Run(context.Background(), []string{"last"}); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // logPath is a test-owned temp file.
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(log)); got != "workspace focus previous" {
		t.Fatalf("herdr args = %q", got)
	}
	h, err := state.LoadHistory(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"previous", "current", "older"}
	if !reflect.DeepEqual(h.Workspaces, want) {
		t.Fatalf("workspaces=%#v want %#v", h.Workspaces, want)
	}
}

func runPickerJSON(t *testing.T, cfgPath, zoxideOutput string) []model.Session {
	t.Helper()
	configureFakeSources(t, zoxideOutput)

	var out bytes.Buffer
	a := &App{Out: &out, Err: &bytes.Buffer{}}
	if err := a.Run(context.Background(), []string{"picker", "--json", "--config", cfgPath}); err != nil {
		t.Fatal(err)
	}
	var sessions []model.Session
	if err := json.Unmarshal(out.Bytes(), &sessions); err != nil {
		t.Fatalf("decode picker JSON: %v\n%s", err, out.String())
	}
	return sessions
}

func runListJSON(t *testing.T, cfgPath, zoxideOutput string, extraArgs ...string) []model.Session {
	t.Helper()
	configureFakeSources(t, zoxideOutput)

	args := append([]string{"list", "--json", "--config", cfgPath}, extraArgs...)
	var out bytes.Buffer
	a := &App{Out: &out, Err: &bytes.Buffer{}}
	if err := a.Run(context.Background(), args); err != nil {
		t.Fatal(err)
	}
	var sessions []model.Session
	if err := json.Unmarshal(out.Bytes(), &sessions); err != nil {
		t.Fatalf("decode list JSON: %v\n%s", err, out.String())
	}
	return sessions
}

func configureHerdrScript(t *testing.T, script string) {
	t.Helper()
	fakeBin := t.TempDir()
	//nolint:gosec // test creates local executable fixtures.
	if err := os.WriteFile(filepath.Join(fakeBin, "herdr"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // test creates local executable fixtures.
	if err := os.WriteFile(filepath.Join(fakeBin, "zoxide"), []byte("#!/bin/sh\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_BIN_PATH", filepath.Join(fakeBin, "herdr"))
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func configureFakeSources(t *testing.T, zoxideOutput string) {
	t.Helper()
	fakeBin := t.TempDir()
	for name, script := range map[string]string{
		"herdr":  "#!/bin/sh\nexit 1\n",
		"zoxide": "#!/bin/sh\nprintf '%s' \"$FAKE_ZOXIDE_OUTPUT\"\n",
	} {
		//nolint:gosec // test creates local executable fixtures.
		if err := os.WriteFile(filepath.Join(fakeBin, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HERDR_BIN_PATH", filepath.Join(fakeBin, "herdr"))
	t.Setenv("FAKE_ZOXIDE_OUTPUT", zoxideOutput)
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

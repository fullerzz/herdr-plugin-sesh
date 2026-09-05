package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/fullerzz/herdr-plugin-sesh/internal/config"
	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/fullerzz/herdr-plugin-sesh/internal/state"
	"github.com/pelletier/go-toml/v2"
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

func TestPickerOptionsPropagateWorkspaceSort(t *testing.T) {
	cfg := config.Default()
	if got := pickerOptionsFromConfig(context.Background(), io.Discard, cfg).WorkspaceSort; got != "workspace" {
		t.Fatalf("default workspace sort = %q, want workspace", got)
	}
	cfg.TUI.DefaultSort = "agent"
	if got := pickerOptionsFromConfig(context.Background(), io.Discard, cfg).WorkspaceSort; got != "agent" {
		t.Fatalf("workspace sort = %q, want agent", got)
	}
}

func TestPickerOptionsPropagateWorktreeIconReplacement(t *testing.T) {
	cfg := config.Default()
	if opts := pickerOptionsFromConfig(context.Background(), io.Discard, cfg); opts.DisableWorktreeIconReplacement {
		t.Fatal("default config disabled worktree icon replacement")
	}
	cfg.TUI.ReplaceWorktreeIcon = false
	if opts := pickerOptionsFromConfig(context.Background(), io.Discard, cfg); !opts.DisableWorktreeIconReplacement {
		t.Fatal("disabled worktree icon replacement was not propagated")
	}
}

func TestPickerOptionsPropagatePreviewVisibility(t *testing.T) {
	cfg := config.Default()
	if opts := pickerOptionsFromConfig(context.Background(), io.Discard, cfg); opts.HidePreview {
		t.Fatal("default config hid native picker preview")
	}
	cfg.TUI.ShowPreview = false
	if opts := pickerOptionsFromConfig(context.Background(), io.Discard, cfg); !opts.HidePreview {
		t.Fatal("show_preview=false was not propagated")
	}
}

func TestPickerOptionsPropagatePreviewMode(t *testing.T) {
	cfg := config.Default()
	if got := pickerOptionsFromConfig(context.Background(), io.Discard, cfg).PreviewMode; got != "command" {
		t.Fatalf("default preview mode = %q, want command", got)
	}
	cfg.TUI.PreviewMode = "pane"
	if got := pickerOptionsFromConfig(context.Background(), io.Discard, cfg).PreviewMode; got != "pane" {
		t.Fatalf("preview mode = %q, want pane", got)
	}
}

func TestPickerOptionsPropagateHomePrioritization(t *testing.T) {
	cfg := config.Default()
	if opts := pickerOptionsFromConfig(context.Background(), io.Discard, cfg); opts.DisableHomePrioritization {
		t.Fatal("default config disabled home prioritization")
	}
	cfg.TUI.PrioritizeHome = false
	if opts := pickerOptionsFromConfig(context.Background(), io.Discard, cfg); !opts.DisableHomePrioritization {
		t.Fatal("prioritize_home=false was not propagated")
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

func TestNativePickerFailsWhenWorkspaceHistoryCannotResolve(t *testing.T) {
	configureFakeSources(t, "")
	statePath := filepath.Join(t.TempDir(), "state")
	if err := os.WriteFile(statePath, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_PLUGIN_STATE_DIR", statePath)
	t.Setenv("HERDR_SOCKET_PATH", filepath.Join(t.TempDir(), "herdr", "herdr.sock"))

	err := (&App{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}).Run(
		context.Background(),
		[]string{"picker", "--config", filepath.Join("..", "..", "testdata", "sesh.toml")},
	)
	if err == nil || !strings.Contains(err.Error(), "resolve workspace history") {
		t.Fatalf("picker error=%v, want workspace history resolution failure", err)
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
			t.Setenv("HERDR_SOCKET_PATH", "")
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
	t.Setenv("HERDR_SOCKET_PATH", "")
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

func TestPluginManifestStartsHistoryWatcherForRuntimeLifecycleEvents(t *testing.T) {
	//nolint:gosec // The manifest is a repository-owned test fixture.
	b, err := os.ReadFile("../../herdr-plugin.toml")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		MinHerdrVersion string `toml:"min_herdr_version"`
		Events          []struct {
			On      string   `toml:"on"`
			Command []string `toml:"command"`
		} `toml:"events"`
	}
	if err := toml.Unmarshal(b, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.MinHerdrVersion != "0.8.2" {
		t.Fatalf("min_herdr_version=%q, want 0.8.2", manifest.MinHerdrVersion)
	}
	wantCommand := []string{"./bin/herdr-sesh", "plugin", "watch-history"}
	got := make(map[string][]string, len(manifest.Events))
	for _, event := range manifest.Events {
		got[event.On] = event.Command
	}
	for _, event := range []string{"workspace.focused", "workspace.closed"} {
		if !reflect.DeepEqual(got[event], wantCommand) {
			t.Errorf("%s command=%#v want %#v", event, got[event], wantCommand)
		}
	}
}

func TestApplyHistoryHookLeavesFocusedEventsToSubscriber(t *testing.T) {
	historyDir := t.TempDir()
	want := state.History{Workspaces: []string{"current", "older"}}
	if err := state.SaveHistory(historyDir, want); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_PLUGIN_EVENT", "workspace.focused")
	t.Setenv("HERDR_WORKSPACE_ID", "late-hook")

	err := applyHistoryHook(historyDir)
	if err != nil {
		t.Fatal(err)
	}
	history, err := state.LoadHistory(historyDir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(history, want) {
		t.Fatalf("history=%#v want %#v", history, want)
	}
}

func TestApplyHistoryHookUsesClosedEventPayloadWorkspace(t *testing.T) {
	historyDir := t.TempDir()
	if err := state.SaveHistory(historyDir, state.History{Workspaces: []string{"context", "closed", "older"}}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_PLUGIN_EVENT", "workspace.closed")
	t.Setenv("HERDR_WORKSPACE_ID", "context")
	t.Setenv("HERDR_PLUGIN_EVENT_JSON", `{"data":{"workspace_id":"closed"}}`)

	if err := applyHistoryHook(historyDir); err != nil {
		t.Fatal(err)
	}
	history, err := state.LoadHistory(historyDir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"context", "older"}
	if !reflect.DeepEqual(history.Workspaces, want) {
		t.Fatalf("workspaces=%#v want %#v", history.Workspaces, want)
	}
}

func TestPluginWatchHistoryBoundsClosedHookLockWait(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	socketPath := filepath.Join(t.TempDir(), "herdr.sock")
	historyDir, err := state.SessionHistoryDir(stateDir, socketPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.SaveHistory(historyDir, state.History{Workspaces: []string{"closed"}}); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // Test lock path is derived from t.TempDir().
	lock, err := os.OpenFile(filepath.Join(historyDir, "history.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() })
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) })

	t.Setenv("HERDR_PLUGIN_STATE_DIR", stateDir)
	t.Setenv("HERDR_SOCKET_PATH", socketPath)
	t.Setenv("HERDR_PLUGIN_EVENT", "workspace.closed")
	t.Setenv("HERDR_PLUGIN_EVENT_JSON", `{"data":{"workspace_id":"closed"}}`)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err = (&App{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}).Run(ctx, []string{"plugin", "watch-history"})
	if !errors.Is(err, state.ErrHistoryLockTimeout) {
		t.Fatalf("watch error=%v, want history lock timeout", err)
	}
}

func TestPluginWatchHistoryAppliesClosedHookBeforeNoHistoryStream(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	socketDir, err := os.MkdirTemp("/tmp", "herdr-sesh-hook-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })
	socketPath := filepath.Join(socketDir, "herdr.sock")
	historyDir, err := state.SessionHistoryDir(stateDir, socketPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.SaveHistory(historyDir, state.History{Workspaces: []string{"closed", "other"}}); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	streamReady := make(chan struct{})
	releaseServer := make(chan struct{})
	serverDone := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer func() { _ = conn.Close() }()
		var request struct {
			Method string `json:"method"`
		}
		if err := json.NewDecoder(conn).Decode(&request); err != nil {
			serverDone <- err
			return
		}
		if request.Method != "events.subscribe" {
			serverDone <- fmt.Errorf("method=%q", request.Method)
			return
		}
		enc := json.NewEncoder(conn)
		if err := enc.Encode(map[string]any{"id": "herdr-sesh-history", "result": map[string]any{"type": "subscription_started"}}); err != nil {
			serverDone <- err
			return
		}
		if err := serveHistoryWatcherPing(listener); err != nil {
			serverDone <- err
			return
		}
		snapshotConn, err := listener.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		if err := json.NewDecoder(snapshotConn).Decode(&request); err != nil {
			_ = snapshotConn.Close()
			serverDone <- err
			return
		}
		if err := json.NewEncoder(snapshotConn).Encode(map[string]any{
			"id":     "herdr-sesh-history-snapshot",
			"result": map[string]any{"type": "session_snapshot", "snapshot": map[string]any{"focused_workspace_id": ""}},
		}); err != nil {
			_ = snapshotConn.Close()
			serverDone <- err
			return
		}
		_ = snapshotConn.Close()
		// A duplicate live close event remains harmless.
		if err := enc.Encode(map[string]any{"event": "workspace_closed", "data": map[string]any{"workspace_id": "closed"}}); err != nil {
			serverDone <- err
			return
		}
		close(streamReady)
		<-releaseServer
		serverDone <- nil
	}()

	t.Setenv("HERDR_PLUGIN_STATE_DIR", stateDir)
	t.Setenv("HERDR_SOCKET_PATH", socketPath)
	t.Setenv("HERDR_PLUGIN_EVENT", "workspace.closed")
	t.Setenv("HERDR_WORKSPACE_ID", "")
	t.Setenv("HERDR_PLUGIN_EVENT_JSON", `{"data":{"workspace_id":"closed"}}`)
	ctx, cancel := context.WithCancel(context.Background())
	watchDone := make(chan error, 1)
	go func() {
		a := &App{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}
		watchDone <- a.Run(ctx, []string{"plugin", "watch-history"})
	}()
	select {
	case <-streamReady:
	case <-time.After(time.Second):
		t.Fatal("watcher did not bootstrap")
	}

	deadline := time.Now().Add(time.Second)
	for {
		history, loadErr := state.LoadHistory(historyDir)
		if loadErr != nil {
			t.Fatal(loadErr)
		}
		if reflect.DeepEqual(history.Workspaces, []string{"other"}) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("history after hook and duplicate replay=%#v", history.Workspaces)
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	close(releaseServer)
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
	if err := <-watchDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("watch error=%v, want context cancellation", err)
	}
}

func TestPluginWatchHistoryNonWinningFocusDoesNotMigrateLegacyHistory(t *testing.T) {
	root := t.TempDir()
	stateDir := filepath.Join(root, "state")
	socketPath := filepath.Join(root, "herdr", "herdr.sock")
	if err := state.SaveHistory(stateDir, state.History{Workspaces: []string{"legacy"}}); err != nil {
		t.Fatal(err)
	}
	release, acquired, err := state.TryHistoryWatcherLock(stateDir, socketPath)
	if err != nil {
		t.Fatal(err)
	}
	if !acquired {
		t.Fatal("failed to hold watcher election lock")
	}
	t.Cleanup(func() {
		if err := release(); err != nil {
			t.Error(err)
		}
	})

	t.Setenv("HERDR_PLUGIN_STATE_DIR", stateDir)
	t.Setenv("HERDR_SOCKET_PATH", socketPath)
	t.Setenv("HERDR_PLUGIN_EVENT", "workspace.focused")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := (&App{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}).Run(ctx, []string{"plugin", "watch-history"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "history")); !os.IsNotExist(err) {
		t.Fatalf("non-winning focus hook migrated legacy history: %v", err)
	}
}

func TestPluginWatchHistoryReturnsWhenWatcherAlreadyRunning(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	socketDir, err := os.MkdirTemp("/tmp", "herdr-sesh-watch-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })
	socketPath := filepath.Join(socketDir, "herdr.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	watcherReady := make(chan struct{})
	releaseServer := make(chan struct{})
	serverDone := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer func() { _ = conn.Close() }()
		var request struct {
			Method string `json:"method"`
		}
		if err := json.NewDecoder(conn).Decode(&request); err != nil {
			serverDone <- err
			return
		}
		if request.Method != "events.subscribe" {
			serverDone <- fmt.Errorf("method=%q", request.Method)
			return
		}
		if err := json.NewEncoder(conn).Encode(map[string]any{
			"id": "herdr-sesh-history", "result": map[string]any{"type": "subscription_started"},
		}); err != nil {
			serverDone <- err
			return
		}
		if err := serveHistoryWatcherPing(listener); err != nil {
			serverDone <- err
			return
		}

		snapshotConn, err := listener.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer func() { _ = snapshotConn.Close() }()
		if err := json.NewDecoder(snapshotConn).Decode(&request); err != nil {
			serverDone <- err
			return
		}
		if request.Method != "session.snapshot" {
			serverDone <- fmt.Errorf("snapshot method=%q", request.Method)
			return
		}
		if err := json.NewEncoder(snapshotConn).Encode(map[string]any{
			"id": "herdr-sesh-history-snapshot",
			"result": map[string]any{
				"type": "session_snapshot", "snapshot": map[string]any{"focused_workspace_id": ""},
			},
		}); err != nil {
			serverDone <- err
			return
		}
		close(watcherReady)
		<-releaseServer
		serverDone <- nil
	}()

	t.Setenv("HERDR_PLUGIN_STATE_DIR", stateDir)
	t.Setenv("HERDR_SOCKET_PATH", socketPath)
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	defer cancelFirst()
	firstDone := make(chan error, 1)
	go func() {
		a := &App{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}
		firstDone <- a.Run(firstCtx, []string{"plugin", "watch-history"})
	}()
	select {
	case <-watcherReady:
	case <-time.After(2 * time.Second):
		t.Fatal("first watcher did not subscribe")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	a := &App{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}
	secondErr := a.Run(ctx, []string{"plugin", "watch-history"})
	cancelFirst()
	close(releaseServer)
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
	if err := <-firstDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("first watcher error=%v, want context cancellation", err)
	}
	if secondErr != nil {
		t.Fatalf("second watcher returned %v, want a successful no-op", secondErr)
	}
}

func TestRetryHistoryMutationPreservesOrderAfterLockTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	blocked := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	firstAttempt := true
	var got []string

	go func() {
		for _, workspaceID := range []string{"B", "C", "D"} {
			err := retryHistoryMutation(ctx, func() error {
				if workspaceID == "B" && firstAttempt {
					firstAttempt = false
					close(blocked)
					select {
					case <-release:
						return fmt.Errorf("contended: %w", state.ErrHistoryLockTimeout)
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				got = append(got, workspaceID)
				return nil
			})
			if err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()

	select {
	case <-blocked:
	case <-ctx.Done():
		t.Fatal("history mutation did not reach lock contention")
	}
	if len(got) != 0 {
		t.Fatalf("later mutations overtook the contended mutation: %#v", got)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if want := []string{"B", "C", "D"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("mutations=%#v want %#v", got, want)
	}
}

func TestPluginWatchHistoryPreservesFocusOrderThroughLockContention(t *testing.T) {
	historyDir := filepath.Join(t.TempDir(), "history")
	if err := state.SaveHistory(historyDir, state.History{Workspaces: []string{"A", "closed"}}); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // Test lock path is derived from t.TempDir().
	lock, err := os.OpenFile(filepath.Join(historyDir, "history.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	locked := true
	defer func() {
		if locked {
			_ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
		}
	}()

	socketDir, err := os.MkdirTemp("/tmp", "herdr-sesh-watch-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })
	socketPath := filepath.Join(socketDir, "herdr.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	releaseServer := make(chan struct{})
	serverDone := make(chan error, 1)
	go func() { serverDone <- serveContendedHistoryStream(listener, releaseServer) }()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	lockTimedOut := make(chan struct{}, 1)
	retry := func(mutate func() error) error {
		return retryHistoryMutation(ctx, func() error {
			err := mutate()
			if errors.Is(err, state.ErrHistoryLockTimeout) {
				select {
				case lockTimedOut <- struct{}{}:
				default:
				}
			}
			return err
		})
	}
	watchDone := make(chan error, 1)
	go func() {
		watchDone <- herdr.WatchWorkspaceEvents(ctx, socketPath,
			func(workspaceID string) error {
				return retry(func() error { return state.Record(historyDir, workspaceID) })
			},
			func(workspaceID string) error {
				return retry(func() error { return state.RemoveWorkspace(historyDir, workspaceID) })
			},
		)
	}()

	select {
	case <-lockTimedOut:
	case <-ctx.Done():
		t.Fatal("watcher did not retry the contended snapshot mutation")
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	locked = false

	want := []string{"D", "C", "B", "A"}
	for {
		history, err := state.LoadHistory(historyDir)
		if err != nil {
			t.Fatal(err)
		}
		if reflect.DeepEqual(history.Workspaces, want) {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("history=%#v want %#v", history.Workspaces, want)
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	close(releaseServer)
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
	if err := <-watchDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("watch error=%v want context cancellation", err)
	}
}

func serveContendedHistoryStream(listener net.Listener, release <-chan struct{}) error {
	stream, err := listener.Accept()
	if err != nil {
		return err
	}
	defer func() { _ = stream.Close() }()
	var request struct {
		Method string `json:"method"`
	}
	if err := json.NewDecoder(stream).Decode(&request); err != nil {
		return err
	}
	if request.Method != "events.subscribe" {
		return fmt.Errorf("method=%q want events.subscribe", request.Method)
	}
	enc := json.NewEncoder(stream)
	if err := enc.Encode(map[string]any{
		"id": "herdr-sesh-history", "result": map[string]any{"type": "subscription_started"},
	}); err != nil {
		return err
	}
	if err := serveHistoryWatcherPing(listener); err != nil {
		return err
	}
	snapshot, err := listener.Accept()
	if err != nil {
		return err
	}
	defer func() { _ = snapshot.Close() }()
	if err := json.NewDecoder(snapshot).Decode(&request); err != nil {
		return err
	}
	if request.Method != "session.snapshot" {
		return fmt.Errorf("method=%q want session.snapshot", request.Method)
	}
	for _, workspaceID := range []string{"C", "D"} {
		if err := enc.Encode(map[string]any{
			"event": "workspace_focused", "data": map[string]any{"workspace_id": workspaceID},
		}); err != nil {
			return err
		}
	}
	if err := json.NewEncoder(snapshot).Encode(map[string]any{
		"id": "herdr-sesh-history-snapshot",
		"result": map[string]any{
			"type": "session_snapshot", "snapshot": map[string]any{"focused_workspace_id": "B"},
		},
	}); err != nil {
		return err
	}
	if err := enc.Encode(map[string]any{
		"event": "workspace_closed", "data": map[string]any{"workspace_id": "closed"},
	}); err != nil {
		return err
	}
	<-release
	return nil
}

func serveHistoryWatcherPing(listener net.Listener) error {
	conn, err := listener.Accept()
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	var request struct {
		Method string `json:"method"`
	}
	if err := json.NewDecoder(conn).Decode(&request); err != nil {
		return err
	}
	if request.Method != "ping" {
		return fmt.Errorf("method=%q want ping", request.Method)
	}
	return json.NewEncoder(conn).Encode(map[string]any{
		"id": "herdr-sesh-history-ping",
		"result": map[string]any{
			"type": "pong", "version": "0.8.2-preview", "protocol": 21,
		},
	})
}

func TestPluginOpenPickerStillOpensPickerPane(t *testing.T) {
	d := t.TempDir()
	logPath := filepath.Join(d, "herdr.log")
	fakeHerdr := filepath.Join(d, "herdr")
	//nolint:gosec // test creates a local executable fixture.
	if err := os.WriteFile(fakeHerdr, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" > \"$HERDR_FAKE_LOG\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_BIN_PATH", fakeHerdr)
	t.Setenv("HERDR_FAKE_LOG", logPath)

	a := &App{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}
	if err := a.Run(context.Background(), []string{"plugin", "open-picker"}); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // logPath is a test-owned temp file.
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(log)); got != "plugin pane open --plugin fullerzz.sesh --entrypoint picker --placement overlay" {
		t.Fatalf("herdr args=%q", got)
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

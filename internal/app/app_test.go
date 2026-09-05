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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVersionCommand(t *testing.T) {
	var out bytes.Buffer
	a := &App{Out: &out, Err: &bytes.Buffer{}}
	require.NoError(t, a.Run(context.Background(), []string{"--version"}))
	assert.Equal(t, "herdr-sesh dev", strings.TrimSpace(out.String()))
}

func TestConfigPathCommand(t *testing.T) {
	d := t.TempDir()
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", d)
	t.Setenv("HERDR_SESH_CONFIG", "")
	t.Setenv("HOME", d)

	var out bytes.Buffer
	a := &App{Out: &out, Err: &bytes.Buffer{}}
	require.NoError(t, a.Run(context.Background(), []string{"config", "path"}))
	want := filepath.Join(d, "config.toml")
	require.Equal(t, want, strings.TrimSpace(out.String()))

	legacy := filepath.Join(d, "sesh.toml")
	require.NoError(t, os.WriteFile(legacy, []byte("cache = true\n"), 0600))
	out.Reset()
	require.NoError(t, a.Run(context.Background(), []string{"config", "path"}))
	assert.Equal(t, legacy, strings.TrimSpace(out.String()))
}

func TestConfigValidatePrintsResolvedNativePath(t *testing.T) {
	d := t.TempDir()
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", d)
	t.Setenv("HERDR_SESH_CONFIG", "")
	t.Setenv("HOME", d)
	native := filepath.Join(d, config.NativeFileName)
	require.NoError(t, os.WriteFile(native, []byte("version = 1\n"), 0600))

	var out bytes.Buffer
	a := &App{Out: &out, Err: &bytes.Buffer{}}
	require.NoError(t, a.Run(context.Background(), []string{"config", "validate"}))
	assert.Equal(t, native, strings.TrimSpace(out.String()))
}

func TestConfigValidateWarnsForLegacyConfig(t *testing.T) {
	legacy := filepath.Join(t.TempDir(), config.LegacyFileName)
	require.NoError(t, os.WriteFile(legacy, []byte("cache = true\n"), 0600))

	var out, errb bytes.Buffer
	a := &App{Out: &out, Err: &errb}
	require.NoError(t, a.Run(context.Background(), []string{"config", "validate", legacy}))
	require.Equal(t, legacy, strings.TrimSpace(out.String()))
	assert.Contains(t, errb.String(), "deprecated Sesh-compatible schema")
}

func TestConfigValidateRejectsUnknownLegacyKey(t *testing.T) {
	legacy := filepath.Join(t.TempDir(), config.LegacyFileName)
	require.NoError(t, os.WriteFile(legacy, []byte("cahe = true\n"), 0600))

	a := &App{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}
	err := a.Run(context.Background(), []string{"config", "validate", legacy})
	require.ErrorContains(t, err, "cahe")
}

func TestConfigValidateRejectsUnknownLegacyKeyInImport(t *testing.T) {
	d := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(d, "extra.toml"), []byte("cahe = true\n"), 0600))
	legacy := filepath.Join(d, config.LegacyFileName)
	require.NoError(t, os.WriteFile(legacy, []byte("import = [\"extra.toml\"]\n"), 0600))

	a := &App{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}
	err := a.Run(context.Background(), []string{"config", "validate", legacy})
	require.ErrorContains(t, err, "cahe")
}

func TestConfigValidateRejectsInvalidNativeConfig(t *testing.T) {
	native := filepath.Join(t.TempDir(), config.NativeFileName)
	require.NoError(t, os.WriteFile(native, []byte("version = 1\nunknown = true\n"), 0600))

	a := &App{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}
	err := a.Run(context.Background(), []string{"config", "validate", native})
	require.ErrorContains(t, err, "unknown keys")
}

func TestConfigValidateRequiresExistingConfig(t *testing.T) {
	d := t.TempDir()
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", d)
	t.Setenv("HERDR_SESH_CONFIG", "")
	t.Setenv("HOME", d)

	a := &App{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}
	err := a.Run(context.Background(), []string{"config", "validate"})
	require.ErrorContains(t, err, "no config file found")
}

func TestConfigValidateAcceptsAtMostOnePath(t *testing.T) {
	a := &App{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}
	err := a.Run(context.Background(), []string{"config", "validate", "first.toml", "second.toml"})
	require.ErrorContains(t, err, "accepts at most one path")
}

func TestConfigValidateRejectsEmptyPath(t *testing.T) {
	d := t.TempDir()
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", d)
	t.Setenv("HERDR_SESH_CONFIG", "")
	t.Setenv("HOME", d)

	a := &App{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}
	err := a.Run(context.Background(), []string{"config", "validate", ""})
	require.ErrorContains(t, err, "path must not be empty")
}

func TestConfigInitDoesNotShadowActiveLegacyConfig(t *testing.T) {
	d := t.TempDir()
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", d)
	t.Setenv("HERDR_SESH_CONFIG", "")
	t.Setenv("HOME", d)
	legacy := filepath.Join(d, "sesh.toml")
	require.NoError(t, os.WriteFile(legacy, []byte("cache = true\n"), 0600))

	var out bytes.Buffer
	a := &App{Out: &out, Err: &bytes.Buffer{}}
	require.NoError(t, a.Run(context.Background(), []string{"config", "init"}))
	require.Equal(t, legacy, strings.TrimSpace(out.String()))
	_, err := os.Stat(filepath.Join(d, "config.toml"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestConfigInitHonorsEnvTarget(t *testing.T) {
	d := t.TempDir()
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", d)
	t.Setenv("HOME", d)
	target := filepath.Join(d, "custom", "mine.toml")
	t.Setenv("HERDR_SESH_CONFIG", target)

	var out bytes.Buffer
	a := &App{Out: &out, Err: &bytes.Buffer{}}
	require.NoError(t, a.Run(context.Background(), []string{"config", "init"}))
	require.Equal(t, target, strings.TrimSpace(out.String()))
	cfg, _, err := config.Load(config.LoadOptions{Path: target, Warn: &bytes.Buffer{}})
	require.NoError(t, err)
	assert.Equal(t, config.DefaultPreviewCommand, cfg.DefaultSessionConfig.PreviewCommand)
}

func TestConfigInitRejectsDirectoryCandidate(t *testing.T) {
	d := t.TempDir()
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", d)
	t.Setenv("HERDR_SESH_CONFIG", "")
	t.Setenv("HOME", d)
	require.NoError(t, os.Mkdir(filepath.Join(d, config.NativeFileName), 0700))

	a := &App{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}
	err := a.Run(context.Background(), []string{"config", "init"})
	require.ErrorContains(t, err, "not a regular file")
}

func TestConfigInitPropagatesResolveError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", "")
	t.Setenv("HERDR_SESH_CONFIG", "")
	t.Setenv("HOME", home)
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".config"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".config", "sesh"), []byte("blocked\n"), 0600))

	a := &App{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}
	err := a.Run(context.Background(), []string{"config", "init"})
	require.ErrorIs(t, err, syscall.ENOTDIR)
	_, statErr := os.Stat(filepath.Join(home, ".config", "herdr-sesh", config.NativeFileName))
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestConfigMigrateCommand(t *testing.T) {
	d := t.TempDir()
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", d)
	t.Setenv("HERDR_SESH_CONFIG", "")
	t.Setenv("HOME", d)
	legacy := filepath.Join(d, "sesh.toml")
	require.NoError(t, os.WriteFile(legacy, []byte("cache = true\n[[session]]\nname = \"api\"\npath = \"/tmp/api\"\n"), 0600))

	var out, errb bytes.Buffer
	a := &App{Out: &out, Err: &errb}
	require.NoError(t, a.Run(context.Background(), []string{"config", "migrate"}))
	native := filepath.Join(d, "config.toml")
	require.Equal(t, native, strings.TrimSpace(out.String()))
	require.Contains(t, errb.String(), "delete the legacy file")
	cfg, _, err := config.Load(config.LoadOptions{Path: native, Warn: &bytes.Buffer{}})
	require.NoError(t, err)
	require.True(t, cfg.Cache)
	require.Len(t, cfg.SessionConfigs, 1)
	require.Equal(t, "api", cfg.SessionConfigs[0].Name)
	// The native file now wins discovery over the untouched legacy file.
	got, err := config.ResolvePath(config.LoadOptions{})
	require.NoError(t, err)
	assert.Equal(t, native, got)
}

func TestConfigMigrateForceOverwritesExistingNativeConfig(t *testing.T) {
	d := t.TempDir()
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", d)
	t.Setenv("HERDR_SESH_CONFIG", "")
	t.Setenv("HOME", d)
	legacy := filepath.Join(d, "sesh.toml")
	require.NoError(t, os.WriteFile(legacy, []byte("cache = true\n"), 0600))
	native := filepath.Join(d, config.NativeFileName)
	require.NoError(t, os.WriteFile(native, []byte("version = 1\n[list]\ncache = false\n"), 0600))

	var out bytes.Buffer
	a := &App{Out: &out, Err: &bytes.Buffer{}}
	require.NoError(t, a.Run(context.Background(), []string{"config", "migrate", legacy, "--force"}))
	require.Equal(t, native, strings.TrimSpace(out.String()))
	cfg, _, err := config.Load(config.LoadOptions{Path: native, Warn: &bytes.Buffer{}})
	require.NoError(t, err)
	assert.True(t, cfg.Cache, "forced migration did not replace existing native config")
}

func TestConfigMigrateWarnsToRepointEnvBeforeDeletingLegacy(t *testing.T) {
	d := t.TempDir()
	legacy := filepath.Join(d, config.LegacyFileName)
	native := filepath.Join(d, config.NativeFileName)
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", d)
	t.Setenv("HERDR_SESH_CONFIG", legacy)
	t.Setenv("HOME", d)
	require.NoError(t, os.WriteFile(legacy, []byte("cache = true\n"), 0600))

	var errb bytes.Buffer
	a := &App{Out: &bytes.Buffer{}, Err: &errb}
	require.NoError(t, a.Run(context.Background(), []string{"config", "migrate"}))
	warning := errb.String()
	assert.Contains(t, warning, "set HERDR_SESH_CONFIG="+native+" before deleting "+legacy)
}

func TestConfigMigrateKeepsUnrelatedEnvSelection(t *testing.T) {
	d := t.TempDir()
	active := filepath.Join(d, "active", config.LegacyFileName)
	legacy := filepath.Join(d, "other", config.LegacyFileName)
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", d)
	t.Setenv("HERDR_SESH_CONFIG", active)
	t.Setenv("HOME", d)
	for _, p := range []string{active, legacy} {
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0700))
		require.NoError(t, os.WriteFile(p, []byte("cache = true\n"), 0600))
	}

	var errb bytes.Buffer
	a := &App{Out: &bytes.Buffer{}, Err: &errb}
	require.NoError(t, a.Run(context.Background(), []string{"config", "migrate", "--config", legacy}))
	warning := errb.String()
	require.NotContains(t, warning, "set HERDR_SESH_CONFIG=")
	assert.Contains(t, warning, "HERDR_SESH_CONFIG continues to select "+active)
}

func TestConfigMigrateExplicitPathWarnsWhenNativeIsNotDiscovered(t *testing.T) {
	home := t.TempDir()
	legacyDir := t.TempDir()
	legacy := filepath.Join(legacyDir, config.LegacyFileName)
	native := filepath.Join(legacyDir, config.NativeFileName)
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", filepath.Join(home, "plugin-config"))
	t.Setenv("HERDR_SESH_CONFIG", "")
	t.Setenv("HOME", home)
	require.NoError(t, os.WriteFile(legacy, []byte("cache = true\n"), 0600))

	var errb bytes.Buffer
	a := &App{Out: &bytes.Buffer{}, Err: &errb}
	require.NoError(t, a.Run(context.Background(), []string{"config", "migrate", legacy}))
	warning := errb.String()
	require.NotContains(t, warning, "now takes precedence")
	assert.Contains(t, warning, "set HERDR_SESH_CONFIG="+native+" before deleting "+legacy)
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
	require.NoError(t, os.MkdirAll(pluginDir, 0700))
	require.NoError(t, os.WriteFile(legacy, []byte("cache = true\n"), 0600))
	require.NoError(t, os.Symlink(native, legacyLink))

	var errb bytes.Buffer
	a := &App{Out: &bytes.Buffer{}, Err: &errb}
	require.NoError(t, a.Run(context.Background(), []string{"config", "migrate", legacy}))
	cfg, resolved, err := config.Load(config.LoadOptions{Warn: &bytes.Buffer{}})
	require.NoError(t, err)
	require.Equal(t, legacyLink, resolved)
	require.False(t, cfg.Cache)
	warning := errb.String()
	require.NotContains(t, warning, "now takes precedence")
	assert.Contains(t, warning, "set HERDR_SESH_CONFIG="+native)
}

func TestListIgnoresCorruptSessionCache(t *testing.T) {
	d := t.TempDir()
	cfgPath := filepath.Join(d, "sesh.toml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("cache = true\n[[session]]\nname = \"api\"\npath = \"/tmp/api\"\n"), 0600))
	stateDir := filepath.Join(d, "state")
	require.NoError(t, os.MkdirAll(stateDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, "sessions.json"), []byte("{"), 0600))
	t.Setenv("HERDR_PLUGIN_STATE_DIR", stateDir)

	var out, errb bytes.Buffer
	a := &App{Out: &out, Err: &errb}
	require.NoError(t, a.Run(context.Background(), []string{"list", "--json", "--config", cfgPath}))
	require.Contains(t, out.String(), `"name": "api"`)
	assert.Contains(t, errb.String(), "warning: ignoring session cache")
}

func TestListWarnsWhenSessionCacheCannotBeSaved(t *testing.T) {
	d := t.TempDir()
	cfgPath := filepath.Join(d, "sesh.toml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("cache = true\n[[session]]\nname = \"api\"\npath = \"/tmp/api\"\n"), 0600))
	statePath := filepath.Join(d, "state-file")
	require.NoError(t, os.WriteFile(statePath, []byte("not a directory"), 0600))
	t.Setenv("HERDR_PLUGIN_STATE_DIR", statePath)

	var out, errb bytes.Buffer
	a := &App{Out: &out, Err: &errb}
	require.NoError(t, a.Run(context.Background(), []string{"list", "--json", "--config", cfgPath}))
	require.Contains(t, out.String(), `"name": "api"`)
	require.Contains(t, errb.String(), "warning: ignoring session cache")
	assert.Contains(t, errb.String(), "warning: could not save session cache")
}

func TestListCacheDoesNotMaskBlacklistedResults(t *testing.T) {
	d := t.TempDir()
	cfgPath := filepath.Join(d, "sesh.toml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(`cache = true
blacklist = ["^scratch$"]

[[session]]
name = "api"
path = "/tmp/api"

[[session]]
name = "scratch"
path = "/tmp/scratch"
`), 0600))
	t.Setenv("HERDR_PLUGIN_STATE_DIR", filepath.Join(d, "state"))

	got := runListJSON(t, cfgPath, "")
	require.Len(t, got, 1)
	assert.Equal(t, "api", got[0].Name)
	got = runListJSON(t, cfgPath, "", "--blacklisted")
	require.Len(t, got, 1)
	assert.Equal(t, "scratch", got[0].Name)
}

func TestListCacheDoesNotMaskDuplicateResults(t *testing.T) {
	d := t.TempDir()
	cfgPath := filepath.Join(d, "sesh.toml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(`cache = true
sort_order = ["config", "zoxide"]

[[session]]
name = "api"
path = "/configured/api"
`), 0600))
	t.Setenv("HERDR_PLUGIN_STATE_DIR", filepath.Join(d, "state"))
	zoxideOutput := "42 /discovered/api\n"

	require.Len(t, runListJSON(t, cfgPath, zoxideOutput), 1)
	require.Len(t, runListJSON(t, cfgPath, zoxideOutput, "--hide-duplicates=false"), 2)
}

func TestListCacheDoesNotCrossConfigFiles(t *testing.T) {
	d := t.TempDir()
	firstConfig := filepath.Join(d, "first.toml")
	secondConfig := filepath.Join(d, "second.toml")
	require.NoError(t, os.WriteFile(firstConfig, []byte("cache = true\n[[session]]\nname = \"api\"\npath = \"/tmp/api\"\n"), 0600))
	require.NoError(t, os.WriteFile(secondConfig, []byte("cache = true\n[[session]]\nname = \"web\"\npath = \"/tmp/web\"\n"), 0600))
	t.Setenv("HERDR_PLUGIN_STATE_DIR", filepath.Join(d, "state"))

	got := runListJSON(t, firstConfig, "")
	require.Len(t, got, 1)
	assert.Equal(t, "api", got[0].Name)
	got = runListJSON(t, secondConfig, "")
	require.Len(t, got, 1)
	assert.Equal(t, "web", got[0].Name)
}

func TestListCacheDistinguishesRelativeConfigsAcrossWorkingDirectories(t *testing.T) {
	d := t.TempDir()
	firstDir := filepath.Join(d, "first")
	secondDir := filepath.Join(d, "second")
	for dir, name := range map[string]string{firstDir: "api", secondDir: "web"} {
		require.NoError(t, os.Mkdir(dir, 0700))
		body := fmt.Sprintf("cache = true\n[[session]]\nname = %q\npath = %q\n", name, filepath.Join("/tmp", name))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "sesh.toml"), []byte(body), 0600))
	}
	t.Setenv("HERDR_PLUGIN_STATE_DIR", filepath.Join(d, "state"))

	t.Chdir(firstDir)
	got := runListJSON(t, "sesh.toml", "")
	require.Len(t, got, 1)
	assert.Equal(t, "api", got[0].Name)
	t.Chdir(secondDir)
	got = runListJSON(t, "sesh.toml", "")
	require.Len(t, got, 1)
	assert.Equal(t, "web", got[0].Name)
}

func TestPickerOptionsPropagateWorkspaceSort(t *testing.T) {
	cfg := config.Default()
	require.Equal(t, "workspace", pickerOptionsFromConfig(context.Background(), io.Discard, cfg).WorkspaceSort)
	cfg.TUI.DefaultSort = "agent"
	assert.Equal(t, "agent", pickerOptionsFromConfig(context.Background(), io.Discard, cfg).WorkspaceSort)
}

func TestPickerOptionsPropagateWorktreeIconReplacement(t *testing.T) {
	cfg := config.Default()
	require.False(t, pickerOptionsFromConfig(context.Background(), io.Discard, cfg).DisableWorktreeIconReplacement, "default config disabled worktree icon replacement")
	cfg.TUI.ReplaceWorktreeIcon = false
	assert.True(t, pickerOptionsFromConfig(context.Background(), io.Discard, cfg).DisableWorktreeIconReplacement, "disabled worktree icon replacement was not propagated")
}

func TestPickerOptionsPropagatePreviewVisibility(t *testing.T) {
	cfg := config.Default()
	require.False(t, pickerOptionsFromConfig(context.Background(), io.Discard, cfg).HidePreview, "default config hid native picker preview")
	cfg.TUI.ShowPreview = false
	assert.True(t, pickerOptionsFromConfig(context.Background(), io.Discard, cfg).HidePreview, "show_preview=false was not propagated")
}

func TestPickerOptionsPropagatePreviewMode(t *testing.T) {
	cfg := config.Default()
	require.Equal(t, "command", pickerOptionsFromConfig(context.Background(), io.Discard, cfg).PreviewMode)
	cfg.TUI.PreviewMode = "pane"
	assert.Equal(t, "pane", pickerOptionsFromConfig(context.Background(), io.Discard, cfg).PreviewMode)
}

func TestPickerOptionsPropagateHomePrioritization(t *testing.T) {
	cfg := config.Default()
	require.False(t, pickerOptionsFromConfig(context.Background(), io.Discard, cfg).DisableHomePrioritization, "default config disabled home prioritization")
	cfg.TUI.PrioritizeHome = false
	assert.True(t, pickerOptionsFromConfig(context.Background(), io.Discard, cfg).DisableHomePrioritization, "prioritize_home=false was not propagated")
}

func TestPickerJSONCommand(t *testing.T) {
	var out bytes.Buffer
	a := &App{Out: &out, Err: &bytes.Buffer{}}
	require.NoError(t, a.Run(context.Background(), []string{"picker", "--json", "--config", filepath.Join("..", "..", "testdata", "sesh.toml")}))
	assert.Contains(t, out.String(), `"name": "sesh"`)
}

func TestNativePickerFailsWhenWorkspaceHistoryCannotResolve(t *testing.T) {
	configureFakeSources(t, "")
	statePath := filepath.Join(t.TempDir(), "state")
	require.NoError(t, os.WriteFile(statePath, []byte("not a directory"), 0600))
	t.Setenv("HERDR_PLUGIN_STATE_DIR", statePath)
	t.Setenv("HERDR_SOCKET_PATH", filepath.Join(t.TempDir(), "herdr", "herdr.sock"))

	err := (&App{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}).Run(
		context.Background(),
		[]string{"picker", "--config", filepath.Join("..", "..", "testdata", "sesh.toml")},
	)
	require.ErrorContains(t, err, "resolve workspace history")
}

func TestPickerJSONAppliesDefaultStartupCommand(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "sesh.toml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(`[default_session]
startup_command = "printf default:{}"

[[session]]
name = "api"
path = "/tmp/api"
`), 0600))

	sessions := runPickerJSON(t, cfgPath, "")
	require.Len(t, sessions, 1)
	assert.Equal(t, "printf default:{}", sessions[0].StartupCommand)
}

func TestPickerJSONAppliesWildcardSettings(t *testing.T) {
	project := filepath.Join(t.TempDir(), "project")
	require.NoError(t, os.Mkdir(project, 0700))
	cfgPath := filepath.Join(t.TempDir(), "sesh.toml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(`strict_mode = true

[[wildcard]]
pattern = "`+project+`"
startup_command = "printf wildcard:{}"
preview_command = "printf preview:{}"
disable_startup_command = true
windows = ["git"]

[[window]]
name = "git"
startup_script = "git status"
`), 0600))

	sessions := runPickerJSON(t, cfgPath, "42 "+project+"\n")
	require.Len(t, sessions, 1)
	s := sessions[0]
	require.Empty(t, s.StartupCommand)
	require.Equal(t, "printf preview:{}", s.PreviewCommand)
	require.True(t, s.DisableStartupCommand)
	require.Equal(t, []string{"git"}, s.WindowNames)
	assert.Empty(t, s.WindowConfigs)
}

func TestPickerJSONExplicitFalseOverridesWildcardDisable(t *testing.T) {
	project := filepath.Join(t.TempDir(), "project")
	require.NoError(t, os.Mkdir(project, 0700))
	cfgPath := filepath.Join(t.TempDir(), "sesh.toml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(`[default_session]
startup_command = "printf default:{}"

[[session]]
name = "project"
path = "`+project+`"
disable_startup_command = false

[[wildcard]]
pattern = "`+project+`"
startup_command = "printf wildcard:{}"
disable_startup_command = true
`), 0600))

	sessions := runPickerJSON(t, cfgPath, "")
	require.Len(t, sessions, 1)
	require.False(t, sessions[0].DisableStartupCommand)
	assert.Equal(t, "printf wildcard:{}", sessions[0].StartupCommand)
}

func TestCollectDirectPathUsesConfiguredDirLength(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "parent")
	target := filepath.Join(parent, "child")
	require.NoError(t, os.MkdirAll(target, 0700))
	configureFakeSources(t, "")
	cfg := config.Default()
	cfg.DirLength = 2

	sessions, err := (&App{}).collectAllowUnavailableHerdr(context.Background(), cfg, target)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Equal(t, filepath.Join("parent", "child"), sessions[0].Name)
}

func TestCollectPropagatesHerdrErrors(t *testing.T) {
	configureFakeSources(t, "")

	_, err := (&App{}).collect(context.Background(), config.Default(), "")
	require.Error(t, err)
}

func TestCollectPickerPreservesHerdrError(t *testing.T) {
	configureFakeSources(t, "")

	col, err := (&App{}).collectPicker(context.Background(), config.Default())
	require.NoError(t, err)
	require.Error(t, col.HerdrErr)
	require.Empty(t, col.Sessions)
	require.Nil(t, col.HerdrWorkspaces)
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
	require.NoError(t, err)
	require.NoError(t, col.HerdrErr)
	want := []model.Session{{Source: "herdr", Name: "api", Path: "/live/api", WorkspaceID: "w1"}}
	assert.Equal(t, want, col.HerdrWorkspaces)
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
			require.NoError(t, state.SaveHistory(stateDir, state.History{Workspaces: []string{"focused", "previous"}}))
			t.Setenv("HERDR_PLUGIN_STATE_DIR", stateDir)
			t.Setenv("HERDR_SOCKET_PATH", "")
			pickerWorkspaceID := "closed-workspace"
			var warnings []string
			warn := func(format string, args ...any) {
				warnings = append(warnings, fmt.Sprintf(format, args...))
			}

			result, err := (&App{}).reloadPickerState(context.Background(), config.Default(), herdr.NewCLIClient(), &pickerWorkspaceID, warn)
			require.NoError(t, err)
			require.Equal(t, tt.wantID, pickerWorkspaceID)
			require.Equal(t, tt.wantUnknown, result.LastWorkspaceUnknown)
			if !tt.wantUnknown {
				assert.Equal(t, tt.wantLast, result.LastWorkspaceID)
			}
			if tt.wantWarning == "" {
				assert.Empty(t, warnings)
			} else {
				require.Len(t, warnings, 1)
				assert.Contains(t, warnings[0], tt.wantWarning)
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
		require.NoError(t, os.WriteFile(filepath.Join(d, name), []byte(script), 0700))
	}
	t.Setenv("HERDR_BIN_PATH", filepath.Join(d, "herdr"))
	t.Setenv("CONCURRENT_SOURCE_MARKER", marker)
	t.Setenv("PATH", d+string(os.PathListSeparator)+os.Getenv("PATH"))

	col, err := (&App{}).collectPicker(context.Background(), config.Default())
	require.NoError(t, err)
	require.NoError(t, col.HerdrErr)
}

func TestPreviewCommandUsesExplicitConfig(t *testing.T) {
	d := t.TempDir()
	targetDir := filepath.Join(d, "target")
	require.NoError(t, os.Mkdir(targetDir, 0700))
	cfgPath := filepath.Join(d, "sesh.toml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("[default_session]\npreview_command = \"printf configured:%s {}\"\n"), 0600))
	fakeBin := filepath.Join(d, "bin")
	require.NoError(t, os.MkdirAll(fakeBin, 0700))
	for _, name := range []string{"herdr", "zoxide"} {
		//nolint:gosec // test creates local executable fixtures.
		require.NoError(t, os.WriteFile(filepath.Join(fakeBin, name), []byte("#!/bin/sh\nexit 1\n"), 0700))
	}
	//nolint:gosec // test creates a local executable fixture.
	require.NoError(t, os.WriteFile(filepath.Join(fakeBin, "eza"), []byte("#!/bin/sh\nprintf 'default:%s\\n' \"$*\"\n"), 0700))
	t.Setenv("HERDR_BIN_PATH", filepath.Join(fakeBin, "herdr"))
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	var out bytes.Buffer
	a := &App{Out: &out, Err: &bytes.Buffer{}}
	require.NoError(t, a.Run(context.Background(), []string{"preview", "--config", cfgPath, targetDir}))
	require.Contains(t, out.String(), "configured:")
	assert.Contains(t, out.String(), targetDir)
}

func TestLastFocusesPreviousWorkspaceAndRotatesHistory(t *testing.T) {
	d := t.TempDir()
	stateDir := filepath.Join(d, "state")
	require.NoError(t, state.SaveHistory(stateDir, state.History{Workspaces: []string{"current", "previous", "older"}}))
	fakeHerdr := filepath.Join(d, "herdr")
	logPath := filepath.Join(d, "herdr.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" > \"$HERDR_FAKE_LOG\"\n"
	//nolint:gosec // test creates a local executable fixture.
	require.NoError(t, os.WriteFile(fakeHerdr, []byte(script), 0700))
	t.Setenv("HERDR_BIN_PATH", fakeHerdr)
	t.Setenv("HERDR_FAKE_LOG", logPath)
	t.Setenv("HERDR_PLUGIN_STATE_DIR", stateDir)
	t.Setenv("HERDR_SOCKET_PATH", "")
	t.Setenv("HERDR_WORKSPACE_ID", "current")

	a := &App{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}
	require.NoError(t, a.Run(context.Background(), []string{"last"}))
	//nolint:gosec // logPath is a test-owned temp file.
	log, err := os.ReadFile(logPath)
	require.NoError(t, err)
	require.Equal(t, "workspace focus previous", strings.TrimSpace(string(log)))
	h, err := state.LoadHistory(stateDir)
	require.NoError(t, err)
	want := []string{"previous", "current", "older"}
	assert.Equal(t, want, h.Workspaces)
}

func TestPluginManifestStartsHistoryWatcherForRuntimeLifecycleEvents(t *testing.T) {
	//nolint:gosec // The manifest is a repository-owned test fixture.
	b, err := os.ReadFile("../../herdr-plugin.toml")
	require.NoError(t, err)
	var manifest struct {
		MinHerdrVersion string `toml:"min_herdr_version"`
		Events          []struct {
			On      string   `toml:"on"`
			Command []string `toml:"command"`
		} `toml:"events"`
	}
	require.NoError(t, toml.Unmarshal(b, &manifest))
	require.Equal(t, "0.8.2", manifest.MinHerdrVersion)
	wantCommand := []string{"./bin/herdr-sesh", "plugin", "watch-history"}
	got := make(map[string][]string, len(manifest.Events))
	for _, event := range manifest.Events {
		got[event.On] = event.Command
	}
	for _, event := range []string{"workspace.focused", "workspace.closed"} {
		assert.Equal(t, wantCommand, got[event])
	}
}

func TestApplyHistoryHookLeavesFocusedEventsToSubscriber(t *testing.T) {
	historyDir := t.TempDir()
	want := state.History{Workspaces: []string{"current", "older"}}
	require.NoError(t, state.SaveHistory(historyDir, want))
	t.Setenv("HERDR_PLUGIN_EVENT", "workspace.focused")
	t.Setenv("HERDR_WORKSPACE_ID", "late-hook")

	err := applyHistoryHook(historyDir)
	require.NoError(t, err)
	history, err := state.LoadHistory(historyDir)
	require.NoError(t, err)
	assert.Equal(t, want, history)
}

func TestApplyHistoryHookUsesClosedEventPayloadWorkspace(t *testing.T) {
	historyDir := t.TempDir()
	require.NoError(t, state.SaveHistory(historyDir, state.History{Workspaces: []string{"context", "closed", "older"}}))
	t.Setenv("HERDR_PLUGIN_EVENT", "workspace.closed")
	t.Setenv("HERDR_WORKSPACE_ID", "context")
	t.Setenv("HERDR_PLUGIN_EVENT_JSON", `{"data":{"workspace_id":"closed"}}`)

	require.NoError(t, applyHistoryHook(historyDir))
	history, err := state.LoadHistory(historyDir)
	require.NoError(t, err)
	want := []string{"context", "older"}
	assert.Equal(t, want, history.Workspaces)
}

func TestPluginWatchHistoryBoundsClosedHookLockWait(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	socketPath := filepath.Join(t.TempDir(), "herdr.sock")
	historyDir, err := state.SessionHistoryDir(stateDir, socketPath)
	require.NoError(t, err)
	require.NoError(t, state.SaveHistory(historyDir, state.History{Workspaces: []string{"closed"}}))
	//nolint:gosec // Test lock path is derived from t.TempDir().
	lock, err := os.OpenFile(filepath.Join(historyDir, "history.lock"), os.O_CREATE|os.O_RDWR, 0600)
	require.NoError(t, err)
	t.Cleanup(func() { _ = lock.Close() })
	require.NoError(t, syscall.Flock(int(lock.Fd()), syscall.LOCK_EX))
	t.Cleanup(func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) })

	t.Setenv("HERDR_PLUGIN_STATE_DIR", stateDir)
	t.Setenv("HERDR_SOCKET_PATH", socketPath)
	t.Setenv("HERDR_PLUGIN_EVENT", "workspace.closed")
	t.Setenv("HERDR_PLUGIN_EVENT_JSON", `{"data":{"workspace_id":"closed"}}`)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err = (&App{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}).Run(ctx, []string{"plugin", "watch-history"})
	require.ErrorIs(t, err, state.ErrHistoryLockTimeout)
}

func TestPluginWatchHistoryAppliesClosedHookBeforeNoHistoryStream(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	socketDir, err := os.MkdirTemp("/tmp", "herdr-sesh-hook-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })
	socketPath := filepath.Join(socketDir, "herdr.sock")
	historyDir, err := state.SessionHistoryDir(stateDir, socketPath)
	require.NoError(t, err)
	require.NoError(t, state.SaveHistory(historyDir, state.History{Workspaces: []string{"closed", "other"}}))
	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
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
		require.FailNow(t, "watcher did not bootstrap")
	}

	deadline := time.Now().Add(time.Second)
	for {
		history, loadErr := state.LoadHistory(historyDir)
		require.NoError(t, loadErr)
		if reflect.DeepEqual(history.Workspaces, []string{"other"}) {
			break
		}
		require.False(t, time.Now().After(deadline))
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	close(releaseServer)
	require.NoError(t, <-serverDone)
	require.ErrorIs(t, <-watchDone, context.Canceled)
}

func TestPluginWatchHistoryNonWinningFocusDoesNotMigrateLegacyHistory(t *testing.T) {
	root := t.TempDir()
	stateDir := filepath.Join(root, "state")
	socketPath := filepath.Join(root, "herdr", "herdr.sock")
	require.NoError(t, state.SaveHistory(stateDir, state.History{Workspaces: []string{"legacy"}}))
	release, acquired, err := state.TryHistoryWatcherLock(stateDir, socketPath)
	require.NoError(t, err)
	require.True(t, acquired, "failed to hold watcher election lock")
	t.Cleanup(func() {
		assert.NoError(t, release())
	})

	t.Setenv("HERDR_PLUGIN_STATE_DIR", stateDir)
	t.Setenv("HERDR_SOCKET_PATH", socketPath)
	t.Setenv("HERDR_PLUGIN_EVENT", "workspace.focused")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, (&App{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}).Run(ctx, []string{"plugin", "watch-history"}))
	_, err = os.Stat(filepath.Join(stateDir, "history"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestPluginWatchHistoryReturnsWhenWatcherAlreadyRunning(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	socketDir, err := os.MkdirTemp("/tmp", "herdr-sesh-watch-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })
	socketPath := filepath.Join(socketDir, "herdr.sock")
	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
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
		require.FailNow(t, "first watcher did not subscribe")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	a := &App{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}
	secondErr := a.Run(ctx, []string{"plugin", "watch-history"})
	cancelFirst()
	close(releaseServer)
	require.NoError(t, <-serverDone)
	require.ErrorIs(t, <-firstDone, context.Canceled)
	require.NoError(t, secondErr)
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
		require.FailNow(t, "history mutation did not reach lock contention")
	}
	require.Empty(t, got)
	close(release)
	require.NoError(t, <-done)
	assert.Equal(t, []string{"B", "C", "D"}, got)
}

func TestPluginWatchHistoryPreservesFocusOrderThroughLockContention(t *testing.T) {
	historyDir := filepath.Join(t.TempDir(), "history")
	require.NoError(t, state.SaveHistory(historyDir, state.History{Workspaces: []string{"A", "closed"}}))
	//nolint:gosec // Test lock path is derived from t.TempDir().
	lock, err := os.OpenFile(filepath.Join(historyDir, "history.lock"), os.O_CREATE|os.O_RDWR, 0600)
	require.NoError(t, err)
	defer func() { _ = lock.Close() }()
	require.NoError(t, syscall.Flock(int(lock.Fd()), syscall.LOCK_EX))
	locked := true
	defer func() {
		if locked {
			_ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
		}
	}()

	socketDir, err := os.MkdirTemp("/tmp", "herdr-sesh-watch-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })
	socketPath := filepath.Join(socketDir, "herdr.sock")
	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
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
		require.FailNow(t, "watcher did not retry the contended snapshot mutation")
	}
	require.NoError(t, syscall.Flock(int(lock.Fd()), syscall.LOCK_UN))
	locked = false

	want := []string{"D", "C", "B", "A"}
	for {
		history, err := state.LoadHistory(historyDir)
		require.NoError(t, err)
		if reflect.DeepEqual(history.Workspaces, want) {
			break
		}
		select {
		case <-ctx.Done():
			require.FailNow(t, fmt.Sprintf("history=%#v want %#v", history.Workspaces, want))
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	close(releaseServer)
	require.NoError(t, <-serverDone)
	require.ErrorIs(t, <-watchDone, context.Canceled)
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
	require.NoError(t, os.WriteFile(fakeHerdr, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" > \"$HERDR_FAKE_LOG\"\n"), 0700))
	t.Setenv("HERDR_BIN_PATH", fakeHerdr)
	t.Setenv("HERDR_FAKE_LOG", logPath)

	a := &App{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}
	require.NoError(t, a.Run(context.Background(), []string{"plugin", "open-picker"}))
	//nolint:gosec // logPath is a test-owned temp file.
	log, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.Equal(t, "plugin pane open --plugin fullerzz.sesh --entrypoint picker --placement overlay", strings.TrimSpace(string(log)))
}

func runPickerJSON(t *testing.T, cfgPath, zoxideOutput string) []model.Session {
	t.Helper()
	configureFakeSources(t, zoxideOutput)

	var out bytes.Buffer
	a := &App{Out: &out, Err: &bytes.Buffer{}}
	require.NoError(t, a.Run(context.Background(), []string{"picker", "--json", "--config", cfgPath}))
	var sessions []model.Session
	require.NoError(t, json.Unmarshal(out.Bytes(), &sessions))
	return sessions
}

func runListJSON(t *testing.T, cfgPath, zoxideOutput string, extraArgs ...string) []model.Session {
	t.Helper()
	configureFakeSources(t, zoxideOutput)

	args := append([]string{"list", "--json", "--config", cfgPath}, extraArgs...)
	var out bytes.Buffer
	a := &App{Out: &out, Err: &bytes.Buffer{}}
	require.NoError(t, a.Run(context.Background(), args))
	var sessions []model.Session
	require.NoError(t, json.Unmarshal(out.Bytes(), &sessions))
	return sessions
}

func configureHerdrScript(t *testing.T, script string) {
	t.Helper()
	fakeBin := t.TempDir()
	//nolint:gosec // test creates local executable fixtures.
	require.NoError(t, os.WriteFile(filepath.Join(fakeBin, "herdr"), []byte(script), 0700))
	//nolint:gosec // test creates local executable fixtures.
	require.NoError(t, os.WriteFile(filepath.Join(fakeBin, "zoxide"), []byte("#!/bin/sh\n"), 0700))
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
		require.NoError(t, os.WriteFile(filepath.Join(fakeBin, name), []byte(script), 0700))
	}
	t.Setenv("HERDR_BIN_PATH", filepath.Join(fakeBin, "herdr"))
	t.Setenv("FAKE_ZOXIDE_OUTPUT", zoxideOutput)
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestPickerOptionsPropagateCyclePreviewModeKey(t *testing.T) {
	for _, binding := range []string{"ctrl+o", "alt+p", ""} {
		cfg := config.Default()
		cfg.Keys.CyclePreviewMode = binding
		opts := pickerOptionsFromConfig(context.Background(), io.Discard, cfg)
		require.NotNil(t, opts.CyclePreviewModeKey)
		assert.Equal(t, binding, *opts.CyclePreviewModeKey)
	}
}

func TestPickerOptionsPropagatePathVisibility(t *testing.T) {
	cfg := config.Default()
	require.False(t, pickerOptionsFromConfig(context.Background(), io.Discard, cfg).HidePath)
	cfg.TUI.ShowPath = false
	assert.True(t, pickerOptionsFromConfig(context.Background(), io.Discard, cfg).HidePath)
}

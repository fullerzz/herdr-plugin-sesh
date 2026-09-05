package config

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, err)
	require.Equal(t, legacy, gotLegacy)
	require.Equal(t, filepath.Join(d, NativeFileName), gotNative)

	legacyCfg, _, err := Load(LoadOptions{Path: legacy, Warn: &bytes.Buffer{}})
	require.NoError(t, err)
	nativeCfg, _, err := Load(LoadOptions{Path: gotNative, Warn: &bytes.Buffer{}})
	require.NoError(t, err)
	legacyCfg.ImportPaths = nil
	assert.Equal(t, legacyCfg, nativeCfg)
}

func TestMigrateFlattensImports(t *testing.T) {
	d := t.TempDir()
	mustWrite(t, filepath.Join(d, "extra.toml"), "[[session]]\nname = \"extra\"\npath = \"/extra\"\n")
	legacy := filepath.Join(d, LegacyFileName)
	mustWrite(t, legacy, "import = [\"extra.toml\"]\n[[session]]\nname = \"main\"\npath = \"/main\"\n")

	_, native, err := Migrate(LoadOptions{Path: legacy}, "", false)
	require.NoError(t, err)
	cfg, _, err := Load(LoadOptions{Path: native, Warn: &bytes.Buffer{}})
	require.NoError(t, err)
	require.Len(t, cfg.SessionConfigs, 2)
	require.Equal(t, "extra", cfg.SessionConfigs[0].Name)
	assert.Equal(t, "main", cfg.SessionConfigs[1].Name)
}

func TestMigrateSharedSeshDirUsesFallback(t *testing.T) {
	home := t.TempDir()
	seshDir := filepath.Join(home, ".config", "sesh")
	require.NoError(t, os.MkdirAll(seshDir, 0700))
	mustWrite(t, filepath.Join(seshDir, LegacyFileName), "cache = true\n")
	fallback := filepath.Join(home, ".config", "herdr-sesh")

	_, native, err := Migrate(LoadOptions{Home: home, Env: map[string]string{}}, fallback, false)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(fallback, NativeFileName), native)
	_, err = os.Stat(filepath.Join(seshDir, NativeFileName))
	require.ErrorIs(t, err, os.ErrNotExist, "wrote native file into undiscovered ~/.config/sesh")
}

func TestMigrateRelativeSharedSeshPathUsesFallback(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	seshDir := filepath.Join(home, ".config", "sesh")
	require.NoError(t, os.MkdirAll(seshDir, 0700))
	legacy := filepath.Join(seshDir, LegacyFileName)
	mustWrite(t, legacy, "cache = true\n")
	fallback := filepath.Join(home, ".config", "herdr-sesh")

	oldWD, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(home))
	t.Cleanup(func() {
		assert.NoError(t, os.Chdir(oldWD))
	})

	relativeLegacy := filepath.Join(".config", "sesh", LegacyFileName)
	wantLegacy, err := filepath.Abs(relativeLegacy)
	require.NoError(t, err)
	gotLegacy, native, err := Migrate(LoadOptions{Path: relativeLegacy, Home: home}, fallback, false)
	require.NoError(t, err)
	require.Equal(t, wantLegacy, gotLegacy)
	require.Equal(t, filepath.Join(fallback, NativeFileName), native)
	resolved, err := ResolvePath(LoadOptions{Home: home, Env: map[string]string{}})
	require.NoError(t, err)
	assert.Equal(t, native, resolved)
}

func migrateAndRead(t *testing.T, legacyBody string) (string, Config) {
	t.Helper()
	d := t.TempDir()
	legacy := filepath.Join(d, LegacyFileName)
	mustWrite(t, legacy, legacyBody)
	_, native, err := Migrate(LoadOptions{Path: legacy}, "", false)
	require.NoError(t, err)
	//nolint:gosec // native is a test-owned temporary path returned by Migrate.
	data, err := os.ReadFile(native)
	require.NoError(t, err)
	cfg, _, err := Load(LoadOptions{Path: native, Warn: &bytes.Buffer{}})
	require.NoError(t, err)
	return string(data), cfg
}

func TestMigratePreservesAgentWorkspaceSort(t *testing.T) {
	text, cfg := migrateAndRead(t, "[tui]\ndefault_sort = \"agent\"\n")
	require.Contains(t, text, "workspace_sort")
	assert.Equal(t, "agent", cfg.TUI.DefaultSort)
}

func TestMigrateEmitsShowIconsExplicitly(t *testing.T) {
	t.Run("explicit true survives", func(t *testing.T) {
		text, cfg := migrateAndRead(t, "[tui]\nshow_icons = true\n")
		require.Contains(t, text, "show_icons = true")
		assert.True(t, cfg.TUI.ShowIcons)
	})
	t.Run("explicit false survives as text", func(t *testing.T) {
		text, cfg := migrateAndRead(t, "[tui]\nshow_icons = false\n")
		require.Contains(t, text, "show_icons = false")
		assert.False(t, cfg.TUI.ShowIcons)
	})
	t.Run("absent enables icons", func(t *testing.T) {
		text, cfg := migrateAndRead(t, "cache = true\n")
		require.Contains(t, text, "show_icons = true")
		assert.True(t, cfg.TUI.ShowIcons)
	})
	t.Run("explicit false in import survives", func(t *testing.T) {
		d := t.TempDir()
		mustWrite(t, filepath.Join(d, "extra.toml"), "[tui]\nshow_icons = false\n")
		legacy := filepath.Join(d, LegacyFileName)
		mustWrite(t, legacy, "import = [\"extra.toml\"]\n")
		_, native, err := Migrate(LoadOptions{Path: legacy}, "", false)
		require.NoError(t, err)
		cfg, _, err := Load(LoadOptions{Path: native, Warn: &bytes.Buffer{}})
		require.NoError(t, err)
		assert.False(t, cfg.TUI.ShowIcons, "imported explicit show_icons=false was overridden")
	})
}

func TestMigratePreservesDisabledHerdrThemeInheritance(t *testing.T) {
	text, cfg := migrateAndRead(t, "[tui]\nherdr_theme_inherit = false\n")
	require.Contains(t, text, "herdr_theme_inherit = false")
	assert.False(t, cfg.TUI.HerdrThemeInherit, "migrated herdr_theme_inherit = true, want false")
}

func TestMigratePreservesWorktreeIconReplacement(t *testing.T) {
	t.Run("default enabled", func(t *testing.T) {
		text, cfg := migrateAndRead(t, "cache = true\n")
		require.Contains(t, text, "replace_worktree_icon = true")
		assert.True(t, cfg.TUI.ReplaceWorktreeIcon)
	})
	t.Run("explicit false", func(t *testing.T) {
		text, cfg := migrateAndRead(t, "[tui]\nreplace_worktree_icon = false\n")
		require.Contains(t, text, "replace_worktree_icon = false")
		assert.False(t, cfg.TUI.ReplaceWorktreeIcon)
	})
}

func TestMigratePreservesLastWorkspacePickerSettings(t *testing.T) {
	text, cfg := migrateAndRead(t, "[tui]\nshow_last_workspace = false\nshow_last_workspace_path = false\n")
	require.Contains(t, text, "show_last_workspace = false")
	require.Contains(t, text, "show_last_workspace_path = false")
	require.False(t, cfg.TUI.ShowLastWorkspace)
	assert.False(t, cfg.TUI.ShowLastWorkspacePath)
}

// The exact shape older releases generated via `config init`: no [tui] table
// and the former colorless default preview baked in as an explicit value.
func TestMigrateFormerGeneratedStarterShape(t *testing.T) {
	text, cfg := migrateAndRead(t, "[default_session]\npreview_command = \"eza --icons=always -la {}\"\n")
	require.Contains(t, text, "show_icons = true")
	require.True(t, cfg.TUI.ShowIcons)
	require.NotContains(t, text, "preview")
	assert.Equal(t, DefaultPreviewCommand, cfg.DefaultSessionConfig.PreviewCommand)
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
	require.Equal(t, DefaultPreviewCommand, cfg.SessionConfigs[0].PreviewCommand)
	assert.Equal(t, DefaultPreviewCommand, cfg.WildcardConfigs[0].PreviewCommand)
}

func TestMigrateLeavesDefaultPreviewToRuntime(t *testing.T) {
	text, cfg := migrateAndRead(t, "cache = true\n")
	require.NotContains(t, text, "preview")
	assert.Equal(t, DefaultPreviewCommand, cfg.DefaultSessionConfig.PreviewCommand)
}

func TestMigrateLeavesListAndNamingDefaultsToRuntime(t *testing.T) {
	text, cfg := migrateAndRead(t, "cache = true\n")
	require.NotContains(t, text, "source_order")
	require.NotContains(t, text, "path_components")
	defaults := Default()
	require.True(t, cfg.Cache)
	require.True(t, slices.Equal(cfg.SortOrder, defaults.SortOrder))
	assert.Equal(t, defaults.DirLength, cfg.DirLength)
}

func TestMigrateKeepsCustomPreviewVerbatim(t *testing.T) {
	text, cfg := migrateAndRead(t, "[default_session]\npreview_command = \"printf custom {}\"\n")
	require.Contains(t, text, "printf custom {}")
	assert.Equal(t, "printf custom {}", cfg.DefaultSessionConfig.PreviewCommand)
}

func TestMigrateForceRejectsUnrelatedTarget(t *testing.T) {
	d := t.TempDir()
	legacy := filepath.Join(d, LegacyFileName)
	target := filepath.Join(d, NativeFileName)
	mustWrite(t, legacy, "cache = true\n")
	const unrelated = "not a herdr-sesh config\n"
	//nolint:gosec // an existing non-private target verifies forced migration rejects it unchanged.
	require.NoError(t, os.WriteFile(target, []byte(unrelated), 0644))

	_, _, err := Migrate(LoadOptions{Path: legacy}, "", true)
	require.ErrorContains(t, err, "not a native config")
	//nolint:gosec // target is a test-owned temporary path.
	got, readErr := os.ReadFile(target)
	require.NoError(t, readErr)
	assert.Equal(t, unrelated, string(got))
}

func TestMigrateForceReplacesNativeTargetWithPrivateMode(t *testing.T) {
	d := t.TempDir()
	legacy := filepath.Join(d, LegacyFileName)
	target := filepath.Join(d, NativeFileName)
	mustWrite(t, legacy, "cache = true\n")
	//nolint:gosec // an existing non-private target verifies migration replaces its mode with 0600.
	require.NoError(t, os.WriteFile(target, []byte("version = 1\n"), 0644))

	_, _, err := Migrate(LoadOptions{Path: legacy}, "", true)
	require.NoError(t, err)
	info, err := os.Stat(target)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
	cfg, _, err := Load(LoadOptions{Path: target, Warn: &bytes.Buffer{}})
	require.NoError(t, err)
	assert.True(t, cfg.Cache, "forced migration did not replace native target")
}

func TestMigrateErrors(t *testing.T) {
	t.Run("no config", func(t *testing.T) {
		_, _, err := Migrate(LoadOptions{Home: t.TempDir(), Env: map[string]string{}}, "", false)
		require.ErrorContains(t, err, "no config file")
	})
	t.Run("already native", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), NativeFileName)
		mustWrite(t, p, "version = 1\n")
		_, _, err := Migrate(LoadOptions{Path: p}, "", false)
		require.ErrorContains(t, err, "already a native config")
	})
	t.Run("target exists", func(t *testing.T) {
		d := t.TempDir()
		legacy := filepath.Join(d, LegacyFileName)
		mustWrite(t, legacy, "cache = true\n")
		mustWrite(t, filepath.Join(d, NativeFileName), "version = 1\n")
		_, _, err := Migrate(LoadOptions{Path: legacy}, "", false)
		require.ErrorContains(t, err, "refusing to overwrite")
	})
	t.Run("forced target cannot replace source", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), NativeFileName)
		const body = "cache = true\n"
		mustWrite(t, p, body)
		_, _, err := Migrate(LoadOptions{Path: p}, "", true)
		require.ErrorContains(t, err, "same file")
		//nolint:gosec // p is a test-owned temporary path.
		got, readErr := os.ReadFile(p)
		require.NoError(t, readErr)
		assert.Equal(t, body, string(got))
	})
	t.Run("invalid legacy value fails native validation without writing", func(t *testing.T) {
		d := t.TempDir()
		legacy := filepath.Join(d, LegacyFileName)
		mustWrite(t, legacy, "blacklist = [\"[\"]\n")
		_, _, err := Migrate(LoadOptions{Path: legacy}, "", false)
		require.ErrorContains(t, err, "native validation")
		_, err = os.Stat(filepath.Join(d, NativeFileName))
		require.ErrorIs(t, err, os.ErrNotExist, "wrote native file despite validation failure")
	})
}

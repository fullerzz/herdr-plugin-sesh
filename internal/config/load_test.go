package config

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadMissingExplicitPathIncludesPath(t *testing.T) {
	p := filepath.Join(t.TempDir(), "missing.toml")
	_, _, err := Load(LoadOptions{Warn: io.Discard, Path: p})
	require.ErrorIs(t, err, os.ErrNotExist)
	require.ErrorContains(t, err, p)
}

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
	cfg, path, err := Load(LoadOptions{Warn: io.Discard, Path: cfgp, Home: "/home/zach"})
	require.NoError(t, err)
	require.Equal(t, cfgp, path)
	require.Equal(t, 2, cfg.DirLength)
	require.Len(t, cfg.SessionConfigs, 1)
	require.Equal(t, "~/projects/api", cfg.SessionConfigs[0].Path)
	require.Len(t, cfg.WindowConfigs, 1)
	require.Equal(t, "git", cfg.WindowConfigs[0].Name)
	assert.Equal(t, DefaultPreviewCommand, cfg.DefaultSessionConfig.PreviewCommand)
}

func TestLoadStrictRejectsUnknown(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "sesh.toml")
	mustWrite(t, p, "strict_mode = true\nwat = 1\n")
	_, _, err := Load(LoadOptions{Warn: io.Discard, Path: p})
	require.Error(t, err)
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
			_, _, err := Load(LoadOptions{Warn: io.Discard, Path: p})
			require.Error(t, err)
		})
	}
}

func TestLoadStrictRejectsUnsupportedSeshKeysInImports(t *testing.T) {
	d := t.TempDir()
	mustWrite(t, filepath.Join(d, "extra.toml"), "tmux_command = \"psmux\"\n")
	p := filepath.Join(d, "sesh.toml")
	mustWrite(t, p, "strict_mode = true\nimport = [\"extra.toml\"]\n")
	_, _, err := Load(LoadOptions{Warn: io.Discard, Path: p})
	require.Error(t, err)
}

func TestLoadImportOrder(t *testing.T) {
	d := t.TempDir()
	mustWrite(t, filepath.Join(d, "extra.toml"), `[[session]]
name="extra"
path="/extra"
`)
	p := filepath.Join(d, "sesh.toml")
	mustWrite(t, p, "import=[\"extra.toml\"]\n[[session]]\nname=\"main\"\npath=\"/main\"\n")
	cfg, _, err := Load(LoadOptions{Warn: io.Discard, Path: p})
	require.NoError(t, err)
	got := []string{cfg.SessionConfigs[0].Name, cfg.SessionConfigs[1].Name}
	require.Equal(t, "extra", got[0])
	assert.Equal(t, "main", got[1])
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
	cfg, _, err := Load(LoadOptions{Warn: io.Discard, Path: p})
	require.NoError(t, err)
	require.Equal(t, "make test", cfg.DefaultSessionConfig.StartupCommand)
	require.Equal(t, "printf extra {}", cfg.DefaultSessionConfig.PreviewCommand)
	require.Equal(t, "Extra> ", cfg.TUI.Prompt)
	assert.Equal(t, "Search workspaces", cfg.TUI.Placeholder)
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
	cfg, _, err := Load(LoadOptions{Warn: io.Discard, Path: p})
	require.NoError(t, err)
	assert.Equal(t, DefaultPreviewCommand, cfg.DefaultSessionConfig.PreviewCommand)
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
	cfg, _, err := Load(LoadOptions{Warn: io.Discard, Path: p})
	require.NoError(t, err)
	require.Empty(t, cfg.TUI.Prompt)
	assert.Empty(t, cfg.TUI.Placeholder)
}

func TestLoadExplicitFalseShowIconsOverridesImportedValue(t *testing.T) {
	d := t.TempDir()
	mustWrite(t, filepath.Join(d, "extra.toml"), "[tui]\nshow_icons = true\n")
	p := filepath.Join(d, "sesh.toml")
	mustWrite(t, p, "import = [\"extra.toml\"]\n[tui]\nshow_icons = false\n")
	cfg, _, err := Load(LoadOptions{Warn: io.Discard, Path: p})
	require.NoError(t, err)
	assert.False(t, cfg.TUI.ShowIcons, "show_icons = true, want false")
}

func TestLoadExplicitFalseHerdrThemeInheritOverridesImportedValue(t *testing.T) {
	d := t.TempDir()
	mustWrite(t, filepath.Join(d, "extra.toml"), "[tui]\nherdr_theme_inherit = true\n")
	p := filepath.Join(d, "sesh.toml")
	mustWrite(t, p, "import = [\"extra.toml\"]\n[tui]\nherdr_theme_inherit = false\n")
	cfg, _, err := Load(LoadOptions{Warn: io.Discard, Path: p})
	require.NoError(t, err)
	assert.False(t, cfg.TUI.HerdrThemeInherit, "herdr_theme_inherit = true, want false")
}

func TestLoadExplicitFalseReplaceWorktreeIconOverridesImportedValue(t *testing.T) {
	d := t.TempDir()
	mustWrite(t, filepath.Join(d, "extra.toml"), "[tui]\nreplace_worktree_icon = true\n")
	p := filepath.Join(d, "sesh.toml")
	mustWrite(t, p, "import = [\"extra.toml\"]\n[tui]\nreplace_worktree_icon = false\n")
	cfg, _, err := Load(LoadOptions{Warn: io.Discard, Path: p})
	require.NoError(t, err)
	assert.False(t, cfg.TUI.ReplaceWorktreeIcon, "replace_worktree_icon=true, want false")
}

func TestLoadExplicitFalseShowLastWorkspacePathOverridesImportedValue(t *testing.T) {
	d := t.TempDir()
	mustWrite(t, filepath.Join(d, "extra.toml"), "[tui]\nshow_last_workspace_path = true\n")
	p := filepath.Join(d, "sesh.toml")
	mustWrite(t, p, "import = [\"extra.toml\"]\n[tui]\nshow_last_workspace_path = false\n")
	cfg, _, err := Load(LoadOptions{Path: p})
	require.NoError(t, err)
	assert.False(t, cfg.TUI.ShowLastWorkspacePath, "show_last_workspace_path = true, want false")
}

func TestLoadExplicitFalseShowLastWorkspaceOverridesImportedValue(t *testing.T) {
	d := t.TempDir()
	mustWrite(t, filepath.Join(d, "extra.toml"), "[tui]\nshow_last_workspace = true\n")
	p := filepath.Join(d, "sesh.toml")
	mustWrite(t, p, "import = [\"extra.toml\"]\n[tui]\nshow_last_workspace = false\n")
	cfg, _, err := Load(LoadOptions{Path: p})
	require.NoError(t, err)
	assert.False(t, cfg.TUI.ShowLastWorkspace, "show_last_workspace = true, want false")
}

func TestDefaultInheritsHerdrTheme(t *testing.T) {
	assert.True(t, Default().TUI.HerdrThemeInherit, "herdr_theme_inherit default = false, want true")
}

func TestDefaultShowsLastWorkspacePath(t *testing.T) {
	cfg := Default()
	require.True(t, cfg.TUI.ShowLastWorkspace)
	assert.True(t, cfg.TUI.ShowLastWorkspacePath)
}

func TestDefaultReplacesWorktreeIcon(t *testing.T) {
	assert.True(t, Default().TUI.ReplaceWorktreeIcon, "worktree icon replacement disabled by default")
}

func TestLoadTUIDefaultSort(t *testing.T) {
	for _, sortMode := range []string{"recent", "agent"} {
		t.Run(sortMode, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "sesh.toml")
			mustWrite(t, p, "[tui]\ndefault_sort = \""+sortMode+"\"\n")
			cfg, _, err := Load(LoadOptions{Warn: io.Discard, Path: p})
			require.NoError(t, err)
			assert.Equal(t, sortMode, cfg.TUI.DefaultSort)
		})
	}
}

func TestLoadRejectsInvalidTUIDefaultSort(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sesh.toml")
	mustWrite(t, p, "[tui]\ndefault_sort = \"newest\"\n")
	_, _, err := Load(LoadOptions{Warn: io.Discard, Path: p})
	require.Error(t, err)
	for _, want := range []string{"tui.default_sort", "workspace", "recent", "agent"} {
		require.ErrorContains(t, err, want)
	}
}

func TestDefaultPreviewCommandUsesEzaIcons(t *testing.T) {
	cfg := Default()
	require.Equal(t, DefaultPreviewCommand, cfg.DefaultSessionConfig.PreviewCommand)
	// Preview output is captured through sh, never a TTY, so eza's automatic
	// color detection would disable ANSI colors without an explicit force.
	assert.Equal(t, "eza --icons=always --color=always -la {}", DefaultPreviewCommand)
}

func TestInitConfigDoesNotAdvertiseUnsupportedSeshSchema(t *testing.T) {
	p, err := InitConfig(t.TempDir())
	require.NoError(t, err)
	//nolint:gosec // p is returned from InitConfig using a test-owned temporary directory.
	data, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "sesh.schema.json")
}

func mustWrite(t *testing.T, p, s string) {
	t.Helper()
	require.NoError(t, os.WriteFile(p, []byte(s), 0600))
}

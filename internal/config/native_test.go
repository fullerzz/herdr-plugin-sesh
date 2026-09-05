package config

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"

	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadNative(t *testing.T, body string) (Config, error) {
	t.Helper()
	p := filepath.Join(t.TempDir(), NativeFileName)
	mustWrite(t, p, body)
	cfg, _, err := Load(LoadOptions{Path: p, Warn: &bytes.Buffer{}})
	return cfg, err
}

func TestNativeHappyPathEveryKey(t *testing.T) {
	cfg, err := loadNative(t, `version = 1

[list]
cache = true
source_order = ["config", "herdr"]
blacklist = ["^scratch$"]

[naming]
path_components = 2

[picker]
show_icons = true
show_preview = false
preview_mode = "pane"
prioritize_home = false
herdr_theme_inherit = true
show_last_workspace = true
show_last_workspace_path = true
prompt = "P> "
placeholder = "find"
separator_aware = true
workspace_sort = "recent"

[workspace_defaults]
startup = "make dev"
preview = "ls {}"

[[tab]]
name = "git"
startup = "git status"
path = "~/projects"

[[workspace]]
name = "brain"
path = "~/brain"
startup = "nvim"
preview = "cat README.md"
disable_startup = true
tabs = ["git"]

[[rule]]
path_glob = "~/projects/**"
startup = "git status"
preview = "ls {}"
disable_startup = false
tabs = ["git"]
`)
	require.NoError(t, err)
	disable := true
	want := Config{
		Keys:           KeyConfig{CyclePreviewMode: "ctrl+o"},
		Cache:          true,
		DirLength:      2,
		SeparatorAware: true,
		SortOrder:      []string{"config", "herdr"},
		Blacklist:      []string{"^scratch$"},
		TUI:            TUIConfig{ShowIcons: true, PreviewMode: "pane", PrioritizeHome: false, HerdrThemeInherit: true, ReplaceWorktreeIcon: true, ShowLastWorkspace: true, ShowLastWorkspacePath: true, Prompt: "P> ", Placeholder: "find", DefaultSort: "recent"},
		DefaultSessionConfig: DefaultSessionConfig{
			StartupCommand: "make dev",
			PreviewCommand: "ls {}",
		},
		WindowConfigs: []model.WindowConfig{{Name: "git", StartupScript: "git status", Path: "~/projects"}},
		SessionConfigs: []SessionConfig{{
			DefaultSessionConfig: DefaultSessionConfig{StartupCommand: "nvim", PreviewCommand: "cat README.md"},
			Name:                 "brain",
			Path:                 "~/brain",
			DisableStartCommand:  &disable,
			Windows:              []string{"git"},
		}},
		WildcardConfigs: []WildcardConfig{{
			Pattern:        "~/projects/**",
			StartupCommand: "git status",
			PreviewCommand: "ls {}",
			Windows:        []string{"git"},
		}},
	}
	assert.Equal(t, want, cfg)
}

func TestNativeMinimalFileKeepsDefaults(t *testing.T) {
	cfg, err := loadNative(t, "version = 1\n")
	require.NoError(t, err)
	d := Default()
	assert.Equal(t, d.DirLength, cfg.DirLength)
	assert.Equal(t, d.TUI.DefaultSort, cfg.TUI.DefaultSort)
	assert.True(t, cfg.TUI.PrioritizeHome, "prioritize_home")
	assert.True(t, cfg.TUI.HerdrThemeInherit, "herdr_theme_inherit")
	assert.True(t, cfg.TUI.ShowLastWorkspace, "show_last_workspace")
	assert.True(t, cfg.TUI.ShowLastWorkspacePath, "show_last_workspace_path")
	assert.True(t, cfg.TUI.ReplaceWorktreeIcon, "replace_worktree_icon")
	assert.Equal(t, DefaultPreviewCommand, cfg.DefaultSessionConfig.PreviewCommand)
}

func TestNativePickerCanDisableHomePrioritization(t *testing.T) {
	cfg, err := loadNative(t, "version = 1\n[picker]\nprioritize_home = false\n")
	require.NoError(t, err)
	assert.False(t, cfg.TUI.PrioritizeHome, "prioritize_home=true, want false")
}

func TestNativePickerAcceptsAgentWorkspaceSort(t *testing.T) {
	cfg, err := loadNative(t, "version = 1\n[picker]\nworkspace_sort = \"agent\"\n")
	require.NoError(t, err)
	assert.Equal(t, "agent", cfg.TUI.DefaultSort)
}

func TestNativePickerCanDisableHerdrThemeInheritance(t *testing.T) {
	cfg, err := loadNative(t, `version = 1

[picker]
herdr_theme_inherit = false
`)
	require.NoError(t, err)
	assert.False(t, cfg.TUI.HerdrThemeInherit, "herdr_theme_inherit = true, want false")
}

func TestNativePickerCanDisableWorktreeIconReplacement(t *testing.T) {
	cfg, err := loadNative(t, `version = 1

[picker]
replace_worktree_icon = false
`)
	require.NoError(t, err)
	assert.False(t, cfg.TUI.ReplaceWorktreeIcon, "replace_worktree_icon=true, want false")
}

func TestNativePickerCanDisablePreview(t *testing.T) {
	cfg, err := loadNative(t, `version = 1

[picker]
show_preview = false
`)
	require.NoError(t, err)
	require.False(t, cfg.TUI.ShowPreview, "show_preview=true, want false")

	defaults, err := loadNative(t, "version = 1\n")
	require.NoError(t, err)
	assert.True(t, defaults.TUI.ShowPreview, "show_preview=false by default, want true")
}

func TestNativePickerPreviewMode(t *testing.T) {
	for _, tc := range []struct {
		name, setting, want string
	}{
		{name: "omitted", want: "command"},
		{name: "command", setting: `preview_mode = "command"`, want: "command"},
		{name: "pane", setting: `preview_mode = "pane"`, want: "pane"},
		{name: "empty", setting: `preview_mode = ""`, want: "command"},
		{name: "invalid", setting: `preview_mode = "terminal"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := loadNative(t, "version = 1\n[picker]\n"+tc.setting+"\n")
			if tc.want == "" {
				assert.ErrorContains(t, err, "picker.preview_mode")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, cfg.TUI.PreviewMode)
		})
	}
}

func TestNativePickerAcceptsLastWorkspaceSettings(t *testing.T) {
	cfg, err := loadNative(t, `version = 1

[picker]
show_last_workspace = false
show_last_workspace_path = false
`)
	require.NoError(t, err)
	require.False(t, cfg.TUI.ShowLastWorkspace)
	assert.False(t, cfg.TUI.ShowLastWorkspacePath)
}

func TestNativeEmptyPreviewFallsBackToDefault(t *testing.T) {
	cfg, err := loadNative(t, "version = 1\n[workspace_defaults]\npreview = \"\"\n")
	require.NoError(t, err)
	assert.Equal(t, DefaultPreviewCommand, cfg.DefaultSessionConfig.PreviewCommand)
}

func TestNativeFailures(t *testing.T) {
	tests := map[string]struct {
		body string
		want string
	}{
		"unsupported version":  {"version = 2\n", "version"},
		"zero version":         {"version = 0\n", "version"},
		"unknown field":        {"version = 1\nwat = 1\n", "wat"},
		"unknown nested field": {"version = 1\n[picker]\ntheme = \"dark\"\n", "theme"},
		"legacy key rejected":  {"version = 1\nstrict_mode = true\n", "strict_mode"},
		"import rejected":      {"version = 1\nimport = [\"x.toml\"]\n", "import"},
		"bad sort":             {"version = 1\n[picker]\nworkspace_sort = \"newest\"\n", "picker.workspace_sort: must be \"workspace\" or \"recent\" or \"agent\""},
		"bad path components":  {"version = 1\n[naming]\npath_components = 0\n", "path_components"},
		"unknown source":       {"version = 1\n[list]\nsource_order = [\"tmux\"]\n", "source_order"},
		"duplicate source":     {"version = 1\n[list]\nsource_order = [\"dir\", \"dir\"]\n", "source_order"},
		"bad regex":            {"version = 1\n[list]\nblacklist = [\"[\"]\n", "blacklist"},
		"bad glob":             {"version = 1\n[[rule]]\npath_glob = \"[\"\n", "path_glob"},
		"empty glob":           {"version = 1\n[[rule]]\npath_glob = \"\"\n", "path_glob"},
		"empty tab name":       {"version = 1\n[[tab]]\nstartup = \"x\"\n", "tab.name"},
		"duplicate tab":        {"version = 1\n[[tab]]\nname = \"g\"\n[[tab]]\nname = \"g\"\n", "tab.name"},
		"empty workspace name": {"version = 1\n[[workspace]]\npath = \"/x\"\n", "workspace.name"},
		"empty workspace path": {"version = 1\n[[workspace]]\nname = \"x\"\n", "workspace.path"},
		"duplicate workspace":  {"version = 1\n[[workspace]]\nname = \"x\"\npath = \"/x\"\n[[workspace]]\nname = \"x\"\npath = \"/y\"\n", "workspace.name"},
		"missing tab ref":      {"version = 1\n[[workspace]]\nname = \"x\"\npath = \"/x\"\ntabs = [\"nope\"]\n", "workspace.tabs"},
		"missing rule tab ref": {"version = 1\n[[rule]]\npath_glob = \"/x/**\"\ntabs = [\"nope\"]\n", "rule.tabs"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := loadNative(t, tc.body)
			assert.ErrorContains(t, err, tc.want)
		})
	}
}

func TestNativeLegacyParity(t *testing.T) {
	d := t.TempDir()
	legacyPath := filepath.Join(d, LegacyFileName)
	mustWrite(t, legacyPath, `cache = true
sort_order = ["config", "zoxide"]
dir_length = 2
separator_aware = true
blacklist = ["^scratch$"]

[tui]
show_icons = true
prompt = "P> "
placeholder = "find"
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
`)
	nativePath := filepath.Join(d, NativeFileName)
	mustWrite(t, nativePath, `version = 1

[list]
cache = true
source_order = ["config", "zoxide"]
blacklist = ["^scratch$"]

[naming]
path_components = 2

[picker]
show_icons = true
prompt = "P> "
placeholder = "find"
separator_aware = true
workspace_sort = "recent"

[workspace_defaults]
startup = "make dev"
preview = "ls {}"

[[tab]]
name = "git"
startup = "git status"

[[workspace]]
name = "brain"
path = "~/brain"
disable_startup = true
tabs = ["git"]

[[rule]]
path_glob = "~/projects/**"
startup = "git status"
tabs = ["git"]
`)
	var warn bytes.Buffer
	legacyCfg, _, err := Load(LoadOptions{Path: legacyPath, Warn: &warn})
	require.NoError(t, err)
	nativeCfg, _, err := Load(LoadOptions{Path: nativePath, Warn: &warn})
	require.NoError(t, err)
	// Normalize the one representational difference: legacy leaves ImportPaths
	// nil and native never sets it.
	legacyCfg.ImportPaths = nil
	assert.Equal(t, legacyCfg, nativeCfg)
}

func TestDiscoveredNativeFileRequiresVersion(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, NativeFileName), "[list]\ncache = true\n")
	_, _, err := Load(LoadOptions{
		Home: t.TempDir(),
		Env:  map[string]string{"HERDR_PLUGIN_CONFIG_DIR": dir},
		Warn: &bytes.Buffer{},
	})
	require.ErrorContains(t, err, "version")
}

func TestExplicitPathWithoutVersionLoadsAsLegacy(t *testing.T) {
	p := filepath.Join(t.TempDir(), NativeFileName)
	mustWrite(t, p, "cache = true\n")
	cfg, _, err := Load(LoadOptions{Path: p, Warn: &bytes.Buffer{}})
	require.NoError(t, err)
	assert.True(t, cfg.Cache)
}

func TestExplicitPathVersionKeySelectsNative(t *testing.T) {
	p := filepath.Join(t.TempDir(), "anything.toml")
	mustWrite(t, p, "version = 1\nstrict_mode = true\n")
	_, _, err := Load(LoadOptions{Path: p, Warn: &bytes.Buffer{}})
	require.ErrorContains(t, err, "strict_mode")
}

func TestLegacyLoadWarnsOnStderrWriter(t *testing.T) {
	p := filepath.Join(t.TempDir(), LegacyFileName)
	mustWrite(t, p, "cache = true\n")
	var warn bytes.Buffer
	_, _, err := Load(LoadOptions{Path: p, Warn: &warn})
	require.NoError(t, err)
	assert.Contains(t, warn.String(), "deprecated")
}

func TestNativeFixtureLoads(t *testing.T) {
	cfg, _, err := Load(LoadOptions{Path: filepath.Join("..", "..", "testdata", "herdr-sesh.toml"), Warn: &bytes.Buffer{}})
	require.NoError(t, err)
	require.Len(t, cfg.SessionConfigs, 1)
	assert.Equal(t, "sesh", cfg.SessionConfigs[0].Name)
}

func TestDiscoveryOrderAndPrecedence(t *testing.T) {
	home := t.TempDir()
	pluginDir := filepath.Join(home, "plugin-config")
	mkdirs := []string{
		pluginDir,
		filepath.Join(home, ".config", "herdr-sesh"),
		filepath.Join(home, ".config", "sesh"),
	}
	for _, d := range mkdirs {
		require.NoError(t, os.MkdirAll(d, 0700))
	}
	write := func(rel string) string {
		p := filepath.Join(home, rel)
		body := "version = 1\n"
		if filepath.Base(p) == LegacyFileName {
			body = "cache = true\n"
		}
		mustWrite(t, p, body)
		return p
	}
	order := []string{
		filepath.Join("plugin-config", NativeFileName),
		filepath.Join("plugin-config", LegacyFileName),
		filepath.Join(".config", "herdr-sesh", NativeFileName),
		filepath.Join(".config", "herdr-sesh", LegacyFileName),
		filepath.Join(".config", "sesh", LegacyFileName),
	}
	// Create from lowest precedence upward; each added higher candidate must win.
	for i := len(order) - 1; i >= 0; i-- {
		want := write(order[i])
		got, err := ResolvePath(LoadOptions{Home: home, Env: map[string]string{"HERDR_PLUGIN_CONFIG_DIR": pluginDir}})
		require.NoError(t, err)
		require.Equal(t, want, got)
	}
}

func TestMissingEnvConfigErrors(t *testing.T) {
	_, err := ResolvePath(LoadOptions{
		Home: t.TempDir(),
		Env:  map[string]string{"HERDR_SESH_CONFIG": "/nope/missing.toml"},
	})
	require.Error(t, err)
}

func TestResolvePathPropagatesFilesystemErrors(t *testing.T) {
	home := t.TempDir()
	notDir := filepath.Join(home, "not-a-directory")
	mustWrite(t, notDir, "blocked\n")

	t.Run("explicit path", func(t *testing.T) {
		_, err := ResolvePath(LoadOptions{Path: filepath.Join(notDir, NativeFileName), Home: home, Env: map[string]string{}})
		require.ErrorIs(t, err, syscall.ENOTDIR)
	})

	t.Run("environment path", func(t *testing.T) {
		_, err := ResolvePath(LoadOptions{
			Home: home,
			Env:  map[string]string{"HERDR_SESH_CONFIG": filepath.Join(notDir, NativeFileName)},
		})
		require.ErrorIs(t, err, syscall.ENOTDIR)
	})

	t.Run("default discovery", func(t *testing.T) {
		fallbackDir := filepath.Join(home, ".config", "herdr-sesh")
		require.NoError(t, os.MkdirAll(fallbackDir, 0700))
		mustWrite(t, filepath.Join(fallbackDir, NativeFileName), "version = 1\n")

		_, err := ResolvePath(LoadOptions{
			Home: home,
			Env:  map[string]string{"HERDR_PLUGIN_CONFIG_DIR": notDir},
		})
		require.ErrorIs(t, err, syscall.ENOTDIR)
	})
}

func TestInitConfigWritesNativeStarter(t *testing.T) {
	dir := t.TempDir()
	p, err := InitConfig(dir)
	require.NoError(t, err)
	require.Equal(t, NativeFileName, filepath.Base(p))
	cfg, _, err := Load(LoadOptions{Path: p, Warn: &bytes.Buffer{}})
	require.NoError(t, err)
	require.Equal(t, DefaultPreviewCommand, cfg.DefaultSessionConfig.PreviewCommand)
	info, err := os.Stat(p)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestInitConfigAtRejectsDirectory(t *testing.T) {
	p := filepath.Join(t.TempDir(), NativeFileName)
	require.NoError(t, os.Mkdir(p, 0700))
	_, err := InitConfigAt(p)
	require.ErrorContains(t, err, "not a regular file")
}

func TestNativeCyclePreviewModeKey(t *testing.T) {
	for _, tc := range []struct{ name, setting, want string }{
		{"omitted", "", "ctrl+o"},
		{"custom", `cycle_preview_mode = "alt+p"`, "alt+p"},
		{"disabled", `cycle_preview_mode = ""`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := loadNative(t, "version = 1\n[keys]\n"+tc.setting+"\n")
			require.NoError(t, err)
			assert.Equal(t, tc.want, cfg.Keys.CyclePreviewMode)
		})
	}
}

func TestNativeCyclePreviewModeKeyValidation(t *testing.T) {
	for _, binding := range []string{"ctrl-o", "Ctrl+o", "ctrl+", "ctrl+ctrl+o", "alt+ctrl+o", "ctrl+unknown", "f64", "escape", "ctrl+o ", "\n"} {
		t.Run(binding, func(t *testing.T) {
			_, err := loadNative(t, "version = 1\n[keys]\ncycle_preview_mode = "+strconv.Quote(binding)+"\n")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "keys.cycle_preview_mode")
		})
	}
	for _, binding := range []string{"", "ctrl+o", "alt+p", "f2", "ctrl+alt+shift+f12", "enter", "space", "esc", "+", "ctrl++", "é", "A", "?"} {
		t.Run("valid "+binding, func(t *testing.T) {
			cfg, err := loadNative(t, "version = 1\n[keys]\ncycle_preview_mode = "+strconv.Quote(binding)+"\n")
			require.NoError(t, err)
			assert.Equal(t, binding, cfg.Keys.CyclePreviewMode)
		})
	}
}

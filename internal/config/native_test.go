package config

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
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
	if err != nil {
		t.Fatal(err)
	}
	disable := true
	want := Config{
		Cache:          true,
		DirLength:      2,
		SeparatorAware: true,
		SortOrder:      []string{"config", "herdr"},
		Blacklist:      []string{"^scratch$"},
		TUI:            TUIConfig{ShowIcons: true, Prompt: "P> ", Placeholder: "find", DefaultSort: "recent"},
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
	if !configsEqual(cfg, want) {
		t.Fatalf("decoded config mismatch:\ngot  %#v\nwant %#v", cfg, want)
	}
}

func TestNativeMinimalFileKeepsDefaults(t *testing.T) {
	cfg, err := loadNative(t, "version = 1\n")
	if err != nil {
		t.Fatal(err)
	}
	d := Default()
	if cfg.DirLength != d.DirLength || cfg.TUI.DefaultSort != d.TUI.DefaultSort || cfg.DefaultSessionConfig.PreviewCommand != DefaultPreviewCommand {
		t.Fatalf("defaults lost: %#v", cfg)
	}
}

func TestNativeEmptyPreviewFallsBackToDefault(t *testing.T) {
	cfg, err := loadNative(t, "version = 1\n[workspace_defaults]\npreview = \"\"\n")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultSessionConfig.PreviewCommand != DefaultPreviewCommand {
		t.Fatalf("preview = %q", cfg.DefaultSessionConfig.PreviewCommand)
	}
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
		"bad sort":             {"version = 1\n[picker]\nworkspace_sort = \"newest\"\n", "workspace_sort"},
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
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not mention %q", err, tc.want)
			}
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
	if err != nil {
		t.Fatal(err)
	}
	nativeCfg, _, err := Load(LoadOptions{Path: nativePath, Warn: &warn})
	if err != nil {
		t.Fatal(err)
	}
	// Normalize the one representational difference: legacy leaves ImportPaths
	// nil and native never sets it.
	legacyCfg.ImportPaths = nil
	if !configsEqual(legacyCfg, nativeCfg) {
		t.Fatalf("parity mismatch:\nlegacy %#v\nnative %#v", legacyCfg, nativeCfg)
	}
}

func configsEqual(a, b Config) bool {
	ad := a.SessionConfigs[0].DisableStartCommand
	bd := b.SessionConfigs[0].DisableStartCommand
	if (ad == nil) != (bd == nil) || (ad != nil && *ad != *bd) {
		return false
	}
	a.SessionConfigs[0].DisableStartCommand = nil
	b.SessionConfigs[0].DisableStartCommand = nil
	return reflect.DeepEqual(a, b)
}

func TestDiscoveredNativeFileRequiresVersion(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, NativeFileName), "[list]\ncache = true\n")
	_, _, err := Load(LoadOptions{
		Home: t.TempDir(),
		Env:  map[string]string{"HERDR_PLUGIN_CONFIG_DIR": dir},
		Warn: &bytes.Buffer{},
	})
	if err == nil || !strings.Contains(err.Error(), "version") {
		t.Fatalf("expected version error, got %v", err)
	}
}

func TestExplicitPathWithoutVersionLoadsAsLegacy(t *testing.T) {
	p := filepath.Join(t.TempDir(), NativeFileName)
	mustWrite(t, p, "cache = true\n")
	cfg, _, err := Load(LoadOptions{Path: p, Warn: &bytes.Buffer{}})
	if err != nil || !cfg.Cache {
		t.Fatalf("cfg %#v err %v", cfg, err)
	}
}

func TestExplicitPathVersionKeySelectsNative(t *testing.T) {
	p := filepath.Join(t.TempDir(), "anything.toml")
	mustWrite(t, p, "version = 1\nstrict_mode = true\n")
	_, _, err := Load(LoadOptions{Path: p, Warn: &bytes.Buffer{}})
	if err == nil || !strings.Contains(err.Error(), "strict_mode") {
		t.Fatalf("expected native strict rejection, got %v", err)
	}
}

func TestLegacyLoadWarnsOnStderrWriter(t *testing.T) {
	p := filepath.Join(t.TempDir(), LegacyFileName)
	mustWrite(t, p, "cache = true\n")
	var warn bytes.Buffer
	if _, _, err := Load(LoadOptions{Path: p, Warn: &warn}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(warn.String(), "deprecated") {
		t.Fatalf("warning = %q", warn.String())
	}
}

func TestNativeFixtureLoads(t *testing.T) {
	cfg, _, err := Load(LoadOptions{Path: filepath.Join("..", "..", "testdata", "herdr-sesh.toml"), Warn: &bytes.Buffer{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.SessionConfigs) != 1 || cfg.SessionConfigs[0].Name != "sesh" {
		t.Fatalf("fixture workspaces = %#v", cfg.SessionConfigs)
	}
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
		if err := os.MkdirAll(d, 0700); err != nil {
			t.Fatal(err)
		}
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
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("after creating %s: resolved %s, want %s", order[i], got, want)
		}
	}
}

func TestMissingEnvConfigErrors(t *testing.T) {
	_, err := ResolvePath(LoadOptions{
		Home: t.TempDir(),
		Env:  map[string]string{"HERDR_SESH_CONFIG": "/nope/missing.toml"},
	})
	if err == nil {
		t.Fatal("expected error for missing HERDR_SESH_CONFIG target")
	}
}

func TestInitConfigWritesNativeStarter(t *testing.T) {
	dir := t.TempDir()
	p, err := InitConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(p) != NativeFileName {
		t.Fatalf("init path = %s", p)
	}
	cfg, _, err := Load(LoadOptions{Path: p, Warn: &bytes.Buffer{}})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultSessionConfig.PreviewCommand != DefaultPreviewCommand {
		t.Fatalf("starter preview = %q", cfg.DefaultSessionConfig.PreviewCommand)
	}
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode = %v", info.Mode().Perm())
	}
}

package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

type LoadOptions struct {
	Path string
	Env  map[string]string
	Home string
	// Warn receives legacy-schema deprecation warnings; nil means os.Stderr.
	Warn io.Writer
}

// NativeFileName and LegacyFileName are the default config basenames looked up
// inside plugin and standalone config directories.
const (
	NativeFileName = "config.toml"
	LegacyFileName = "sesh.toml"
)

type fileKind int

const (
	// kindExplicit paths come from --config or HERDR_SESH_CONFIG and may hold
	// either schema; a top-level version key selects native decoding.
	kindExplicit fileKind = iota
	kindNative
	kindLegacy
)

func Load(opts LoadOptions) (Config, string, error) {
	cfg := Default()
	path, kind, err := resolve(opts)
	if err != nil {
		return cfg, "", err
	}
	if path == "" {
		return cfg, "", nil
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return cfg, path, err
	}
	path = filepath.Clean(path)
	//nolint:gosec // the config path is user-selected by design.
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, path, err
	}
	if kind == kindExplicit {
		if hasVersionKey(data) {
			kind = kindNative
		} else {
			kind = kindLegacy
		}
	}
	if kind == kindNative {
		cfg, err = decodeNative(path, data)
		return cfg, path, err
	}
	warn := opts.Warn
	if warn == nil {
		warn = os.Stderr
	}
	_, _ = fmt.Fprintf(warn, "warning: %s uses the deprecated Sesh-compatible schema; run 'herdr-sesh config migrate' to convert it (see docs/config.md)\n", path)
	if err := decodeLegacy(&cfg, path); err != nil {
		return cfg, path, err
	}
	return cfg, path, nil
}

func decodeLegacy(cfg *Config, path string) error {
	seen := map[string]bool{}
	if err := loadInto(cfg, path, seen, false); err != nil {
		return err
	}
	attachDefaults(cfg)
	if cfg.TUI.DefaultSort != "workspace" && cfg.TUI.DefaultSort != "recent" {
		return fmt.Errorf("load %s: tui.default_sort must be \"workspace\" or \"recent\"", path)
	}
	return nil
}

// ResolvePath returns the config file Load would read, or "" when no candidate
// exists.
func ResolvePath(opts LoadOptions) (string, error) {
	p, _, err := resolve(opts)
	return p, err
}

func resolve(opts LoadOptions) (string, fileKind, error) {
	env := opts.Env
	if env == nil {
		env = getenvMap()
	}
	home := opts.Home
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	if opts.Path != "" {
		p := ExpandHome(opts.Path, home)
		if _, err := os.Stat(p); err != nil {
			return "", kindExplicit, os.ErrNotExist
		}
		return p, kindExplicit, nil
	}
	if v := env["HERDR_SESH_CONFIG"]; v != "" {
		p := ExpandHome(v, home)
		if _, err := os.Stat(p); err != nil {
			return "", kindExplicit, fmt.Errorf("HERDR_SESH_CONFIG %s: %w", p, os.ErrNotExist)
		}
		return p, kindExplicit, nil
	}
	type candidate struct {
		path string
		kind fileKind
	}
	candidates := []candidate{}
	if dir := env["HERDR_PLUGIN_CONFIG_DIR"]; dir != "" {
		candidates = append(candidates,
			candidate{filepath.Join(dir, NativeFileName), kindNative},
			candidate{filepath.Join(dir, LegacyFileName), kindLegacy},
		)
	}
	if home != "" {
		candidates = append(candidates,
			candidate{filepath.Join(home, ".config", "herdr-sesh", NativeFileName), kindNative},
			candidate{filepath.Join(home, ".config", "herdr-sesh", LegacyFileName), kindLegacy},
			candidate{filepath.Join(home, ".config", "sesh", LegacyFileName), kindLegacy},
		)
	}
	for _, c := range candidates {
		p := ExpandHome(c.path, home)
		if _, err := os.Stat(p); err == nil {
			return p, c.kind, nil
		}
	}
	return "", kindNative, nil
}

func loadInto(dst *Config, path string, seen map[string]bool, strict bool) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if seen[abs] {
		return nil
	}
	seen[abs] = true
	//nolint:gosec // config imports intentionally read user-selected paths.
	data, err := os.ReadFile(abs)
	if err != nil {
		return err
	}
	var probe struct {
		StrictMode           bool     `toml:"strict_mode"`
		ImportPaths          []string `toml:"import"`
		DefaultSessionConfig struct {
			PreviewCommand *string `toml:"preview_command"`
		} `toml:"default_session"`
		TUI struct {
			Prompt                *string `toml:"prompt"`
			Placeholder           *string `toml:"placeholder"`
			ShowIcons             *bool   `toml:"show_icons"`
			ShowLastWorkspace     *bool   `toml:"show_last_workspace"`
			ShowLastWorkspacePath *bool   `toml:"show_last_workspace_path"`
			DefaultSort           *string `toml:"default_sort"`
		} `toml:"tui"`
	}
	_ = toml.Unmarshal(data, &probe)
	strict = strict || probe.StrictMode
	dec := toml.NewDecoder(bytes.NewReader(data))
	if strict {
		dec.DisallowUnknownFields()
	}
	var next Config
	if err := dec.Decode(&next); err != nil {
		return fmt.Errorf("load %s: %w", path, err)
	}
	base := filepath.Dir(abs)
	for _, imp := range next.ImportPaths {
		ip := ExpandHome(imp, "")
		if !filepath.IsAbs(ip) {
			ip = filepath.Join(base, ip)
		}
		if err := loadInto(dst, ip, seen, strict); err != nil {
			return err
		}
	}
	merge(dst, next,
		probe.DefaultSessionConfig.PreviewCommand != nil,
		probe.TUI.Prompt != nil,
		probe.TUI.Placeholder != nil,
		probe.TUI.ShowIcons != nil,
		probe.TUI.ShowLastWorkspace != nil,
		probe.TUI.ShowLastWorkspacePath != nil,
		probe.TUI.DefaultSort != nil,
	)
	return nil
}

func merge(dst *Config, src Config, previewCommandSet, promptSet, placeholderSet, showIconsSet, showLastWorkspaceSet, showLastWorkspacePathSet, defaultSortSet bool) {
	if src.Cache {
		dst.Cache = true
	}
	if src.StrictMode {
		dst.StrictMode = true
	}
	if len(src.Blacklist) > 0 {
		dst.Blacklist = append(dst.Blacklist, src.Blacklist...)
	}
	if len(src.SortOrder) > 0 {
		dst.SortOrder = src.SortOrder
	}
	if src.DirLength > 0 {
		dst.DirLength = src.DirLength
	}
	if src.SeparatorAware {
		dst.SeparatorAware = true
	}
	if showIconsSet {
		dst.TUI.ShowIcons = src.TUI.ShowIcons
	}
	if showLastWorkspaceSet {
		dst.TUI.ShowLastWorkspace = src.TUI.ShowLastWorkspace
	}
	if showLastWorkspacePathSet {
		dst.TUI.ShowLastWorkspacePath = src.TUI.ShowLastWorkspacePath
	}
	if promptSet {
		dst.TUI.Prompt = src.TUI.Prompt
	}
	if placeholderSet {
		dst.TUI.Placeholder = src.TUI.Placeholder
	}
	if defaultSortSet {
		dst.TUI.DefaultSort = src.TUI.DefaultSort
	}
	if src.DefaultSessionConfig.StartupCommand != "" {
		dst.DefaultSessionConfig.StartupCommand = src.DefaultSessionConfig.StartupCommand
	}
	if previewCommandSet {
		dst.DefaultSessionConfig.PreviewCommand = src.DefaultSessionConfig.PreviewCommand
	}
	dst.SessionConfigs = append(dst.SessionConfigs, src.SessionConfigs...)
	dst.WindowConfigs = append(dst.WindowConfigs, src.WindowConfigs...)
	dst.WildcardConfigs = append(dst.WildcardConfigs, src.WildcardConfigs...)
}

func attachDefaults(cfg *Config) {
	if cfg.DirLength < 1 {
		cfg.DirLength = 1
	}
	if len(cfg.SortOrder) == 0 {
		cfg.SortOrder = []string{"herdr", "config", "zoxide", "dir"}
	}
	if cfg.DefaultSessionConfig.PreviewCommand == "" {
		cfg.DefaultSessionConfig.PreviewCommand = DefaultPreviewCommand
	}
	if cfg.TUI.DefaultSort == "" {
		cfg.TUI.DefaultSort = DefaultWorkspaceSort
	}
}
func ExpandHome(p, home string) string {
	if p == "" {
		return p
	}
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	if p == "~" {
		return home
	}
	if len(p) > 2 && p[:2] == "~/" {
		return filepath.Join(home, p[2:])
	}
	return p
}
func getenvMap() map[string]string {
	m := map[string]string{}
	for _, kv := range os.Environ() {
		for i, c := range kv {
			if c == '=' {
				m[kv[:i]] = kv[i+1:]
				break
			}
		}
	}
	return m
}
func InitConfig(dir string) (string, error) {
	if dir == "" {
		return "", errors.New("config dir required")
	}
	return InitConfigAt(filepath.Join(dir, NativeFileName))
}

// InitConfigAt writes the native starter file at the given path, creating
// parent directories. An existing file is returned untouched.
func InitConfigAt(p string) (string, error) {
	if p == "" {
		return "", errors.New("config path required")
	}
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return "", err
	}
	if _, err := os.Stat(p); err == nil {
		return p, nil
	}
	starter := fmt.Sprintf("version = %d\n\n[workspace_defaults]\npreview = %q\n\n# [[workspace]]\n# name = \"Example\"\n# path = \"~/projects/example\"\n", NativeVersion, DefaultPreviewCommand)
	return p, os.WriteFile(p, []byte(starter), 0600)
}

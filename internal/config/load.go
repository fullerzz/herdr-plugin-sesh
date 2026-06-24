package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/pelletier/go-toml/v2"
)

type LoadOptions struct {
	Path string
	Env  map[string]string
	Home string
}

func Load(opts LoadOptions) (Config, string, error) {
	cfg := Default()
	path, err := ResolvePath(opts)
	if err != nil {
		return cfg, "", err
	}
	if path == "" {
		return cfg, "", nil
	}
	seen := map[string]bool{}
	if err := loadInto(&cfg, path, seen); err != nil {
		return cfg, path, err
	}
	attachDefaults(&cfg)
	return cfg, path, nil
}

func ResolvePath(opts LoadOptions) (string, error) {
	env := opts.Env
	if env == nil {
		env = getenvMap()
	}
	home := opts.Home
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	candidates := []string{}
	if opts.Path != "" {
		candidates = append(candidates, opts.Path)
	}
	if env["HERDR_SESH_CONFIG"] != "" {
		candidates = append(candidates, env["HERDR_SESH_CONFIG"])
	}
	if env["HERDR_PLUGIN_CONFIG_DIR"] != "" {
		candidates = append(candidates, filepath.Join(env["HERDR_PLUGIN_CONFIG_DIR"], "sesh.toml"))
	}
	if home != "" {
		candidates = append(candidates, filepath.Join(home, ".config", "sesh", "sesh.toml"))
	}
	for _, c := range candidates {
		p := ExpandHome(c, home)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
		if opts.Path != "" && c == opts.Path {
			return "", os.ErrNotExist
		}
	}
	return "", nil
}

func loadInto(dst *Config, path string, seen map[string]bool) error {
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
		StrictMode  bool     `toml:"strict_mode"`
		ImportPaths []string `toml:"import"`
	}
	_ = toml.Unmarshal(data, &probe)
	dec := toml.NewDecoder(bytes.NewReader(data))
	if probe.StrictMode {
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
		if err := loadInto(dst, ip, seen); err != nil {
			return err
		}
	}
	merge(dst, next)
	return nil
}

func merge(dst *Config, src Config) {
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
	if src.TmuxCommand != "" {
		dst.TmuxCommand = src.TmuxCommand
	}
	if src.TUI != (TUIConfig{}) {
		dst.TUI = src.TUI
	}
	if !reflect.DeepEqual(src.DefaultSessionConfig, DefaultSessionConfig{}) {
		dst.DefaultSessionConfig = src.DefaultSessionConfig
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
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	p := filepath.Join(dir, "sesh.toml")
	_, err := os.Stat(p)
	if err == nil {
		return p, nil
	}
	starter := "#:schema https://github.com/joshmedeski/sesh/raw/main/sesh.schema.json\n\n[default_session]\npreview_command = \"ls -la {}\"\n\n# [[session]]\n# name = \"Example\"\n# path = \"~/projects/example\"\n"
	return p, os.WriteFile(p, []byte(starter), 0600)
}

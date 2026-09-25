package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/pelletier/go-toml/v2"
)

// legacyDefaultPreviewCommand is the colorless preview that older releases
// baked into generated starter configs. Migration treats it as "use the
// built-in default", never as a deliberate custom command.
const legacyDefaultPreviewCommand = "eza --icons=always -la {}"

func isDefaultPreview(cmd string) bool {
	return cmd == DefaultPreviewCommand || cmd == legacyDefaultPreviewCommand
}

func upgradePreview(cmd string) string {
	if cmd == legacyDefaultPreviewCommand {
		return DefaultPreviewCommand
	}
	return cmd
}

// legacyShowIconsConfigured reports whether tui.show_icons is explicitly set
// in the legacy file or any of its imports. The runtime Config cannot answer
// this: its bool collapses "explicit false" and "absent".
func legacyShowIconsConfigured(path string, seen map[string]bool) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	if seen[abs] {
		return false
	}
	seen[abs] = true
	//nolint:gosec // the config path is user-selected by design.
	data, err := os.ReadFile(abs)
	if err != nil {
		return false
	}
	var probe struct {
		ImportPaths []string `toml:"import"`
		TUI         struct {
			ShowIcons *bool `toml:"show_icons"`
		} `toml:"tui"`
	}
	_ = toml.Unmarshal(data, &probe)
	if probe.TUI.ShowIcons != nil {
		return true
	}
	base := filepath.Dir(abs)
	for _, imp := range probe.ImportPaths {
		ip := ExpandHome(imp, "")
		if !filepath.IsAbs(ip) {
			ip = filepath.Join(base, ip)
		}
		if legacyShowIconsConfigured(ip, seen) {
			return true
		}
	}
	return false
}

// Migrate converts the resolved legacy config file into a native config.toml
// and returns both paths. The legacy file is left untouched. fallbackDir is
// the native destination used when the legacy file lives in the shared
// ~/.config/sesh directory, which discovery never scans for native files.
func Migrate(opts LoadOptions, fallbackDir string, force bool) (string, string, error) {
	migration, err := prepareMigration(opts, fallbackDir, force)
	if err != nil {
		return "", "", err
	}
	if err := migration.Save(); err != nil {
		return "", "", err
	}
	return migration.LegacyPath, migration.NativePath, nil
}

// PrepareMigration validates conversion without creating the native destination.
func PrepareMigration(opts LoadOptions, fallbackDir string) (*Migration, error) {
	return prepareMigration(opts, fallbackDir, false)
}

func prepareMigration(opts LoadOptions, fallbackDir string, force bool) (*Migration, error) {
	path, _, err := resolve(opts)
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, fmt.Errorf("no config file found to migrate")
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	path = filepath.Clean(path)
	dependencies, err := migrationSources(path, map[string]bool{})
	if err != nil {
		return nil, err
	}
	//nolint:gosec // the config path is user-selected by design.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if hasVersionKey(data) {
		return nil, fmt.Errorf("%s is already a native config", path)
	}
	cfg := Default()
	if err := decodeLegacy(&cfg, path, false); err != nil {
		return nil, err
	}
	// A legacy file that never mentions show_icons migrates to the
	// icon-enabled picker; an explicit legacy value (true or false) is kept.
	if !legacyShowIconsConfigured(path, map[string]bool{}) {
		cfg.TUI.ShowIcons = true
	}

	home := opts.Home
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	targetDir := filepath.Dir(path)
	if targetDir == filepath.Join(home, ".config", "sesh") {
		if fallbackDir == "" {
			return nil, fmt.Errorf("no native destination for %s: config dir required", path)
		}
		targetDir = fallbackDir
	}
	target := filepath.Join(targetDir, NativeFileName)
	if targetInfo, statErr := os.Stat(target); statErr == nil {
		sourceInfo, sourceErr := os.Stat(path)
		if sourceErr != nil {
			return nil, sourceErr
		}
		if os.SameFile(sourceInfo, targetInfo) {
			return nil, fmt.Errorf("source and destination are the same file: %s; rename the legacy file before migrating", path)
		}
		if !force {
			return nil, fmt.Errorf("refusing to overwrite existing %s", target)
		}
		//nolint:gosec // the destination is derived from the user-selected config path.
		targetData, readErr := os.ReadFile(target)
		if readErr != nil {
			return nil, readErr
		}
		if !hasVersionKey(targetData) {
			return nil, fmt.Errorf("refusing to overwrite %s: not a native config", target)
		}
		if _, decodeErr := decodeNative(target, targetData); decodeErr != nil {
			return nil, fmt.Errorf("refusing to overwrite invalid native config: %w", decodeErr)
		}
	} else if !os.IsNotExist(statErr) {
		return nil, statErr
	}

	out, err := marshalNative(cfg)
	if err != nil {
		return nil, err
	}
	// Legacy decoding never validated regexes, globs, or name uniqueness;
	// native decoding does. Surface those errors before writing anything.
	if _, err := decodeNative(path, out); err != nil {
		return nil, fmt.Errorf("legacy config does not satisfy native validation: %w", err)
	}

	targetDoc, err := OpenSettings(LoadOptions{Path: target})
	if err != nil {
		return nil, err
	}
	return &Migration{LegacyPath: path, NativePath: target, data: out, target: targetDoc, sources: dependencies}, nil
}

func marshalNative(cfg Config) ([]byte, error) {
	defaults := Default()
	dirLength := cfg.DirLength
	herdrThemeInherit := cfg.TUI.HerdrThemeInherit
	replaceWorktreeIcon := cfg.TUI.ReplaceWorktreeIcon
	showLastWorkspace := cfg.TUI.ShowLastWorkspace
	showLastWorkspacePath := cfg.TUI.ShowLastWorkspacePath
	n := nativeConfig{
		Version: NativeVersion,
		List: nativeList{
			Cache:       cfg.Cache,
			SourceOrder: cfg.SortOrder,
			Blacklist:   cfg.Blacklist,
		},
		Naming: nativeNaming{PathComponents: &dirLength},
		Picker: nativePicker{
			ShowIcons:             cfg.TUI.ShowIcons,
			HerdrThemeInherit:     &herdrThemeInherit,
			ReplaceWorktreeIcon:   &replaceWorktreeIcon,
			ShowLastWorkspace:     &showLastWorkspace,
			ShowLastWorkspacePath: &showLastWorkspacePath,
			Prompt:                cfg.TUI.Prompt,
			Placeholder:           cfg.TUI.Placeholder,
			SeparatorAware:        cfg.SeparatorAware,
			WorkspaceSort:         cfg.TUI.DefaultSort,
		},
		WorkspaceDefaults: nativeDefaults{
			Startup: cfg.DefaultSessionConfig.StartupCommand,
			Preview: cfg.DefaultSessionConfig.PreviewCommand,
		},
	}
	if slices.Equal(n.List.SourceOrder, defaults.SortOrder) {
		n.List.SourceOrder = nil
	}
	if *n.Naming.PathComponents == defaults.DirLength {
		n.Naming.PathComponents = nil
	}
	// attachDefaults injected the built-in preview when the legacy file never
	// set one, and older generated configs carry the former colorless default
	// as an explicit value. Omit both so the migrated file keeps tracking the
	// runtime default instead of freezing a stale command as user config.
	if isDefaultPreview(n.WorkspaceDefaults.Preview) {
		n.WorkspaceDefaults.Preview = ""
	}
	for _, w := range cfg.WindowConfigs {
		n.Tabs = append(n.Tabs, nativeTab{Name: w.Name, Startup: w.StartupScript, Path: w.Path})
	}
	for _, s := range cfg.SessionConfigs {
		n.Workspaces = append(n.Workspaces, nativeWorkspace{
			Name:           s.Name,
			Path:           s.Path,
			Startup:        s.StartupCommand,
			Preview:        upgradePreview(s.PreviewCommand),
			DisableStartup: s.DisableStartCommand,
			Tabs:           s.Windows,
		})
	}
	for _, r := range cfg.WildcardConfigs {
		n.Rules = append(n.Rules, nativeRule{
			PathGlob:       r.Pattern,
			Startup:        r.StartupCommand,
			Preview:        upgradePreview(r.PreviewCommand),
			DisableStartup: r.DisableStartCommand,
			Tabs:           r.Windows,
		})
	}
	return toml.Marshal(n)
}

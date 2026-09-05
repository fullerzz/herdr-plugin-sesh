package config

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/pelletier/go-toml/v2"

	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
)

const NativeVersion = 1

type nativeKeys struct {
	// nil keeps the default; an explicit empty string disables cycling.
	CyclePreviewMode *string `toml:"cycle_preview_mode,omitempty"`
}

type nativeConfig struct {
	Keys              nativeKeys        `toml:"keys,omitempty"`
	Version           int               `toml:"version"`
	List              nativeList        `toml:"list,omitempty"`
	Naming            nativeNaming      `toml:"naming"`
	Picker            nativePicker      `toml:"picker,omitempty"`
	WorkspaceDefaults nativeDefaults    `toml:"workspace_defaults,omitempty"`
	Tabs              []nativeTab       `toml:"tab,omitempty"`
	Workspaces        []nativeWorkspace `toml:"workspace,omitempty"`
	Rules             []nativeRule      `toml:"rule,omitempty"`
}

type nativeList struct {
	Cache       bool     `toml:"cache,omitempty"`
	SourceOrder []string `toml:"source_order,omitempty"`
	Blacklist   []string `toml:"blacklist,omitempty"`
}

type nativeNaming struct {
	// Pointer distinguishes an absent key (use the default) from an explicit
	// invalid zero, which strict validation must reject.
	PathComponents *int `toml:"path_components"`
}

type nativePicker struct {
	// No omitempty: migration must keep an explicitly configured false, and
	// an always-present value keeps icon behavior self-documenting.
	ShowIcons             bool   `toml:"show_icons"`
	ShowPreview           *bool  `toml:"show_preview,omitempty"`
	PreviewMode           string `toml:"preview_mode,omitempty"`
	PrioritizeHome        *bool  `toml:"prioritize_home,omitempty"`
	HerdrThemeInherit     *bool  `toml:"herdr_theme_inherit,omitempty"`
	ReplaceWorktreeIcon   *bool  `toml:"replace_worktree_icon,omitempty"`
	ShowLastWorkspace     *bool  `toml:"show_last_workspace,omitempty"`
	ShowLastWorkspacePath *bool  `toml:"show_last_workspace_path,omitempty"`
	Prompt                string `toml:"prompt,omitempty"`
	Placeholder           string `toml:"placeholder,omitempty"`
	SeparatorAware        bool   `toml:"separator_aware,omitempty"`
	WorkspaceSort         string `toml:"workspace_sort,omitempty"`
}

type nativeDefaults struct {
	Startup string `toml:"startup,omitempty"`
	Preview string `toml:"preview,omitempty"`
}

type nativeTab struct {
	Name    string `toml:"name"`
	Startup string `toml:"startup,omitempty"`
	Path    string `toml:"path,omitempty"`
}

type nativeWorkspace struct {
	Name           string   `toml:"name"`
	Path           string   `toml:"path,omitempty"`
	Startup        string   `toml:"startup,omitempty"`
	Preview        string   `toml:"preview,omitempty"`
	DisableStartup *bool    `toml:"disable_startup,omitempty"`
	Tabs           []string `toml:"tabs,omitempty"`
}

type nativeRule struct {
	PathGlob       string   `toml:"path_glob"`
	Startup        string   `toml:"startup,omitempty"`
	Preview        string   `toml:"preview,omitempty"`
	DisableStartup bool     `toml:"disable_startup,omitempty"`
	Tabs           []string `toml:"tabs,omitempty"`
}

// hasVersionKey reports whether the document carries a top-level version key of
// any type, which selects native decoding for explicitly chosen paths.
func hasVersionKey(data []byte) bool {
	var probe struct {
		Version any `toml:"version"`
	}
	_ = toml.Unmarshal(data, &probe)
	return probe.Version != nil
}

func decodeNative(path string, data []byte) (Config, error) {
	cfg := Default()
	dec := toml.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var n nativeConfig
	if err := dec.Decode(&n); err != nil {
		var missing *toml.StrictMissingError
		if errors.As(err, &missing) {
			return cfg, fmt.Errorf("load %s: unknown keys:\n%s", path, missing.String())
		}
		return cfg, fmt.Errorf("load %s: %w", path, err)
	}
	if n.Version != NativeVersion {
		return cfg, fmt.Errorf("load %s: version: must be %d, got %d", path, NativeVersion, n.Version)
	}
	if err := n.validate(path); err != nil {
		return cfg, err
	}
	n.apply(&cfg)
	return cfg, nil
}

var knownSources = map[string]bool{"herdr": true, "config": true, "zoxide": true, "dir": true}

func (n nativeConfig) validate(path string) error {
	fail := func(key, format string, args ...any) error {
		return fmt.Errorf("load %s: %s: %s", path, key, fmt.Sprintf(format, args...))
	}
	if n.Naming.PathComponents != nil && *n.Naming.PathComponents < 1 {
		return fail("naming.path_components", "must be at least 1")
	}
	if s := n.Picker.WorkspaceSort; s != "" && s != "workspace" && s != "recent" && s != "agent" {
		return fail("picker.workspace_sort", "must be \"workspace\" or \"recent\" or \"agent\", got %q", s)
	}
	if mode := n.Picker.PreviewMode; mode != "" && mode != "command" && mode != "pane" {
		return fail("picker.preview_mode", "must be \"command\" or \"pane\", got %q", mode)
	}
	if binding := n.Keys.CyclePreviewMode; !validCyclePreviewKey(binding) {
		return fail("keys.cycle_preview_mode", "unsupported key %q; use Bubble Tea key names such as ctrl+o, alt+p, or f2, or an empty string to disable; for shifted printable keys use the resulting character (e.g. P instead of shift+p), and with other modifiers use the unshifted base key (e.g. ctrl+shift+p or alt+shift+/)", *binding)
	}
	seenSources := map[string]bool{}
	for _, s := range n.List.SourceOrder {
		if !knownSources[s] {
			return fail("list.source_order", "unknown source %q", s)
		}
		if seenSources[s] {
			return fail("list.source_order", "duplicate source %q", s)
		}
		seenSources[s] = true
	}
	for _, expr := range n.List.Blacklist {
		if _, err := regexp.Compile(expr); err != nil {
			return fail("list.blacklist", "invalid regex %q: %v", expr, err)
		}
	}
	tabNames := map[string]bool{}
	for _, t := range n.Tabs {
		if t.Name == "" {
			return fail("tab.name", "must not be empty")
		}
		if tabNames[t.Name] {
			return fail("tab.name", "duplicate tab %q", t.Name)
		}
		tabNames[t.Name] = true
	}
	workspaceNames := map[string]bool{}
	for _, w := range n.Workspaces {
		if w.Name == "" {
			return fail("workspace.name", "must not be empty")
		}
		if w.Path == "" {
			return fail("workspace.path", "must not be empty for workspace %q", w.Name)
		}
		if workspaceNames[w.Name] {
			return fail("workspace.name", "duplicate workspace %q", w.Name)
		}
		workspaceNames[w.Name] = true
		for _, ref := range w.Tabs {
			if !tabNames[ref] {
				return fail("workspace.tabs", "workspace %q references unknown tab %q", w.Name, ref)
			}
		}
	}
	for _, r := range n.Rules {
		if r.PathGlob == "" {
			return fail("rule.path_glob", "must not be empty")
		}
		glob := strings.TrimSuffix(r.PathGlob, "/**")
		if _, err := filepath.Match(glob, ""); err != nil {
			return fail("rule.path_glob", "invalid glob %q: %v", r.PathGlob, err)
		}
		for _, ref := range r.Tabs {
			if !tabNames[ref] {
				return fail("rule.tabs", "rule %q references unknown tab %q", r.PathGlob, ref)
			}
		}
	}
	return nil
}

// apply converts the validated native document onto a Default()-initialized
// runtime Config so downstream consumers see the same shape as legacy loads.
func (n nativeConfig) apply(cfg *Config) {
	if n.Keys.CyclePreviewMode != nil {
		cfg.Keys.CyclePreviewMode = *n.Keys.CyclePreviewMode
	}
	cfg.Cache = n.List.Cache
	cfg.Blacklist = n.List.Blacklist
	if len(n.List.SourceOrder) > 0 {
		cfg.SortOrder = n.List.SourceOrder
	}
	if n.Naming.PathComponents != nil {
		cfg.DirLength = *n.Naming.PathComponents
	}
	cfg.SeparatorAware = n.Picker.SeparatorAware
	cfg.TUI.ShowIcons = n.Picker.ShowIcons
	if n.Picker.ShowPreview != nil {
		cfg.TUI.ShowPreview = *n.Picker.ShowPreview
	}
	if n.Picker.PreviewMode != "" {
		cfg.TUI.PreviewMode = n.Picker.PreviewMode
	}
	if n.Picker.PrioritizeHome != nil {
		cfg.TUI.PrioritizeHome = *n.Picker.PrioritizeHome
	}
	if n.Picker.HerdrThemeInherit != nil {
		cfg.TUI.HerdrThemeInherit = *n.Picker.HerdrThemeInherit
	}
	if n.Picker.ReplaceWorktreeIcon != nil {
		cfg.TUI.ReplaceWorktreeIcon = *n.Picker.ReplaceWorktreeIcon
	}
	if n.Picker.ShowLastWorkspace != nil {
		cfg.TUI.ShowLastWorkspace = *n.Picker.ShowLastWorkspace
	}
	if n.Picker.ShowLastWorkspacePath != nil {
		cfg.TUI.ShowLastWorkspacePath = *n.Picker.ShowLastWorkspacePath
	}
	cfg.TUI.Prompt = n.Picker.Prompt
	cfg.TUI.Placeholder = n.Picker.Placeholder
	if n.Picker.WorkspaceSort != "" {
		cfg.TUI.DefaultSort = n.Picker.WorkspaceSort
	}
	cfg.DefaultSessionConfig.StartupCommand = n.WorkspaceDefaults.Startup
	if n.WorkspaceDefaults.Preview != "" {
		cfg.DefaultSessionConfig.PreviewCommand = n.WorkspaceDefaults.Preview
	}
	for _, t := range n.Tabs {
		cfg.WindowConfigs = append(cfg.WindowConfigs, model.WindowConfig{Name: t.Name, StartupScript: t.Startup, Path: t.Path})
	}
	for _, w := range n.Workspaces {
		cfg.SessionConfigs = append(cfg.SessionConfigs, SessionConfig{
			DefaultSessionConfig: DefaultSessionConfig{StartupCommand: w.Startup, PreviewCommand: w.Preview},
			Name:                 w.Name,
			Path:                 w.Path,
			DisableStartCommand:  w.DisableStartup,
			Windows:              w.Tabs,
		})
	}
	for _, r := range n.Rules {
		cfg.WildcardConfigs = append(cfg.WildcardConfigs, WildcardConfig{
			Pattern:             r.PathGlob,
			StartupCommand:      r.Startup,
			PreviewCommand:      r.Preview,
			DisableStartCommand: r.DisableStartup,
			Windows:             r.Tabs,
		})
	}
}

// Keep UI dependencies out of config: their initialization runs in every
// non-UI consumer too. TestCyclePreviewKeyNamesMatchBubbleTea checks this list.
const cyclePreviewKeyNames = `enter tab backspace esc space up down left right
begin find insert delete select pgup pgdown home end equal mul plus comma minus
period div sep capslock scrolllock numlock printscreen pause menu mediaplay
mediapause mediaplaypause mediareverse mediastop mediafastforward mediarewind
medianext mediaprev mediarecord lowervol raisevol mute leftshift leftalt leftctrl
leftsuper lefthyper leftmeta rightshift rightalt rightctrl rightsuper righthyper
rightmeta isolevel3shift isolevel5shift`

func validCyclePreviewKey(configured *string) bool {
	if configured == nil || *configured == "" {
		return true
	}
	name := *configured
	shifted := false
	for _, modifier := range []string{"ctrl", "alt", "shift", "meta", "hyper", "super"} {
		if rest, ok := strings.CutPrefix(name, modifier+"+"); ok {
			name = rest
			shifted = shifted || modifier == "shift"
			// Bubble Tea omits the modifier when it is itself the pressed key.
			if strings.HasSuffix(name, "+left"+modifier) || strings.HasSuffix(name, "+right"+modifier) || name == "left"+modifier || name == "right"+modifier {
				return false
			}
		}
	}
	if code, size := utf8.DecodeRuneInString(name); size == len(name) && unicode.IsPrint(code) {
		// Explicit Shift combinations use the unshifted PC-101 base key.
		if shifted && (unicode.IsUpper(code) || strings.ContainsRune("~!@#$%^&*()_+{}|:\"<>?", code)) {
			return false
		}
		// Shift-only printable events carry their resulting text, not shift+key.
		return size > 0 && code != ' ' && *configured != "shift+"+name
	}
	if number, ok := strings.CutPrefix(name, "f"); ok {
		n, err := strconv.Atoi(number)
		if err == nil {
			return n >= 1 && n <= 63 && strconv.Itoa(n) == number
		}
	}
	for _, key := range strings.Fields(cyclePreviewKeyNames) {
		if name == key {
			return true
		}
	}
	return false
}

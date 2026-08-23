package config

import "github.com/fullerzz/herdr-plugin-sesh/internal/model"

const (
	// Preview output is captured through sh rather than a TTY, so eza's
	// automatic modes must be forced on explicitly.
	DefaultPreviewCommand = "eza --icons=always --color=always -la {}"
	DefaultWorkspaceSort  = "workspace"
)

type Config struct {
	Cache                bool                 `toml:"cache"`
	StrictMode           bool                 `toml:"strict_mode"`
	ImportPaths          []string             `toml:"import"`
	DefaultSessionConfig DefaultSessionConfig `toml:"default_session"`
	Blacklist            []string             `toml:"blacklist"`
	SessionConfigs       []SessionConfig      `toml:"session"`
	SortOrder            []string             `toml:"sort_order"`
	WindowConfigs        []model.WindowConfig `toml:"window"`
	WildcardConfigs      []WildcardConfig     `toml:"wildcard"`
	DirLength            int                  `toml:"dir_length"`
	SeparatorAware       bool                 `toml:"separator_aware"`
	TUI                  TUIConfig            `toml:"tui"`
}

type DefaultSessionConfig struct {
	StartupCommand string `toml:"startup_command"`
	PreviewCommand string `toml:"preview_command"`
}

type SessionConfig struct {
	DefaultSessionConfig

	Name                string   `toml:"name"`
	Path                string   `toml:"path"`
	DisableStartCommand *bool    `toml:"disable_startup_command"`
	Windows             []string `toml:"windows"`
}

type TUIConfig struct {
	ShowIcons             bool   `toml:"show_icons"`
	HerdrThemeInherit     bool   `toml:"herdr_theme_inherit"`
	ShowLastWorkspace     bool   `toml:"show_last_workspace"`
	ShowLastWorkspacePath bool   `toml:"show_last_workspace_path"`
	Prompt                string `toml:"prompt"`
	Placeholder           string `toml:"placeholder"`
	DefaultSort           string `toml:"default_sort"`
}

type WildcardConfig struct {
	Pattern             string   `toml:"pattern"`
	StartupCommand      string   `toml:"startup_command"`
	DisableStartCommand bool     `toml:"disable_startup_command"`
	PreviewCommand      string   `toml:"preview_command"`
	Windows             []string `toml:"windows"`
}

func Default() Config {
	return Config{
		DirLength:            1,
		SortOrder:            []string{"herdr", "config", "zoxide", "dir"},
		DefaultSessionConfig: DefaultSessionConfig{PreviewCommand: DefaultPreviewCommand},
		TUI: TUIConfig{
			DefaultSort:           DefaultWorkspaceSort,
			HerdrThemeInherit:     true,
			ShowLastWorkspace:     true,
			ShowLastWorkspacePath: true,
		},
	}
}

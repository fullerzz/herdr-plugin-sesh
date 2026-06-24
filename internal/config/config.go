package config

import "forgejo.local/fullerzz/herdr-plugin-sesh/internal/model"

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
	TmuxCommand          string               `toml:"tmux_command"`
	TUI                  TUIConfig            `toml:"tui"`
}

type DefaultSessionConfig struct {
	StartupCommand string   `toml:"startup_command"`
	Tmuxp          string   `toml:"tmuxp"`
	Tmuxinator     string   `toml:"tmuxinator"`
	PreviewCommand string   `toml:"preview_command"`
	Windows        []string `toml:"windows"`
}

type SessionConfig struct {
	DefaultSessionConfig

	Name                string `toml:"name"`
	Path                string `toml:"path"`
	DisableStartCommand bool   `toml:"disable_startup_command"`
}

type TUIConfig struct {
	ShowIcons   bool   `toml:"show_icons"`
	Prompt      string `toml:"prompt"`
	Placeholder string `toml:"placeholder"`
}

type WildcardConfig struct {
	Pattern             string   `toml:"pattern"`
	StartupCommand      string   `toml:"startup_command"`
	DisableStartCommand bool     `toml:"disable_startup_command"`
	PreviewCommand      string   `toml:"preview_command"`
	Windows             []string `toml:"windows"`
}

func Default() Config {
	return Config{DirLength: 1, SortOrder: []string{"herdr", "config", "zoxide", "dir"}}
}

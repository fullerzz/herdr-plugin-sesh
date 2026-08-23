package picker

import (
	"image/color"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
	toml "github.com/pelletier/go-toml/v2"
)

var herdrTokenRoles = map[string]*color.Color{
	"text":     &textColor,
	"subtext0": &mutedColor,
	"green":    &greenColor,
	"yellow":   &amberColor,
	"red":      &redColor,
	"accent":   &skyColor,
	"mauve":    &violetColor,
	"overlay1": &ghostColor,
}

// herdrThemePalettes mirrors Herdr's built-in palettes (herdrdev/herdr
// src/app/state.rs) for the tokens the picker consumes.
var herdrThemePalettes = map[string]map[string]string{
	"catppuccin": {
		"accent":   "#89b4fa",
		"mauve":    "#cba6f7",
		"text":     "#cdd6f4",
		"subtext0": "#a6adc8",
		"green":    "#a6e3a1",
		"yellow":   "#f9e2af",
		"red":      "#f38ba8",
		"overlay1": "#7f849c",
	},
	"catppuccin-latte": {
		"accent":   "#1e66f5",
		"mauve":    "#8839ef",
		"text":     "#4c4f69",
		"subtext0": "#6c6f85",
		"green":    "#40a02b",
		"yellow":   "#df8e1d",
		"red":      "#d20f39",
		"overlay1": "#8c8fa1",
	},
	"tokyo-night": {
		"accent":   "#7aa2f7",
		"mauve":    "#bb9af7",
		"text":     "#c0caf5",
		"subtext0": "#a9b1d6",
		"green":    "#9ece6a",
		"yellow":   "#e0af68",
		"red":      "#f7768e",
		"overlay1": "#697196",
	},
	"tokyo-night-day": {
		"accent":   "#2e7de9",
		"mauve":    "#7847bd",
		"text":     "#3760bf",
		"subtext0": "#6172b0",
		"green":    "#587539",
		"yellow":   "#8c6c3e",
		"red":      "#f52a65",
		"overlay1": "#68709a",
	},
	"dracula": {
		"accent":   "#bd93f9",
		"mauve":    "#ff79c6",
		"text":     "#f8f8f2",
		"subtext0": "#d2d2dc",
		"green":    "#50fa7b",
		"yellow":   "#f1fa8c",
		"red":      "#ff5555",
		"overlay1": "#828cb4",
	},
	"nord": {
		"accent":   "#88c0d0",
		"mauve":    "#b48ead",
		"text":     "#eceff4",
		"subtext0": "#d8dee9",
		"green":    "#a3be8c",
		"yellow":   "#ebcb8b",
		"red":      "#bf616a",
		"overlay1": "#646e82",
	},
	"gruvbox": {
		"accent":   "#d79921",
		"mauve":    "#d3869b",
		"text":     "#ebdbb2",
		"subtext0": "#d5c4a1",
		"green":    "#b8bb26",
		"yellow":   "#fabd2f",
		"red":      "#fb4934",
		"overlay1": "#a89984",
	},
	"gruvbox-light": {
		"accent":   "#076678",
		"mauve":    "#8f3f71",
		"text":     "#3c3836",
		"subtext0": "#504945",
		"green":    "#79740e",
		"yellow":   "#b57614",
		"red":      "#9d0006",
		"overlay1": "#7c6f64",
	},
	"one-dark": {
		"accent":   "#61afef",
		"mauve":    "#c678dd",
		"text":     "#abb2bf",
		"subtext0": "#969ca8",
		"green":    "#98c379",
		"yellow":   "#e5c07b",
		"red":      "#e06c75",
		"overlay1": "#737a87",
	},
	"one-light": {
		"accent":   "#4078f2",
		"mauve":    "#a626a4",
		"text":     "#383a42",
		"subtext0": "#686b77",
		"green":    "#50a14f",
		"yellow":   "#c18401",
		"red":      "#e45649",
		"overlay1": "#686b77",
	},
	"solarized": {
		"accent":   "#268bd2",
		"mauve":    "#d33682",
		"text":     "#93a1a1",
		"subtext0": "#839496",
		"green":    "#859900",
		"yellow":   "#b58900",
		"red":      "#dc322f",
		"overlay1": "#657b83",
	},
	"solarized-light": {
		"accent":   "#268bd2",
		"mauve":    "#d33682",
		"text":     "#657b83",
		"subtext0": "#839496",
		"green":    "#859900",
		"yellow":   "#b58900",
		"red":      "#dc322f",
		"overlay1": "#586e75",
	},
	"kanagawa": {
		"accent":   "#7e9cd8",
		"mauve":    "#957fb8",
		"text":     "#dcd7ba",
		"subtext0": "#c8c3aa",
		"green":    "#76946a",
		"yellow":   "#c0a36e",
		"red":      "#c34043",
		"overlay1": "#87867d",
	},
	"kanagawa-lotus": {
		"accent":   "#4d699b",
		"mauve":    "#624c83",
		"text":     "#545464",
		"subtext0": "#43436c",
		"green":    "#6f894e",
		"yellow":   "#77713f",
		"red":      "#c84053",
		"overlay1": "#8a8980",
	},
	"rose-pine": {
		"accent":   "#c4a7e7",
		"mauve":    "#c4a7e7",
		"text":     "#e0def4",
		"subtext0": "#c8c5dc",
		"green":    "#31748f",
		"yellow":   "#f6c177",
		"red":      "#eb6f92",
		"overlay1": "#908caa",
	},
	"rose-pine-dawn": {
		"accent":   "#907aa9",
		"mauve":    "#907aa9",
		"text":     "#464261",
		"subtext0": "#797593",
		"green":    "#286983",
		"yellow":   "#ea9d34",
		"red":      "#b4637a",
		"overlay1": "#797593",
	},
	"vesper": {
		"accent":   "#ffc799",
		"mauve":    "#ffd1a8",
		"text":     "#ffffff",
		"subtext0": "#a0a0a0",
		"green":    "#99ffe4",
		"yellow":   "#ffc799",
		"red":      "#ff8080",
		"overlay1": "#7e7e7e",
	},
}

const defaultHerdrTheme = "catppuccin"

var herdrThemeAliases = map[string]string{
	"catppuccin":       "catppuccin",
	"catppuccin-mocha": "catppuccin",
	"latte":            "catppuccin-latte",
	"light":            "catppuccin-latte",
	"terminal":         "terminal",
	"tokyo-night":      "tokyo-night",
	"tokyonight":       "tokyo-night",
	"tokyo-night-day":  "tokyo-night-day",
	"tokyo-day":        "tokyo-night-day",
	"tokyonight-day":   "tokyo-night-day",
	"dracula":          "dracula",
	"nord":             "nord",
	"gruvbox":          "gruvbox",
	"gruvbox-dark":     "gruvbox",
	"gruvbox-light":    "gruvbox-light",
	"one-dark":         "one-dark",
	"onedark":          "one-dark",
	"one-light":        "one-light",
	"onelight":         "one-light",
	"solarized":        "solarized",
	"solarized-dark":   "solarized",
	"solarized-light":  "solarized-light",
	"kanagawa":         "kanagawa",
	"kanagawa-lotus":   "kanagawa-lotus",
	"lotus":            "kanagawa-lotus",
	"rose-pine":        "rose-pine",
	"rosepine":         "rose-pine",
	"rose-pine-dawn":   "rose-pine-dawn",
	"rosepine-dawn":    "rose-pine-dawn",
	"dawn":             "rose-pine-dawn",
	"vesper":           "vesper",
}

var hexColorPattern = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

type herdrThemeConfig struct {
	Theme struct {
		Name   string            `toml:"name"`
		Custom map[string]string `toml:"custom"`
	} `toml:"theme"`
}

func herdrConfigPath() string {
	if path := os.Getenv("HERDR_CONFIG_PATH"); path != "" {
		return path
	}
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		configDir = filepath.Join(home, ".config")
	}
	return filepath.Join(configDir, "herdr", "config.toml")
}

func loadHerdrThemeConfig(path string) (string, map[string]string) {
	if path == "" {
		return "", nil
	}
	//nolint:gosec // the config path is user-selected by design.
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil
	}
	var parsed herdrThemeConfig
	if err := toml.Unmarshal(data, &parsed); err != nil {
		return "", nil
	}
	return parsed.Theme.Name, parsed.Theme.Custom
}

func validHexColor(value string) bool {
	return hexColorPattern.MatchString(value)
}

func resolveHerdrThemeTokens(name string, custom map[string]string) map[string]string {
	tokens := map[string]string{}
	switch key := herdrThemeAliases[canonicalHerdrThemeName(name)]; {
	case key == "terminal":
	case key != "":
		for token, value := range herdrThemePalettes[key] {
			tokens[token] = value
		}
	default:
		for token, value := range herdrThemePalettes[defaultHerdrTheme] {
			tokens[token] = value
		}
	}
	for token, value := range custom {
		if value == "" {
			continue
		}
		tokens[token] = value
	}
	return tokens
}

func canonicalHerdrThemeName(raw string) string {
	name := strings.ToLower(strings.TrimSpace(raw))
	name = strings.ReplaceAll(name, " ", "-")
	return strings.ReplaceAll(name, "_", "-")
}

func rebuildPickerStyles() {
	titleStyle = lipgloss.NewStyle().
		Foreground(violetColor).
		Bold(true)

	countStyle = lipgloss.NewStyle().
		Foreground(mutedColor)

	sectionStyle = lipgloss.NewStyle().
		Foreground(violetColor).
		Bold(true)

	ruleStyle = lipgloss.NewStyle().
		Foreground(mutedColor)

	rowLabelStyle = lipgloss.NewStyle().
		Foreground(textColor)

	selectedLabelStyle = rowLabelStyle.Bold(true)

	matchStyle = lipgloss.NewStyle().
		Foreground(violetColor).
		Bold(true)

	selectionRailStyle = lipgloss.NewStyle().
		Foreground(skyColor).
		Bold(true)

	smearTrailStyle = lipgloss.NewStyle().
		Foreground(violetColor)

	pathStyle = lipgloss.NewStyle().
		Foreground(mutedColor)

	emptyStyle = lipgloss.NewStyle().
		Foreground(amberColor)

	moreStyle = lipgloss.NewStyle().
		Foreground(mutedColor)

	helpStyle = lipgloss.NewStyle().
		Foreground(mutedColor)
}

func applyHerdrTheme(tokens map[string]string) {
	if len(tokens) == 0 {
		return
	}
	for token, value := range tokens {
		target, ok := herdrTokenRoles[token]
		if !ok || !validHexColor(value) {
			continue
		}
		*target = lipgloss.Color(value)
	}
	rebuildPickerStyles()
}

func ApplyHerdrThemeFromConfig() {
	name, custom := loadHerdrThemeConfig(herdrConfigPath())
	applyHerdrTheme(resolveHerdrThemeTokens(name, custom))
}

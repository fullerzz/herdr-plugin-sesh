package picker

import (
	"image/color"
	"os"
	"path/filepath"
	"regexp"

	"charm.land/lipgloss/v2"
	toml "github.com/pelletier/go-toml/v2"
)

// herdrTokenRoles maps Herdr [theme.custom] token names to the picker color
// roles they feed. Tokens that are absent or invalid leave the built-in
// default in place.
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

var hexColorPattern = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

type herdrThemeConfig struct {
	Theme struct {
		Custom map[string]string `toml:"custom"`
	} `toml:"theme"`
}

// herdrConfigPath mirrors Herdr's own lookup order for its config file.
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

// loadHerdrThemeCustom reads the [theme.custom] token table from a Herdr
// config file. A missing file, missing table, or unparsable file yields nil.
func loadHerdrThemeCustom(path string) map[string]string {
	if path == "" {
		return nil
	}
	//nolint:gosec // the config path is user-selected by design.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var parsed herdrThemeConfig
	if err := toml.Unmarshal(data, &parsed); err != nil {
		return nil
	}
	return parsed.Theme.Custom
}

// validHexColor reports whether value is a #RGB or #RRGGBB hex color.
func validHexColor(value string) bool {
	return hexColorPattern.MatchString(value)
}

// rebuildPickerStyles recreates every style that captured a color variable at
// package initialization time.
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

// applyHerdrTheme overrides the picker palette from Herdr [theme.custom]
// tokens and refreshes the dependent styles. Unknown or invalid tokens are
// ignored, so partial tables only affect the roles they define.
func applyHerdrTheme(custom map[string]string) {
	if len(custom) == 0 {
		return
	}
	for token, value := range custom {
		target, ok := herdrTokenRoles[token]
		if !ok || !validHexColor(value) {
			continue
		}
		*target = lipgloss.Color(value)
	}
	rebuildPickerStyles()
}

// ApplyHerdrThemeFromConfig loads Herdr's own config file and applies its
// [theme.custom] tokens to the picker palette. A missing or malformed file
// leaves the built-in palette untouched.
func ApplyHerdrThemeFromConfig() {
	applyHerdrTheme(loadHerdrThemeCustom(herdrConfigPath()))
}

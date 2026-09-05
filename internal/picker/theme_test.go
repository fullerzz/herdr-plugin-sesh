package picker

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidHexColor(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "six digit hex", value: "#7FA563", want: true},
		{name: "three digit hex", value: "#abc", want: true},
		{name: "uppercase hex", value: "#D8647E", want: true},
		{name: "missing hash", value: "7FA563", want: false},
		{name: "too short", value: "#ab", want: false},
		{name: "too long", value: "#aabbcce", want: false},
		{name: "named color", value: "red", want: false},
		{name: "empty", value: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, validHexColor(tt.value))
		})
	}
}

func TestHerdrConfigPath(t *testing.T) {
	t.Run("HERDR_CONFIG_PATH wins", func(t *testing.T) {
		t.Setenv("HERDR_CONFIG_PATH", "/custom/herdr.toml")
		t.Setenv("XDG_CONFIG_HOME", "/xdg")
		assert.Equal(t, "/custom/herdr.toml", herdrConfigPath())
	})

	t.Run("XDG_CONFIG_HOME fallback", func(t *testing.T) {
		t.Setenv("HERDR_CONFIG_PATH", "")
		t.Setenv("XDG_CONFIG_HOME", "/xdg")
		want := filepath.Join("/xdg", "herdr", "config.toml")
		assert.Equal(t, want, herdrConfigPath())
	})

	t.Run("home config fallback", func(t *testing.T) {
		t.Setenv("HERDR_CONFIG_PATH", "")
		t.Setenv("XDG_CONFIG_HOME", "")
		got := herdrConfigPath()
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skipf("no home directory: %v", err)
		}
		want := filepath.Join(home, ".config", "herdr", "config.toml")
		assert.Equal(t, want, got)
	})
}

func TestLoadHerdrThemeConfig(t *testing.T) {
	t.Run("reads name and custom tokens, ignores unrelated tables", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.toml")
		content := `
[theme]
name = "catppuccin"

[theme.custom]
text = "#cdcdcd"

[[keys.command]]
key = "prefix+t"
type = "plugin_action"
command = "fullerzz.sesh.open-picker"
`
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

		name, custom := loadHerdrThemeConfig(path)
		assert.Equal(t, "catppuccin", name)
		want := map[string]string{"text": "#cdcdcd"}
		assert.Equal(t, want, custom)
	})

	t.Run("missing file yields empty results", func(t *testing.T) {
		name, custom := loadHerdrThemeConfig(filepath.Join(t.TempDir(), "absent.toml"))
		assert.Empty(t, name)
		assert.Nil(t, custom)
	})

	t.Run("invalid TOML yields empty results", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.toml")
		require.NoError(t, os.WriteFile(path, []byte("[theme"), 0o600))
		name, custom := loadHerdrThemeConfig(path)
		assert.Empty(t, name)
		assert.Nil(t, custom)
	})
}

func TestApplyHerdrTheme(t *testing.T) {
	saved := []struct {
		target *color.Color
		value  color.Color
	}{
		{&textColor, textColor},
		{&mutedColor, mutedColor},
		{&greenColor, greenColor},
		{&amberColor, amberColor},
		{&redColor, redColor},
		{&skyColor, skyColor},
		{&violetColor, violetColor},
		{&ghostColor, ghostColor},
	}
	t.Cleanup(func() {
		for _, s := range saved {
			*s.target = s.value
		}
		rebuildPickerStyles()
	})

	t.Run("overrides valid tokens and skips the rest", func(t *testing.T) {
		applyHerdrTheme(map[string]string{
			"text":   "#112233",
			"accent": "#445566",
			"red":    "not-a-color",
			"bogus":  "#abcdef",
		})

		assert.Equal(t, lipgloss.Color("#112233"), textColor)
		assert.Equal(t, lipgloss.Color("#445566"), skyColor)
		assert.Equal(t, saved[4].value, redColor)
		assert.Equal(t, saved[6].value, violetColor)

		assert.Equal(t, lipgloss.Color("#112233"), rowLabelStyle.GetForeground())
		assert.Equal(t, lipgloss.Color("#112233"), selectedLabelStyle.GetForeground())
		assert.Equal(t, lipgloss.Color("#445566"), selectionRailStyle.GetForeground())
	})

	t.Run("empty table is a no-op", func(t *testing.T) {
		before := textColor
		applyHerdrTheme(map[string]string{})
		assert.Equal(t, before, textColor)
	})
}

func TestResolveHerdrThemeTokens(t *testing.T) {
	tests := []struct {
		name        string
		rawName     string
		custom      map[string]string
		contains    map[string]string
		notContains []string
	}{
		{
			name:    "named theme resolves its palette",
			rawName: "dracula",
			contains: map[string]string{
				"accent": "#bd93f9",
				"red":    "#ff5555",
				"text":   "#f8f8f2",
			},
		},
		{
			name:    "aliases, casing, and spacing normalize like Herdr",
			rawName: "Tokyo Night",
			contains: map[string]string{
				"accent": "#7aa2f7",
				"mauve":  "#bb9af7",
			},
		},
		{
			name:    "unknown names fall back to the default theme",
			rawName: "not-a-theme",
			contains: map[string]string{
				"accent": "#89b4fa",
			},
		},
		{
			name:     "unset name resolves to the default theme",
			contains: map[string]string{"text": "#cdd6f4"},
		},
		{
			name:    "custom tokens override the base palette",
			rawName: "nord",
			custom:  map[string]string{"accent": "#112233", "bogus-token": "#abcdef"},
			contains: map[string]string{
				"accent":      "#112233",
				"text":        "#eceff4",
				"bogus-token": "#abcdef",
			},
		},
		{
			name:    "canonical light name resolves its own palette",
			rawName: "catppuccin-latte",
			contains: map[string]string{
				"accent": "#1e66f5",
				"text":   "#4c4f69",
			},
		},
		{
			name:    "invalid custom tokens fall back to the base palette",
			rawName: "nord",
			custom:  map[string]string{"accent": "#112233", "yellow": "bogus"},
			contains: map[string]string{
				"accent": "#112233",
				"yellow": "#ebcb8b",
			},
		},
		{
			name:        "terminal theme has no hex base",
			rawName:     "terminal",
			custom:      map[string]string{"green": "#446688"},
			contains:    map[string]string{"green": "#446688"},
			notContains: []string{"accent", "text"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveHerdrThemeTokens(tt.rawName, tt.custom)
			for token, want := range tt.contains {
				assert.Equal(t, want, got[token])
			}
			for _, token := range tt.notContains {
				value, ok := got[token]
				assert.Falsef(t, ok, "token %q = %q, want absent", token, value)
			}
		})
	}
}

func TestHerdrThemePalettesAreComplete(t *testing.T) {
	for name, palette := range herdrThemePalettes {
		assert.Len(t, palette, len(herdrTokenRoles), name)
		for token, value := range palette {
			assert.Truef(t, validHexColor(value), "palette %q token %q = %q", name, token, value)
			_, ok := herdrTokenRoles[token]
			assert.Truef(t, ok, "palette %q token %q is not a picker role", name, token)
		}
	}
}

func TestConfigureHerdrThemeResetsColorsBetweenRuns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	content := "[theme]\nname = \"dracula\"\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	t.Setenv("HERDR_CONFIG_PATH", path)

	saved := []struct {
		target *color.Color
		value  color.Color
	}{
		{&textColor, textColor},
		{&mutedColor, mutedColor},
		{&greenColor, greenColor},
		{&amberColor, amberColor},
		{&redColor, redColor},
		{&skyColor, skyColor},
		{&violetColor, violetColor},
		{&ghostColor, ghostColor},
	}
	t.Cleanup(func() {
		for _, s := range saved {
			*s.target = s.value
		}
		rebuildPickerStyles()
	})

	configureHerdrTheme(true)
	require.Equal(t, lipgloss.Color("#bd93f9"), skyColor)

	configureHerdrTheme(false)

	colors := []struct {
		name string
		got  color.Color
		want color.Color
	}{
		{name: "text", got: textColor, want: lipgloss.Color("#C0CAF5")},
		{name: "muted", got: mutedColor, want: lipgloss.Color("#565F89")},
		{name: "green", got: greenColor, want: lipgloss.Color("#9ECE6A")},
		{name: "amber", got: amberColor, want: lipgloss.Color("#E0AF68")},
		{name: "red", got: redColor, want: lipgloss.Color("#F7768E")},
		{name: "sky", got: skyColor, want: lipgloss.Color("#7DCFFF")},
		{name: "violet", got: violetColor, want: lipgloss.Color("#BB9AF7")},
		{name: "ghost", got: ghostColor, want: lipgloss.Color("#737AA2")},
	}
	for _, c := range colors {
		assert.Equal(t, c.want, c.got)
	}

	styles := []struct {
		name string
		got  color.Color
		want color.Color
	}{
		{name: "title", got: titleStyle.GetForeground(), want: lipgloss.Color("#BB9AF7")},
		{name: "count", got: countStyle.GetForeground(), want: lipgloss.Color("#565F89")},
		{name: "row label", got: rowLabelStyle.GetForeground(), want: lipgloss.Color("#C0CAF5")},
		{name: "selection rail", got: selectionRailStyle.GetForeground(), want: lipgloss.Color("#7DCFFF")},
		{name: "empty", got: emptyStyle.GetForeground(), want: lipgloss.Color("#E0AF68")},
	}
	for _, s := range styles {
		assert.Equal(t, s.want, s.got)
	}
}

func TestRebuildPickerStylesTracksColorVars(t *testing.T) {
	original := violetColor
	t.Cleanup(func() {
		violetColor = original
		rebuildPickerStyles()
	})

	violetColor = lipgloss.Color("#010203")
	rebuildPickerStyles()

	assert.Equal(t, lipgloss.Color("#010203"), titleStyle.GetForeground())
	assert.Equal(t, lipgloss.Color("#010203"), smearTrailStyle.GetForeground())
}

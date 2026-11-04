package uitheme

import (
	"os"
	"path/filepath"
	"testing"

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
			assert.Equal(t, tt.want, ValidHexColor(tt.value))
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

		got := loadHerdrThemeConfig(path)
		assert.Equal(t, "catppuccin", got.Name)
		assert.Equal(t, map[string]string{"text": "#cdcdcd"}, stringTokens(got.Custom))
	})

	t.Run("reads auto_switch names and mode override tables", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.toml")
		content := `
[theme]
name = "nord"
auto_switch = true
dark_name = "dracula"
light_name = "one-light"

[theme.custom]
text = "#cdcdcd"

[theme.custom.light]
accent = "#123456"
`
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

		got := loadHerdrThemeConfig(path)
		assert.True(t, got.AutoSwitch)
		assert.Equal(t, "dracula", got.DarkName)
		assert.Equal(t, "one-light", got.LightName)
		assert.Equal(t, map[string]string{"text": "#cdcdcd"}, stringTokens(got.Custom))
		assert.Equal(t, map[string]string{"accent": "#123456"}, stringTokens(got.Custom["light"]))
	})

	t.Run("missing file yields empty results", func(t *testing.T) {
		assert.Zero(t, loadHerdrThemeConfig(filepath.Join(t.TempDir(), "absent.toml")))
	})

	t.Run("invalid TOML yields empty results", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.toml")
		require.NoError(t, os.WriteFile(path, []byte("[theme"), 0o600))
		assert.Zero(t, loadHerdrThemeConfig(path))
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
			got := resolveHerdrThemeTokens(tt.rawName, defaultHerdrTheme, tt.custom)
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

func TestHerdrThemeTokensFollowAutoSwitch(t *testing.T) {
	theme := herdrThemeConfig{
		Name:       "nord",
		AutoSwitch: true,
		DarkName:   "dracula",
		LightName:  "one-light",
		Custom: map[string]any{
			"text":  "#cdcdcd",
			"light": map[string]any{"accent": "#123456"},
			"dark":  map[string]any{"accent": "#654321"},
		},
	}

	dark := theme.tokens(true)
	assert.Equal(t, "#ff5555", dark["red"], "dark appearance uses dark_name")
	assert.Equal(t, "#654321", dark["accent"], "dark mode overrides apply")
	assert.Equal(t, "#cdcdcd", dark["text"], "flat overrides apply")

	light := theme.tokens(false)
	assert.Equal(t, "#e45649", light["red"], "light appearance uses light_name")
	assert.Equal(t, "#123456", light["accent"], "light mode overrides apply")

	assert.Equal(t, "#1e66f5", herdrThemeConfig{AutoSwitch: true}.tokens(false)["accent"], "unset light_name falls back to catppuccin-latte")

	theme.AutoSwitch = false
	manual := theme.tokens(false)
	assert.Equal(t, "#bf616a", manual["red"], "without auto_switch name wins")
	assert.Equal(t, "#88c0d0", manual["accent"], "mode overrides need auto_switch")
}

func TestHerdrThemePalettesAreComplete(t *testing.T) {
	for name, palette := range herdrThemePalettes {
		assert.Len(t, palette, 8, name)
		for token, value := range palette {
			assert.Truef(t, ValidHexColor(value), "palette %q token %q = %q", name, token, value)
			_, ok := herdrThemePalettes[defaultHerdrTheme][token]
			assert.Truef(t, ok, "palette %q token %q is not a picker role", name, token)
		}
	}
}

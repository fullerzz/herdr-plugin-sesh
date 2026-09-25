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

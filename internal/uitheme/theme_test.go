package uitheme

import (
	"os"
	"path/filepath"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveIsIndependentAndHonorsInheritance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "herdr.toml")
	require.NoError(t, os.WriteFile(path, []byte("[theme]\nname='catppuccin-latte'\n[theme.custom]\naccent='#123456'\n"), 0600))
	t.Setenv("HERDR_CONFIG_PATH", path)
	light := Resolve(true)
	builtin := Resolve(false)
	assert.Equal(t, lipgloss.Color("#123456"), light.Accent)
	assert.Equal(t, lipgloss.Color("#4c4f69"), light.Text)
	assert.Equal(t, lipgloss.Color("#7DCFFF"), builtin.Accent)
	assert.Equal(t, lipgloss.Color("#123456"), light.Accent, "resolving another screen must not mutate the first")
}

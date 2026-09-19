package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSettingsSavePreservesDocument(t *testing.T) {
	const original = "# personal config\nversion = 1\n\n[picker] # appearance\nshow_icons = true # keep this\nprompt = '''old\nprompt'''\n\n[[workspace]]\nname = 'Example'\npath = '~/example'\n"
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(original), 0600))
	doc, err := OpenSettings(LoadOptions{Path: path})
	require.NoError(t, err)
	changes := map[string]any{"picker.show_icons": false, "picker.prompt": "new \"prompt\"", "picker.show_path": false, "naming.path_components": 2}
	preview, err := doc.Preview(changes)
	require.NoError(t, err)
	data, err := os.ReadFile(path) //nolint:gosec // Test-owned temporary file.
	require.NoError(t, err)
	assert.Equal(t, original, string(data), "preview must not write")
	require.NoError(t, doc.Save(changes))
	data, err = os.ReadFile(path) //nolint:gosec // Test-owned temporary file.
	require.NoError(t, err)
	assert.Equal(t, preview, data)
	assert.Contains(t, string(data), "show_icons = false # keep this")
	assert.Contains(t, string(data), "[picker] # appearance")
	assert.Contains(t, string(data), "[[workspace]]\nname = 'Example'\npath = '~/example'\n")
	cfg, _, err := Load(LoadOptions{Path: path})
	require.NoError(t, err)
	assert.False(t, cfg.TUI.ShowIcons)
	assert.False(t, cfg.TUI.ShowPath)
	assert.Equal(t, "new \"prompt\"", cfg.TUI.Prompt)
	assert.Equal(t, 2, cfg.DirLength)
}

func TestSettingsRejectsInvalidAndStaleDrafts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	original := []byte("version = 1\n")
	require.NoError(t, os.WriteFile(path, original, 0600))
	doc, err := OpenSettings(LoadOptions{Path: path})
	require.NoError(t, err)
	require.Error(t, doc.Save(map[string]any{"naming.path_components": 0}))
	data, err := os.ReadFile(path) //nolint:gosec // Test-owned temporary file.
	require.NoError(t, err)
	assert.Equal(t, original, data)
	external := []byte("version = 1\n# external edit\n")
	require.NoError(t, os.WriteFile(path, external, 0600))
	require.ErrorContains(t, doc.Save(map[string]any{"picker.show_icons": true}), "changed on disk")
	data, err = os.ReadFile(path) //nolint:gosec // Test-owned temporary file.
	require.NoError(t, err)
	assert.Equal(t, external, data)
}

func TestSettingsNativeLayouts(t *testing.T) {
	for _, test := range []struct {
		name, input string
		success     bool
	}{
		{"dotted", "version = 1\npicker.show_icons = true\n", true},
		{"quoted", "version = 1\n[\"picker\"]\n\"show_icons\" = true\n", true},
		{"no final newline", "version = 1\n[picker]", true},
		{"inline", "version = 1\npicker = {show_icons = true}\n", true},
		{"legacy", "[tui]\nshow_icons = true\n", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			require.NoError(t, os.WriteFile(path, []byte(test.input), 0600))
			doc, err := OpenSettings(LoadOptions{Path: path})
			if err == nil {
				err = doc.Save(map[string]any{"picker.show_icons": false})
			}
			if test.success {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				data, readErr := os.ReadFile(path) //nolint:gosec // Test-owned temporary file.
				require.NoError(t, readErr)
				assert.Equal(t, test.input, string(data))
			}
		})
	}
}

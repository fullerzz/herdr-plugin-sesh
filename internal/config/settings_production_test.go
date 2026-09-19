package config

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSettingsProductionLayouts(t *testing.T) {
	for _, input := range []string{
		"version = 1\npicker = { show_icons = true, prompt = 'keep' }\n",
		"version = 1\npicker.show_icons = true\n",
		"version = 1\r\n[picker]\r\nshow_icons = true # keep\r\n",
		"version = 1\n[list]\nblacklist = [\n '^old$', # pattern note\n]\n",
	} {
		t.Run(strings.ReplaceAll(input, "\n", " "), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			//nolint:gosec // Verify existing permissions survive editing.
			require.NoError(t, os.WriteFile(path, []byte(input), 0640))
			doc, err := OpenSettings(LoadOptions{Path: path})
			require.NoError(t, err)
			require.NoError(t, doc.Save(map[string]any{"picker.show_icons": false, "picker.show_path": false, "list.blacklist": []string{"^new$", "猫"}, "list.source_order": []string{"config", "herdr"}}))
			cfg, _, err := Load(LoadOptions{Path: path})
			require.NoError(t, err)
			assert.False(t, cfg.TUI.ShowPath)
			assert.False(t, cfg.TUI.ShowIcons)
			assert.Equal(t, []string{"^new$", "猫"}, cfg.Blacklist)
			assert.Equal(t, []string{"config", "herdr"}, cfg.SortOrder)
			info, err := os.Stat(path)
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0640), info.Mode().Perm())
			data, err := os.ReadFile(path) //nolint:gosec // Test-owned file.
			require.NoError(t, err)
			if strings.Contains(input, "pattern note") {
				assert.Contains(t, string(data), "# pattern note")
			}
			if strings.Contains(input, "\r\n") {
				assert.NotContains(t, strings.ReplaceAll(string(data), "\r\n", ""), "\n")
			}
		})
	}
}

func TestSettingsMissingAndConcurrentCreation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new", "config.toml")
	doc, err := OpenSettings(LoadOptions{Path: path})
	require.NoError(t, err)
	_, err = os.Stat(path)
	require.ErrorIs(t, err, os.ErrNotExist)
	require.NoError(t, doc.Save(map[string]any{"picker.show_icons": true}))
	cfg, _, err := Load(LoadOptions{Path: path})
	require.NoError(t, err)
	assert.True(t, cfg.TUI.ShowIcons)
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())

	other := filepath.Join(t.TempDir(), "config.toml")
	draft, err := OpenSettings(LoadOptions{Path: other})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(other, []byte("version = 1\n# external\n"), 0600))
	require.Error(t, draft.Save(map[string]any{"picker.show_icons": true}))
}

func TestSettingsRejectsRetargetedSymlink(t *testing.T) {
	dir := t.TempDir()
	first, second, link := filepath.Join(dir, "first"), filepath.Join(dir, "second"), filepath.Join(dir, "config.toml")
	for _, p := range []string{first, second} {
		require.NoError(t, os.WriteFile(p, []byte("version = 1\n"), 0600))
	}
	require.NoError(t, os.Symlink(first, link))
	doc, err := OpenSettings(LoadOptions{Path: link})
	require.NoError(t, err)
	require.NoError(t, os.Remove(link))
	require.NoError(t, os.Symlink(second, link))
	require.Error(t, doc.Save(map[string]any{"picker.show_icons": true}))
}

func TestSettingsOnlyOneConcurrentDraftWins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("version = 1\n"), 0600))
	docs := make([]*SettingsDocument, 2)
	for i := range docs {
		var err error
		docs[i], err = OpenSettings(LoadOptions{Path: path})
		require.NoError(t, err)
	}
	var wg sync.WaitGroup
	errs := make([]error, 2)
	start := make(chan struct{})
	for i := range docs {
		wg.Go(func() { <-start; errs[i] = docs[i].Save(map[string]any{"naming.path_components": i + 2}) })
	}
	close(start)
	wg.Wait()
	successes := 0
	for _, err := range errs {
		if err == nil {
			successes++
		}
	}
	assert.Equal(t, 1, successes)
}

func TestMigrationPreparationDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "sesh.toml")
	imported := filepath.Join(dir, "shared.toml")
	require.NoError(t, os.WriteFile(imported, []byte("dir_length = 2\n"), 0600))
	require.NoError(t, os.WriteFile(source, []byte("import = ['shared.toml']\n"), 0600))
	migration, err := PrepareMigration(LoadOptions{Path: source}, dir)
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, "config.toml"))
	require.ErrorIs(t, err, os.ErrNotExist)
	require.NoError(t, os.WriteFile(imported, []byte("dir_length = 3\n"), 0600))
	require.ErrorIs(t, migration.Save(), ErrSettingsConflict)
	migration, err = PrepareMigration(LoadOptions{Path: source}, dir)
	require.NoError(t, err)
	require.NoError(t, migration.Save())
	cfg, _, err := Load(LoadOptions{Path: migration.NativePath})
	require.NoError(t, err)
	assert.Equal(t, 3, cfg.DirLength)
	_, err = os.Stat(source)
	require.NoError(t, err)
	_, err = PrepareMigration(LoadOptions{Path: source}, dir)
	require.Error(t, err, "existing destination must not be overwritten")
}

func TestSettingsQuotedArrayDelimiters(t *testing.T) {
	input := "version = 1\n[list]\nblacklist = [\"\"\"a\\\"b\"\"\", '''ends in quote'''', '\\]'] # keep\n"
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(input), 0600))
	doc, err := OpenSettings(LoadOptions{Path: path})
	require.NoError(t, err)
	require.NoError(t, doc.Save(map[string]any{"list.blacklist": []string{"new"}}))
	cfg, _, err := Load(LoadOptions{Path: path})
	require.NoError(t, err)
	assert.Equal(t, []string{"new"}, cfg.Blacklist)
}

func TestSettingsDeletedTargetRequiresReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("version=1\n"), 0600))
	doc, err := OpenSettings(LoadOptions{Path: path})
	require.NoError(t, err)
	require.NoError(t, os.Remove(path))
	require.ErrorIs(t, doc.Save(map[string]any{"picker.show_icons": true}), ErrSettingsConflict)
}

func TestSettingsLockFailurePreservesOriginal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	original := []byte("version=1\n# untouched\n")
	require.NoError(t, os.WriteFile(path, original, 0600))
	doc, err := OpenSettings(LoadOptions{Path: path})
	require.NoError(t, err)
	require.NoError(t, os.Mkdir(path+".settings.lock", 0700))
	require.Error(t, doc.Save(map[string]any{"picker.show_icons": true}))
	data, err := os.ReadFile(path) //nolint:gosec // Test-owned file.
	require.NoError(t, err)
	assert.Equal(t, original, data)
}

func TestMigrationForceSecuresUnchangedDestination(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "sesh.toml")
	require.NoError(t, os.WriteFile(source, []byte("dir_length=2\n"), 0600))
	_, target, err := Migrate(LoadOptions{Path: source}, dir, false)
	require.NoError(t, err)
	//nolint:gosec // Exercise correcting an existing permissive destination.
	require.NoError(t, os.Chmod(target, 0644))
	_, _, err = Migrate(LoadOptions{Path: source}, dir, true)
	require.NoError(t, err)
	info, err := os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestSettingsInlineTableTrailingComments(t *testing.T) {
	for _, table := range []string{"{ show_icons = true, # note\n}", "{ show_icons = true # note\n}", "{ # note\n}"} {
		path := filepath.Join(t.TempDir(), "config.toml")
		require.NoError(t, os.WriteFile(path, []byte("version=1\npicker = "+table+"\n"), 0600))
		doc, err := OpenSettings(LoadOptions{Path: path})
		require.NoError(t, err)
		data, err := doc.Preview(map[string]any{"picker.show_path": false})
		require.NoError(t, err)
		assert.Contains(t, string(data), "# note")
	}
}

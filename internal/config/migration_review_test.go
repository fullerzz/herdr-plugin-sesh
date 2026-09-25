package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrationSourcesTracksSymlinkAliasesAndTheirImports(t *testing.T) {
	dir := t.TempDir()
	shared := filepath.Join(dir, "shared.toml")
	require.NoError(t, os.WriteFile(shared, []byte("import = ['child.toml']\n"), 0600))

	aliases := []string{filepath.Join(dir, "first", "shared.toml"), filepath.Join(dir, "second", "shared.toml")}
	children := []string{filepath.Join(dir, "first", "child.toml"), filepath.Join(dir, "second", "child.toml")}
	for i := range aliases {
		require.NoError(t, os.Mkdir(filepath.Dir(aliases[i]), 0700))
		require.NoError(t, os.Symlink(shared, aliases[i]))
		require.NoError(t, os.WriteFile(children[i], []byte("dir_length = 2\n"), 0600))
	}
	root := filepath.Join(dir, "sesh.toml")
	require.NoError(t, os.WriteFile(root, []byte("import = ['first/shared.toml', 'second/shared.toml']\n"), 0600))

	sources, err := migrationSources(root, map[string]bool{})
	require.NoError(t, err)
	require.Len(t, sources, 5)
	selected := make([]string, 0, len(sources))
	for _, source := range sources {
		selected = append(selected, source.SelectedPath)
	}
	assert.ElementsMatch(t, []string{root, aliases[0], children[0], aliases[1], children[1]}, selected)

	require.NoError(t, os.WriteFile(children[1], []byte("dir_length = 3\n"), 0600))
	migration := Migration{sources: sources}
	assert.ErrorIs(t, migration.sourcesUnchanged(), ErrSettingsConflict)
}

func TestMigrationSourcesDetectsRetargetedAlias(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.toml")
	second := filepath.Join(dir, "second.toml")
	alias := filepath.Join(dir, "alias.toml")
	for _, path := range []string{first, second} {
		require.NoError(t, os.WriteFile(path, []byte("dir_length = 2\n"), 0600))
	}
	require.NoError(t, os.Symlink(first, alias))
	sources, err := migrationSources(alias, map[string]bool{})
	require.NoError(t, err)
	require.NoError(t, os.Remove(alias))
	require.NoError(t, os.Symlink(second, alias))

	migration := Migration{sources: sources}
	assert.ErrorIs(t, migration.sourcesUnchanged(), ErrSettingsConflict)
}

func TestPersistModeCheckedValidatesAfterStaging(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	original := []byte("version = 1\n")
	require.NoError(t, os.WriteFile(path, original, 0600))
	doc, err := OpenSettings(LoadOptions{Path: path})
	require.NoError(t, err)
	conflict := errors.New("source changed")
	checks := 0

	err = doc.persistModeChecked([]byte("version = 1\n[picker]\nshow_icons = true\n"), 0600, func() error {
		checks++
		if checks == 1 {
			return nil
		}
		staged, globErr := filepath.Glob(filepath.Join(dir, ".settings-*.tmp"))
		require.NoError(t, globErr)
		require.Len(t, staged, 1)
		return conflict
	})
	require.ErrorIs(t, err, conflict)
	assert.Equal(t, 2, checks)
	data, err := os.ReadFile(path) //nolint:gosec // Test-owned temporary config.
	require.NoError(t, err)
	assert.Equal(t, original, data)
}

func TestPersistModeCheckedValidatesNoOpWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	data := []byte("version = 1\n")
	require.NoError(t, os.WriteFile(path, data, 0600))
	doc, err := OpenSettings(LoadOptions{Path: path})
	require.NoError(t, err)
	conflict := errors.New("source changed")

	err = doc.persistModeChecked(data, 0600, func() error { return conflict })
	assert.ErrorIs(t, err, conflict)
}

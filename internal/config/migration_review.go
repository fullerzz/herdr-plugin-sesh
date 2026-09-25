package config

import (
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

type Migration struct {
	LegacyPath string
	NativePath string
	data       []byte
	target     *SettingsDocument
	sources    []*SettingsDocument
}

func (m *Migration) Save() error {
	// Recheck sources under the destination lock at both persistence boundaries.
	// Source files stay lock-free because imports may live in read-only directories.
	return m.target.persistModeChecked(m.data, 0600, m.sourcesUnchanged)
}

func (m *Migration) sourcesUnchanged() error {
	for _, source := range m.sources {
		if err := source.unchanged(); err != nil {
			return err
		}
	}
	return nil
}

func migrationSources(path string, seen map[string]bool) ([]*SettingsDocument, error) {
	selected, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if seen[selected] {
		return nil, nil
	}
	seen[selected] = true
	resolved, err := filepath.EvalSymlinks(selected)
	if err != nil {
		return nil, err
	}
	//nolint:gosec // User-selected legacy config and its imports.
	data, err := os.ReadFile(resolved)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, err
	}
	result := []*SettingsDocument{{Path: resolved, SelectedPath: selected, original: data, info: info}}
	var imports struct {
		Paths []string `toml:"import"`
	}
	if err := toml.Unmarshal(data, &imports); err != nil {
		return nil, err
	}
	for _, imp := range imports.Paths {
		next := ExpandHome(imp, "")
		if !filepath.IsAbs(next) {
			next = filepath.Join(filepath.Dir(selected), next)
		}
		sources, err := migrationSources(next, seen)
		if err != nil {
			return nil, err
		}
		result = append(result, sources...)
	}
	return result, nil
}

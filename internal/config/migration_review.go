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
	for _, source := range m.sources {
		if err := source.unchanged(); err != nil {
			return err
		}
	}
	return m.target.persistMode(m.data, 0600)
}

func migrationSources(path string, seen map[string]bool) ([]*SettingsDocument, error) {
	selected, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(selected)
	if err != nil {
		return nil, err
	}
	if seen[resolved] {
		return nil, nil
	}
	seen[resolved] = true
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

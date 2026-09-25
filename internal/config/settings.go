package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"syscall"

	"github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
)

var ErrSettingsLegacy = errors.New("legacy config requires migration")

var ErrSettingsConflict = errors.New("config changed on disk; reload before saving")

// SettingsDocument keeps the source document separate from effective defaults.
// A document belongs to one settings screen; Save runs while that screen is busy.
type SettingsDocument struct {
	Path         string
	SelectedPath string
	Config       Config
	Explicit     map[string]any
	Missing      bool
	original     []byte
	info         os.FileInfo
}

// SettingsDestination mirrors config init without creating anything.
func SettingsDestination(opts LoadOptions) string {
	env := opts.Env
	if env == nil {
		env = getenvMap()
	}
	home := opts.Home
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	if opts.Path != "" {
		return ExpandHome(opts.Path, home)
	}
	if env["HERDR_SESH_CONFIG"] != "" {
		return ExpandHome(env["HERDR_SESH_CONFIG"], home)
	}
	return filepath.Join(PluginConfigDir(LoadOptions{Env: env, Home: home}), NativeFileName)
}

// PluginConfigDir is the plugin-owned native config directory. Unlike
// SettingsDestination it ignores HERDR_SESH_CONFIG, so it is a safe legacy
// migration target even when that variable points into ~/.config/sesh.
func PluginConfigDir(opts LoadOptions) string {
	env := opts.Env
	if env == nil {
		env = getenvMap()
	}
	home := opts.Home
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	dir := env["HERDR_PLUGIN_CONFIG_DIR"]
	if dir == "" {
		dir = filepath.Join(home, ".config", "herdr-sesh")
	}
	return ExpandHome(dir, home)
}

func OpenSettings(opts LoadOptions) (*SettingsDocument, error) {
	path, err := ResolvePath(opts)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if path == "" {
		path = SettingsDestination(opts)
	}
	selected, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	resolved, err := settingsTarget(selected)
	if err != nil {
		return nil, err
	}
	d := &SettingsDocument{Path: resolved, SelectedPath: selected}
	//nolint:gosec // User-selected config.
	data, err := os.ReadFile(resolved)
	if errors.Is(err, os.ErrNotExist) {
		d.Missing = true
		data = []byte("version = 1\n")
	} else if err != nil {
		return nil, err
	}
	if !hasVersionKey(data) {
		return nil, ErrSettingsLegacy
	}
	if err := d.setBaseline(data); err != nil {
		return nil, err
	}
	return d, nil
}

// Resolve existing parent symlinks even when the final file does not exist.
func settingsTarget(path string) (string, error) {
	if _, err := os.Lstat(path); err == nil {
		return filepath.EvalSymlinks(path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	parent := filepath.Dir(path)
	if parent == path {
		return "", os.ErrNotExist
	}
	resolved, err := settingsTarget(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolved, filepath.Base(path)), nil
}

func (d *SettingsDocument) setBaseline(data []byte) error {
	cfg, err := decodeNative(d.Path, data)
	if err != nil {
		return err
	}
	var explicit map[string]any
	if err := toml.Unmarshal(data, &explicit); err != nil {
		return err
	}
	var info os.FileInfo
	if !d.Missing {
		info, err = os.Stat(d.Path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("config must be a regular file")
		}
	}
	d.Config, d.Explicit, d.original, d.info = cfg, explicit, bytes.Clone(data), info
	return nil
}

// Preview patches only the requested global keys and validates the entire result.
func (d *SettingsDocument) Preview(changes map[string]any) ([]byte, error) {
	keys := make([]string, 0, len(changes))
	for key := range changes {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	data := bytes.Clone(d.original)
	for _, key := range keys {
		parts := strings.Split(key, ".")
		if len(parts) != 2 || !editableSetting(key) {
			return nil, fmt.Errorf("unsupported setting %q", key)
		}
		encoded, err := toml.Marshal(map[string]any{"value": changes[key]})
		if err != nil {
			return nil, err
		}
		value, err := serializedSettingValue(encoded)
		if err != nil {
			return nil, err
		}
		data, err = patchSetting(data, parts[0], parts[1], value)
		if err != nil {
			return nil, err
		}
	}
	if _, err := decodeNative(d.Path, data); err != nil {
		return nil, err
	}
	var values map[string]any
	if err := toml.Unmarshal(data, &values); err != nil {
		return nil, err
	}
	// Decode the requested values through TOML too: int/int64 and []string/[]any
	// are serialization details, not changes in meaning.
	for _, key := range keys {
		parts := strings.Split(key, ".")
		wantData, err := toml.Marshal(map[string]any{"value": changes[key]})
		if err != nil {
			return nil, err
		}
		var want map[string]any
		if err := toml.Unmarshal(wantData, &want); err != nil {
			return nil, err
		}
		section, ok := values[parts[0]].(map[string]any)
		if !ok || !reflect.DeepEqual(section[parts[1]], want["value"]) {
			return nil, fmt.Errorf("could not faithfully edit %s", key)
		}
	}
	return data, nil
}

func serializedSettingValue(encoded []byte) (string, error) {
	var p unstable.Parser
	p.Reset(encoded)
	if !p.NextExpression() {
		if err := p.Error(); err != nil {
			return "", fmt.Errorf("serialize setting value: %w", err)
		}
		return "", errors.New("serialize setting value: missing value assignment")
	}
	n := p.Expression()
	keys := nodeKeys(n)
	if n.Kind != unstable.KeyValue || len(keys) != 1 || keys[0] != "value" {
		return "", errors.New("serialize setting value: missing value assignment")
	}
	start := valueStart(encoded, n)
	end, _ := valueEnd(encoded, n.Value(), start)
	if start >= end || end > len(encoded) {
		return "", errors.New("serialize setting value: missing value")
	}
	return strings.TrimSpace(string(encoded[start:end])), nil
}

func editableSetting(key string) bool {
	switch key {
	case "picker.show_icons", "picker.show_path", "picker.show_preview", "picker.preview_mode", "picker.workspace_sort", "picker.prioritize_home", "picker.herdr_theme_inherit", "picker.replace_worktree_icon", "picker.show_last_workspace", "picker.show_last_workspace_path", "picker.separator_aware", "picker.prompt", "picker.placeholder", "list.cache", "list.source_order", "list.blacklist", "naming.path_components", "keys.cycle_preview_mode", "workspace_defaults.startup", "workspace_defaults.preview":
		return true
	default:
		return false
	}
}

func (d *SettingsDocument) unchanged() error {
	path, err := settingsTarget(d.SelectedPath)
	if err != nil || path != d.Path {
		return ErrSettingsConflict
	}
	info, err := os.Stat(d.Path)
	if d.Missing {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		return ErrSettingsConflict
	}
	if errors.Is(err, os.ErrNotExist) {
		return ErrSettingsConflict
	}
	if err != nil {
		return err
	}
	if !os.SameFile(d.info, info) || info.Mode() != d.info.Mode() {
		return ErrSettingsConflict
	}
	//nolint:gosec // User-selected config.
	data, err := os.ReadFile(d.Path)
	if err != nil {
		return err
	}
	if !bytes.Equal(data, d.original) {
		return ErrSettingsConflict
	}
	return nil
}

func (d *SettingsDocument) Save(changes map[string]any) error {
	data, err := d.Preview(changes)
	if err != nil {
		return err
	}
	if err := d.persist(data); err != nil {
		return err
	}
	return d.setBaseline(data)
}

func (d *SettingsDocument) persist(data []byte) error { return d.persistMode(data, 0) }

func (d *SettingsDocument) persistMode(data []byte, modeOverride os.FileMode) error {
	return d.persistModeChecked(data, modeOverride, nil)
}

func (d *SettingsDocument) persistModeChecked(data []byte, modeOverride os.FileMode, check func() error) error {
	if err := os.MkdirAll(filepath.Dir(d.Path), 0700); err != nil {
		return err
	}
	// Keep the lock inode: deleting lock files lets concurrent writers lock different inodes.
	//nolint:gosec // User-selected directory; refuse symlink lock files.
	lock, err := os.OpenFile(d.Path+".settings.lock", os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fmt.Errorf("config is busy; retry saving: %w", err)
	}
	defer func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) }()
	if err := d.unchanged(); err != nil {
		return err
	}
	if check != nil {
		if err := check(); err != nil {
			return err
		}
	}
	if !d.Missing && bytes.Equal(data, d.original) && (modeOverride == 0 || d.info.Mode().Perm() == modeOverride) {
		return nil
	}
	tmp, err := os.CreateTemp(filepath.Dir(d.Path), ".settings-*.tmp")
	if err != nil {
		return err
	}
	defer func() { _ = tmp.Close(); _ = os.Remove(tmp.Name()) }()
	mode := os.FileMode(0600)
	if d.info != nil {
		mode = d.info.Mode().Perm()
	}
	if modeOverride != 0 {
		mode = modeOverride
	}
	if err := tmp.Chmod(mode); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if check != nil {
		if err := check(); err != nil {
			return err
		}
	}
	if err := d.unchanged(); err != nil {
		return err
	}
	if d.Missing {
		// Linking an adjacent complete file gives atomic no-clobber creation.
		if err := os.Link(tmp.Name(), d.Path); err != nil {
			return err
		}
	} else if err := os.Rename(tmp.Name(), d.Path); err != nil {
		return err
	}
	d.Missing = false
	return nil
}

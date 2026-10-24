package remote

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
)

type Config struct {
	Enabled      bool   `toml:"enabled"`
	ID           string `toml:"id"`
	Label        string `toml:"label"`
	Socket       string `toml:"socket"`
	HerdrSession string `toml:"herdr_session"`
	Peers        []Peer `toml:"peers"`
}
type Peer struct {
	ID      string `toml:"id"`
	Label   string `toml:"label"`
	SSH     string `toml:"ssh"`
	Session string `toml:"session"`
}

var identifier = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`)
var sshHost = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.@:-]{0,253}$`)

func ConfigPath() (string, error) {
	p := os.Getenv("HERDR_SESH_REMOTE_CONFIG")
	if p == "" {
		d := os.Getenv("XDG_CONFIG_HOME")
		if d == "" {
			var e error
			d, e = os.UserConfigDir()
			if e != nil {
				return "", e
			}
		} else if !filepath.IsAbs(d) {
			return "", errors.New("XDG_CONFIG_HOME must be an absolute path")
		}
		p = filepath.Join(d, "herdr-sesh", "remote.toml")
	}
	return filepath.Abs(p)
}

func LoadConfig() (Config, error) {
	p, e := ConfigPath()
	if e != nil {
		return Config{}, e
	}
	b, e := os.ReadFile(p) // #nosec G304 G703 -- explicit user configuration path.
	if errors.Is(e, os.ErrNotExist) && os.Getenv("HERDR_SESH_REMOTE_CONFIG") == "" {
		return Config{}, nil
	}
	if e != nil {
		return Config{}, e
	}
	return decodeConfig(b)
}

func decodeConfig(b []byte) (Config, error) {
	var c Config
	if err := toml.NewDecoder(bytes.NewReader(b)).DisallowUnknownFields().Decode(&c); err != nil {
		return c, err
	}
	if c.HerdrSession == "" {
		c.HerdrSession = "default"
	}
	for i := range c.Peers {
		if c.Peers[i].Session == "" {
			c.Peers[i].Session = "default"
		}
	}
	return c, c.Validate()
}

// ConfigEdit binds an edit to the file observed before interactive input.
type ConfigEdit struct {
	Config Config
	Exists bool
	path   string
	before configSnapshot
}

// PrepareConfig reads and validates the same snapshot that a later save checks.
// A missing file is represented by Exists=false and an empty Config.
func PrepareConfig(path string) (*ConfigEdit, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	before, err := readConfigSnapshot(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	edit := &ConfigEdit{path: path, before: before, Exists: before.info != nil}
	if edit.Exists {
		edit.Config, err = decodeConfig(before.data)
		if err != nil {
			return nil, err
		}
	}
	return edit, nil
}

// Save rejects changes made since preparation and writes private permissions.
func (e *ConfigEdit) Save(cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	b, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}
	return writeConfig(e.path, b, e.before)
}

// SaveConfig atomically replaces a regular config file with private permissions.
func SaveConfig(path string, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	before, err := readConfigSnapshot(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return (&ConfigEdit{path: path, before: before}).Save(cfg)
}

// SetEnabled preserves comments, formatting, and every other config value.
func SetEnabled(path string, enabled bool) error {
	before, err := readConfigSnapshot(path)
	if err != nil {
		return err
	}
	return (&ConfigEdit{path: path, before: before}).SetEnabled(enabled)
}

// SetEnabled preserves the prepared text and rejects changes since preparation.
func (e *ConfigEdit) SetEnabled(enabled bool) error {
	if e.before.info == nil {
		return os.ErrNotExist
	}
	b := e.before.data
	var cfg Config
	if err := toml.NewDecoder(bytes.NewReader(b)).DisallowUnknownFields().Decode(&cfg); err != nil {
		return err
	}
	cfg.Enabled = enabled
	if err := cfg.Validate(); err != nil {
		return err
	}
	var parser unstable.Parser
	parser.Reset(b)
	for parser.NextExpression() {
		node := parser.Expression()
		if node.Kind == unstable.Table || node.Kind == unstable.ArrayTable {
			break
		}
		if node.Kind != unstable.KeyValue {
			continue
		}
		key := node.Key()
		if !key.Next() || string(key.Node().Data) != "enabled" || !key.IsLast() {
			continue
		}
		r := node.Value().Raw
		updated := append([]byte{}, b[:r.Offset]...)
		updated = append(updated, strconv.FormatBool(enabled)...)
		updated = append(updated, b[r.Offset+r.Length:]...)
		return writeConfig(e.path, updated, e.before)
	}
	if err := parser.Error(); err != nil {
		return err
	}
	return writeConfig(e.path, append([]byte("enabled = "+strconv.FormatBool(enabled)+"\n"), b...), e.before)
}

type configSnapshot struct {
	info os.FileInfo
	data []byte
}

func readConfigSnapshot(path string) (configSnapshot, error) {
	info, err := configFileInfo(path)
	if err != nil {
		return configSnapshot{}, err
	}
	b, err := os.ReadFile(path) // #nosec G304 G703 -- explicit user configuration path.
	return configSnapshot{info: info, data: b}, err
}

func configFileInfo(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("remote config must be a regular file: %s", path)
	}
	return info, nil
}

func writeConfig(path string, data []byte, before configSnapshot) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".remote-*.toml")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	after, err := readConfigSnapshot(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if (before.info == nil) != (after.info == nil) || (before.info != nil && !os.SameFile(before.info, after.info)) || !bytes.Equal(before.data, after.data) {
		return errors.New("remote config changed while saving")
	}
	if before.info == nil {
		// Linking publishes the complete file without replacing a concurrent creator.
		if err := os.Link(f.Name(), path); err != nil {
			if errors.Is(err, os.ErrExist) {
				return errors.New("remote config changed while saving")
			}
			return err
		}
		return nil
	}
	return os.Rename(f.Name(), path)
}

func (c Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	if !identifier.MatchString(c.ID) {
		return errors.New("remote machine id must be a stable identifier")
	}
	if c.HerdrSession != "" && !identifier.MatchString(c.HerdrSession) {
		return errors.New("invalid Herdr session")
	}
	if strings.ContainsFunc(c.Label, unicode.IsControl) {
		return errors.New("invalid machine label")
	}
	seen := map[string]bool{c.ID: true}
	for _, p := range c.Peers {
		if !identifier.MatchString(p.ID) || seen[p.ID] {
			return fmt.Errorf("invalid or duplicate peer id %q", p.ID)
		}
		seen[p.ID] = true
		if (p.Session != "" && !identifier.MatchString(p.Session)) || strings.ContainsFunc(p.Label, unicode.IsControl) {
			return errors.New("invalid peer session or label")
		}
		if !sshHost.MatchString(p.SSH) {
			return fmt.Errorf("invalid SSH destination %q", p.SSH)
		}
	}
	if c.Socket != "" && !filepath.IsAbs(c.Socket) {
		return errors.New("remote socket must be absolute")
	}
	return nil
}
func SocketPath(c Config) (string, error) {
	if c.Socket != "" {
		return c.Socket, nil
	}
	d, e := os.UserCacheDir()
	if e != nil {
		return "", e
	}
	return filepath.Join(d, "herdr-sesh", "remote", "daemon.sock"), nil
}

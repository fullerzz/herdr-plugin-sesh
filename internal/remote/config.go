package remote

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/pelletier/go-toml/v2"
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

func LoadConfig() (Config, error) {
	p := os.Getenv("HERDR_SESH_REMOTE_CONFIG")
	explicit := p != ""
	if !explicit {
		d, e := os.UserConfigDir()
		if e != nil {
			return Config{}, e
		}
		p = filepath.Join(d, "herdr-sesh", "remote.toml")
	}
	b, e := os.ReadFile(p) // #nosec G304 G703 -- explicit user configuration path.
	if errors.Is(e, os.ErrNotExist) && !explicit {
		return Config{}, nil
	}
	if e != nil {
		return Config{}, e
	}
	var c Config
	if e = toml.NewDecoder(bytes.NewReader(b)).DisallowUnknownFields().Decode(&c); e != nil {
		return c, e
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

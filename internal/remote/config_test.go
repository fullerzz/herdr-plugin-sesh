package remote

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigLocationPrecedence(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HERDR_SESH_REMOTE_CONFIG", "")
	native, err := os.UserConfigDir()
	require.NoError(t, err)
	xdg := t.TempDir()
	explicit := filepath.Join(t.TempDir(), "remote.toml")
	for path, id := range map[string]string{
		filepath.Join(native, "herdr-sesh", "remote.toml"): "native",
		filepath.Join(xdg, "herdr-sesh", "remote.toml"):    "xdg",
		explicit: "explicit",
	} {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
		require.NoError(t, os.WriteFile(path, []byte("enabled=true\nid='"+id+"'\n"), 0600))
	}
	for _, tc := range []struct {
		name, xdg, explicit, want string
	}{
		{"native default", "", "", "native"},
		{"XDG overrides native", xdg, "", "xdg"},
		{"explicit overrides XDG", xdg, explicit, "explicit"},
		{"missing XDG config disables remote", t.TempDir(), "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", tc.xdg)
			t.Setenv("HERDR_SESH_REMOTE_CONFIG", tc.explicit)
			cfg, err := LoadConfig()
			require.NoError(t, err)
			assert.Equal(t, tc.want, cfg.ID)
			assert.Equal(t, tc.want != "", cfg.Enabled)
		})
	}
}

func TestConfigRejectsRelativeXDGDirectory(t *testing.T) {
	t.Setenv("HERDR_SESH_REMOTE_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "relative")
	_, err := LoadConfig()
	require.ErrorContains(t, err, "XDG_CONFIG_HOME must be an absolute path")
}

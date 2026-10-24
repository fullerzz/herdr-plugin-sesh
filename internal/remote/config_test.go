package remote

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"

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

func TestConfigPathResolvesExplicitRelativePath(t *testing.T) {
	t.Setenv("HERDR_SESH_REMOTE_CONFIG", "config/../remote.toml")
	t.Setenv("XDG_CONFIG_HOME", "invalid-relative")
	want, err := filepath.Abs("remote.toml")
	require.NoError(t, err)
	got, err := ConfigPath()
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestSaveConfigPrivateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new", "remote.toml")
	cfg := Config{Enabled: true, ID: "local", Label: "My machine", Peers: []Peer{{ID: "peer", SSH: "user@host"}}}
	require.NoError(t, SaveConfig(path, cfg))
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
	t.Setenv("HERDR_SESH_REMOTE_CONFIG", path)
	loaded, err := LoadConfig()
	require.NoError(t, err)
	cfg.HerdrSession = "default"
	cfg.Peers[0].Session = "default"
	assert.Equal(t, cfg, loaded)
	require.NoError(t, os.Chmod(path, 0644)) // #nosec G302 -- exercise tightening existing file permissions.
	require.NoError(t, SaveConfig(path, cfg))
	info, err = os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestSetEnabledPreservesConfigText(t *testing.T) {
	for _, tc := range []struct {
		name, before, after string
	}{
		{"plain", "# opt in\nenabled = false # keep\nid = 'local'\n", "# opt in\nenabled = true # keep\nid = 'local'\n"},
		{"quoted", "'enabled'=false\nid='local'\n", "'enabled'=true\nid='local'\n"},
		{"absent", "id='local'\n[[peers]]\nid='peer'\nssh='host'\n", "enabled = true\nid='local'\n[[peers]]\nid='peer'\nssh='host'\n"},
		{"literal string", "id='local'\nlabel='''enabled = false'''\nenabled=false\n", "id='local'\nlabel='''enabled = false'''\nenabled=true\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "remote.toml")
			require.NoError(t, os.WriteFile(path, []byte(tc.before), 0600))
			require.NoError(t, SetEnabled(path, true))
			got, err := os.ReadFile(path) // #nosec G304 -- test-owned temporary fixture.
			require.NoError(t, err)
			assert.Equal(t, tc.after, string(got))
			require.NoError(t, SetEnabled(path, false))
			got, err = os.ReadFile(path) // #nosec G304 -- test-owned temporary fixture.
			require.NoError(t, err)
			assert.Equal(t, strings.Replace(tc.after, "true", "false", 1), string(got))
		})
	}
}

func TestConfigFailedWritePreservesExistingSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "remote.toml")
	before := "enabled=false\nid='invalid machine id' # keep until fixed\n"
	require.NoError(t, os.WriteFile(path, []byte(before), 0600))
	require.Error(t, SetEnabled(path, true))
	require.Error(t, SaveConfig(path, Config{Enabled: true, ID: "invalid id"}))
	got, err := os.ReadFile(path) // #nosec G304 -- test-owned temporary fixture.
	require.NoError(t, err)
	assert.Equal(t, before, string(got))
	t.Setenv("HERDR_SESH_REMOTE_CONFIG", path)
	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.False(t, cfg.Enabled)
	require.NoError(t, os.WriteFile(path, []byte("enabled=true\nid='invalid machine id'\n"), 0600))
	require.NoError(t, SetEnabled(path, false))
	_, err = LoadConfig()
	require.NoError(t, err)
}

func TestConfigWritesRejectUnsafeTargets(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.toml")
	before := []byte("enabled=false\nid='local'\n")
	require.NoError(t, os.WriteFile(target, before, 0600))
	link := filepath.Join(dir, "remote.toml")
	require.NoError(t, os.Symlink(target, link))
	for _, path := range []string{link, dir} {
		require.ErrorContains(t, SaveConfig(path, Config{Enabled: true, ID: "local"}), "regular file")
		require.ErrorContains(t, SetEnabled(path, true), "regular file")
	}
	got, err := os.ReadFile(target) // #nosec G304 -- test-owned temporary fixture.
	require.NoError(t, err)
	assert.Equal(t, before, got)
	missing := filepath.Join(dir, "missing.toml")
	require.ErrorIs(t, SetEnabled(missing, false), os.ErrNotExist)
	_, err = os.Stat(missing)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestSaveConfigPermissionFailurePreservesOriginal(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write read-only directories")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "remote.toml")
	before := []byte("enabled=false\nid='local'\n")
	require.NoError(t, os.WriteFile(path, before, 0600))
	require.NoError(t, os.Chmod(dir, 0500)) // #nosec G302 -- owner-only test directory needs search permission.
	t.Cleanup(func() {
		require.NoError(t, os.Chmod(dir, 0700)) // #nosec G302 -- restore owner access for temporary directory cleanup.
	})
	require.Error(t, SetEnabled(path, true))
	got, err := os.ReadFile(path) // #nosec G304 -- test-owned temporary fixture.
	require.NoError(t, err)
	assert.Equal(t, before, got)
}

func TestConfigRejectsRelativeXDGDirectory(t *testing.T) {
	t.Setenv("HERDR_SESH_REMOTE_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "relative")
	_, err := LoadConfig()
	require.ErrorContains(t, err, "XDG_CONFIG_HOME must be an absolute path")
}

func TestConfigWritePreservesConcurrentEdits(t *testing.T) {
	for _, replace := range []bool{false, true} {
		path := filepath.Join(t.TempDir(), "remote.toml")
		require.NoError(t, os.WriteFile(path, []byte("enabled=false\nid='local'\n"), 0600))
		before, err := readConfigSnapshot(path)
		require.NoError(t, err)
		changed := []byte("enabled=false\nid='changed-by-user'\n")
		if replace {
			replacement := filepath.Join(filepath.Dir(path), "replacement.toml")
			require.NoError(t, os.WriteFile(replacement, changed, 0600))
			require.NoError(t, os.Rename(replacement, path))
		} else {
			require.NoError(t, os.WriteFile(path, changed, 0600))
		}
		_, err = writeConfig(path, []byte("enabled=true\nid='local'\n"), before)
		require.ErrorContains(t, err, "changed while saving")
		got, err := os.ReadFile(path) // #nosec G304 -- test-owned temporary fixture.
		require.NoError(t, err)
		assert.Equal(t, changed, got)
		entries, err := os.ReadDir(filepath.Dir(path))
		require.NoError(t, err)
		require.Len(t, entries, 2)
		assert.Equal(t, "remote.toml", entries[0].Name())
		assert.Equal(t, "remote.toml.lock", entries[1].Name())
	}
}

func TestConfigWritersRespectHeldLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "remote.toml")
	require.NoError(t, SaveConfig(path, Config{ID: "local"}))
	edit, err := PrepareConfig(path)
	require.NoError(t, err)
	lock, err := os.OpenFile(path+".lock", os.O_RDWR, 0600) // #nosec G304 -- test-owned temporary lock file.
	require.NoError(t, err)
	t.Cleanup(func() { _ = lock.Close() })
	require.NoError(t, unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB))
	for _, save := range []func() error{
		func() error { return SaveConfig(path, Config{ID: "other"}) },
		func() error { return SetEnabled(path, true) },
		func() error { return edit.Save(Config{ID: "other"}) },
		func() error { return edit.SetEnabled(true) },
	} {
		require.ErrorContains(t, save(), "busy")
	}
	require.NoError(t, unix.Flock(int(lock.Fd()), unix.LOCK_UN))
	require.NoError(t, SaveConfig(path, Config{ID: "winner"}))
	require.ErrorContains(t, edit.Save(Config{ID: "stale"}), "changed while saving")
	info, err := os.Stat(path + ".lock")
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
	held, err := lock.Stat()
	require.NoError(t, err)
	assert.True(t, os.SameFile(held, info))
}

func TestConcurrentPreparedConfigWritersHaveOneWinner(t *testing.T) {
	for _, exists := range []bool{false, true} {
		path := filepath.Join(t.TempDir(), "remote.toml")
		if exists {
			require.NoError(t, SaveConfig(path, Config{ID: "original"}))
		}
		const writers = 16
		edits := make([]*ConfigEdit, writers)
		for i := range edits {
			var err error
			edits[i], err = PrepareConfig(path)
			require.NoError(t, err)
		}
		start := make(chan struct{})
		results := make(chan error, writers)
		for _, edit := range edits {
			go func() {
				<-start
				results <- edit.Save(Config{ID: "winner"})
			}()
		}
		close(start)
		wins := 0
		for range writers {
			err := <-results
			if err == nil {
				wins++
			} else {
				assert.True(t, strings.Contains(err.Error(), "busy") || strings.Contains(err.Error(), "changed while saving"), "%v", err)
			}
		}
		assert.Equal(t, 1, wins)
		edit, err := PrepareConfig(path)
		require.NoError(t, err)
		assert.Equal(t, "winner", edit.Config.ID)
	}
}

func TestConfigRejectsUnsafeLocks(t *testing.T) {
	for _, kind := range []string{"symlink", "public", "directory", "hardlink", "fifo"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "remote.toml")
			before := []byte("enabled=false\nid='local'\n")
			require.NoError(t, os.WriteFile(path, before, 0600))
			lock := path + ".lock"
			switch kind {
			case "symlink":
				require.NoError(t, os.Symlink(path, lock))
			case "public":
				require.NoError(t, os.WriteFile(lock, nil, 0600))
				require.NoError(t, os.Chmod(lock, 0644)) // #nosec G302 -- deliberately unsafe lock fixture.
			case "directory":
				require.NoError(t, os.Mkdir(lock, 0700))
			case "hardlink":
				require.NoError(t, os.Link(path, lock))
			case "fifo":
				require.NoError(t, unix.Mkfifo(lock, 0600))
			}
			require.Error(t, SaveConfig(path, Config{ID: "other"}))
			got, err := os.ReadFile(path) // #nosec G304 -- test-owned temporary fixture.
			require.NoError(t, err)
			assert.Equal(t, before, got)
		})
	}
}

func TestPreparedConfigPreservesChangesDuringEditing(t *testing.T) {
	for _, change := range []string{"create", "edit", "replace"} {
		t.Run(change, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "remote.toml")
			if change != "create" {
				require.NoError(t, os.WriteFile(path, []byte("enabled=false\nlabel='unfinished'\n"), 0600))
			}
			edit, err := PrepareConfig(path)
			require.NoError(t, err)
			assert.Equal(t, change != "create", edit.Exists)
			changed := []byte("enabled=false\nid='other'\n[[peers]]\nid='peer'\nssh='host'\n")
			if change == "replace" {
				replacement := path + ".new"
				require.NoError(t, os.WriteFile(replacement, changed, 0600))
				require.NoError(t, os.Rename(replacement, path))
			} else {
				require.NoError(t, os.WriteFile(path, changed, 0600))
			}
			require.ErrorContains(t, edit.Save(Config{Enabled: true, ID: "local"}), "changed while saving")
			got, err := os.ReadFile(path) // #nosec G304 -- test-owned temporary fixture.
			require.NoError(t, err)
			assert.Equal(t, changed, got)
		})
	}
}

func TestPreparedConfigSavesSnapshotSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "remote.toml")
	before := "# keep settings\nenabled=false\nid='local'\nsocket='/tmp/custom.sock'\n[[peers]]\nid='peer'\nssh='host'\n"
	require.NoError(t, os.WriteFile(path, []byte(before), 0600))
	edit, err := PrepareConfig(path)
	require.NoError(t, err)
	assert.Equal(t, "/tmp/custom.sock", edit.Config.Socket)
	require.Len(t, edit.Config.Peers, 1)
	assert.Equal(t, "default", edit.Config.Peers[0].Session)
	require.NoError(t, edit.SetEnabled(true))
	got, err := os.ReadFile(path) // #nosec G304 -- test-owned temporary fixture.
	require.NoError(t, err)
	assert.Equal(t, strings.Replace(before, "false", "true", 1), string(got))

	missing := filepath.Join(t.TempDir(), "new", "remote.toml")
	edit, err = PrepareConfig(missing)
	require.NoError(t, err)
	assert.False(t, edit.Exists)
	require.NoError(t, edit.Save(Config{Enabled: true, ID: "new"}))
	edit, err = PrepareConfig(missing)
	require.NoError(t, err)
	assert.Equal(t, "new", edit.Config.ID)
}

func TestPrepareConfigRejectsUnsafeTargets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "remote.toml")
	require.NoError(t, os.Symlink(filepath.Join(dir, "missing"), path))
	for _, target := range []string{dir, path} {
		_, err := PrepareConfig(target)
		require.ErrorContains(t, err, "regular file")
	}
}

func TestPreparedConfigEnableRejectsReplacementWithIdenticalBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "remote.toml")
	before := []byte("# retained\nenabled=false\nid='local'\n")
	require.NoError(t, os.WriteFile(path, before, 0600))
	edit, err := PrepareConfig(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path+".new", before, 0600))
	require.NoError(t, os.Rename(path+".new", path))
	require.ErrorContains(t, edit.SetEnabled(true), "changed while saving")
	got, err := os.ReadFile(path) // #nosec G304 -- test-owned temporary fixture.
	require.NoError(t, err)
	assert.Equal(t, before, got)
}

func TestConfigRestoreUsesPublicationReceipt(t *testing.T) {
	for _, original := range []string{"", "# draft\nenabled = false\nid = ''\n", "# retained\nenabled = false\nid = 'local'\n"} {
		for _, change := range []string{"none", "edit", "replace"} {
			t.Run(original+change, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "remote.toml")
				if original != "" {
					require.NoError(t, os.WriteFile(path, []byte(original), 0o600))
				}
				edit, err := PrepareConfig(path)
				require.NoError(t, err)
				if edit.Config.ID != "" {
					require.NoError(t, edit.SetEnabled(true))
				} else {
					require.NoError(t, edit.Save(Config{Enabled: true, ID: "configured"}))
				}
				// Edit immediately after publication, before any caller can inspect the path.
				newer := []byte("# concurrent editor\nenabled = false\nid = 'newer'\n")
				switch change {
				case "edit":
					require.NoError(t, os.WriteFile(path, newer, 0o600))
				case "replace":
					// Identical bytes still belong to another writer's replacement.
					newer = edit.written.data
					require.NoError(t, os.WriteFile(path+".new", newer, 0o600))
					require.NoError(t, os.Rename(path+".new", path))
				}
				if change != "none" {
					require.ErrorContains(t, edit.Restore(), "changed while saving")
					data, err := os.ReadFile(path) //nolint:gosec // Test-owned temporary configuration.
					require.NoError(t, err)
					assert.Equal(t, newer, data)
					return
				}
				require.NoError(t, edit.Restore())
				if original == "" {
					assert.NoFileExists(t, path)
				} else {
					data, err := os.ReadFile(path) //nolint:gosec // Test-owned temporary configuration.
					require.NoError(t, err)
					assert.Equal(t, original, string(data))
				}
			})
		}
	}
}

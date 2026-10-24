package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/fullerzz/herdr-plugin-sesh/internal/remote"
	"github.com/fullerzz/herdr-plugin-sesh/internal/remoteservice"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRemoteService struct {
	state     remoteservice.Status
	options   remoteservice.Options
	enableErr error
	statusErr error
	stopErr   error
	enables   int
	stops     int
	onEnable  func()
	locked    bool
	lockErr   error
	onUnlock  func()
}

func (s *fakeRemoteService) WithLock(ctx context.Context, fn func(context.Context) error) error {
	if s.lockErr != nil {
		return s.lockErr
	}
	s.locked = true
	defer func() {
		if s.onUnlock != nil {
			s.onUnlock()
		}
		s.locked = false
	}()
	return fn(ctx)
}

func (s *fakeRemoteService) Enable(_ context.Context, options remoteservice.Options) error {
	s.options = options
	s.enables++
	if s.onEnable != nil {
		s.onEnable()
	}
	if s.enableErr == nil {
		s.state = remoteservice.Status{Installed: true, Running: true, Autostart: true}
	}
	return s.enableErr
}

func (s *fakeRemoteService) Disable(context.Context) error {
	s.stops++
	if s.stopErr == nil {
		s.state = remoteservice.Status{}
	}
	return s.stopErr
}

func (s *fakeRemoteService) Status(context.Context) (remoteservice.Status, error) {
	return s.state, s.statusErr
}

func (*fakeRemoteService) Path() string { return "/test/service" }
func (*fakeRemoteService) Logs() string { return "/test/log" }

func remoteCommandFixture(t *testing.T) (string, *App, *bytes.Buffer, *fakeRemoteService, remoteCommands) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "remote.toml")
	t.Setenv("HERDR_SESH_REMOTE_CONFIG", path)
	herdrBin := filepath.Join(dir, "herdr")
	//nolint:gosec // Executable fixture stays in the test-owned temporary directory.
	require.NoError(t, os.WriteFile(herdrBin, []byte("#!/bin/sh\nexit 0\n"), 0o700))
	t.Setenv("HERDR_BIN_PATH", herdrBin)
	output := new(bytes.Buffer)
	a := &App{Out: output, Err: output}
	service := new(fakeRemoteService)
	commands := remoteCommands{service: service, configure: func(context.Context, remote.Config) (remote.Config, error) {
		return remote.Config{Enabled: true, ID: "laptop", HerdrSession: "default"}, nil
	}, call: func(_ context.Context, cfg remote.Config, _ remote.Request) (remote.Response, error) {
		if !service.state.Running {
			return remote.Response{}, errors.New("not running")
		}
		return remote.Response{Catalog: []remote.Machine{{ID: cfg.ID, Session: cfg.HerdrSession, Stale: true, LastError: "Herdr session not running"}}}, nil
	}}
	return path, a, output, service, commands
}

func TestRemoteEnableCreatesConfigAndVerifiesDaemon(t *testing.T) {
	path, a, output, service, commands := remoteCommandFixture(t)
	require.NoError(t, commands.run(context.Background(), a, "enable"))
	cfg, err := remote.LoadConfig()
	require.NoError(t, err)
	assert.True(t, cfg.Enabled)
	assert.Equal(t, "laptop", cfg.ID)
	assert.Equal(t, path, service.options.ConfigPath)
	assert.True(t, filepath.IsAbs(service.options.Executable))
	assert.True(t, filepath.IsAbs(service.options.HerdrBinary))
	assert.Contains(t, output.String(), "Remote support enabled")
	assert.Contains(t, output.String(), "Local Herdr session laptop (default): unavailable")
}

func TestRemoteEnableHoldsLockThroughSetupAndReadiness(t *testing.T) {
	_, a, _, service, commands := remoteCommandFixture(t)
	configure, call := commands.configure, commands.call
	commands.configure = func(ctx context.Context, cfg remote.Config) (remote.Config, error) {
		assert.True(t, service.locked)
		return configure(ctx, cfg)
	}
	commands.call = func(ctx context.Context, cfg remote.Config, request remote.Request) (remote.Response, error) {
		assert.True(t, service.locked)
		return call(ctx, cfg, request)
	}
	require.NoError(t, commands.run(context.Background(), a, "enable"))
	assert.False(t, service.locked)
}

func TestRemoteMutationBusyLeavesConfigurationAndServiceUntouched(t *testing.T) {
	for _, action := range []string{"enable", "disable"} {
		t.Run(action, func(t *testing.T) {
			path, a, _, service, commands := remoteCommandFixture(t)
			contents := []byte("enabled = false\nid = 'existing'\n")
			require.NoError(t, os.WriteFile(path, contents, 0o600))
			service.lockErr = errors.New("another remote service operation is in progress")
			require.ErrorContains(t, commands.run(context.Background(), a, action), "another remote service operation")
			data, err := os.ReadFile(path) //nolint:gosec // Test-owned temporary configuration.
			require.NoError(t, err)
			assert.Equal(t, contents, data)
			assert.Zero(t, service.enables)
			assert.Zero(t, service.stops)
		})
	}
}

func TestRemoteEnableReusesExistingConfigWithoutPrompt(t *testing.T) {
	path, a, _, service, commands := remoteCommandFixture(t)
	contents := "# retain this comment\nenabled = false\nid = 'configured'\nherdr_session = 'work'\n"
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
	commands.configure = func(context.Context, remote.Config) (remote.Config, error) {
		t.Fatal("existing config must not prompt")
		return remote.Config{}, nil
	}
	require.NoError(t, commands.run(context.Background(), a, "enable"))
	require.NoError(t, commands.run(context.Background(), a, "enable"))
	cfg, err := remote.LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, "configured", cfg.ID)
	assert.Equal(t, "work", cfg.HerdrSession)
	data, err := os.ReadFile(path) //nolint:gosec // Test-owned temporary configuration.
	require.NoError(t, err)
	assert.Contains(t, string(data), "# retain this comment")
	assert.Equal(t, 2, service.enables)
}

func TestRemoteEnableCancellationHasNoSideEffects(t *testing.T) {
	path, a, _, service, commands := remoteCommandFixture(t)
	commands.configure = func(context.Context, remote.Config) (remote.Config, error) {
		return remote.Config{}, context.Canceled
	}
	require.ErrorIs(t, commands.run(context.Background(), a, "enable"), context.Canceled)
	assert.NoFileExists(t, path)
	assert.Zero(t, service.enables)
}

func TestRemoteEnablePreservesConfigChangedDuringSetup(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprintf("existing=%t", existing), func(t *testing.T) {
			path, a, _, service, commands := remoteCommandFixture(t)
			if existing {
				require.NoError(t, os.WriteFile(path, []byte("enabled = false\n"), 0o600))
			}
			edited := []byte("# saved in another terminal\nenabled = false\nid = 'other-machine'\n[[peers]]\nid = 'important-peer'\nssh = 'peer-host'\n")
			commands.configure = func(context.Context, remote.Config) (remote.Config, error) {
				require.NoError(t, os.WriteFile(path, edited, 0o600))
				return remote.Config{Enabled: true, ID: "prompt-machine", HerdrSession: "default"}, nil
			}
			require.ErrorContains(t, commands.run(context.Background(), a, "enable"), "changed while saving")
			data, err := os.ReadFile(path) //nolint:gosec // Test-owned temporary configuration.
			require.NoError(t, err)
			assert.Equal(t, edited, data)
			assert.Zero(t, service.enables)
		})
	}
}

func TestRemoteEnableUnavailableManagerDoesNotCreateConfig(t *testing.T) {
	path, a, _, service, commands := remoteCommandFixture(t)
	service.statusErr = errors.New("systemd user manager unavailable")
	require.ErrorContains(t, commands.run(context.Background(), a, "enable"), "user manager unavailable")
	assert.NoFileExists(t, path)
	assert.Zero(t, service.enables)
}

func TestRemoteEnableFailureRestoresOptOut(t *testing.T) {
	path, a, _, service, commands := remoteCommandFixture(t)
	require.NoError(t, remote.SaveConfig(path, remote.Config{ID: "laptop", HerdrSession: "default"}))
	service.enableErr = errors.New("bootstrap failed")
	require.ErrorContains(t, commands.run(context.Background(), a, "enable"), "bootstrap failed")
	cfg, err := remote.LoadConfig()
	require.NoError(t, err)
	assert.False(t, cfg.Enabled)
	assert.Equal(t, "laptop", cfg.ID)
}

func TestRemoteEnableFailureRemovesNewConfig(t *testing.T) {
	path, a, _, service, commands := remoteCommandFixture(t)
	service.enableErr = errors.New("bootstrap failed")
	service.onUnlock = func() {
		assert.NoFileExists(t, path, "configuration rollback must finish before unlocking")
	}
	require.ErrorContains(t, commands.run(context.Background(), a, "enable"), "bootstrap failed")
	assert.NoFileExists(t, path)
}

func TestRemoteEnableFailurePreservesConcurrentConfigEdits(t *testing.T) {
	for _, replace := range []bool{false, true} {
		t.Run(fmt.Sprintf("replace=%t", replace), func(t *testing.T) {
			path, a, _, service, commands := remoteCommandFixture(t)
			edited := []byte("enabled = false\nid = 'edited'\n")
			service.enableErr = errors.New("bootstrap failed")
			service.onEnable = func() {
				if replace {
					require.NoError(t, os.Remove(path))
				}
				require.NoError(t, os.WriteFile(path, edited, 0o600))
			}
			err := commands.run(context.Background(), a, "enable")
			require.ErrorContains(t, err, "bootstrap failed")
			require.ErrorContains(t, err, "configuration changed during service activation")
			data, err := os.ReadFile(path) //nolint:gosec // Test-owned temporary configuration.
			require.NoError(t, err)
			assert.Equal(t, edited, data)
		})
	}
}

func TestRemoteEnableRefusesUnmanagedDaemon(t *testing.T) {
	path, a, _, service, commands := remoteCommandFixture(t)
	commands.call = func(context.Context, remote.Config, remote.Request) (remote.Response, error) {
		return remote.Response{}, nil
	}
	require.ErrorContains(t, commands.run(context.Background(), a, "enable"), "unmanaged remote daemon")
	assert.Zero(t, service.enables)
	assert.NoFileExists(t, path)
}

func TestRemoteDisableRetainsPeers(t *testing.T) {
	path, a, output, service, commands := remoteCommandFixture(t)
	cfg := remote.Config{Enabled: true, ID: "laptop", HerdrSession: "default", Peers: []remote.Peer{{ID: "server", SSH: "user@host", Session: "work"}}}
	require.NoError(t, remote.SaveConfig(path, cfg))
	service.onUnlock = func() {
		saved, err := remote.LoadConfig()
		require.NoError(t, err)
		assert.False(t, saved.Enabled, "opt-out must be saved before unlocking")
	}
	require.NoError(t, commands.run(context.Background(), a, "disable"))
	require.NoError(t, commands.run(context.Background(), a, "disable"))
	saved, err := remote.LoadConfig()
	require.NoError(t, err)
	assert.False(t, saved.Enabled)
	assert.Equal(t, cfg.Peers, saved.Peers)
	assert.Equal(t, 2, service.stops)
	assert.Contains(t, output.String(), "peer settings retained")
}

func TestRemoteDisableReportsStopFailureAndStillOptsOut(t *testing.T) {
	path, a, _, service, commands := remoteCommandFixture(t)
	require.NoError(t, remote.SaveConfig(path, remote.Config{Enabled: true, ID: "laptop"}))
	service.stopErr = errors.New("stop failed")
	require.ErrorContains(t, commands.run(context.Background(), a, "disable"), "stop failed")
	cfg, err := remote.LoadConfig()
	require.NoError(t, err)
	assert.False(t, cfg.Enabled)
}

func TestRemoteDisableReportsUnmanagedDaemonEvenWithDisabledConfig(t *testing.T) {
	path, a, _, _, commands := remoteCommandFixture(t)
	require.NoError(t, remote.SaveConfig(path, remote.Config{ID: "laptop"}))
	commands.call = func(_ context.Context, cfg remote.Config, _ remote.Request) (remote.Response, error) {
		assert.True(t, cfg.Enabled, "the health probe must bypass the saved opt-out")
		return remote.Response{}, nil
	}
	require.ErrorContains(t, commands.run(context.Background(), a, "disable"), "a daemon is still responding")
	cfg, err := remote.LoadConfig()
	require.NoError(t, err)
	assert.False(t, cfg.Enabled)
}

func TestRemoteStatusDoesNotEnableOrContactPeersWhenDisabled(t *testing.T) {
	path, a, output, service, commands := remoteCommandFixture(t)
	require.NoError(t, remote.SaveConfig(path, remote.Config{Enabled: false}))
	service.lockErr = errors.New("status must not acquire a mutation lock")
	commands.call = func(context.Context, remote.Config, remote.Request) (remote.Response, error) {
		t.Fatal("disabled status should not query daemon")
		return remote.Response{}, nil
	}
	require.NoError(t, commands.run(context.Background(), a, "status"))
	assert.Contains(t, output.String(), "Remote enabled: false")
	assert.Zero(t, service.enables)
	assert.Zero(t, service.stops)
}

func TestRemoteCommandsRejectInvalidArguments(t *testing.T) {
	a := &App{Out: new(bytes.Buffer), Err: new(bytes.Buffer)}
	for _, args := range [][]string{{"remote"}, {"remote", "enable", "extra"}, {"remote", "unknown"}} {
		require.ErrorContains(t, a.Run(context.Background(), args), "usage: herdr-sesh remote enable|status|disable|doctor")
	}
}

func TestRemoteDisableRejectsUnsafeConfigBeforeStoppingService(t *testing.T) {
	for _, kind := range []string{"symlink", "dangling symlink", "directory"} {
		t.Run(kind, func(t *testing.T) {
			path, a, _, service, commands := remoteCommandFixture(t)
			target := path + ".target"
			contents := []byte("enabled = true\nid = 'laptop'\n")
			if kind == "directory" {
				require.NoError(t, os.Mkdir(path, 0o700))
			} else {
				if kind == "symlink" {
					require.NoError(t, os.WriteFile(target, contents, 0o600))
				}
				require.NoError(t, os.Symlink(target, path))
			}
			err := commands.run(context.Background(), a, "disable")
			require.ErrorContains(t, err, "regular file")
			if kind != "directory" {
				require.ErrorContains(t, err, "configuration is a symlink")
				require.ErrorContains(t, err, "set HERDR_SESH_REMOTE_CONFIG to its real regular-file target")
				link, linkErr := os.Readlink(path)
				require.NoError(t, linkErr)
				assert.Equal(t, target, link)
			}
			assert.Zero(t, service.stops)
			switch kind {
			case "symlink":
				data, err := os.ReadFile(target) //nolint:gosec // Test-owned configuration.
				require.NoError(t, err)
				assert.Equal(t, contents, data)
			case "dangling symlink":
				assert.NoFileExists(t, target)
			}
		})
	}
}

func TestRemoteDisableStopsServiceWithMissingOrInvalidConfig(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		t.Run(fmt.Sprintf("invalid=%t", invalid), func(t *testing.T) {
			path, a, _, service, commands := remoteCommandFixture(t)
			if invalid {
				require.NoError(t, os.WriteFile(path, []byte("enabled = ["), 0o600))
			}
			err := commands.run(context.Background(), a, "disable")
			if invalid {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, 1, service.stops)
		})
	}
}

func TestRemoteStatusValidatesLocalCatalog(t *testing.T) {
	for _, tc := range []struct {
		name    string
		catalog []remote.Machine
		valid   bool
	}{
		{name: "empty"},
		{name: "wrong ID", catalog: []remote.Machine{{ID: "other", Session: "default"}}},
		{name: "wrong session", catalog: []remote.Machine{{ID: "laptop", Session: "other"}}},
		{name: "matching", catalog: []remote.Machine{{ID: "laptop", Session: "default"}}, valid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, a, output, service, commands := remoteCommandFixture(t)
			require.NoError(t, remote.SaveConfig(path, remote.Config{Enabled: true, ID: "laptop", HerdrSession: "default"}))
			service.state.Autostart = true
			commands.call = func(context.Context, remote.Config, remote.Request) (remote.Response, error) {
				return remote.Response{Catalog: tc.catalog}, nil
			}
			err := commands.run(context.Background(), a, "status")
			if tc.valid {
				require.NoError(t, err)
				assert.Contains(t, output.String(), "Local Herdr session laptop (default): ready")
			} else {
				require.ErrorContains(t, err, "daemon identity does not match")
				assert.NotContains(t, output.String(), ": ready")
			}
			assert.Contains(t, output.String(), "Autostart: true")
		})
	}
}

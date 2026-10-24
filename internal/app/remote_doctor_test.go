package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fullerzz/herdr-plugin-sesh/internal/remote"
	"github.com/fullerzz/herdr-plugin-sesh/internal/remoteservice"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func doctorFixture(t *testing.T) (string, *App, *bytes.Buffer, *fakeRemoteService, remoteCommands) {
	t.Helper()
	path, a, output, service, commands := remoteCommandFixture(t)
	cfg := remote.Config{Enabled: true, ID: "laptop", HerdrSession: "default", Peers: []remote.Peer{{ID: "server", SSH: "user@host", Session: "work"}}}
	require.NoError(t, remote.SaveConfig(path, cfg))
	service.state = remoteservice.Status{Installed: true, Running: true, Autostart: true}
	service.lockErr = errors.New("doctor must not acquire a mutation lock")
	commands.call = func(ctx context.Context, got remote.Config, request remote.Request) (remote.Response, error) {
		assert.Equal(t, cfg, got)
		assert.Equal(t, "list", request.Op)
		deadline, ok := ctx.Deadline()
		assert.True(t, ok)
		assert.LessOrEqual(t, time.Until(deadline), 2*time.Second)
		return remote.Response{Catalog: []remote.Machine{{ID: "laptop", Session: "default"}, {ID: "server", Session: "work"}}}, nil
	}
	commands.checkSSH = func(ctx context.Context, peer remote.Peer) error {
		assert.Equal(t, cfg.Peers[0], peer)
		_, ok := ctx.Deadline()
		assert.True(t, ok)
		return nil
	}
	return path, a, output, service, commands
}

func TestRemoteDoctorHealthyReadOnly(t *testing.T) {
	path, a, output, service, commands := doctorFixture(t)
	before, err := os.ReadFile(path) //nolint:gosec // Test-owned configuration.
	require.NoError(t, err)
	entriesBefore, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	require.NoError(t, commands.run(context.Background(), a, "doctor"))
	for _, want := range []string{"OK config: " + path, "OK service:", "OK daemon:", "OK local session:", "OK peer server catalog:", "OK SSH server:"} {
		assert.Contains(t, output.String(), want)
	}
	after, err := os.ReadFile(path) //nolint:gosec // Test-owned configuration.
	require.NoError(t, err)
	assert.Equal(t, before, after)
	entriesAfter, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	assert.Equal(t, entriesBefore, entriesAfter)
	assert.Zero(t, service.enables)
	assert.Zero(t, service.stops)
}

func TestRemoteDoctorAcceptsSymlinkConfiguration(t *testing.T) {
	path, a, output, _, commands := doctorFixture(t)
	target := path + ".target"
	require.NoError(t, os.Rename(path, target))
	require.NoError(t, os.Symlink(target, path))
	require.NoError(t, commands.run(context.Background(), a, "doctor"))
	assert.Contains(t, output.String(), "OK config:")
	assert.Contains(t, output.String(), "OK daemon:")
	assert.Contains(t, output.String(), "OK SSH server:")
	link, err := os.Readlink(path)
	require.NoError(t, err)
	assert.Equal(t, target, link)
}

func TestRemoteDoctorOptOutAndConfigErrorsSkipNetwork(t *testing.T) {
	for _, tc := range []struct {
		name, contents, want     string
		missing, explicit, fails bool
	}{
		{name: "disabled", contents: "enabled = false\n", want: "SKIP config:"},
		{name: "missing default", missing: true, want: "SKIP config:"},
		{name: "missing explicit", missing: true, explicit: true, fails: true, want: "explicit configuration does not exist"},
		{name: "syntax", contents: "enabled = [", fails: true, want: "repair the selected remote.toml"},
		{name: "validation", contents: "enabled = true\nid = ''\n", fails: true, want: "stable identifier"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, a, output, service, commands := remoteCommandFixture(t)
			if !tc.explicit {
				t.Setenv("HERDR_SESH_REMOTE_CONFIG", "")
				t.Setenv("XDG_CONFIG_HOME", filepath.Dir(path))
				var err error
				path, err = remote.ConfigPath()
				require.NoError(t, err)
			}
			if !tc.missing {
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
				require.NoError(t, os.WriteFile(path, []byte(tc.contents), 0o600))
			}
			service.lockErr = errors.New("must not lock")
			commands.call = func(context.Context, remote.Config, remote.Request) (remote.Response, error) {
				t.Fatal("must not query daemon")
				return remote.Response{}, nil
			}
			commands.checkSSH = func(context.Context, remote.Peer) error {
				t.Fatal("must not contact SSH")
				return nil
			}
			err := commands.run(context.Background(), a, "doctor")
			if tc.fails {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Contains(t, output.String(), tc.want)
			assert.Contains(t, output.String(), "SKIP daemon/catalog/SSH:")
			assert.Zero(t, service.enables)
			assert.Zero(t, service.stops)
			if tc.missing {
				assert.NoFileExists(t, path)
			}
		})
	}
}

func TestRemoteDoctorIndependentFailures(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"service unavailable", "FAIL service:"},
		{"service construction", "FAIL service:"},
		{"autostart disabled", "autostart=false"},
		{"service stopped", "installed=true running=false"},
		{"service missing", "installed=false running=false"},
		{"daemon unreachable", "FAIL daemon:"},
		{"identity mismatch", "identity/session does not match"},
		{"local stale", "FAIL local session:"},
		{"peer stale", "FAIL peer server catalog:"},
		{"peer missing", "missing from daemon catalog"},
		{"peer session mismatch", "catalog session does not match"},
		{"SSH failed", "FAIL SSH server:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, a, output, service, commands := doctorFixture(t)
			sshCalls := 0
			commands.checkSSH = func(context.Context, remote.Peer) error {
				sshCalls++
				if tc.name == "SSH failed" {
					return errors.New("permission denied\n\x1b[31m")
				}
				return nil
			}
			call := commands.call
			commands.call = func(ctx context.Context, cfg remote.Config, req remote.Request) (remote.Response, error) {
				r, err := call(ctx, cfg, req)
				switch tc.name {
				case "daemon unreachable":
					return r, errors.New("connection refused")
				case "identity mismatch":
					r.Catalog[0].ID = "other"
				case "local stale":
					r.Catalog[0].Stale = true
					r.Catalog[0].LastError = "Herdr unavailable\n\x1b[31m"
				case "peer stale":
					r.Catalog[1].Stale = true
				case "peer missing":
					r.Catalog = r.Catalog[:1]
				case "peer session mismatch":
					r.Catalog[1].Session = "other"
				}
				return r, err
			}
			switch tc.name {
			case "service unavailable":
				service.statusErr = errors.New("manager unavailable")
			case "service construction":
				commands.service = nil
				commands.serviceErr = errors.New("unsupported platform")
			case "autostart disabled":
				service.state.Autostart = false
			case "service stopped":
				service.state.Running = false
			case "service missing":
				service.state = remoteservice.Status{}
			}
			require.Error(t, commands.run(context.Background(), a, "doctor"))
			assert.Contains(t, output.String(), tc.want)
			assert.Equal(t, 1, sshCalls)
			assert.NotContains(t, output.String(), "\x1b")
			assert.Zero(t, service.enables)
			assert.Zero(t, service.stops)
		})
	}
}

func TestRemoteDoctorFlagsRunningServiceWhenDisabled(t *testing.T) {
	path, a, output, _, commands := doctorFixture(t)
	require.NoError(t, remote.SetEnabled(path, false))
	require.Error(t, commands.run(context.Background(), a, "doctor"))
	assert.Contains(t, output.String(), "while remote support is opted out")
	assert.Contains(t, output.String(), "remote disable")
}

func TestRemoteDoctorCancellation(t *testing.T) {
	_, a, output, _, commands := doctorFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	commands.call = func(ctx context.Context, _ remote.Config, _ remote.Request) (remote.Response, error) {
		return remote.Response{}, ctx.Err()
	}
	commands.checkSSH = func(context.Context, remote.Peer) error { t.Fatal("canceled check should not start SSH"); return nil }
	require.ErrorIs(t, commands.run(ctx, a, "doctor"), context.Canceled)
	assert.Contains(t, output.String(), "SKIP SSH server: check canceled")
}

func TestRemoteDoctorCLIReportsConfigEvenWhenManagerConstructionFails(t *testing.T) {
	_, a, output, _, _ := remoteCommandFixture(t)
	t.Setenv("HOME", "relative-home")
	err := a.Run(context.Background(), []string{"remote", "doctor"})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "usage:")
	assert.Contains(t, output.String(), "FAIL config:")
	assert.Contains(t, output.String(), "FAIL service:")
}

func TestRemoteDoctorSkipsServiceHealthWithInvalidConfig(t *testing.T) {
	path, a, output, _, commands := doctorFixture(t)
	require.NoError(t, os.WriteFile(path, []byte("enabled = ["), 0o600))
	require.Error(t, commands.run(context.Background(), a, "doctor"))
	assert.Contains(t, output.String(), "SKIP service: configuration invalid; installed=true running=true autostart=true")
	assert.NotContains(t, output.String(), "OK service:")
}

func TestRemoteDoctorFlagsInstalledStoppedServiceWhenDisabled(t *testing.T) {
	path, a, output, service, commands := doctorFixture(t)
	require.NoError(t, remote.SetEnabled(path, false))
	service.state.Running = false
	require.Error(t, commands.run(context.Background(), a, "doctor"))
	assert.Contains(t, output.String(), "FAIL service: installed=true running=false autostart=true while remote support is opted out")
	assert.Contains(t, output.String(), "remote disable")
}

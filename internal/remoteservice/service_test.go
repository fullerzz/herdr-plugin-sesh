package remoteservice

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeManager struct {
	t                        *testing.T
	manager                  *Manager
	calls                    []string
	loaded, running, enabled bool
	external, linked         bool
	fail                     string
	failure                  error
}

func setup(t *testing.T, platform string) (*Manager, *fakeManager, Options) {
	t.Helper()
	home := t.TempDir()
	manager, err := newManager(platform, home, "", 501)
	require.NoError(t, err)
	fake := &fakeManager{t: t, manager: manager}
	manager.run = fake.run
	binary := filepath.Join(home, `binary & <space> "quote" %n $BIN \name`)
	require.NoError(t, os.WriteFile(binary, []byte("unused fake executable"), 0o600))
	require.NoError(t, os.Chmod(binary, 0o700)) // #nosec G302 -- executable permission is the validation fixture.
	opts := Options{Executable: binary, HerdrBinary: binary, ConfigPath: filepath.Join(home, "remote.toml")}
	return manager, fake, opts
}

func (f *fakeManager) run(_ context.Context, name string, args ...string) ([]byte, error) {
	call := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, call)
	if call == f.fail {
		f.fail = ""
		return []byte("simulated failure"), f.failure
	}
	if name == "systemctl" {
		return f.systemctl(args[1:])
	}
	return f.launchctl(args)
}

func (f *fakeManager) systemctl(args []string) ([]byte, error) {
	switch args[0] {
	case "show":
		if args[1] == "--property=Version" {
			return []byte("Version=260"), nil
		}
		if !f.loaded {
			return []byte("LoadState=not-found\nActiveState=inactive\nFragmentPath=\nUnitFileState=\n"), nil
		}
		state, enabled := "inactive", "disabled"
		if f.running {
			state = "active"
		}
		if f.enabled {
			enabled = "enabled"
		} else if f.linked {
			enabled = "linked"
		}
		return fmt.Appendf(nil, "LoadState=loaded\nActiveState=%s\nFragmentPath=%s\nUnitFileState=%s\n", state, f.manager.path, enabled), nil
	case "daemon-reload":
		_, err := os.Stat(f.manager.path)
		f.loaded = err == nil && (!f.external || f.linked)
	case "enable":
		if f.external && args[len(args)-1] != f.manager.path {
			return nil, errors.New("unit is outside the manager search path")
		}
		f.linked, f.loaded = f.external, true
		f.enabled = true
		if args[1] == "--now" {
			f.running = true
		}
	case "disable":
		f.enabled, f.linked = false, false
		if f.external {
			f.loaded = false
		}
		if args[1] == "--now" {
			f.running = false
		}
	case "restart":
		if !f.loaded {
			return nil, errors.New("unit not found")
		}
		f.running = true
	case "link":
		f.linked, f.loaded = true, true
	case "stop":
		f.running = false
	default:
		f.t.Errorf("unexpected systemctl command: %v", args)
	}
	return nil, nil
}

func (f *fakeManager) launchctl(args []string) ([]byte, error) {
	switch args[0] {
	case "print":
		if args[1] == f.manager.domain {
			return []byte("gui domain"), nil
		}
		if !f.loaded {
			return []byte(`Could not find service "` + label + `" in domain for user gui: 501`), errors.New("exit status 113")
		}
		state := "waiting"
		if f.running {
			state = "running"
		}
		return fmt.Appendf(nil, "%s = {\n\tpath = %s\n\tstate = %s\n}\n", args[1], f.manager.path, state), nil
	case "print-disabled":
		return fmt.Appendf(nil, `"%s" => %t`, label, !f.enabled), nil
	case "enable":
		f.enabled = true
	case "disable":
		f.enabled = false
	case "bootstrap":
		f.loaded, f.running = true, true
	case "bootout":
		f.loaded, f.running = false, false
	default:
		f.t.Errorf("unexpected launchctl command: %v", args)
	}
	return nil, nil
}

func TestNewManagerDoesNotInstall(t *testing.T) {
	for _, platform := range []string{"linux", "darwin"} {
		t.Run(platform, func(t *testing.T) {
			home := t.TempDir()
			manager, err := newManager(platform, home, "", 501)
			require.NoError(t, err)
			assert.NoFileExists(t, manager.Path())
			entries, err := os.ReadDir(home)
			require.NoError(t, err)
			assert.Empty(t, entries)
			assert.NotEmpty(t, manager.Logs())
		})
	}
	_, err := newManager("windows", t.TempDir(), "", 0)
	require.ErrorContains(t, err, "unsupported")
	_, err = newManager("linux", t.TempDir(), "relative", 0)
	require.ErrorContains(t, err, "absolute")
}

func TestServiceDefinitionsEscapeAndFilterEnvironment(t *testing.T) {
	for _, platform := range []string{"linux", "darwin"} {
		t.Run(platform, func(t *testing.T) {
			manager, _, opts := setup(t, platform)
			opts.Environment = []string{
				`PATH=/custom path/bin:/usr/bin`,
				`XDG_CONFIG_HOME=/config home`, `XDG_CACHE_HOME=/cache home`,
				`HERDR_CONFIG_PATH=/herdr config.toml`, `SSH_AUTH_SOCK=/tmp/ssh-agent.sock`,
				`HERDR_SESH_REMOTE_CONFIG=/wrong/config`, `HERDR_BIN_PATH=/wrong/binary`,
				`SECRET_TOKEN=never-persist-this`, `MY_VAR=ignored`,
			}
			data, err := manager.definition(opts)
			require.NoError(t, err)
			text := string(data)
			for _, value := range []string{"/config home", "/cache home", "/herdr config.toml", "/custom path/bin:/usr/bin"} {
				assert.Contains(t, text, value)
			}
			assert.NotContains(t, text, "never-persist-this")
			assert.NotContains(t, text, "/wrong")
			assert.Contains(t, text, "HERDR_SESH_REMOTE_CONFIG")
			assert.Contains(t, text, "HERDR_BIN_PATH")
			if platform == "linux" {
				assert.Contains(t, text, "SSH_AUTH_SOCK=/tmp/ssh-agent.sock")
				assert.Contains(t, text, `ExecStart=:"`)
				assert.Contains(t, text, `\"quote\" %%n $BIN \\name`)
				assert.Contains(t, text, "RestartSec=5")
				assert.Contains(t, text, "WantedBy=default.target")
				return
			}
			assert.NotContains(t, text, "SSH_AUTH_SOCK")
			assert.NotContains(t, text, "/tmp/ssh-agent.sock")
			decoder := xml.NewDecoder(strings.NewReader(text))
			var values []string
			for {
				token, decodeErr := decoder.Token()
				if errors.Is(decodeErr, io.EOF) {
					break
				}
				require.NoError(t, decodeErr)
				if content, ok := token.(xml.CharData); ok {
					values = append(values, string(content))
				}
			}
			assert.Contains(t, values, opts.Executable)
			assert.Contains(t, values, opts.ConfigPath)
			assert.Contains(t, values, manager.logPath)
			if runtime.GOOS == "darwin" {
				path := filepath.Join(t.TempDir(), "daemon.plist")
				require.NoError(t, os.WriteFile(path, data, 0o600))
				output, lintErr := exec.CommandContext(t.Context(), "/usr/bin/plutil", "-lint", path).CombinedOutput() // #nosec G204 -- validates only a generated temporary plist.
				require.NoError(t, lintErr, "%s", output)
			}
		})
	}
}

func TestEnableDisableOrderingAndIdempotency(t *testing.T) {
	for _, platform := range []string{"linux", "darwin"} {
		t.Run(platform, func(t *testing.T) {
			manager, fake, opts := setup(t, platform)
			require.NoError(t, os.WriteFile(opts.ConfigPath, []byte("preserved remote config"), 0o600))
			require.NoError(t, manager.Enable(t.Context(), opts))
			assert.True(t, fake.running)
			assert.True(t, fake.enabled)
			require.FileExists(t, manager.path)
			if platform == "linux" {
				assert.Equal(t, []string{"systemctl --user daemon-reload", "systemctl --user enable --now " + manager.path}, fake.calls[len(fake.calls)-2:])
			} else {
				assert.Equal(t, []string{"launchctl enable gui/501/" + label, "launchctl bootstrap gui/501 " + manager.path}, fake.calls[len(fake.calls)-2:])
			}
			require.NoError(t, manager.Enable(t.Context(), opts))
			status, err := manager.Status(t.Context())
			require.NoError(t, err)
			assert.True(t, status.Installed)
			assert.True(t, status.Running)
			if platform == "linux" {
				assert.Contains(t, fake.calls, "systemctl --user restart "+unit)
			} else {
				assert.Contains(t, fake.calls, "launchctl bootout gui/501/"+label)
			}
			require.NoError(t, manager.Disable(t.Context()))
			assert.False(t, fake.running)
			assert.False(t, fake.enabled)
			assert.NoFileExists(t, manager.path)
			config, readErr := os.ReadFile(opts.ConfigPath)
			require.NoError(t, readErr)
			assert.Equal(t, "preserved remote config", string(config))
			require.NoError(t, manager.Disable(t.Context()))
			status, err = manager.Status(t.Context())
			require.NoError(t, err)
			assert.False(t, status.Installed)
			assert.False(t, status.Running)
		})
	}
}

func TestMissingManagerIsAnErrorBeforeInstallation(t *testing.T) {
	for _, platform := range []string{"linux", "darwin"} {
		t.Run(platform, func(t *testing.T) {
			manager, _, opts := setup(t, platform)
			manager.run = func(context.Context, string, ...string) ([]byte, error) { return nil, exec.ErrNotFound }
			_, err := manager.Status(t.Context())
			require.Error(t, err)
			require.Error(t, manager.Enable(t.Context(), opts))
			require.Error(t, manager.Disable(t.Context()))
			assert.NoFileExists(t, manager.path)
		})
	}
}

func TestLinuxServiceOutsideManagerSearchPath(t *testing.T) {
	manager, fake, opts := setup(t, "linux")
	custom, err := newManager("linux", t.TempDir(), filepath.Join(t.TempDir(), "custom config"), 501)
	require.NoError(t, err)
	manager.path = custom.path
	fake.external = true

	require.NoError(t, manager.Enable(t.Context(), opts))
	assert.Contains(t, fake.calls, "systemctl --user enable --now "+manager.path)
	assert.True(t, fake.linked)
	fake.fail, fake.failure = "systemctl --user restart "+unit, errors.New("restart failed")
	require.ErrorContains(t, manager.Enable(t.Context(), opts), "service recovery completed")
	assert.Contains(t, fake.calls, "systemctl --user enable "+manager.path)
	assert.True(t, fake.running)
	assert.True(t, fake.enabled)

	// A linked unit can be running without being enabled for the next login.
	fake.enabled = false
	fake.fail, fake.failure = "systemctl --user restart "+unit, errors.New("restart failed")
	require.ErrorContains(t, manager.Enable(t.Context(), opts), "service recovery completed")
	assert.True(t, fake.running)
	assert.True(t, fake.linked)
	assert.False(t, fake.enabled)
	require.NoError(t, manager.Disable(t.Context()))
	assert.False(t, fake.linked)
	assert.False(t, fake.running)
	assert.NoFileExists(t, manager.path)

	original := manager.run
	manager.run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		output, runErr := original(ctx, name, args...)
		if len(args) > 1 && args[1] == "enable" && runErr == nil {
			return nil, errors.New("start failed after linking")
		}
		return output, runErr
	}
	require.ErrorContains(t, manager.Enable(t.Context(), opts), "service recovery completed")
	assert.False(t, fake.linked)
	assert.False(t, fake.running)
	assert.NoFileExists(t, manager.path)
}

func TestRefuseUnmanagedDefinitionsAndLoadedJobs(t *testing.T) {
	for _, platform := range []string{"linux", "darwin"} {
		t.Run(platform, func(t *testing.T) {
			manager, fake, opts := setup(t, platform)
			require.NoError(t, os.MkdirAll(filepath.Dir(manager.path), 0o700))
			require.NoError(t, os.WriteFile(manager.path, []byte("unrelated"), 0o600))
			require.ErrorContains(t, manager.Enable(t.Context(), opts), "unmanaged")
			require.ErrorContains(t, manager.Disable(t.Context()), "unmanaged")
			assert.Empty(t, fake.calls)
			require.NoError(t, os.Remove(manager.path))
			fake.loaded = true
			require.ErrorContains(t, manager.Enable(t.Context(), opts), "without a managed definition")
			require.ErrorContains(t, manager.Disable(t.Context()), "without a managed definition")
			_, err := manager.Status(t.Context())
			require.ErrorContains(t, err, "without a managed definition")
			assert.True(t, fake.loaded)
			require.NoError(t, os.Symlink(opts.Executable, manager.path))
			require.ErrorContains(t, manager.Enable(t.Context(), opts), "not a regular file")
		})
	}
}

func TestInvalidOptionsCannotInstall(t *testing.T) {
	manager, fake, original := setup(t, "linux")
	for _, change := range []func(*Options){
		func(o *Options) { o.Executable = "relative" },
		func(o *Options) { o.HerdrBinary = "/does/not/exist" },
		func(o *Options) { o.ConfigPath = "/config\nInjected=yes" },
		func(o *Options) { o.Environment = []string{"PATH=/bin\nInjected=yes"} },
		func(o *Options) { o.Environment = []string{"XDG_CONFIG_HOME=relative"} },
	} {
		opts := original
		change(&opts)
		require.Error(t, manager.Enable(t.Context(), opts))
	}
	assert.Empty(t, fake.calls)
	assert.NoFileExists(t, manager.path)
}

func TestActivationFailureRollsBack(t *testing.T) {
	for _, platform := range []string{"linux", "darwin"} {
		for _, installed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/installed=%t", platform, installed), func(t *testing.T) {
				manager, fake, opts := setup(t, platform)
				var original []byte
				if installed {
					require.NoError(t, manager.Enable(t.Context(), opts))
					var err error
					original, err = os.ReadFile(manager.path)
					require.NoError(t, err)
				}
				fake.failure = errors.New("start failed")
				if platform == "darwin" {
					fake.fail = "launchctl bootstrap gui/501 " + manager.path
				} else if installed {
					fake.fail = "systemctl --user restart " + unit
				} else {
					fake.fail = "systemctl --user enable --now " + manager.path
				}
				opts.Environment = []string{"PATH=/new/bin"}
				err := manager.Enable(t.Context(), opts)
				require.ErrorContains(t, err, "previous definition restored and service recovery completed")
				assert.Equal(t, installed, fake.running)
				assert.Equal(t, installed, fake.enabled)
				if installed {
					restored, readErr := os.ReadFile(manager.path)
					require.NoError(t, readErr)
					assert.Equal(t, original, restored)
				} else {
					assert.NoFileExists(t, manager.path)
				}
			})
		}
	}
}

func TestDisableFailurePreservesManagedDefinition(t *testing.T) {
	for _, platform := range []string{"linux", "darwin"} {
		t.Run(platform, func(t *testing.T) {
			manager, fake, opts := setup(t, platform)
			require.NoError(t, manager.Enable(t.Context(), opts))
			fake.failure = errors.New("stop failed")
			if platform == "darwin" {
				fake.fail = "launchctl bootout gui/501/" + label
			} else {
				fake.fail = "systemctl --user disable --now " + unit
			}
			require.ErrorContains(t, manager.Disable(t.Context()), "partially disabled")
			assert.FileExists(t, manager.path)
			assert.True(t, fake.running)
			require.NoError(t, manager.Disable(t.Context()))
		})
	}
}

func TestAtomicWriteFailureLeavesDestinationIntact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing-directory")
	require.NoError(t, os.Mkdir(path, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(path, "sentinel"), []byte("keep"), 0o600))
	require.Error(t, writeAtomic(path, []byte("replacement")))
	contents, err := os.ReadFile(filepath.Join(path, "sentinel")) // #nosec G304 -- isolated test fixture.
	require.NoError(t, err)
	assert.Equal(t, "keep", string(contents))
	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

func TestStatusDoesNotHideInspectionErrors(t *testing.T) {
	for _, platform := range []string{"linux", "darwin"} {
		t.Run(platform, func(t *testing.T) {
			manager, fake, opts := setup(t, platform)
			require.NoError(t, manager.Enable(t.Context(), opts))
			original := manager.run
			manager.run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
				if (name == "systemctl" && len(args) > 2 && args[2] == unit) || (name == "launchctl" && len(args) > 1 && args[0] == "print" && args[1] == manager.domain+"/"+label) {
					return []byte("permission denied"), errors.New("inspection failed")
				}
				return original(ctx, name, args...)
			}
			status, err := manager.Status(t.Context())
			require.ErrorContains(t, err, "permission denied")
			assert.True(t, status.Installed)
			require.Error(t, manager.Disable(t.Context()))
			assert.True(t, fake.running)
		})
	}
}

func TestFailedRollbackKeepsRecoverableDefinition(t *testing.T) {
	manager, _, opts := setup(t, "linux")
	original := manager.run
	manager.run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if len(args) > 1 && (args[1] == "enable" || args[1] == "disable") {
			return nil, errors.New("manager mutation failed")
		}
		return original(ctx, name, args...)
	}
	require.ErrorContains(t, manager.Enable(t.Context(), opts), "service recovery incomplete")
	assert.FileExists(t, manager.path)
	manager.run = original
	require.NoError(t, manager.Disable(t.Context()))
}

func TestRollbackPreservesDisabledAutostart(t *testing.T) {
	for _, platform := range []string{"linux", "darwin"} {
		t.Run(platform, func(t *testing.T) {
			manager, fake, opts := setup(t, platform)
			require.NoError(t, manager.Enable(t.Context(), opts))
			fake.enabled = false
			fake.failure = errors.New("restart failed")
			if platform == "darwin" {
				fake.fail = "launchctl bootstrap gui/501 " + manager.path
			} else {
				fake.fail = "systemctl --user restart " + unit
			}
			require.Error(t, manager.Enable(t.Context(), opts))
			assert.False(t, fake.enabled)
			assert.True(t, fake.running)
		})
	}
}

func TestStatusReportsAutostart(t *testing.T) {
	for _, platform := range []string{"linux", "darwin"} {
		t.Run(platform, func(t *testing.T) {
			manager, fake, opts := setup(t, platform)
			fake.enabled = true
			status, err := manager.Status(t.Context())
			require.NoError(t, err)
			assert.False(t, status.Autostart)
			require.NoError(t, manager.Enable(t.Context(), opts))
			status, err = manager.Status(t.Context())
			require.NoError(t, err)
			assert.True(t, status.Autostart)
			fake.enabled = false
			status, err = manager.Status(t.Context())
			require.NoError(t, err)
			assert.True(t, status.Running)
			assert.False(t, status.Autostart)
		})
	}
}

func TestDarwinWaitsForUnloadBeforeActivationAndRollback(t *testing.T) {
	for _, rollback := range []bool{false, true} {
		t.Run(fmt.Sprintf("rollback=%t", rollback), func(t *testing.T) {
			manager, fake, opts := setup(t, "darwin")
			require.NoError(t, manager.Enable(t.Context(), opts))
			original := manager.run
			pending := 0
			manager.run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
				if args[0] == "bootstrap" {
					require.Zero(t, pending, "bootstrap must wait for job removal")
				}
				if args[0] == "print" && args[1] != manager.domain && pending > 0 {
					pending--
					if pending == 0 {
						fake.loaded, fake.running = false, false
					}
				}
				output, err := original(ctx, name, args...)
				if args[0] == "bootout" && err == nil {
					pending = 3
					fake.loaded, fake.running = true, true
				}
				return output, err
			}
			if rollback {
				data, err := os.ReadFile(manager.path)
				require.NoError(t, err)
				require.NoError(t, manager.rollback(t.Context(), data, nativeStatus{loaded: true, running: true, enabled: true}))
			} else {
				require.NoError(t, manager.Enable(t.Context(), opts))
			}
			assert.True(t, fake.running)
		})
	}
}

func TestDarwinUnloadPropagatesWaitFailures(t *testing.T) {
	for _, mode := range []string{"canceled", "timeout", "inspection"} {
		t.Run(mode, func(t *testing.T) {
			manager, fake, opts := setup(t, "darwin")
			require.NoError(t, manager.Enable(t.Context(), opts))
			original := manager.run
			ctx, cancel := context.WithTimeout(t.Context(), 40*time.Millisecond)
			defer cancel()
			manager.run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
				if args[0] == "bootout" {
					if mode == "canceled" {
						cancel()
					}
					return nil, nil // Simulate teardown that has not completed.
				}
				if mode == "inspection" && args[0] == "print" && args[1] != manager.domain {
					return []byte("inspection unavailable"), errors.New("inspection failure")
				}
				return original(ctx, name, args...)
			}
			err := manager.unload(ctx)
			switch mode {
			case "canceled":
				require.ErrorIs(t, err, context.Canceled)
			case "timeout":
				require.ErrorIs(t, err, context.DeadlineExceeded)
			case "inspection":
				require.ErrorContains(t, err, "inspection unavailable")
			}
			assert.True(t, fake.loaded)
		})
	}
}

func TestDarwinRollbackPreservesStoppedJob(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("enabled=%t", enabled), func(t *testing.T) {
			manager, fake, opts := setup(t, "darwin")
			require.NoError(t, manager.Enable(t.Context(), opts))
			previous, err := os.ReadFile(manager.path)
			require.NoError(t, err)
			fake.running, fake.enabled = false, enabled
			fake.fail = "launchctl bootstrap gui/501 " + manager.path
			fake.failure = errors.New("bootstrap failed")
			opts.Environment = []string{"PATH=/new/bin"}
			require.ErrorContains(t, manager.Enable(t.Context(), opts), "service recovery completed")
			assert.False(t, fake.running)
			assert.False(t, fake.loaded)
			assert.Equal(t, enabled, fake.enabled)
			restored, err := os.ReadFile(manager.path)
			require.NoError(t, err)
			assert.Equal(t, previous, restored)
		})
	}
}

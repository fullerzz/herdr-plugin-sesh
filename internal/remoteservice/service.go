package remoteservice

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type Status struct {
	Installed bool
	Running   bool
	Autostart bool
	Detail    string
}

type Manager struct {
	platform string
	path     string
	logPath  string
	lockPath string
	domain   string
	run      func(context.Context, string, ...string) ([]byte, error)
}

func New() (*Manager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return newManager(runtime.GOOS, home, os.Getenv("XDG_CONFIG_HOME"), os.Getuid())
}

func newManager(platform, home, configHome string, uid int) (*Manager, error) {
	if err := validPath("home", home); err != nil {
		return nil, err
	}
	m := &Manager{platform: platform, run: runCommand, lockPath: filepath.Join(home, ".herdr-sesh-service.lock")}
	switch platform {
	case "darwin":
		m.path = filepath.Join(home, "Library", "LaunchAgents", label+".plist")
		m.logPath = filepath.Join(home, "Library", "Logs", "herdr-sesh", "daemon.log")
		m.domain = "gui/" + strconv.Itoa(uid)
	case "linux":
		if configHome == "" {
			configHome = filepath.Join(home, ".config")
		}
		if err := validPath("XDG_CONFIG_HOME", configHome); err != nil {
			return nil, err
		}
		m.path = filepath.Join(configHome, "systemd", "user", unit)
	default:
		return nil, fmt.Errorf("remote daemon services are unsupported on %s; run herdr-sesh daemon serve in the foreground", platform)
	}
	return m, nil
}

func (m *Manager) Path() string { return m.path }
func (m *Manager) Logs() string {
	if m.platform == "darwin" {
		return m.logPath
	}
	return "journalctl --user -u " + unit
}

func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...) // #nosec G204 -- fixed native service manager and separate arguments.
	cmd.WaitDelay = time.Second
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return output, ctx.Err()
	}
	return output, err
}

func commandError(name string, output []byte, err error) error {
	return fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(string(output)))
}

func (m *Manager) command(ctx context.Context, args ...string) error {
	name := "launchctl"
	if m.platform == "linux" {
		name = "systemctl"
		args = append([]string{"--user"}, args...)
	}
	output, err := m.run(ctx, name, args...)
	if err != nil {
		return commandError(name+" "+strings.Join(args, " "), output, err)
	}
	return nil
}

func (m *Manager) available(ctx context.Context) error {
	var err error
	if m.platform == "linux" {
		err = m.command(ctx, "show", "--property=Version")
	} else {
		err = m.command(ctx, "print", m.domain)
	}
	if err != nil {
		if m.platform == "linux" {
			return fmt.Errorf("systemd user services are unavailable; use a systemd user login session or run herdr-sesh daemon serve in the foreground: %w", err)
		}
		return fmt.Errorf("LaunchAgent requires an active macOS GUI login session: %w", err)
	}
	return nil
}

func (m *Manager) readManaged() ([]byte, error) {
	info, err := os.Lstat(m.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("refusing unmanaged service file %s (not a regular file)", m.path)
	}
	data, err := os.ReadFile(m.path) // #nosec G304 -- derived per-user service path.
	if err != nil {
		return nil, err
	}
	prefix := "# " + marker + "\n"
	if m.platform == "darwin" {
		prefix = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<!-- " + marker + " -->\n"
	}
	if !bytes.HasPrefix(data, []byte(prefix)) {
		return nil, fmt.Errorf("refusing unmanaged service file %s", m.path)
	}
	return data, nil
}

type nativeStatus struct {
	loaded, running, enabled bool
	linked                   bool
	detail                   string
}

func (m *Manager) inspect(ctx context.Context) (nativeStatus, error) {
	if m.platform == "linux" {
		output, err := m.run(ctx, "systemctl", "--user", "show", unit, "--property=LoadState", "--property=ActiveState", "--property=FragmentPath", "--property=UnitFileState")
		fields := make(map[string]string)
		for line := range strings.SplitSeq(string(output), "\n") {
			key, value, ok := strings.Cut(line, "=")
			if ok {
				fields[key] = value
			}
		}
		if fields["LoadState"] == "not-found" && (err == nil || exitCode(err) == 1) {
			if fields["ActiveState"] != "inactive" && fields["ActiveState"] != "failed" {
				return nativeStatus{}, errors.New("service has no definition but may still be running; inspect systemctl --user status herdr-sesh-daemon.service")
			}
			return nativeStatus{detail: "not loaded"}, nil
		}
		if err != nil {
			return nativeStatus{}, commandError("systemctl --user show", output, err)
		}
		if fields["LoadState"] == "" {
			return nativeStatus{}, errors.New("systemctl returned no service load state")
		}
		if fields["FragmentPath"] != m.path {
			return nativeStatus{}, fmt.Errorf("refusing service loaded from another definition: %s", fields["FragmentPath"])
		}
		state := fields["ActiveState"]
		return nativeStatus{loaded: true, running: state == "active", enabled: fields["UnitFileState"] == "enabled", linked: fields["UnitFileState"] == "linked", detail: state}, nil
	}
	output, err := m.run(ctx, "launchctl", "print", m.domain+"/"+label)
	if err != nil {
		if strings.Contains(string(output), "Could not find service \""+label+"\"") {
			enabled, enabledErr := m.launchdEnabled(ctx)
			return nativeStatus{enabled: enabled, detail: "not loaded"}, enabledErr
		}
		return nativeStatus{}, commandError("launchctl print", output, err)
	}
	var path, state string
	for line := range strings.SplitSeq(string(output), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), " = ")
		if ok && key == "path" && path == "" {
			path = value
		}
		if ok && key == "state" && state == "" {
			state = value
		}
	}
	if path != m.path {
		return nativeStatus{}, fmt.Errorf("refusing service loaded from another definition: %s", path)
	}
	enabled, err := m.launchdEnabled(ctx)
	return nativeStatus{loaded: true, running: state == "running", enabled: enabled, detail: state}, err
}

func (m *Manager) launchdEnabled(ctx context.Context) (bool, error) {
	disabled, err := m.run(ctx, "launchctl", "print-disabled", m.domain)
	if err != nil {
		return false, commandError("launchctl print-disabled", disabled, err)
	}
	return !strings.Contains(string(disabled), `"`+label+`" => true`), nil
}

func exitCode(err error) int {
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	return -1
}

func (m *Manager) Status(ctx context.Context) (Status, error) {
	data, err := m.readManaged()
	if err != nil {
		return Status{}, err
	}
	status := Status{Installed: data != nil}
	if err = m.available(ctx); err != nil {
		return status, err
	}
	native, err := m.inspect(ctx)
	if err != nil {
		return status, err
	}
	if data == nil && native.loaded {
		return status, errors.New("refusing loaded service without a managed definition")
	}
	status.Running, status.Detail = native.running, native.detail
	status.Autostart = status.Installed && native.enabled
	return status, nil
}

// Enable restores configuration before recovering a previous service after activation fails.
func (m *Manager) Enable(ctx context.Context, opts Options, restoreConfig func() error) error {
	return m.WithLock(ctx, func(ctx context.Context) error { return m.enable(ctx, opts, restoreConfig) })
}

func (m *Manager) enable(ctx context.Context, opts Options, restoreConfig func() error) error {
	definition, err := m.definition(opts)
	if err != nil {
		return err
	}
	previous, err := m.readManaged()
	if err != nil {
		return err
	}
	if err = m.available(ctx); err != nil {
		return err
	}
	native, err := m.inspect(ctx)
	if err != nil {
		return err
	}
	if previous == nil && native.loaded {
		return errors.New("refusing loaded service without a managed definition")
	}
	if m.platform == "darwin" {
		if err = os.MkdirAll(filepath.Dir(m.logPath), 0o700); err != nil {
			return err
		}
	}
	if err = writeAtomic(m.path, definition); err != nil {
		return fmt.Errorf("write service definition (previous definition preserved): %w", err)
	}
	if err = m.activate(ctx, native.loaded); err != nil {
		recoveryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		var restoreErr error
		if restoreConfig != nil {
			restoreErr = restoreConfig()
		}
		if restoreErr != nil {
			// Stop any newly active daemon rather than restart against uncertain settings.
			native.running = false
		}
		recoveryErr := errors.Join(restoreErr, m.rollback(recoveryCtx, previous, native))
		if recoveryErr != nil {
			return errors.Join(fmt.Errorf("service activation failed: %w", err), fmt.Errorf("service recovery incomplete; inspect %s and service status: %w", m.path, recoveryErr))
		}
		return fmt.Errorf("service activation failed; previous definition restored and service recovery completed: %w", err)
	}
	return nil
}

func (m *Manager) rollback(ctx context.Context, previous []byte, before nativeStatus) error {
	if previous == nil {
		// Keep the definition if stopping fails so a later disable can still identify it.
		if err := m.Disable(ctx); err != nil {
			return err
		}
		return nil
	}
	if err := writeAtomic(m.path, previous); err != nil {
		return err
	}
	if m.platform == "linux" {
		if err := m.command(ctx, "daemon-reload"); err != nil {
			return err
		}
		action, target := "disable", unit
		if before.enabled {
			action, target = "enable", m.path
		}
		enableErr := m.command(ctx, action, target)
		if before.linked {
			// Disabling removes the search-path link as well as autostart links.
			enableErr = errors.Join(enableErr, m.command(ctx, "link", m.path))
		}
		action = "stop"
		if before.running {
			action = "restart"
		}
		return errors.Join(enableErr, m.command(ctx, action, unit))
	}
	current, err := m.inspect(ctx)
	if err != nil {
		return err
	}
	if current.loaded {
		if err = m.unload(ctx); err != nil {
			return err
		}
	}
	// RunAtLoad and KeepAlive would restart a previously stopped job on bootstrap.
	if before.loaded && before.running {
		if err = m.command(ctx, "enable", m.domain+"/"+label); err != nil {
			return err
		}
		if err = m.command(ctx, "bootstrap", m.domain, m.path); err != nil {
			return err
		}
	}
	action := "disable"
	if before.enabled {
		action = "enable"
	}
	return m.command(ctx, action, m.domain+"/"+label)
}

func (m *Manager) activate(ctx context.Context, loaded bool) error {
	if m.platform == "linux" {
		if err := m.command(ctx, "daemon-reload"); err != nil {
			return err
		}
		// The user manager may have started with a different XDG_CONFIG_HOME.
		if err := m.command(ctx, "enable", "--now", m.path); err != nil {
			return err
		}
		if loaded {
			return m.command(ctx, "restart", unit)
		}
		return nil
	}
	target := m.domain + "/" + label
	if err := m.command(ctx, "enable", target); err != nil {
		return err
	}
	if loaded {
		if err := m.unload(ctx); err != nil {
			return err
		}
	}
	return m.command(ctx, "bootstrap", m.domain, m.path)
}

func (m *Manager) unload(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := m.command(ctx, "bootout", m.domain+"/"+label); err != nil {
		return err
	}
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("wait for LaunchAgent removal: %w", err)
		}
		native, err := m.inspect(ctx)
		if err != nil {
			return err
		}
		if !native.loaded {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for LaunchAgent removal: %w", ctx.Err())
		case <-time.After(25 * time.Millisecond):
		}
	}
}

func (m *Manager) Disable(ctx context.Context) error {
	return m.WithLock(ctx, m.disable)
}

func (m *Manager) disable(ctx context.Context) error {
	data, err := m.readManaged()
	if err != nil {
		return err
	}
	if err = m.available(ctx); err != nil {
		return err
	}
	native, err := m.inspect(ctx)
	if err != nil {
		return err
	}
	if data == nil {
		if native.loaded {
			return errors.New("refusing loaded service without a managed definition")
		}
		return nil
	}
	if m.platform == "linux" {
		err = m.command(ctx, "disable", "--now", unit)
	} else {
		err = m.command(ctx, "disable", m.domain+"/"+label)
		if err == nil && native.loaded {
			err = m.unload(ctx)
		}
	}
	if err != nil {
		return fmt.Errorf("service may be partially disabled; definition preserved at %s: %w", m.path, err)
	}
	if err = os.Remove(m.path); err != nil {
		return fmt.Errorf("service stopped and disabled, but definition removal failed: %w", err)
	}
	if m.platform == "linux" {
		if err = m.command(ctx, "daemon-reload"); err != nil {
			return fmt.Errorf("service stopped and definition removed, but daemon reload failed: %w", err)
		}
	}
	return nil
}

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".herdr-sesh-service-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(file.Name()) }()
	if _, err = file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

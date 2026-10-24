package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
	"github.com/fullerzz/herdr-plugin-sesh/internal/remote"
	"github.com/fullerzz/herdr-plugin-sesh/internal/remoteservice"
	"github.com/fullerzz/herdr-plugin-sesh/internal/remotesetup"
)

type remoteService interface {
	WithLock(context.Context, func(context.Context) error) error
	Enable(context.Context, remoteservice.Options) error
	Disable(context.Context) error
	Status(context.Context) (remoteservice.Status, error)
	Path() string
	Logs() string
}

type remoteCommands struct {
	service    remoteService
	serviceErr error
	checkSSH   func(context.Context, remote.Peer) error
	configure  func(context.Context, remote.Config) (remote.Config, error)
	call       func(context.Context, remote.Config, remote.Request) (remote.Response, error)
}

func (a *App) remote(ctx context.Context, args []string) error {
	if len(args) != 1 || (args[0] != "enable" && args[0] != "disable" && args[0] != "status" && args[0] != "doctor") {
		return errors.New("usage: herdr-sesh remote enable|status|disable|doctor")
	}
	service, err := remoteservice.New()
	if err != nil && args[0] != "doctor" {
		return err
	}
	commands := remoteCommands{service: service, serviceErr: err, checkSSH: remote.CheckSSH, call: remote.Call, configure: func(ctx context.Context, cfg remote.Config) (remote.Config, error) {
		if !term.IsTerminal(os.Stdin.Fd()) || !term.IsTerminal(os.Stdout.Fd()) {
			return cfg, errors.New("initial remote setup requires an interactive terminal; create remote.toml first for unattended setup")
		}
		return remotesetup.Configure(ctx, cfg, os.Stdin, a.Out)
	}}
	return commands.run(ctx, a, args[0])
}

func (c remoteCommands) run(ctx context.Context, a *App, action string) error {
	if action == "doctor" {
		return c.doctor(ctx, a)
	}
	if action == "enable" || action == "disable" {
		return c.service.WithLock(ctx, func(ctx context.Context) error {
			return c.runAction(ctx, a, action)
		})
	}
	return c.runAction(ctx, a, action)
}

func (c remoteCommands) runAction(ctx context.Context, a *App, action string) error {
	path, err := remote.ConfigPath()
	if err != nil {
		return err
	}
	switch action {
	case "enable":
		return c.enable(ctx, a, path)
	case "disable":
		info, statErr := os.Lstat(path)
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return fmt.Errorf("disable remote support: %w", statErr)
		}
		if statErr == nil && !info.Mode().IsRegular() {
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("disable remote support: %s must be a regular file; configuration is a symlink, so service state was retained; set HERDR_SESH_REMOTE_CONFIG to its real regular-file target and rerun 'herdr-sesh remote disable'", path)
			}
			return fmt.Errorf("disable remote support: %s must be a regular file; service state was retained", path)
		}
		stopErr := c.service.Disable(ctx)
		cfg, loadErr := remote.LoadConfig()
		var configErr error
		if statErr == nil {
			configErr = remote.SetEnabled(path, false)
		}
		if loadErr == nil && statErr == nil {
			// Config opt-out does not stop a daemon that already loaded its settings.
			cfg.Enabled = true
			probeCtx, cancel := context.WithTimeout(ctx, time.Second)
			_, probeErr := c.call(probeCtx, cfg, remote.Request{Op: "list"})
			cancel()
			if probeErr == nil {
				stopErr = errors.Join(stopErr, errors.New("a daemon is still responding; stop any manually started daemon and check the service logs"))
			}
		}
		if err := errors.Join(stopErr, configErr); err != nil {
			return fmt.Errorf("disable remote support: %w", err)
		}
		_, err := fmt.Fprintln(a.Out, "Remote support disabled. Service stopped and autostart removed; peer settings retained.")
		return err
	case "status":
		return c.status(ctx, a, path)
	default:
		return errors.New("usage: herdr-sesh remote enable|status|disable|doctor")
	}
}

func (c remoteCommands) enable(ctx context.Context, a *App, path string) error {
	service, err := c.service.Status(ctx)
	if err != nil {
		return err
	}
	edit, err := remote.PrepareConfig(path)
	if err != nil {
		return err
	}
	cfg := edit.Config
	needsSetup := !edit.Exists || cfg.ID == ""
	if needsSetup {
		cfg, err = c.configure(ctx, cfg)
		if err != nil {
			return err
		}
	}
	cfg.Enabled = true
	if err := cfg.Validate(); err != nil {
		return err
	}
	herdrBin, err := exec.LookPath(herdr.NewCLIClient().Bin)
	if err != nil {
		return fmt.Errorf("find Herdr executable: %w", err)
	}
	herdrBin, err = filepath.Abs(herdrBin)
	if err != nil {
		return err
	}
	if len(cfg.Peers) > 0 {
		if _, err := exec.LookPath("ssh"); err != nil {
			return fmt.Errorf("remote peers require ssh: %w", err)
		}
	}
	bin, err := os.Executable()
	if err != nil {
		return err
	}
	if !service.Running {
		probeCtx, cancel := context.WithTimeout(ctx, time.Second)
		_, probeErr := c.call(probeCtx, cfg, remote.Request{Op: "list"})
		cancel()
		if probeErr == nil {
			return errors.New("an unmanaged remote daemon is already running; stop it before enabling the background service")
		}
	}
	if needsSetup {
		err = edit.Save(cfg)
	} else {
		err = edit.SetEnabled(true)
	}
	if err != nil {
		return err
	}
	opts := remoteservice.Options{Executable: bin, ConfigPath: path, HerdrBinary: herdrBin, Environment: os.Environ()}
	if err := c.service.Enable(ctx, opts); err != nil {
		restoreErr := edit.Restore()
		if restoreErr != nil {
			restoreErr = fmt.Errorf("restore remote configuration %s: %w", path, restoreErr)
		}
		return errors.Join(fmt.Errorf("enable background service: %w", err), restoreErr)
	}
	response, err := c.waitReady(ctx, cfg)
	if err != nil {
		return fmt.Errorf("service installed but daemon readiness failed: %w; inspect %s and run 'herdr-sesh remote status'", err, c.service.Logs())
	}
	_, _ = fmt.Fprintf(a.Out, "Remote support enabled.\nConfig: %s\nService: %s\nLogs: %s\n", path, c.service.Path(), c.service.Logs())
	printRemoteCatalog(a, cfg, response)
	_, err = fmt.Fprintln(a.Out, "Run 'herdr-sesh attach' from an outer terminal to switch workspaces. Configure each peer separately with reciprocal machine IDs and noninteractive SSH access.")
	return err
}

func (c remoteCommands) waitReady(ctx context.Context, cfg remote.Config) (remote.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	var lastErr error
	for {
		callCtx, cancelCall := context.WithTimeout(ctx, time.Second)
		response, err := c.call(callCtx, cfg, remote.Request{Op: "list"})
		cancelCall()
		if err == nil {
			if err := validateRemoteIdentity(cfg, response); err != nil {
				return remote.Response{}, err
			}
			return response, nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return remote.Response{}, errors.Join(ctx.Err(), lastErr)
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (c remoteCommands) status(ctx context.Context, a *App, path string) error {
	state, serviceErr := c.service.Status(ctx)
	_, _ = fmt.Fprintf(a.Out, "Config: %s\nService: %s\nInstalled: %t\nRunning: %t\nAutostart: %t\n%s\nLogs: %s\n", path, c.service.Path(), state.Installed, state.Running, state.Autostart, state.Detail, c.service.Logs())
	cfg, err := remote.LoadConfig()
	if err != nil {
		return errors.Join(serviceErr, err)
	}
	_, _ = fmt.Fprintf(a.Out, "Remote enabled: %t\n", cfg.Enabled)
	if !cfg.Enabled {
		return serviceErr
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	response, err := c.call(ctx, cfg, remote.Request{Op: "list"})
	if err != nil {
		_, _ = fmt.Fprintf(a.Out, "Daemon unavailable: %v\n", err)
		return serviceErr
	}
	if err := validateRemoteIdentity(cfg, response); err != nil {
		return errors.Join(serviceErr, err)
	}
	printRemoteCatalog(a, cfg, response)
	return serviceErr
}

func printRemoteCatalog(a *App, cfg remote.Config, response remote.Response) {
	for _, machine := range response.Catalog {
		kind := "Peer"
		if machine.ID == cfg.ID {
			kind = "Local Herdr session"
		}
		if machine.Stale {
			reason := machine.LastError
			if reason == "" {
				reason = "waiting for first refresh"
			}
			_, _ = fmt.Fprintf(a.Out, "%s %s (%s): unavailable — %s\n", kind, machine.ID, machine.Session, reason)
		} else {
			_, _ = fmt.Fprintf(a.Out, "%s %s (%s): ready, %d workspace(s)\n", kind, machine.ID, machine.Session, len(machine.Workspaces))
		}
	}
}

func validateRemoteIdentity(cfg remote.Config, response remote.Response) error {
	if len(response.Catalog) == 0 || response.Catalog[0].ID != cfg.ID || response.Catalog[0].Session != cfg.HerdrSession {
		return errors.New("daemon identity does not match the selected configuration")
	}
	return nil
}

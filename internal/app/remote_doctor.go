package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/fullerzz/herdr-plugin-sesh/internal/remote"
)

// doctor only reads configuration, service state, and live connectivity.
func (c remoteCommands) doctor(ctx context.Context, a *App) error {
	var failures []error
	report := func(status, check, detail string) {
		_, err := fmt.Fprintf(a.Out, "%s %s: %s\n", status, doctorText(check), doctorText(detail))
		if err != nil {
			failures = append(failures, err)
		}
		if status == "FAIL" {
			failures = append(failures, fmt.Errorf("%s: %s", doctorText(check), doctorText(detail)))
		}
	}
	path, configErr := remote.ConfigPath()
	var cfg remote.Config
	if configErr == nil {
		cfg, configErr = remote.LoadConfig()
		switch {
		case errors.Is(configErr, os.ErrNotExist):
			configErr = fmt.Errorf("explicit configuration does not exist: %w", configErr)
		case configErr != nil:
		case !cfg.Enabled:
			if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
				report("SKIP", "config", path+"; missing configuration, remote support is opted out; run 'herdr-sesh remote enable' to set up")
			} else {
				report("SKIP", "config", path+"; remote support is disabled")
			}
		default:
			report("OK", "config", path+"; remote support enabled")
		}
	}
	if configErr != nil {
		report("FAIL", "config", fmt.Sprintf("%s: %v; repair the selected remote.toml syntax and values, then rerun doctor", path, configErr))
	}
	logs := "service logs"
	serviceErr := c.serviceErr
	if serviceErr == nil && c.service != nil {
		logs = c.service.Logs()
		serviceCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		state, err := c.service.Status(serviceCtx)
		cancel()
		serviceErr = err
		if err == nil {
			switch {
			case !cfg.Enabled && configErr == nil && state.Running:
				report("FAIL", "service", "running while remote support is opted out; run 'herdr-sesh remote disable' and stop any manually started daemon; logs: "+logs)
			case cfg.Enabled && (!state.Installed || !state.Running):
				report("FAIL", "service", fmt.Sprintf("installed=%t running=%t; run 'herdr-sesh remote enable' to install/start the background service or inspect a manually started daemon; logs: %s", state.Installed, state.Running, logs))
			default:
				report("OK", "service", fmt.Sprintf("installed=%t running=%t; logs: %s", state.Installed, state.Running, logs))
			}
		}
	} else if serviceErr == nil {
		serviceErr = errors.New("native service manager is unavailable")
	}
	if serviceErr != nil {
		report("FAIL", "service", fmt.Sprintf("%v; inspect the native login service manager or run 'herdr-sesh daemon serve' in the foreground; logs: %s", serviceErr, logs))
	}
	if !cfg.Enabled || configErr != nil {
		report("SKIP", "daemon/catalog/SSH", "remote support is disabled, missing, or configuration is invalid; no network checks performed")
		return errors.Join(append(failures, ctx.Err())...)
	}
	daemonCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	response, err := c.call(daemonCtx, cfg, remote.Request{Op: "list"})
	cancel()
	switch {
	case err != nil:
		report("FAIL", "daemon", fmt.Sprintf("%v; run 'herdr-sesh remote enable' or verify the foreground daemon uses this configuration; logs: %s", err, logs))
		report("SKIP", "catalog", "daemon is unavailable")
	case len(response.Catalog) == 0 || response.Catalog[0].ID != cfg.ID || response.Catalog[0].Session != cfg.HerdrSession:
		report("FAIL", "daemon", "identity/session does not match the selected configuration; stop the daemon using the shared socket and restart it with this configuration; logs: "+logs)
		report("SKIP", "catalog", "daemon identity/session does not match")
	default:
		report("OK", "daemon", fmt.Sprintf("identity %s (%s) matches configuration", cfg.ID, cfg.HerdrSession))
		doctorCatalog(cfg, response, report)
	}
	for _, peer := range cfg.Peers {
		if ctx.Err() != nil {
			report("SKIP", "SSH "+peer.ID, "check canceled")
			continue
		}
		sshCtx, cancelSSH := context.WithTimeout(ctx, 10*time.Second)
		err := c.checkSSH(sshCtx, peer)
		cancelSSH()
		if err != nil {
			report("FAIL", "SSH "+peer.ID, fmt.Sprintf("%s: %v; check network reachability, SSH alias and keys/agent; verify and approve host keys in a normal SSH session (doctor never accepts new keys)", peer.SSH, err))
		} else {
			report("OK", "SSH "+peer.ID, peer.SSH+" accepts noninteractive SSH")
		}
	}
	return errors.Join(append(failures, ctx.Err())...)
}

func doctorCatalog(cfg remote.Config, response remote.Response, report func(string, string, string)) {
	check := func(name, id, session, remedy string) {
		for _, machine := range response.Catalog {
			if machine.ID != id {
				continue
			}
			if machine.Session != session {
				report("FAIL", name, "catalog session does not match configuration; "+remedy)
			} else if machine.Stale {
				reason := machine.LastError
				if reason == "" {
					reason = "waiting for first refresh or catalog expired"
				}
				report("FAIL", name, reason+"; "+remedy)
			} else {
				report("OK", name, fmt.Sprintf("%s (%s), %d workspace(s)", id, session, len(machine.Workspaces)))
			}
			return
		}
		report("FAIL", name, "missing from daemon catalog; "+remedy)
	}
	check("local session", cfg.ID, cfg.HerdrSession, "start the configured Herdr session and inspect daemon logs")
	for _, peer := range cfg.Peers {
		check("peer "+peer.ID+" catalog", peer.ID, peer.Session, "verify the peer daemon/Herdr session, reciprocal machine IDs, and peer logs")
	}
}

func doctorText(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp) {
			return ' '
		}
		return r
	}, value)
	if len(value) > 2048 {
		return strings.ToValidUTF8(value[:2048], "") + "..."
	}
	return value
}

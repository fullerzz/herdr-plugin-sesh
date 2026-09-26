package sources

import (
	"context"
	"path/filepath"
	"slices"

	"github.com/fullerzz/herdr-plugin-sesh/internal/config"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
)

type ConfigSessions struct {
	Config config.Config
	Home   string
}

func (ConfigSessions) Name() string { return "config" }
func (s ConfigSessions) List(context.Context) (model.Sessions, error) {
	out := model.NewSessions()
	win := map[string]model.WindowConfig{}
	for _, w := range s.Config.WindowConfigs {
		win[w.Name] = w
	}
	for _, c := range s.Config.SessionConfigs {
		disableStartup := c.DisableStartCommand != nil && *c.DisableStartCommand
		sess := model.Session{Source: "config", Name: c.Name, Path: config.ExpandHome(c.Path, s.Home), StartupCommand: c.StartupCommand, PreviewCommand: c.PreviewCommand, DisableStartupCommand: disableStartup, DisableStartupSet: c.DisableStartCommand != nil, WindowNames: c.Windows}
		for _, n := range c.Windows {
			if w, ok := win[n]; ok {
				sess.WindowConfigs = append(sess.WindowConfigs, resolveWindowPaths(w, sess.Path, s.Home))
			}
		}
		out.Add(sess)
	}
	return out, nil
}

func ApplyConfig(sessions *model.Sessions, cfg config.Config, home string) {
	windows := make(map[string]model.WindowConfig, len(cfg.WindowConfigs))
	for _, window := range cfg.WindowConfigs {
		windows[window.Name] = window
	}
	for key, session := range sessions.Directory {
		wildcard, matched := config.FindWildcard(cfg, session.Path, home)
		if matched && !session.DisableStartupSet {
			session.DisableStartupCommand = wildcard.DisableStartCommand
		}
		if session.StartupCommand == "" && !session.DisableStartupCommand {
			if matched {
				session.StartupCommand = wildcard.StartupCommand
			}
			if session.StartupCommand == "" && !session.DisableStartupCommand {
				session.StartupCommand = cfg.DefaultSessionConfig.StartupCommand
			}
		}
		if session.PreviewCommand == "" && matched {
			session.PreviewCommand = wildcard.PreviewCommand
		}
		if session.Source != "config" && len(session.WindowNames) == 0 && matched {
			session.WindowNames = wildcard.Windows
		}
		if len(session.WindowNames) > 0 {
			session.WindowConfigs = session.WindowConfigs[:0]
			for _, name := range session.WindowNames {
				if window, ok := windows[name]; ok {
					session.WindowConfigs = append(session.WindowConfigs, resolveWindowPaths(window, session.Path, home))
				}
			}
		}
		sessions.Directory[key] = session
	}
}

// Resolve a copy for each session: named tabs can be shared by several workspaces.
func resolveWindowPaths(window model.WindowConfig, workspacePath, home string) model.WindowConfig {
	window.Path = resolveConfigPath(workspacePath, window.Path, home)
	window.Panes = slices.Clone(window.Panes)
	for i := range window.Panes {
		window.Panes[i].Path = resolveConfigPath(window.Path, window.Panes[i].Path, home)
	}
	return window
}

func resolveConfigPath(parent, path, home string) string {
	path = config.ExpandHome(path, home)
	if path == "" {
		return parent
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(parent, path)
}

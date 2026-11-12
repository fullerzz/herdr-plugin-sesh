package sources

import (
	"context"
	"path/filepath"
	"slices"
	"strings"

	"github.com/fullerzz/herdr-plugin-sesh/internal/config"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
)

type ConfigSessions struct {
	Config config.Config
	Home   string
	// LocalOnly omits [[workspace]] entries on another machine, for backends
	// that cannot open a space there.
	LocalOnly bool
}

func (ConfigSessions) Name() string { return "config" }
func (s ConfigSessions) List(context.Context) (model.Sessions, error) {
	out := model.NewSessions()
	win := map[string]model.WindowConfig{}
	for _, w := range s.Config.WindowConfigs {
		win[w.Name] = w
	}
	for _, c := range s.Config.SessionConfigs {
		if s.LocalOnly && c.Machine != "" {
			continue
		}
		disableStartup := c.DisableStartCommand != nil && *c.DisableStartCommand
		// A remote workspace's paths are on its machine, so a leading ~ waits
		// for that machine's home when the space is created.
		expand := func(p string) string { return config.ExpandHome(p, s.Home) }
		if c.Machine != "" {
			expand = func(p string) string { return p }
		}
		sess := model.Session{Source: "config", Machine: c.Machine, Name: c.Name, Path: expand(c.Path), StartupCommand: c.StartupCommand, PreviewCommand: c.PreviewCommand, DisableStartupCommand: disableStartup, DisableStartupSet: c.DisableStartCommand != nil, WindowNames: c.Windows}
		for _, n := range c.Windows {
			if w, ok := win[n]; ok {
				sess.WindowConfigs = append(sess.WindowConfigs, resolveWindowPaths(w, sess.Path, expand))
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
		if session.Machine != "" {
			continue
		}
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
					session.WindowConfigs = append(session.WindowConfigs, resolveWindowPaths(window, session.Path, func(p string) string { return config.ExpandHome(p, home) }))
				}
			}
		}
		sessions.Directory[key] = session
	}
}

// Resolve a copy for each session: named tabs can be shared by several workspaces.
func resolveWindowPaths(window model.WindowConfig, workspacePath string, expandHome func(string) string) model.WindowConfig {
	window.Path = resolveConfigPath(workspacePath, window.Path, expandHome)
	window.Panes = slices.Clone(window.Panes)
	for i := range window.Panes {
		window.Panes[i].Path = resolveConfigPath(window.Path, window.Panes[i].Path, expandHome)
	}
	return window
}

func resolveConfigPath(parent, path string, expandHome func(string) string) string {
	path = expandHome(path)
	if path == "" {
		return parent
	}
	// An unexpanded ~ is rooted at a remote home, so it is not joined to parent.
	if filepath.IsAbs(path) || config.HasHomePrefix(path) {
		return path
	}
	// Keep a remote parent's ~ anchor until the machine's home is known:
	// cleaning "~/.." whole would turn it into ".".
	if config.HasHomePrefix(parent) {
		rel := filepath.Join(strings.TrimPrefix(parent[1:], "/"), path)
		if rel == "." {
			return "~"
		}
		return "~/" + rel
	}
	return filepath.Join(parent, path)
}

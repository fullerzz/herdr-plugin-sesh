package sources

import (
	"context"

	"forgejo.local/fullerzz/herdr-plugin-sesh/internal/config"
	"forgejo.local/fullerzz/herdr-plugin-sesh/internal/model"
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
		w.Path = config.ExpandHome(w.Path, s.Home)
		win[w.Name] = w
	}
	for _, c := range s.Config.SessionConfigs {
		sess := model.Session{Source: "config", Name: c.Name, Path: config.ExpandHome(c.Path, s.Home), StartupCommand: c.StartupCommand, PreviewCommand: c.PreviewCommand, DisableStartupCommand: c.DisableStartCommand, WindowNames: c.Windows}
		for _, n := range c.Windows {
			if w, ok := win[n]; ok {
				sess.WindowConfigs = append(sess.WindowConfigs, w)
			}
		}
		out.Add(sess)
	}
	return out, nil
}

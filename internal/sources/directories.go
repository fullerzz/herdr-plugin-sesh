package sources

import (
	"context"
	"forgejo.local/fullerzz/herdr-plugin-sesh/internal/model"
	"os"
	"path/filepath"
)

type DirectPath struct {
	Path  string
	Label string
}

func (DirectPath) Name() string { return "dir" }
func (d DirectPath) List(context.Context) (model.Sessions, error) {
	out := model.NewSessions()
	if d.Path == "" {
		return out, nil
	}
	if st, err := os.Stat(d.Path); err == nil && st.IsDir() {
		name := d.Label
		if name == "" {
			name = filepath.Base(d.Path)
		}
		out.Add(model.Session{Source: "dir", Name: name, Path: d.Path})
	}
	return out, nil
}

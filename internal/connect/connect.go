package connect

import (
	"context"
	"errors"
	"os"

	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/fullerzz/herdr-plugin-sesh/internal/sources"
	"github.com/fullerzz/herdr-plugin-sesh/internal/startup"
)

type Options struct {
	NoFocus bool
	Namer   func(context.Context, string) string
}

type Result struct {
	Session model.Session
	Created bool
}

func Connect(ctx context.Context, client herdr.Client, candidates []model.Session, target string, opts Options) (Result, error) {
	if client == nil {
		return Result{}, errors.New("herdr client required")
	}
	match, ok := Resolve(candidates, target)
	if !ok {
		if st, err := os.Stat(target); err == nil && st.IsDir() {
			name := target
			if opts.Namer != nil {
				name = opts.Namer(ctx, target)
			}
			match = model.Session{Source: "dir", Name: name, Path: target}
			ok = true
		}
	}
	if !ok {
		return Result{}, errors.New("no matching session")
	}
	if match.WorkspaceID != "" {
		if !opts.NoFocus {
			if err := client.WorkspaceFocus(ctx, match.WorkspaceID); err != nil {
				return Result{}, err
			}
		}
		return Result{Session: match}, nil
	}
	w, err := client.WorkspaceCreate(ctx, herdr.WorkspaceCreateRequest{CWD: match.Path, Label: match.Name, Focus: !opts.NoFocus})
	if err != nil {
		return Result{}, err
	}
	match.WorkspaceID = w.ID
	sources.AddPath(ctx, match.Path)
	if err := startup.Apply(ctx, client, startup.Plan{WorkspaceID: w.ID, Session: match}); err != nil {
		return Result{}, err
	}
	return Result{Session: match, Created: true}, nil
}

func Resolve(candidates []model.Session, target string) (model.Session, bool) {
	for _, s := range candidates {
		if s.WorkspaceID == target || s.Name == target || s.Path == target {
			return s, true
		}
	}
	return model.Session{}, false
}

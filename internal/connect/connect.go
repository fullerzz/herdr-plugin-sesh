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
	Warnf   func(string, ...any)
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
	if !ok && remoteNamed(candidates, target) && len(candidates) == 1 {
		// The picker passes only the selected item; never turn a remote
		// space's name or folder into a local directory.
		return Result{}, errors.New("remote session requires an existing workspace ID")
	}
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
	if match.Machine != "" {
		return Result{}, errors.New("remote session requires an existing workspace ID")
	}
	initialTab := startup.PlanInitialTab(match)
	req := herdr.WorkspaceCreateRequest{CWD: match.Path, Label: match.Name, Focus: !opts.NoFocus, Env: initialTab.Env}
	w, err := client.WorkspaceCreate(ctx, req)
	if err != nil {
		return Result{}, err
	}
	match.WorkspaceID = w.ID
	sources.AddPath(ctx, match.Path)
	if err := startup.Apply(ctx, client, startup.Plan{WorkspaceID: w.ID, Session: match, Focus: !opts.NoFocus, InitialTab: initialTab, Warnf: opts.Warnf}); err != nil {
		return Result{}, err
	}
	return Result{Session: match, Created: true}, nil
}

func Resolve(candidates []model.Session, target string) (model.Session, bool) {
	for _, s := range candidates {
		if s.WorkspaceID != "" && s.WorkspaceID == target {
			return s, true
		}
	}
	// Remote spaces resolve only by qualified ID, so a local name or path
	// never focuses a space on another machine.
	for _, s := range candidates {
		if s.Machine == "" && (s.Name == target || s.Path == target) {
			return s, true
		}
	}
	return model.Session{}, false
}

func remoteNamed(candidates []model.Session, target string) bool {
	for _, s := range candidates {
		if s.Machine != "" && s.WorkspaceID == "" && (s.Name == target || s.Path == target) {
			return true
		}
	}
	return false
}

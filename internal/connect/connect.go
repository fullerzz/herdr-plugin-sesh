package connect

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/fullerzz/herdr-plugin-sesh/internal/config"
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
	match, dir, ok := ResolveTarget(candidates, target)
	if dir {
		match.Source, match.Name = "dir", target
		if opts.Namer != nil {
			match.Name = opts.Namer(ctx, target)
		}
	}
	if !ok {
		for _, s := range candidates {
			if s.Machine != "" && (s.Name == target || s.Path == target) {
				if s.Source == "config" || s.Configured {
					return Result{}, fmt.Errorf("%q is the path of workspace %q on machine %q; connect with its name", target, s.Name, s.Machine)
				}
				if s.WorkspaceID == "" {
					return Result{}, errors.New("remote session requires an existing workspace ID")
				}
				return Result{}, fmt.Errorf("%q is a remote space; connect with its workspace ID %q", target, s.WorkspaceID)
			}
		}
		return Result{}, errors.New("no matching session")
	}
	return Open(ctx, client, match, opts)
}

// Open focuses match's workspace, or creates it when it has none. Callers
// that already hold the session, such as the picker, use it directly: no
// target has to be resolved, so nothing is looked up on the local machine.
func Open(ctx context.Context, client herdr.Client, match model.Session, opts Options) (Result, error) {
	if client == nil {
		return Result{}, errors.New("herdr client required")
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
		var err error
		if match, err = resolveRemoteHome(ctx, client, match); err != nil {
			return Result{}, err
		}
	}
	initialTab := startup.PlanInitialTab(match)
	req := herdr.WorkspaceCreateRequest{Machine: match.Machine, CWD: match.Path, Label: match.Name, Focus: !opts.NoFocus, Env: initialTab.Env}
	w, err := client.WorkspaceCreate(ctx, req)
	if err != nil {
		return Result{}, err
	}
	match.WorkspaceID = w.ID
	if match.Machine == "" {
		sources.AddPath(ctx, match.Path)
	}
	if err := startup.Apply(ctx, client, startup.Plan{WorkspaceID: w.ID, Session: match, Focus: !opts.NoFocus, InitialTab: initialTab, Warnf: opts.Warnf}); err != nil {
		return Result{}, err
	}
	return Result{Session: match, Created: true}, nil
}

// ResolveTarget finds the session a connect or preview target names, in this
// order: an exact workspace ID on any machine; any local session by name or
// path; a local directory at the target; the [[workspace]] entry on another
// machine with that name. For a bare local directory dir is true and only
// Path is set: the caller names it.
func ResolveTarget(candidates []model.Session, target string) (s model.Session, dir, ok bool) {
	for _, c := range candidates {
		if c.WorkspaceID != "" && c.WorkspaceID == target {
			return c, false, true
		}
	}
	// Remote spaces resolve only by qualified ID here, so a local name or
	// path never focuses a space on another machine.
	for _, c := range candidates {
		if c.Machine == "" && (c.Name == target || c.Path == target) {
			return c, false, true
		}
	}
	if st, err := os.Stat(target); err == nil && st.IsDir() {
		return model.Session{Path: target}, true, true
	}
	// A configured remote workspace's path is a path on that machine, so a
	// target never matches it: a local directory with the same path would
	// otherwise open remotely.
	for _, c := range candidates {
		if c.Machine != "" && (c.Source == "config" || c.Configured) && c.Name == target {
			return c, false, true
		}
	}
	return model.Session{}, false, false
}

// resolveRemoteHome makes a remote session's paths absolute against that
// machine's home: a leading ~, which herdr-app does not expand, and relative
// paths, which herdr-app already starts from the home. A split's shell has to
// enter its path from wherever the pane it splits is. It asks the machine only
// when a path needs it, and fails before anything is created. A ~user path is
// refused: only the connecting user's home is known.
func resolveRemoteHome(ctx context.Context, client herdr.Client, s model.Session) (model.Session, error) {
	paths := []string{s.Path}
	for _, w := range s.WindowConfigs {
		paths = append(paths, w.Path)
		for _, p := range w.Panes {
			paths = append(paths, p.Path)
		}
	}
	for _, p := range paths {
		if strings.HasPrefix(p, "~") && !config.HasHomePrefix(p) {
			return s, fmt.Errorf("workspace %q on machine %q: path %q names another user's home; use an absolute path", s.Name, s.Machine, p)
		}
	}
	mc, ok := client.(herdr.MachineClient)
	if !ok {
		return s, fmt.Errorf("workspace %q on machine %q: %w", s.Name, s.Machine, herdr.ErrNoMachines)
	}
	homeRelative := func(p string) bool { return p != "" && !filepath.IsAbs(p) }
	if !slices.ContainsFunc(paths, homeRelative) {
		return s, nil
	}
	home, err := mc.MachineHome(ctx, s.Machine)
	if err != nil {
		return s, err
	}
	expand := func(p string) string {
		if !homeRelative(p) || config.HasHomePrefix(p) {
			return config.ExpandHome(p, home)
		}
		return filepath.Join(home, p)
	}
	s.Path = expand(s.Path)
	// Copy before expanding: candidates share these slices.
	s.WindowConfigs = slices.Clone(s.WindowConfigs)
	for i := range s.WindowConfigs {
		w := &s.WindowConfigs[i]
		w.Path = expand(w.Path)
		w.Panes = slices.Clone(w.Panes)
		for j := range w.Panes {
			w.Panes[j].Path = expand(w.Panes[j].Path)
		}
	}
	return s, nil
}

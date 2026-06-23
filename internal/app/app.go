package app

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	clonepkg "forgejo.local/fullerzz/herdr-plugin-sesh/internal/clone"
	"forgejo.local/fullerzz/herdr-plugin-sesh/internal/config"
	connectpkg "forgejo.local/fullerzz/herdr-plugin-sesh/internal/connect"
	"forgejo.local/fullerzz/herdr-plugin-sesh/internal/herdr"
	"forgejo.local/fullerzz/herdr-plugin-sesh/internal/model"
	"forgejo.local/fullerzz/herdr-plugin-sesh/internal/namer"
	"forgejo.local/fullerzz/herdr-plugin-sesh/internal/preview"
	"forgejo.local/fullerzz/herdr-plugin-sesh/internal/sources"
	"forgejo.local/fullerzz/herdr-plugin-sesh/internal/state"
)

const Version = "0.1.0-dev"

type App struct {
	Out io.Writer
	Err io.Writer
}

func New() *App { return &App{Out: os.Stdout, Err: os.Stderr} }
func (a *App) Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		a.usage()
		return nil
	}
	switch args[0] {
	case "--version", "version":
		fmt.Fprintf(a.Out, "herdr-sesh %s\n", Version)
		return nil
	case "list":
		return a.list(ctx, args[1:])
	case "connect":
		return a.connect(ctx, args[1:])
	case "preview":
		return a.preview(ctx, args[1:])
	case "clone":
		return a.clone(ctx, args[1:])
	case "root":
		return a.root(ctx, args[1:])
	case "last":
		return a.last(ctx, args[1:])
	case "window":
		return a.window(ctx, args[1:])
	case "plugin":
		return a.plugin(ctx, args[1:])
	case "config":
		return a.config(ctx, args[1:])
	case "picker":
		return a.list(ctx, append([]string{"--json"}, args[1:]...))
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}
func (a *App) usage() {
	fmt.Fprintln(a.Out, "herdr-sesh list|connect|preview|clone|root|last|window|picker|plugin|config|--version")
}

func (a *App) loadConfig(path string) (config.Config, error) {
	cfg, _, err := config.Load(config.LoadOptions{Path: path})
	return cfg, err
}
func (a *App) collect(ctx context.Context, cfg config.Config, target string) ([]model.Session, error) {
	hs := sources.HerdrWorkspaces{Client: herdr.NewCLIClient()}
	srcs := []sources.Source{ignoreSource{hs}, sources.ConfigSessions{Config: cfg}, sources.Zoxide{}}
	if target != "" {
		srcs = append(srcs, sources.DirectPath{Path: target})
	}
	merged, err := sources.Merge(ctx, srcs, cfg.SortOrder, cfg.Blacklist, false, true)
	if err != nil {
		return nil, err
	}
	return merged.Ordered(), nil
}

type ignoreSource struct{ sources.Source }

func (i ignoreSource) List(ctx context.Context) (model.Sessions, error) {
	ss, err := i.Source.List(ctx)
	if err != nil {
		return model.NewSessions(), nil
	}
	return ss, nil
}

func (a *App) list(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	jsonOut := fs.Bool("json", false, "")
	cfgPath := fs.String("config", "", "")
	blacklisted := fs.Bool("blacklisted", false, "")
	hideDup := fs.Bool("hide-duplicates", true, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := a.loadConfig(*cfgPath)
	if err != nil {
		return err
	}
	if cfg.Cache {
		if cached, ok, err := state.LoadSessionCache(os.Getenv("HERDR_PLUGIN_STATE_DIR"), 5*time.Second, time.Now()); err != nil {
			return err
		} else if ok {
			return a.printSessions(cached, *jsonOut)
		}
	}
	ss, err := sources.Merge(ctx, []sources.Source{ignoreSource{sources.HerdrWorkspaces{Client: herdr.NewCLIClient()}}, sources.ConfigSessions{Config: cfg}, sources.Zoxide{}}, cfg.SortOrder, cfg.Blacklist, *blacklisted, *hideDup)
	if err != nil {
		return err
	}
	sessions := ss.Ordered()
	if cfg.Cache {
		_ = state.SaveSessionCache(os.Getenv("HERDR_PLUGIN_STATE_DIR"), sessions, time.Now())
	}
	return a.printSessions(sessions, *jsonOut)
}

func (a *App) printSessions(sessions []model.Session, jsonOut bool) error {
	if jsonOut {
		enc := json.NewEncoder(a.Out)
		enc.SetIndent("", "  ")
		return enc.Encode(sessions)
	}
	for _, s := range sessions {
		if s.Path != "" {
			fmt.Fprintf(a.Out, "%s	%s	%s\n", s.Source, s.Name, s.Path)
		} else {
			fmt.Fprintf(a.Out, "%s	%s\n", s.Source, s.Name)
		}
	}
	return nil
}

func (a *App) connect(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("connect", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	noFocus := fs.Bool("no-focus", false, "")
	cfgPath := fs.String("config", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return errors.New("connect requires target")
	}
	target := fs.Arg(0)
	cfg, err := a.loadConfig(*cfgPath)
	if err != nil {
		return err
	}
	sessions, err := a.collect(ctx, cfg, target)
	if err != nil {
		return err
	}
	res, err := connectpkg.Connect(ctx, herdr.NewCLIClient(), sessions, target, connectpkg.Options{NoFocus: *noFocus, Namer: func(ctx context.Context, p string) string { return namer.Namer{}.Name(ctx, p, cfg.DirLength) }})
	if err != nil {
		return err
	}
	_ = state.Record(os.Getenv("HERDR_PLUGIN_STATE_DIR"), res.Session.WorkspaceID)
	fmt.Fprintf(a.Out, "%s\n", res.Session.Name)
	return nil
}

func (a *App) preview(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return errors.New("preview requires target")
	}
	target := args[0]
	cfg, err := a.loadConfig("")
	if err != nil {
		return err
	}
	sessions, err := a.collect(ctx, cfg, target)
	if err != nil {
		return err
	}
	s, ok := connectpkg.Resolve(sessions, target)
	if !ok {
		s = model.Session{Name: filepath.Base(target), Path: target}
	}
	out, err := preview.Render(ctx, s, cfg.DefaultSessionConfig.PreviewCommand)
	if err != nil {
		return err
	}
	fmt.Fprint(a.Out, out)
	return nil
}
func (a *App) clone(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("clone", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	cmdDir := fs.String("cmdDir", "", "")
	dir := fs.String("dir", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return errors.New("clone requires repo")
	}
	dest, err := clonepkg.Clone(ctx, clonepkg.Request{Repo: fs.Arg(0), CmdDir: *cmdDir, Dir: *dir})
	if err != nil {
		return err
	}
	return a.connect(ctx, []string{dest})
}
func (a *App) root(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("root", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	doConnect := fs.Bool("connect", false, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	root, err := gitRoot(ctx, ".")
	if err != nil {
		return err
	}
	if *doConnect {
		return a.connect(ctx, []string{root})
	}
	fmt.Fprintln(a.Out, root)
	return nil
}
func gitRoot(ctx context.Context, dir string) (string, error) {
	b, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}
func (a *App) last(ctx context.Context, args []string) error {
	id, ok, err := state.Last(os.Getenv("HERDR_PLUGIN_STATE_DIR"))
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("no previous workspace recorded")
	}
	return herdr.NewCLIClient().WorkspaceFocus(ctx, id)
}
func (a *App) window(ctx context.Context, args []string) error {
	c := herdr.NewCLIClient()
	if len(args) == 0 {
		tabs, err := c.TabList(ctx, os.Getenv("HERDR_WORKSPACE_ID"))
		if err != nil {
			return err
		}
		for _, t := range tabs {
			fmt.Fprintf(a.Out, "%s\t%s\n", t.ID, t.Label)
		}
		return nil
	}
	_, err := c.TabCreate(ctx, herdr.TabCreateRequest{WorkspaceID: os.Getenv("HERDR_WORKSPACE_ID"), CWD: args[0], Label: filepath.Base(args[0]), Focus: true})
	return err
}
func (a *App) plugin(ctx context.Context, args []string) error {
	if len(args) >= 1 && args[0] == "open-picker" {
		return herdr.NewCLIClient().PluginPaneOpen(ctx, "fullerzz.sesh", "picker", "overlay")
	}
	return errors.New("unknown plugin command")
}
func (a *App) config(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("config requires path or init")
	}
	dir := os.Getenv("HERDR_PLUGIN_CONFIG_DIR")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config", "herdr-sesh")
	}
	switch args[0] {
	case "path":
		fmt.Fprintln(a.Out, filepath.Join(dir, "sesh.toml"))
		return nil
	case "init":
		p, err := config.InitConfig(dir)
		if err == nil {
			fmt.Fprintln(a.Out, p)
		}
		return err
	default:
		return errors.New("unknown config command")
	}
}

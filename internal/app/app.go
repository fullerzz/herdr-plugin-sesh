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

	clonepkg "github.com/fullerzz/herdr-plugin-sesh/internal/clone"
	"github.com/fullerzz/herdr-plugin-sesh/internal/config"
	connectpkg "github.com/fullerzz/herdr-plugin-sesh/internal/connect"
	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/fullerzz/herdr-plugin-sesh/internal/namer"
	pickerpkg "github.com/fullerzz/herdr-plugin-sesh/internal/picker"
	"github.com/fullerzz/herdr-plugin-sesh/internal/preview"
	"github.com/fullerzz/herdr-plugin-sesh/internal/sources"
	"github.com/fullerzz/herdr-plugin-sesh/internal/state"
)

var Version = "dev"

type App struct {
	Out io.Writer
	Err io.Writer
}

func New() *App { return &App{Out: os.Stdout, Err: os.Stderr} }
func (a *App) Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return a.usage()
	}
	switch args[0] {
	case "--version", "version":
		_, err := fmt.Fprintf(a.Out, "herdr-sesh %s\n", Version)
		return err
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
		return a.picker(ctx, args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}
func (a *App) usage() error {
	_, err := fmt.Fprintln(a.Out, "herdr-sesh list|connect|preview|clone|root|last|window|picker|plugin|config|--version")
	return err
}

func (a *App) warnf(format string, args ...any) {
	if a.Err == nil {
		return
	}
	_, _ = fmt.Fprintf(a.Err, "warning: "+format+"\n", args...)
}

func (a *App) loadConfig(path string) (config.Config, error) {
	cfg, _, err := config.Load(config.LoadOptions{Path: path})
	return cfg, err
}
func (a *App) collect(ctx context.Context, cfg config.Config, target string) ([]model.Session, error) {
	hs := sources.HerdrWorkspaces{Client: herdr.NewCLIClient()}
	return a.collectFrom(ctx, cfg, target, hs)
}

func (a *App) collectAllowUnavailableHerdr(ctx context.Context, cfg config.Config, target string) ([]model.Session, error) {
	hs := sources.HerdrWorkspaces{Client: herdr.NewCLIClient()}
	return a.collectFrom(ctx, cfg, target, ignoreSource{hs})
}

type pickerCollection struct {
	Sessions        []model.Session
	HerdrWorkspaces []model.Session
	// HerdrErr reports a failed Herdr workspace listing that the merge
	// tolerated; callers must surface it or the picker looks empty for no
	// visible reason.
	HerdrErr error
}

func (a *App) collectPicker(ctx context.Context, cfg config.Config) (pickerCollection, error) {
	herdrSource := &capturingSource{Source: sources.HerdrWorkspaces{Client: herdr.NewCLIClient()}}
	sessions, err := a.collectFrom(ctx, cfg, "", herdrSource)
	col := pickerCollection{Sessions: sessions, HerdrErr: herdrSource.err}
	if herdrSource.err == nil {
		col.HerdrWorkspaces = herdrSource.sessions.Ordered()
	}
	return col, err
}

func (a *App) collectFrom(ctx context.Context, cfg config.Config, target string, herdrSource sources.Source) ([]model.Session, error) {
	srcs := []sources.Source{herdrSource, sources.ConfigSessions{Config: cfg}, sources.Zoxide{}}
	if target != "" {
		srcs = append(srcs, sources.DirectPath{
			Path:  target,
			Label: namer.Namer{}.Name(ctx, target, cfg.DirLength),
		})
	}
	merged, err := sources.Merge(ctx, srcs, cfg.SortOrder, cfg.Blacklist, false, true)
	if err != nil {
		return nil, err
	}
	sources.ApplyConfig(&merged, cfg, "")
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

type capturingSource struct {
	sources.Source

	sessions model.Sessions
	err      error
}

func (s *capturingSource) List(ctx context.Context) (model.Sessions, error) {
	s.sessions, s.err = s.Source.List(ctx)
	if s.err != nil {
		return model.NewSessions(), nil
	}
	return s.sessions, nil
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
	cfg, resolvedConfigPath, err := config.Load(config.LoadOptions{Path: *cfgPath})
	if err != nil {
		return err
	}
	cacheable := cfg.Cache && !*blacklisted && *hideDup
	if cacheable {
		if cached, ok, err := state.LoadSessionCache(os.Getenv("HERDR_PLUGIN_STATE_DIR"), resolvedConfigPath, 5*time.Second, time.Now()); err != nil {
			a.warnf("ignoring session cache: %v", err)
		} else if ok {
			return a.printSessions(cached, *jsonOut)
		}
	}
	ss, err := sources.Merge(ctx, []sources.Source{ignoreSource{sources.HerdrWorkspaces{Client: herdr.NewCLIClient()}}, sources.ConfigSessions{Config: cfg}, sources.Zoxide{}}, cfg.SortOrder, cfg.Blacklist, *blacklisted, *hideDup)
	if err != nil {
		return err
	}
	sources.ApplyConfig(&ss, cfg, "")
	sessions := ss.Ordered()
	if cacheable {
		if err := state.SaveSessionCache(os.Getenv("HERDR_PLUGIN_STATE_DIR"), resolvedConfigPath, sessions, time.Now()); err != nil {
			a.warnf("could not save session cache: %v", err)
		}
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
		var err error
		if s.Path != "" {
			_, err = fmt.Fprintf(a.Out, "%s	%s	%s\n", s.Source, s.Name, s.Path)
		} else {
			_, err = fmt.Fprintf(a.Out, "%s	%s\n", s.Source, s.Name)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (a *App) picker(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("picker", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	jsonOut := fs.Bool("json", false, "")
	cfgPath := fs.String("config", "", "")
	fzfPicker := fs.Bool("fzf", false, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := a.loadConfig(*cfgPath)
	if err != nil {
		return err
	}
	useFZF := *fzfPicker || strings.EqualFold(os.Getenv("HERDR_SESH_PICKER"), "fzf")
	var sessions, herdrWorkspaces []model.Session
	if useFZF || *jsonOut {
		sessions, err = a.collectAllowUnavailableHerdr(ctx, cfg, "")
	} else {
		var col pickerCollection
		col, err = a.collectPicker(ctx, cfg)
		sessions, herdrWorkspaces = col.Sessions, col.HerdrWorkspaces
		if col.HerdrErr != nil {
			a.warnf("herdr workspaces unavailable: %v", col.HerdrErr)
		}
	}
	if err != nil {
		return err
	}
	if *jsonOut {
		return a.printSessions(sessions, true)
	}
	pickOpts := pickerpkg.Options{
		Context:               ctx,
		Output:                a.Out,
		Prompt:                cfg.TUI.Prompt,
		Placeholder:           cfg.TUI.Placeholder,
		ShowIcons:             cfg.TUI.ShowIcons,
		HideLastWorkspacePath: !cfg.TUI.ShowLastWorkspacePath,
		SeparatorAware:        cfg.SeparatorAware,
		DefaultPreviewCommand: cfg.DefaultSessionConfig.PreviewCommand,
	}
	var selected model.Session
	var ok bool
	currentWorkspaceID := os.Getenv("HERDR_WORKSPACE_ID")
	pickerWorkspaceID := currentWorkspaceID
	if useFZF {
		selected, ok, err = pickerpkg.RunFZF(ctx, sessions, pickOpts)
	} else {
		client := herdr.NewCLIClient()
		history, historyErr := state.LoadHistory(os.Getenv("HERDR_PLUGIN_STATE_DIR"))
		if historyErr != nil {
			a.warnf("ignoring workspace history: %v", historyErr)
		}
		pickOpts.RecentWorkspaceIDs = append([]string{currentWorkspaceID}, history.Workspaces...)
		pickOpts.RecentWorkspaceSort = cfg.TUI.DefaultSort == "recent"
		pickOpts.HerdrWorkspaces = herdrWorkspaces
		lastWorkspaceID, _, lastWorkspaceErr := lastWorkspace(os.Getenv("HERDR_PLUGIN_STATE_DIR"), currentWorkspaceID)
		if lastWorkspaceErr != nil {
			a.warnf("could not determine last workspace: %v", lastWorkspaceErr)
		}
		pickOpts.LastWorkspaceID = lastWorkspaceID
		pickOpts.LastWorkspaceUnknown = lastWorkspaceErr != nil
		// Warnings raised while the alt-screen TUI owns the terminal would be
		// overwritten and lost; buffer them and flush after the picker exits.
		var deferredWarnings []string
		deferWarn := func(format string, args ...any) {
			deferredWarnings = append(deferredWarnings, fmt.Sprintf(format, args...))
		}
		pickOpts.CloseWorkspace = func(closeCtx context.Context, id string) error {
			if err := client.WorkspaceClose(closeCtx, id); err != nil {
				return err
			}
			if err := state.RemoveWorkspace(os.Getenv("HERDR_PLUGIN_STATE_DIR"), id); err != nil {
				deferWarn("could not prune workspace history: %v", err)
			}
			return nil
		}
		pickOpts.ReloadPicker = func(reloadCtx context.Context) (pickerpkg.ReloadResult, error) {
			return a.reloadPickerState(reloadCtx, cfg, client, &pickerWorkspaceID, deferWarn)
		}
		pickOpts.RefreshAgentStatuses = func() (map[string]string, error) {
			workspaces, err := client.WorkspaceList(ctx)
			if err != nil {
				return nil, err
			}
			statuses := make(map[string]string, len(workspaces))
			for _, workspace := range workspaces {
				statuses[workspace.ID] = workspace.AgentStatus
			}
			return statuses, nil
		}
		selected, ok, err = pickerpkg.Run(sessions, pickOpts)
		for _, warning := range deferredWarnings {
			a.warnf("%s", warning)
		}
	}
	if err != nil || !ok {
		return err
	}
	res, err := connectpkg.Connect(ctx, herdr.NewCLIClient(), []model.Session{selected}, pickerTarget(selected), connectpkg.Options{
		Namer: func(ctx context.Context, p string) string { return namer.Namer{}.Name(ctx, p, cfg.DirLength) },
	})
	if err != nil {
		return err
	}
	a.recordWorkspaceSwitch(pickerWorkspaceID, res.Session.WorkspaceID)
	return nil
}

// reloadPickerState re-collects picker sessions after a workspace close and
// re-resolves which workspace hosts the picker, since the close may have
// destroyed the workspace that launched it. On focus failure
// pickerWorkspaceID is cleared so the eventual switch is recorded without a
// stale "from" workspace.
func (a *App) reloadPickerState(ctx context.Context, cfg config.Config, client *herdr.CLIClient, pickerWorkspaceID *string, warnf func(string, ...any)) (pickerpkg.ReloadResult, error) {
	col, reloadErr := a.collectPicker(ctx, cfg)
	if reloadErr == nil {
		reloadErr = col.HerdrErr
	}
	focusedPane, focusErr := client.PaneFocused(ctx)
	*pickerWorkspaceID = focusedPane.WorkspaceID
	if focusErr == nil && *pickerWorkspaceID == "" {
		focusErr = errors.New("focused pane has no workspace ID")
	}
	lastID, _, lastErr := lastWorkspace(os.Getenv("HERDR_PLUGIN_STATE_DIR"), *pickerWorkspaceID)
	if focusErr != nil {
		*pickerWorkspaceID = ""
		lastErr = fmt.Errorf("find focused workspace after close: %w", focusErr)
	}
	if lastErr != nil {
		warnf("could not determine last workspace: %v", lastErr)
	}
	return pickerpkg.ReloadResult{
		Sessions:             col.Sessions,
		HerdrWorkspaces:      col.HerdrWorkspaces,
		LastWorkspaceID:      lastID,
		LastWorkspaceUnknown: lastErr != nil,
	}, reloadErr
}

func pickerTarget(s model.Session) string {
	if s.WorkspaceID != "" {
		return s.WorkspaceID
	}
	if s.Path != "" {
		return s.Path
	}
	return s.Name
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
	sessions, err := a.collectAllowUnavailableHerdr(ctx, cfg, target)
	if err != nil {
		return err
	}
	currentWorkspaceID := os.Getenv("HERDR_WORKSPACE_ID")
	res, err := connectpkg.Connect(ctx, herdr.NewCLIClient(), sessions, target, connectpkg.Options{NoFocus: *noFocus, Namer: func(ctx context.Context, p string) string { return namer.Namer{}.Name(ctx, p, cfg.DirLength) }})
	if err != nil {
		return err
	}
	if !*noFocus {
		a.recordWorkspaceSwitch(currentWorkspaceID, res.Session.WorkspaceID)
	}
	_, err = fmt.Fprintf(a.Out, "%s\n", res.Session.Name)
	return err
}

func (a *App) preview(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("preview", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	cfgPath := fs.String("config", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return errors.New("preview requires target")
	}
	target := fs.Arg(0)
	cfg, err := a.loadConfig(*cfgPath)
	if err != nil {
		return err
	}
	sessions, err := a.collectAllowUnavailableHerdr(ctx, cfg, target)
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
	_, err = fmt.Fprint(a.Out, out)
	return err
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
	_, err = fmt.Fprintln(a.Out, root)
	return err
}
func gitRoot(ctx context.Context, dir string) (string, error) {
	//nolint:gosec // dir is passed as an argv value to a fixed git command.
	b, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}
func (a *App) last(ctx context.Context, _ []string) error {
	stateDir := os.Getenv("HERDR_PLUGIN_STATE_DIR")
	currentWorkspaceID := os.Getenv("HERDR_WORKSPACE_ID")
	id, ok, err := lastWorkspace(stateDir, currentWorkspaceID)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("no previous workspace recorded")
	}
	if err := herdr.NewCLIClient().WorkspaceFocus(ctx, id); err != nil {
		return err
	}
	if err := state.RecordSwitch(stateDir, currentWorkspaceID, id); err != nil {
		a.warnf("could not record workspace history: %v", err)
	}
	return nil
}

func lastWorkspace(stateDir, currentWorkspaceID string) (string, bool, error) {
	if currentWorkspaceID == "" {
		return state.Last(stateDir)
	}
	return state.Previous(stateDir, currentWorkspaceID)
}

func (a *App) recordWorkspaceSwitch(fromWorkspaceID, toWorkspaceID string) {
	if err := state.RecordSwitch(os.Getenv("HERDR_PLUGIN_STATE_DIR"), fromWorkspaceID, toWorkspaceID); err != nil {
		a.warnf("could not record workspace history: %v", err)
	}
}

func (a *App) window(ctx context.Context, args []string) error {
	c := herdr.NewCLIClient()
	if len(args) == 0 {
		tabs, err := c.TabList(ctx, os.Getenv("HERDR_WORKSPACE_ID"))
		if err != nil {
			return err
		}
		for _, t := range tabs {
			if _, err := fmt.Fprintf(a.Out, "%s\t%s\n", t.ID, t.Label); err != nil {
				return err
			}
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
func (a *App) config(_ context.Context, args []string) error {
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
		_, err := fmt.Fprintln(a.Out, filepath.Join(dir, "sesh.toml"))
		return err
	case "init":
		p, err := config.InitConfig(dir)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(a.Out, p)
		return err
	default:
		return errors.New("unknown config command")
	}
}

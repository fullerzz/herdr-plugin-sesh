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

func sameFile(first, second string) bool {
	//nolint:gosec // config paths are user-selected by design.
	firstInfo, firstErr := os.Stat(first)
	if firstErr != nil {
		return false
	}
	//nolint:gosec // config paths are user-selected by design.
	secondInfo, secondErr := os.Stat(second)
	return secondErr == nil && os.SameFile(firstInfo, secondInfo)
}

func (a *App) loadConfig(path string) (config.Config, error) {
	cfg, _, err := config.Load(config.LoadOptions{Path: path, Warn: a.Err})
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
	cfg, resolvedConfigPath, err := config.Load(config.LoadOptions{Path: *cfgPath, Warn: a.Err})
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

func pickerOptionsFromConfig(ctx context.Context, output io.Writer, cfg config.Config) pickerpkg.Options {
	return pickerpkg.Options{
		Context:                        ctx,
		Output:                         output,
		Prompt:                         cfg.TUI.Prompt,
		Placeholder:                    cfg.TUI.Placeholder,
		ShowIcons:                      cfg.TUI.ShowIcons,
		HerdrThemeInherit:              cfg.TUI.HerdrThemeInherit,
		DisableWorktreeIconReplacement: !cfg.TUI.ReplaceWorktreeIcon,
		HideLastWorkspace:              !cfg.TUI.ShowLastWorkspace,
		HideLastWorkspacePath:          !cfg.TUI.ShowLastWorkspacePath,
		SeparatorAware:                 cfg.SeparatorAware,
		DisableHomePrioritization:      !cfg.TUI.PrioritizeHome,
		HidePreview:                    !cfg.TUI.ShowPreview,
		DefaultPreviewCommand:          cfg.DefaultSessionConfig.PreviewCommand,
		WorkspaceSort:                  cfg.TUI.DefaultSort,
	}
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
	pickOpts := pickerOptionsFromConfig(ctx, a.Out, cfg)
	var selected model.Session
	var ok bool
	currentWorkspaceID := os.Getenv("HERDR_WORKSPACE_ID")
	pickerWorkspaceID := currentWorkspaceID
	if useFZF {
		selected, ok, err = pickerpkg.RunFZF(ctx, sessions, pickOpts)
	} else {
		client := herdr.NewCLIClient()
		historyDir, historyDirErr := historyStateDir()
		if historyDirErr != nil {
			return fmt.Errorf("resolve workspace history: %w", historyDirErr)
		}
		history, historyErr := state.LoadHistory(historyDir)
		if historyErr != nil {
			a.warnf("ignoring workspace history: %v", historyErr)
		}
		pickOpts.RecentWorkspaceIDs = append([]string{currentWorkspaceID}, history.Workspaces...)
		if cfg.TUI.ShowLastWorkspace {
			pickOpts.HerdrWorkspaces = herdrWorkspaces
			lastWorkspaceID, _, lastWorkspaceErr := lastWorkspace(historyDir, currentWorkspaceID)
			if lastWorkspaceErr != nil {
				a.warnf("could not determine last workspace: %v", lastWorkspaceErr)
			}
			pickOpts.LastWorkspaceID = lastWorkspaceID
			pickOpts.LastWorkspaceUnknown = lastWorkspaceErr != nil
		}
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
			if err := state.RemoveWorkspace(historyDir, id); err != nil {
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
	var lastID string
	var lastErr error
	if cfg.TUI.ShowLastWorkspace {
		historyDir, err := historyStateDir()
		if err != nil {
			lastErr = err
		} else {
			lastID, _, lastErr = lastWorkspace(historyDir, *pickerWorkspaceID)
		}
	}
	if focusErr != nil {
		*pickerWorkspaceID = ""
		if cfg.TUI.ShowLastWorkspace {
			lastErr = fmt.Errorf("find focused workspace after close: %w", focusErr)
		} else {
			warnf("could not determine picker workspace after close: %v", focusErr)
		}
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
	historyDir, err := historyStateDir()
	if err != nil {
		return err
	}
	currentWorkspaceID := os.Getenv("HERDR_WORKSPACE_ID")
	id, ok, err := lastWorkspace(historyDir, currentWorkspaceID)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("no previous workspace recorded")
	}
	if err := herdr.NewCLIClient().WorkspaceFocus(ctx, id); err != nil {
		return err
	}
	if err := state.RecordSwitch(historyDir, currentWorkspaceID, id); err != nil {
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
	historyDir, err := historyStateDir()
	if err == nil {
		err = state.RecordSwitch(historyDir, fromWorkspaceID, toWorkspaceID)
	}
	if err != nil {
		a.warnf("could not record workspace history: %v", err)
	}
}

func historyStateDir() (string, error) {
	return state.SessionHistoryDir(os.Getenv("HERDR_PLUGIN_STATE_DIR"), os.Getenv("HERDR_SOCKET_PATH"))
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
	if len(args) == 0 {
		return errors.New("unknown plugin command")
	}
	switch args[0] {
	case "open-picker":
		return herdr.NewCLIClient().PluginPaneOpen(ctx, "fullerzz.sesh", "picker", "overlay")
	case "watch-history":
		return a.watchHistory(ctx)
	default:
		return errors.New("unknown plugin command")
	}
}

func (a *App) watchHistory(ctx context.Context) (err error) {
	stateDir := os.Getenv("HERDR_PLUGIN_STATE_DIR")
	socketPath := os.Getenv("HERDR_SOCKET_PATH")
	// Elect first so non-winning startup and focus hooks cannot migrate history.
	release, acquired, err := state.TryHistoryWatcherLock(stateDir, socketPath)
	if err != nil {
		return err
	}
	if acquired {
		defer func() { err = errors.Join(err, release()) }()
	}
	if !acquired && os.Getenv("HERDR_PLUGIN_EVENT") != "workspace.closed" {
		return nil
	}
	historyDir, err := state.SessionHistoryDir(stateDir, socketPath)
	if err != nil {
		return err
	}
	if err := applyHistoryHook(historyDir); err != nil {
		return err
	}
	if !acquired {
		return nil
	}

	return herdr.WatchWorkspaceEvents(ctx, socketPath,
		func(workspaceID string) error {
			return retryHistoryMutation(ctx, func() error { return state.Record(historyDir, workspaceID) })
		},
		func(workspaceID string) error {
			return retryHistoryMutation(ctx, func() error { return state.RemoveWorkspace(historyDir, workspaceID) })
		},
	)
}

func retryHistoryMutation(ctx context.Context, mutate func() error) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := mutate()
		if !errors.Is(err, state.ErrHistoryLockTimeout) {
			return err
		}
	}
}

func applyHistoryHook(historyDir string) error {
	eventName := os.Getenv("HERDR_PLUGIN_EVENT")
	if eventName == "" || eventName == "startup" {
		return nil
	}
	switch eventName {
	case "workspace.focused":
		return nil
	case "workspace.closed":
		var payload struct {
			Data struct {
				WorkspaceID string `json:"workspace_id"`
			} `json:"data"`
		}
		raw := os.Getenv("HERDR_PLUGIN_EVENT_JSON")
		if raw == "" {
			return errors.New("HERDR_PLUGIN_EVENT_JSON is required for workspace.closed")
		}
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			return fmt.Errorf("decode HERDR_PLUGIN_EVENT_JSON: %w", err)
		}
		if payload.Data.WorkspaceID == "" {
			return errors.New("workspace.closed event payload is missing data.workspace_id")
		}
		return state.RemoveWorkspace(historyDir, payload.Data.WorkspaceID)
	default:
		return fmt.Errorf("unknown Herdr plugin event %q", eventName)
	}
}

func (a *App) config(_ context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("config requires path, init, validate, or migrate")
	}
	dir := os.Getenv("HERDR_PLUGIN_CONFIG_DIR")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config", "herdr-sesh")
	}
	switch args[0] {
	case "path":
		p, err := config.ResolvePath(config.LoadOptions{})
		if err != nil {
			return err
		}
		if p == "" {
			p = filepath.Join(dir, config.NativeFileName)
		}
		_, err = fmt.Fprintln(a.Out, p)
		return err
	case "init":
		// Never shadow a config that already loads: an existing candidate
		// anywhere in the lookup order is returned instead of creating a
		// higher-precedence native file next to it.
		p, err := config.ResolvePath(config.LoadOptions{})
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err == nil && p != "" {
			_, err = fmt.Fprintln(a.Out, p)
			return err
		}
		if target := os.Getenv("HERDR_SESH_CONFIG"); target != "" {
			p, err := config.InitConfigAt(config.ExpandHome(target, ""))
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(a.Out, p)
			return err
		}
		p, err = config.InitConfig(dir)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(a.Out, p)
		return err
	case "validate":
		return a.validateConfig(args[1:])
	case "migrate":
		fs := flag.NewFlagSet("config migrate", flag.ContinueOnError)
		fs.SetOutput(a.Err)
		cfgPath := fs.String("config", "", "")
		force := fs.Bool("force", false, "overwrite an existing native config")
		flagArgs := args[1:]
		positionalPath := ""
		if len(flagArgs) > 0 && !strings.HasPrefix(flagArgs[0], "-") {
			positionalPath = flagArgs[0]
			flagArgs = flagArgs[1:]
		}
		if err := fs.Parse(flagArgs); err != nil {
			return err
		}
		if positionalPath == "" && fs.NArg() == 1 {
			positionalPath = fs.Arg(0)
		} else if fs.NArg() != 0 {
			return errors.New("config migrate accepts at most one path")
		}
		if positionalPath != "" {
			if *cfgPath != "" {
				return errors.New("config migrate path provided twice")
			}
			*cfgPath = positionalPath
		}
		legacy, native, err := config.Migrate(config.LoadOptions{Path: *cfgPath}, dir, *force)
		if err != nil {
			return err
		}
		if envPath := config.ExpandHome(os.Getenv("HERDR_SESH_CONFIG"), ""); envPath != "" {
			if sameFile(envPath, legacy) {
				a.warnf("migrated %s; set HERDR_SESH_CONFIG=%s before deleting %s", legacy, native, legacy)
			} else {
				a.warnf("migrated %s; HERDR_SESH_CONFIG continues to select %s", legacy, envPath)
			}
		} else {
			resolved, resolveErr := config.ResolvePath(config.LoadOptions{})
			if resolveErr == nil && filepath.Base(resolved) == config.NativeFileName && sameFile(resolved, native) {
				a.warnf("migrated %s; the native file now takes precedence — delete the legacy file once satisfied: %s", legacy, legacy)
			} else {
				a.warnf("migrated %s; set HERDR_SESH_CONFIG=%s before deleting %s (or keep using --config %s)", legacy, native, legacy, native)
			}
		}
		_, err = fmt.Fprintln(a.Out, native)
		return err
	default:
		return errors.New("unknown config command")
	}
}

func (a *App) validateConfig(args []string) error {
	if len(args) > 1 {
		return errors.New("config validate accepts at most one path")
	}
	var path string
	if len(args) == 1 {
		path = args[0] //nolint:gosec // len(args) is exactly one.
		if path == "" {
			return errors.New("config validate path must not be empty")
		}
	}
	_, resolved, err := config.Load(config.LoadOptions{Path: path, Warn: a.Err, StrictLegacy: true})
	if err != nil {
		return err
	}
	if resolved == "" {
		return errors.New("no config file found")
	}
	_, err = fmt.Fprintln(a.Out, resolved)
	return err
}

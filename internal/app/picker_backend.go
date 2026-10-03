package app

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/fullerzz/herdr-plugin-sesh/internal/config"
	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	pickerpkg "github.com/fullerzz/herdr-plugin-sesh/internal/picker"
	"github.com/fullerzz/herdr-plugin-sesh/internal/settings"
	"github.com/fullerzz/herdr-plugin-sesh/internal/state"
)

type pickerBackend struct {
	app               *App
	ctx               context.Context
	client            *herdr.CLIClient
	cfg               config.Config
	activeConfigPath  string
	historyDir        string
	pickerWorkspaceID string
	herdrWorkspaces   []model.Session
	warnings          []string
}

func (b *pickerBackend) warnf(format string, args ...any) {
	b.warnings = append(b.warnings, fmt.Sprintf(format, args...))
}

// The alt-screen would overwrite warnings, so print them after it exits.
func (b *pickerBackend) flushWarnings() {
	for _, warning := range b.warnings {
		b.app.warnf("%s", warning)
	}
	b.warnings = nil
}

func (b *pickerBackend) initialize(ctx context.Context) ([]model.Session, pickerpkg.DisplayOptions, error) {
	var err error
	b.historyDir, err = historyStateDir()
	if err != nil {
		return nil, pickerpkg.DisplayOptions{}, fmt.Errorf("resolve workspace history: %w", err)
	}
	col, err := b.app.collectPickerWithClient(ctx, b.cfg, nil, b.client)
	if col.HerdrErr != nil {
		b.warnf("herdr workspaces unavailable: %v", col.HerdrErr)
	}
	if err != nil {
		return nil, pickerpkg.DisplayOptions{}, err
	}
	b.herdrWorkspaces = col.HerdrWorkspaces
	result := b.historyMetadata(b.cfg)
	result.HerdrWorkspaces = col.HerdrWorkspaces
	return col.Sessions, b.displayOptions(b.cfg, result), nil
}

func (b *pickerBackend) displayOptions(cfg config.Config, result pickerpkg.ReloadResult) pickerpkg.DisplayOptions {
	opts := pickerOptionsFromConfig(b.ctx, b.app.Out, cfg)
	opts.RecentWorkspaceIDs = result.RecentWorkspaceIDs
	opts.LastWorkspaceID = result.LastWorkspaceID
	opts.LastWorkspaceUnknown = result.LastWorkspaceUnknown
	opts.HerdrWorkspaces = result.HerdrWorkspaces
	return opts
}

func (b *pickerBackend) historyMetadata(cfg config.Config) pickerpkg.ReloadResult {
	history, err := state.LoadHistory(b.historyDir)
	result := pickerpkg.ReloadResult{RecentWorkspaceIDs: append([]string{b.pickerWorkspaceID}, history.Workspaces...)}
	if err != nil {
		b.warnf("ignoring workspace history: %v", err)
	}
	if !cfg.TUI.ShowLastWorkspace {
		return result
	}
	result.LastWorkspaceUnknown = err != nil
	if err != nil {
		return result
	}
	if b.pickerWorkspaceID == "" {
		if len(history.Workspaces) > 1 {
			result.LastWorkspaceID = history.Workspaces[1]
		}
	} else {
		for _, id := range history.Workspaces {
			if id != "" && id != b.pickerWorkspaceID {
				result.LastWorkspaceID = id
				break
			}
		}
	}
	return result
}

func (b *pickerBackend) CloseWorkspace(ctx context.Context, id string) error {
	if err := b.client.WorkspaceClose(ctx, id); err != nil {
		return err
	}
	if err := state.RemoveWorkspace(b.historyDir, id); err != nil {
		b.warnf("could not prune workspace history: %v", err)
	}
	b.herdrWorkspaces = slices.DeleteFunc(b.herdrWorkspaces, func(s model.Session) bool { return s.WorkspaceID == id })
	for i := range b.herdrWorkspaces {
		if b.herdrWorkspaces[i].Worktree.ParentWorkspaceID == id {
			b.herdrWorkspaces[i].Worktree.ParentWorkspaceID = ""
			b.herdrWorkspaces[i].Worktree.ParentWorkspaceName = ""
		}
	}
	return nil
}

func (b *pickerBackend) ReloadPicker(ctx context.Context) (pickerpkg.ReloadResult, error) {
	col, err := b.app.collectPickerWithClient(ctx, b.cfg, nil, b.client)
	if err == nil {
		err = col.HerdrErr
	}
	return b.reloadMetadata(ctx, b.cfg, col, err)
}

func (b *pickerBackend) reloadMetadata(ctx context.Context, cfg config.Config, col pickerCollection, reloadErr error) (pickerpkg.ReloadResult, error) {
	if err := ctx.Err(); err != nil {
		return pickerpkg.ReloadResult{LastWorkspaceUnknown: true}, err
	}
	if col.HerdrErr == nil {
		b.herdrWorkspaces = col.HerdrWorkspaces
	} else {
		b.warnf("herdr workspaces unavailable: %v", col.HerdrErr)
	}
	focusedPane, focusErr := b.client.PaneFocused(ctx)
	b.pickerWorkspaceID = focusedPane.WorkspaceID
	if focusErr == nil && b.pickerWorkspaceID == "" {
		focusErr = errors.New("focused pane has no workspace ID")
	}
	if focusErr != nil {
		b.pickerWorkspaceID = ""
	}
	result := b.historyMetadata(cfg)
	result.Sessions, result.HerdrWorkspaces = col.Sessions, b.herdrWorkspaces
	if focusErr != nil {
		if cfg.TUI.ShowLastWorkspace {
			result.LastWorkspaceUnknown = true
			b.warnf("could not determine last workspace: find focused workspace after close: %v", focusErr)
		} else {
			b.warnf("could not determine picker workspace after close: %v", focusErr)
		}
	}
	if err := ctx.Err(); err != nil {
		return pickerpkg.ReloadResult{LastWorkspaceUnknown: true}, err
	}
	return result, reloadErr
}

func (b *pickerBackend) RefreshAgentStatuses() (map[string]string, error) {
	workspaces, err := b.client.WorkspaceList(b.ctx)
	if err != nil {
		return nil, err
	}
	statuses := make(map[string]string, len(workspaces))
	for _, workspace := range workspaces {
		statuses[workspace.ID] = workspace.AgentStatus
	}
	return statuses, nil
}

func (b *pickerBackend) OpenSettings() (settings.Model, error) {
	return settings.Open(config.LoadOptions{Path: b.activeConfigPath, Warn: b.app.Err}, b.app.settingsSaved)
}

func (b *pickerBackend) ReloadSettings(ctx context.Context, saved settings.Result) (pickerpkg.DisplayOptions, pickerpkg.ReloadResult, error) {
	cfg, path, err := config.Load(config.LoadOptions{Path: saved.Path, Warn: b.app.Err})
	if err != nil {
		return pickerpkg.DisplayOptions{}, pickerpkg.ReloadResult{}, err
	}
	col, err := b.app.collectPickerWithClient(ctx, cfg, b.herdrWorkspaces, b.client)
	result, err := b.reloadMetadata(ctx, cfg, col, err)
	if err != nil {
		return pickerpkg.DisplayOptions{}, result, err
	}
	b.cfg, b.activeConfigPath = cfg, path
	return b.displayOptions(cfg, result), result, nil
}

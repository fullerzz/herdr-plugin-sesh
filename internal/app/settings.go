package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/fullerzz/herdr-plugin-sesh/internal/config"
	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
	"github.com/fullerzz/herdr-plugin-sesh/internal/settings"
	"github.com/fullerzz/herdr-plugin-sesh/internal/state"
)

func (a *App) refreshOverlayGeometry(ctx context.Context, entry string) {
	if os.Getenv("HERDR_PLUGIN_ENTRYPOINT_ID") == entry {
		if paneID := os.Getenv("HERDR_PANE_ID"); paneID != "" {
			client := herdr.NewCLIClient()
			layout, refreshErr := client.PaneLayout(ctx, paneID)
			if refreshErr == nil && layout.Zoomed {
				// Herdr 0.9.0 starts overlays at the outer pane size. Keeping the
				// already-zoomed pane zoomed synchronizes its bordered PTY geometry.
				refreshErr = client.PaneZoom(ctx, paneID)
			}
			if refreshErr != nil {
				a.warnf("could not refresh %s pane geometry: %v", entry, refreshErr)
			}
		}
	}
}
func (a *App) settingsSaved(_ string) error {
	dir := os.Getenv("HERDR_PLUGIN_STATE_DIR")
	if dir == "" {
		return nil
	}
	//nolint:gosec // Plugin-owned cache directory supplied by Herdr.
	if err := os.Remove(state.SessionCachePath(dir)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove stale session cache: %w", err)
	}
	return nil
}
func (a *App) editSettings(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("config edit", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	path := fs.String("config", "", "config to edit (defaults to normal discovery)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("config edit accepts only --config <path>")
	}
	editor, err := settings.Open(config.LoadOptions{Path: *path, Warn: a.Err}, a.settingsSaved)
	if err != nil {
		return err
	}
	a.refreshOverlayGeometry(ctx, "settings")
	_, err = settings.RunModel(ctx, a.Out, editor)
	return err
}

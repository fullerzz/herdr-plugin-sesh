package app

import (
	"context"
	"errors"

	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
	"github.com/fullerzz/herdr-plugin-sesh/internal/state"
)

// Users render the marker with `$sesh_last` in [ui.sidebar.spaces] rows.
const (
	lastWorkspaceToken      = "sesh_last"
	lastWorkspaceTokenValue = "last"
	metadataSource          = "fullerzz.sesh"
)

type workspaceTokenReporter interface {
	WorkspaceList(context.Context) ([]herdr.Workspace, error)
	WorkspaceReportToken(ctx context.Context, id, source, name, value string) error
}

// syncLastWorkspaceMarker puts the sesh_last token on the workspace that
// `herdr-sesh last` would focus from focusedID and clears it everywhere else.
// It reconciles against Herdr's reported tokens, so stale markers from a
// replaced watcher or a failed report are repaired by the next sync.
func syncLastWorkspaceMarker(ctx context.Context, client workspaceTokenReporter, historyDir, focusedID string) error {
	history, err := state.LoadHistory(historyDir)
	if err != nil {
		return err
	}
	target, _ := history.PreviousWorkspace(focusedID)
	workspaces, err := client.WorkspaceList(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, w := range workspaces {
		value := w.Tokens[lastWorkspaceToken]
		switch {
		case w.ID == target && value != lastWorkspaceTokenValue:
			errs = append(errs, client.WorkspaceReportToken(ctx, w.ID, metadataSource, lastWorkspaceToken, lastWorkspaceTokenValue))
		case w.ID != target && value != "":
			errs = append(errs, client.WorkspaceReportToken(ctx, w.ID, metadataSource, lastWorkspaceToken, ""))
		}
	}
	return errors.Join(errs...)
}

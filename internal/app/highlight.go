package app

import (
	"context"
	"sync"

	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
)

const (
	highlightSource = "fullerzz.sesh"
	highlightToken  = "sesh"
	highlightValue  = "◀ sesh"
	// Renewed by the picker's one-second status tick; expires on its own if the
	// picker dies without clearing.
	highlightTTLMS = 3000
)

// sidebarHighlighter mirrors the picker selection into the Herdr sidebar by
// reporting a workspace metadata token, shown via a $sesh entry in
// [ui.sidebar.spaces] rows. Reports are best-effort; failures are ignored.
type sidebarHighlighter struct {
	mu     sync.Mutex
	client herdr.Client
	last   string
}

func (h *sidebarHighlighter) Report(ctx context.Context, workspaceID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.last != "" && h.last != workspaceID {
		_ = h.client.WorkspaceReportMetadata(ctx, herdr.WorkspaceMetadataRequest{
			WorkspaceID: h.last,
			Source:      highlightSource,
			ClearTokens: []string{highlightToken},
		})
	}
	if workspaceID != "" {
		_ = h.client.WorkspaceReportMetadata(ctx, herdr.WorkspaceMetadataRequest{
			WorkspaceID: workspaceID,
			Source:      highlightSource,
			Tokens:      map[string]string{highlightToken: highlightValue},
			TTLMS:       highlightTTLMS,
		})
	}
	h.last = workspaceID
}

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
	seq    uint64
	closed bool
}

// Report applies the selection stamped with seq. Reports run on concurrent
// goroutines and can arrive out of order; seq is assigned when the report is
// created inside the picker's single-threaded update loop, so any report
// older than one already applied is dropped.
func (h *sidebarHighlighter) Report(ctx context.Context, seq uint64, workspaceID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed || seq <= h.seq {
		return
	}
	h.seq = seq
	h.apply(ctx, workspaceID)
}

// Close clears any remaining highlight and rejects all later reports,
// including ones still in flight when the picker exits.
func (h *sidebarHighlighter) Close(ctx context.Context) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	h.apply(ctx, "")
}

func (h *sidebarHighlighter) apply(ctx context.Context, workspaceID string) {
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

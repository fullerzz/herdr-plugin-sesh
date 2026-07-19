package app

import (
	"context"
	"sync"
	"time"

	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
)

const (
	highlightSource = "fullerzz.sesh"
	highlightToken  = "sesh"
	// Renewed by the spinner tick; expires on its own if the picker dies
	// without clearing.
	highlightTTLMS = 3000

	defaultSpinner = "toggle"
)

// Single-width glyphs only: double-width emoji would jitter the sidebar
// column layout.
var spinnerSets = map[string][]string{
	"braille":       {"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
	"braille-heavy": {"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"},
	"line":          {"|", "/", "-", "\\"},
	"circle":        {"◐", "◓", "◑", "◒"},
	"triangle":      {"◢", "◣", "◤", "◥"},
	"arrows":        {"←", "↖", "↑", "↗", "→", "↘", "↓", "↙"},
	"toggle":        {"⊶", "⊷"},
	"bounce":        {"⠁", "⠂", "⠄", "⠂"},
}

// ponytail: unknown names silently fall back to the default set; strict_mode
// validation if typos become a support burden.
func spinnerFramesFor(name string) []string {
	if frames, ok := spinnerSets[name]; ok {
		return frames
	}
	return spinnerSets[defaultSpinner]
}

// One full cycle takes ~800ms regardless of frame count, so two-frame sets
// blink calmly and ten-frame sets stay smooth. Clamped under herdr's 60 fps
// render cap (MIN_RENDER_INTERVAL 16ms).
func spinnerIntervalFor(frames []string) time.Duration {
	interval := 800 * time.Millisecond / time.Duration(len(frames))
	return min(max(interval, 80*time.Millisecond), 400*time.Millisecond)
}

func spinnerLabel(frames []string, frame int) string {
	return frames[frame%len(frames)] + " sesh"
}

// sidebarHighlighter mirrors the picker selection into the Herdr sidebar by
// reporting a workspace metadata token, shown via a $sesh entry in
// [ui.sidebar.spaces] rows. Reports are best-effort; failures are ignored.
type sidebarHighlighter struct {
	mu       sync.Mutex
	client   herdr.Client
	frames   []string
	interval time.Duration
	last     string
	seq      uint64
	frame    int
	closed   bool
	stop     chan struct{}
}

// newSidebarHighlighter builds a highlighter using the named spinner set
// (see spinnerSets); unknown names fall back to the default.
func newSidebarHighlighter(client herdr.Client, spinner string) *sidebarHighlighter {
	frames := spinnerFramesFor(spinner)
	return &sidebarHighlighter{client: client, frames: frames, interval: spinnerIntervalFor(frames)}
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

// StartSpinner animates the highlight token frame until Close or ctx
// cancellation. Later calls and calls after Close are no-ops.
func (h *sidebarHighlighter) StartSpinner(ctx context.Context) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed || h.stop != nil {
		return
	}
	h.stop = make(chan struct{})
	go h.spin(ctx, h.stop)
}

func (h *sidebarHighlighter) spin(ctx context.Context, stop chan struct{}) {
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-stop:
			return
		case <-ticker.C:
			h.tick(ctx)
		}
	}
}

func (h *sidebarHighlighter) tick(ctx context.Context) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed || h.last == "" {
		return
	}
	h.frame = (h.frame + 1) % len(h.frames)
	h.report(ctx, h.last)
}

// Close clears any remaining highlight and rejects all later reports,
// including ones still in flight when the picker exits.
func (h *sidebarHighlighter) Close(ctx context.Context) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	if h.stop != nil {
		close(h.stop)
		h.stop = nil
	}
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
		h.report(ctx, workspaceID)
	}
	h.last = workspaceID
}

func (h *sidebarHighlighter) report(ctx context.Context, workspaceID string) {
	_ = h.client.WorkspaceReportMetadata(ctx, herdr.WorkspaceMetadataRequest{
		WorkspaceID: workspaceID,
		Source:      highlightSource,
		Tokens:      map[string]string{highlightToken: spinnerLabel(h.frames, h.frame)},
		TTLMS:       highlightTTLMS,
	})
}

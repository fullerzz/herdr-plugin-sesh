package app

import (
	"context"
	"testing"

	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
)

func TestSidebarHighlighterReportsAndClears(t *testing.T) {
	fake := &herdr.FakeClient{}
	h := &sidebarHighlighter{client: fake}
	ctx := context.Background()

	h.Report(ctx, "w1")
	h.Report(ctx, "w1")
	h.Report(ctx, "w2")
	h.Report(ctx, "")
	h.Report(ctx, "")

	calls := fake.ReportedMetadata
	if len(calls) != 5 {
		t.Fatalf("calls=%#v", calls)
	}
	for i, id := range []string{"w1", "w1"} {
		if calls[i].WorkspaceID != id || calls[i].Tokens[highlightToken] != highlightValue || calls[i].TTLMS != highlightTTLMS {
			t.Fatalf("call %d = %#v", i, calls[i])
		}
	}
	if calls[2].WorkspaceID != "w1" || len(calls[2].ClearTokens) != 1 || calls[2].ClearTokens[0] != highlightToken {
		t.Fatalf("expected clear of w1, got %#v", calls[2])
	}
	if calls[3].WorkspaceID != "w2" || calls[3].Tokens[highlightToken] != highlightValue {
		t.Fatalf("expected set of w2, got %#v", calls[3])
	}
	if calls[4].WorkspaceID != "w2" || len(calls[4].ClearTokens) != 1 {
		t.Fatalf("expected clear of w2, got %#v", calls[4])
	}
	if calls[0].Source != highlightSource || calls[2].Source != highlightSource {
		t.Fatalf("source mismatch: %#v", calls)
	}
}

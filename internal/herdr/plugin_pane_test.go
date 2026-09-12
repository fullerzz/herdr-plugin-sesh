package herdr

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type pluginPaneRunner struct {
	calls   [][]string
	open    fixedRunner
	refresh fixedRunner
}

func (r *pluginPaneRunner) Run(ctx context.Context, bin string, args ...string) ([]byte, []byte, error) {
	r.calls = append(r.calls, append([]string{bin}, args...))
	if len(r.calls) == 1 {
		return r.open.Run(ctx, bin, args...)
	}
	return r.refresh.Run(ctx, bin, args...)
}

func TestCLIClientRefreshesOverlayGeometryAfterOpen(t *testing.T) {
	// Herdr 0.9.0 initially gives overlays the outer pane dimensions.
	// Keeping the new overlay zoomed triggers a resize to its bordered interior.
	r := &pluginPaneRunner{open: fixedRunner{stdout: []byte(`{"result":{"type":"plugin_pane_opened","plugin_pane":{"plugin_id":"fullerzz.sesh","entrypoint":"picker","pane":{"pane_id":"w5:pE0"}}}}`)}}
	c := &CLIClient{Bin: "/bin/herdr", Runner: r}
	require.NoError(t, c.PluginPaneOpen(context.Background(), "fullerzz.sesh", "picker", "overlay"))
	assert.Equal(t, [][]string{
		{"/bin/herdr", "plugin", "pane", "open", "--plugin", "fullerzz.sesh", "--entrypoint", "picker", "--placement", "overlay"},
		{"/bin/herdr", "pane", "zoom", "w5:pE0", "--on"},
	}, r.calls)
}

func TestCLIClientOverlayRefreshErrors(t *testing.T) {
	for _, tc := range []struct {
		name      string
		open      fixedRunner
		refresh   fixedRunner
		wantError string
		wantCalls int
	}{
		{name: "open failed", open: fixedRunner{err: errors.New("open failed")}, wantError: "open failed", wantCalls: 1},
		{name: "invalid response", open: fixedRunner{stdout: []byte(`not JSON`)}, wantError: "decode herdr plugin pane open JSON", wantCalls: 1},
		{name: "missing pane ID", open: fixedRunner{stdout: []byte(`{"result":{"plugin_pane":{"pane":{}}}}`)}, wantError: "missing pane ID", wantCalls: 1},
		{name: "refresh failed", open: fixedRunner{stdout: []byte(`{"result":{"plugin_pane":{"pane":{"pane_id":"w5:pE0"}}}}`)}, refresh: fixedRunner{err: errors.New("refresh failed")}, wantError: "refresh failed", wantCalls: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &pluginPaneRunner{open: tc.open, refresh: tc.refresh}
			c := &CLIClient{Bin: "/bin/herdr", Runner: r}
			err := c.PluginPaneOpen(context.Background(), "fullerzz.sesh", "picker", "overlay")
			require.ErrorContains(t, err, tc.wantError)
			assert.Len(t, r.calls, tc.wantCalls)
		})
	}
}

func TestCLIClientDoesNotZoomNonOverlayPluginPanes(t *testing.T) {
	for _, placement := range []string{"split", "tab", "zoomed", "popup"} {
		t.Run(placement, func(t *testing.T) {
			r := &pluginPaneRunner{}
			c := &CLIClient{Bin: "/bin/herdr", Runner: r}
			require.NoError(t, c.PluginPaneOpen(context.Background(), "fullerzz.sesh", "picker", placement))
			assert.Len(t, r.calls, 1)
		})
	}
}

package preview

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderPaneWithoutRunningWorkspace(t *testing.T) {
	t.Setenv("HERDR_BIN_PATH", "/does-not-exist")
	for _, s := range []model.Session{{Source: "config", Path: "/tmp"}, {Source: "herdr"}} {
		text, err := RenderPane(context.Background(), s)
		require.NoError(t, err)
		require.Contains(t, text, "only available for running Herdr workspaces")
	}
}

func TestRenderPaneUsesHerdrWithoutSessionPath(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "herdr")
	script := `#!/bin/sh
case "$*" in
  'api snapshot')
    printf '%s' '{"result":{"snapshot":{"workspaces":[{"workspace_id":"w2","active_tab_id":"w2:t1"}],"layouts":[{"workspace_id":"w2","tab_id":"w2:t1","focused_pane_id":"w2:p2"}]}}}' ;;
  'pane read w2:p2 --source visible --format ansi')
    printf '\033[32mterminal output\033[0m\n' ;;
  *) exit 1 ;;
esac
`
	//nolint:gosec // the fake Herdr binary must be executable for this test.
	require.NoError(t, os.WriteFile(bin, []byte(script), 0700))
	t.Setenv("HERDR_BIN_PATH", bin)
	text, err := RenderPane(context.Background(), model.Session{Source: "herdr", WorkspaceID: "w2", PreviewCommand: "exit 1"})
	require.NoError(t, err)
	assert.Equal(t, "\x1b[32mterminal output\x1b[0m\n", text)
}

func TestRenderUsesPreviewCommand(t *testing.T) {
	out, err := Render(context.Background(), model.Session{Path: "/tmp/has space", PreviewCommand: "printf %s {}"}, "")
	require.NoError(t, err)
	assert.Equal(t, "/tmp/has space", strings.TrimSpace(out))
}

func TestRunShellCleansUpDescendantAfterShellExit(t *testing.T) {
	dir := t.TempDir()
	ready := filepath.Join(dir, "ready")
	release := filepath.Join(dir, "release")
	survived := filepath.Join(dir, "survived")
	t.Setenv("READY_FILE", ready)
	t.Setenv("RELEASE_FILE", release)
	t.Setenv("SURVIVED_FILE", survived)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := runShell(ctx, `sh -c 'printf ready > "$READY_FILE"; while [ ! -e "$RELEASE_FILE" ]; do sleep 0.01; done; printf survived > "$SURVIVED_FILE"' &`)
		done <- err
	}()

	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		require.False(t, time.Now().After(deadline), "descendant process did not start")
		time.Sleep(10 * time.Millisecond)
	}

	returned := false
	var runErr error
	select {
	case runErr = <-done:
		returned = true
	case <-time.After(time.Second):
		assert.Fail(t, "runShell remained blocked after its shell exited")
	}
	cancel()
	require.NoError(t, os.WriteFile(release, []byte("release"), 0600))
	if !returned {
		select {
		case runErr = <-done:
		case <-time.After(time.Second):
			require.FailNow(t, "runShell did not return after descendant cleanup")
		}
	}
	deadline = time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(survived); err == nil {
			require.FailNow(t, "descendant process survived shell exit and cancellation")
		} else {
			require.ErrorIs(t, err, os.ErrNotExist)
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.ErrorIs(t, runErr, exec.ErrWaitDelay)
}

func TestRunShellCleansUpRedirectedDescendantAfterShellExit(t *testing.T) {
	dir := t.TempDir()
	ready := filepath.Join(dir, "ready")
	release := filepath.Join(dir, "release")
	survived := filepath.Join(dir, "survived")
	t.Setenv("READY_FILE", ready)
	t.Setenv("RELEASE_FILE", release)
	t.Setenv("SURVIVED_FILE", survived)
	t.Cleanup(func() { _ = os.WriteFile(release, []byte("release"), 0600) })

	_, err := runShell(context.Background(), `sh -c 'printf ready > "$READY_FILE"; while [ ! -e "$RELEASE_FILE" ]; do sleep 0.01; done; printf survived > "$SURVIVED_FILE"' >/dev/null 2>&1 & while [ ! -e "$READY_FILE" ]; do sleep 0.01; done`)
	require.NoError(t, err)
	_, err = os.Stat(ready)
	require.NoError(t, err, "descendant process did not start")
	require.NoError(t, os.WriteFile(release, []byte("release"), 0600))
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(survived); err == nil {
			require.FailNow(t, "redirected descendant survived shell exit")
		} else {
			require.ErrorIs(t, err, os.ErrNotExist)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRunShellCancellationKillsDescendantProcess(t *testing.T) {
	dir := t.TempDir()
	ready := filepath.Join(dir, "ready")
	release := filepath.Join(dir, "release")
	survived := filepath.Join(dir, "survived")
	t.Setenv("READY_FILE", ready)
	t.Setenv("RELEASE_FILE", release)
	t.Setenv("SURVIVED_FILE", survived)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		_, _ = runShell(ctx, `sh -c 'printf ready > "$READY_FILE"; while [ ! -e "$RELEASE_FILE" ]; do sleep 0.01; done; printf survived > "$SURVIVED_FILE"' & wait`)
		close(done)
	}()

	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		require.False(t, time.Now().After(deadline), "descendant process did not start")
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		require.FailNow(t, "runShell remained blocked after cancellation")
	}
	require.NoError(t, os.WriteFile(release, []byte("release"), 0600))
	deadline = time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(survived); err == nil {
			require.FailNow(t, "descendant process survived cancellation")
		} else {
			require.ErrorIs(t, err, os.ErrNotExist)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRenderMissingPathWithoutWorkspaceReturnsStableText(t *testing.T) {
	out, err := Render(context.Background(), model.Session{Name: "api"}, "printf %s {}")
	require.NoError(t, err)
	assert.Contains(t, out, "No item path available")
}

func TestRenderMissingPathWithWorkspaceReturnsWorkspaceSummary(t *testing.T) {
	out, err := Render(context.Background(), model.Session{Name: "api", WorkspaceID: "ws1"}, "printf %s {}")
	require.NoError(t, err)
	require.Contains(t, out, "workspace: api")
	assert.Contains(t, out, "id: ws1")
}

func TestRenderDirectoryFallbackSorted(t *testing.T) {
	d := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(d, "b"), []byte(""), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(d, "a"), []byte(""), 0600))
	out, err := Render(context.Background(), model.Session{Path: d}, "")
	require.NoError(t, err)
	assert.Contains(t, out, "a\nb")
}

func TestRenderBatUsesBatForReadmePreview(t *testing.T) {
	d := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(d, "README.md"), []byte("hello from readme\n"), 0600))
	bin := t.TempDir()
	bat := filepath.Join(bin, "bat")
	require.NoError(t, os.WriteFile(bat, []byte("#!/bin/sh\nfor last do :; done\nif [ -f \"$last\" ]; then /bin/cat \"$last\"; else /bin/cat; fi\n"), 0600))
	//nolint:gosec // the fake bat binary must be executable for this test.
	require.NoError(t, os.Chmod(bat, 0700))
	t.Setenv("PATH", bin)

	out, err := RenderBat(context.Background(), model.Session{Path: d})
	require.NoError(t, err)
	assert.Contains(t, out, "hello from readme")
}

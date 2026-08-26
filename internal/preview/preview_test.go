package preview

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
)

func TestRenderUsesPreviewCommand(t *testing.T) {
	out, err := Render(context.Background(), model.Session{Path: "/tmp/has space", PreviewCommand: "printf %s {}"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "/tmp/has space" {
		t.Fatalf("got %q", out)
	}
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
		if time.Now().After(deadline) {
			t.Fatal("descendant process did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}

	returned := false
	var runErr error
	select {
	case runErr = <-done:
		returned = true
	case <-time.After(time.Second):
		t.Error("runShell remained blocked after its shell exited")
	}
	cancel()
	if err := os.WriteFile(release, []byte("release"), 0600); err != nil {
		t.Fatal(err)
	}
	if !returned {
		select {
		case runErr = <-done:
		case <-time.After(time.Second):
			t.Fatal("runShell did not return after descendant cleanup")
		}
	}
	if !errors.Is(runErr, exec.ErrWaitDelay) {
		t.Errorf("runShell error = %v, want %v", runErr, exec.ErrWaitDelay)
	}
	deadline = time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(survived); err == nil {
			t.Fatal("descendant process survived shell exit and cancellation")
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ready); err != nil {
		t.Fatalf("descendant process did not start: %v", err)
	}
	if err := os.WriteFile(release, []byte("release"), 0600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(survived); err == nil {
			t.Fatal("redirected descendant survived shell exit")
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
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
		if time.Now().After(deadline) {
			t.Fatal("descendant process did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runShell remained blocked after cancellation")
	}
	if err := os.WriteFile(release, []byte("release"), 0600); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(survived); err == nil {
			t.Fatal("descendant process survived cancellation")
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRenderMissingPathWithoutWorkspaceReturnsStableText(t *testing.T) {
	out, err := Render(context.Background(), model.Session{Name: "api"}, "printf %s {}")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "No item path available") {
		t.Fatalf("missing stable no-path text: %q", out)
	}
}

func TestRenderMissingPathWithWorkspaceReturnsWorkspaceSummary(t *testing.T) {
	out, err := Render(context.Background(), model.Session{Name: "api", WorkspaceID: "ws1"}, "printf %s {}")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "workspace: api") {
		t.Fatalf("missing workspace name: %q", out)
	}
	if !strings.Contains(out, "id: ws1") {
		t.Fatalf("missing workspace id: %q", out)
	}
}

func TestRenderDirectoryFallbackSorted(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "b"), []byte(""), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "a"), []byte(""), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := Render(context.Background(), model.Session{Path: d}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "a\nb") {
		t.Fatalf("not sorted: %q", out)
	}
}

func TestRenderBatUsesBatForReadmePreview(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "README.md"), []byte("hello from readme\n"), 0600); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	bat := filepath.Join(bin, "bat")
	if err := os.WriteFile(bat, []byte("#!/bin/sh\nfor last do :; done\nif [ -f \"$last\" ]; then /bin/cat \"$last\"; else /bin/cat; fi\n"), 0600); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // the fake bat binary must be executable for this test.
	if err := os.Chmod(bat, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	out, err := RenderBat(context.Background(), model.Session{Path: d})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "hello from readme") {
		t.Fatalf("preview missing readme content: %q", out)
	}
}

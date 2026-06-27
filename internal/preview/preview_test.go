package preview

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

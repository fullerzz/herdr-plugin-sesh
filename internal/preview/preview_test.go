package preview

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"forgejo.local/fullerzz/herdr-plugin-sesh/internal/model"
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
	if err := os.WriteFile(filepath.Join(d, "b"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "a"), []byte(""), 0644); err != nil {
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

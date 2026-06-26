package picker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"forgejo.local/fullerzz/herdr-plugin-sesh/internal/model"
)

func TestRunFZFSelectsSessionByHiddenIndex(t *testing.T) {
	fzf := filepath.Join(t.TempDir(), "fzf")
	if err := os.WriteFile(fzf, []byte("#!/bin/sh\ncat >/dev/null\nprintf '1\\tzoxide\\t"+zoxideSourceIcon+" zoxide\\tweb\\t/tmp/web\\tweb\\n'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // the fake fzf binary must be executable for this test.
	if err := os.Chmod(fzf, 0700); err != nil {
		t.Fatal(err)
	}
	selected, ok, err := RunFZF(context.Background(), []model.Session{
		{Source: "config", Name: "api", Path: "/tmp/api"},
		{Source: "zoxide", Name: "web", Path: "/tmp/web"},
	}, Options{FZFCommand: fzf})
	if err != nil {
		t.Fatal(err)
	}
	if !ok || selected.Name != "web" {
		t.Fatalf("selected=%#v ok=%v", selected, ok)
	}
}

func TestFZFInputKeepsIndexHiddenAndAddsSeparatorAwareSearch(t *testing.T) {
	got := fzfInput([]model.Session{{Source: "config", Name: "api-service", Path: "/tmp/api.service"}}, true)
	if !strings.HasPrefix(got, "0\tconfig\t\x1b[1;38;5;214m"+configSourceIcon+" config\x1b[0m\tapi-service\t/tmp/api.service\t") {
		t.Fatalf("input = %q", got)
	}
	if !strings.Contains(got, "api service") || !strings.Contains(got, "tmp api service") {
		t.Fatalf("missing normalized search field: %q", got)
	}
}

func TestFZFInputUsesSourceCategoryColors(t *testing.T) {
	got := fzfInput([]model.Session{
		{Source: "herdr", Name: "herdr"},
		{Source: "config", Name: "config"},
		{Source: "zoxide", Name: "zoxide"},
		{Source: "dir", Name: "dir"},
	}, false)
	for _, want := range []string{
		"\x1b[1;38;5;81m" + herdrSourceIcon + " herdr\x1b[0m",
		"\x1b[1;38;5;214m" + configSourceIcon + " config\x1b[0m",
		"\x1b[1;38;5;114m" + zoxideSourceIcon + " zoxide\x1b[0m",
		"\x1b[1;38;5;176m[dir]\x1b[0m",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("input missing %q:\n%q", want, got)
		}
	}
}

func TestFZFArgsPreviewAllItemsWithBat(t *testing.T) {
	args := strings.Join(fzfArgs(Options{}), "\n")
	for _, want := range []string{"--ansi", "--with-nth=3,4,5", "--preview=", "source={2}", "label={4}", "command -v bat", "/opt/homebrew/bin/bat", "--file-name \"$path\""} {
		if !strings.Contains(args, want) {
			t.Fatalf("args missing %q:\n%s", want, args)
		}
	}
	if strings.Contains(args, "{2} != herdr") {
		t.Fatalf("preview should not be limited to herdr rows:\n%s", args)
	}
}

func TestFZFSelectionIndexRejectsInvalidOutput(t *testing.T) {
	for _, out := range []string{"", "abc\tconfig\tapi", "5\tconfig\tapi"} {
		if idx, ok := fzfSelectionIndex(out, 2); ok {
			t.Fatalf("idx=%d ok=true for %q", idx, out)
		}
	}
}

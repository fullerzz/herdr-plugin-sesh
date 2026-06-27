package picker

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
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
	for _, want := range []string{"--ansi", "--with-nth=3,4,5", "--preview=", "export PATH", "source={2}", "label={4}", "item_path={5}", "command -v bat", "/opt/homebrew/bin/bat", "--file-name \"$item_path\""} {
		if !strings.Contains(args, want) {
			t.Fatalf("args missing %q:\n%s", want, args)
		}
	}
	if strings.Contains(args, "\npath=") {
		t.Fatalf("preview should not assign zsh's special path variable:\n%s", args)
	}
	if strings.Contains(args, "{2} != herdr") {
		t.Fatalf("preview should not be limited to herdr rows:\n%s", args)
	}
}

func TestFZFPreviewCommandFindsSystemToolsWithMinimalPath(t *testing.T) {
	fakeBin := t.TempDir()
	bat := filepath.Join(fakeBin, "bat")
	if err := os.WriteFile(bat, []byte("#!/bin/sh\ncat\n"), 0600); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // the fake bat binary must be executable for this test.
	if err := os.Chmod(bat, 0700); err != nil {
		t.Fatal(err)
	}

	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "note.txt"), []byte("preview\n"), 0600); err != nil {
		t.Fatal(err)
	}
	script := strings.NewReplacer(
		"{2}", "zoxide",
		"{4}", "project",
		"{5}", project,
	).Replace(fzfPreviewCommand())
	run := func(t *testing.T, cmd *exec.Cmd) {
		t.Helper()
		cmd.Env = []string{"PATH=" + fakeBin, "HOME=" + t.TempDir()}
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("preview command failed: %v\n%s", err, out)
		}
		got := string(out)
		if strings.Contains(got, "not found") {
			t.Fatalf("preview command could not find system tools:\n%s", got)
		}
		if !strings.Contains(got, "session: project") || !strings.Contains(got, "note.txt") {
			t.Fatalf("preview output missing expected content:\n%s", got)
		}
	}
	previewShellCommand := func(shell string) *exec.Cmd {
		//nolint:gosec // The shell and script are fixed by this regression test.
		return exec.Command(shell, "-c", script)
	}
	t.Run("sh", func(t *testing.T) {
		run(t, previewShellCommand("/bin/sh"))
	})
	if _, err := os.Stat("/bin/bash"); err == nil {
		t.Run("bash", func(t *testing.T) {
			run(t, previewShellCommand("/bin/bash"))
		})
	}
	if _, err := os.Stat("/bin/zsh"); err == nil {
		t.Run("zsh", func(t *testing.T) {
			run(t, previewShellCommand("/bin/zsh"))
		})
	}
}

func TestFZFSelectionIndexRejectsInvalidOutput(t *testing.T) {
	for _, out := range []string{"", "abc\tconfig\tapi", "5\tconfig\tapi"} {
		if idx, ok := fzfSelectionIndex(out, 2); ok {
			t.Fatalf("idx=%d ok=true for %q", idx, out)
		}
	}
}

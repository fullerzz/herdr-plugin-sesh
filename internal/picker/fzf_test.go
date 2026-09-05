package picker

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunFZFSelectsSessionByHiddenIndex(t *testing.T) {
	fzf := filepath.Join(t.TempDir(), "fzf")
	require.NoError(t, os.WriteFile(fzf, []byte("#!/bin/sh\ncat >/dev/null\nprintf '1\\tzoxide\\t"+zoxideSourceIcon+" zoxide\\tweb\\t/tmp/web\\tweb\\n'\n"), 0600))
	//nolint:gosec // the fake fzf binary must be executable for this test.
	require.NoError(t, os.Chmod(fzf, 0700))
	selected, ok, err := RunFZF(context.Background(), []model.Session{
		{Source: "config", Name: "api", Path: "/tmp/api"},
		{Source: "zoxide", Name: "web", Path: "/tmp/web"},
	}, Options{FZFCommand: fzf})
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "web", selected.Name)
}

func TestFZFInputKeepsIndexHiddenAndAddsSeparatorAwareSearch(t *testing.T) {
	got := fzfInput([]model.Session{{Source: "config", Name: "api-service", Path: "/tmp/api.service"}}, true)
	require.True(t, strings.HasPrefix(got, "0\tconfig\t\x1b[1;38;5;214m"+configSourceIcon+" config\x1b[0m\tapi-service\t/tmp/api.service\t"))
	require.Contains(t, got, "api service")
	assert.Contains(t, got, "tmp api service")
}

func TestFZFInputAddsHomeAliasSearchToken(t *testing.T) {
	t.Setenv("HOME", "/Users/zach")
	got := fzfInput([]model.Session{{Source: "herdr", Name: "zach", Path: "/Users/zach"}}, false)
	require.True(t, strings.HasSuffix(got, "\thome\n"))
	args := strings.Join(fzfArgs(Options{}), "\n")
	assert.Contains(t, args, "--nth=3..6")
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
		assert.Contains(t, got, want)
	}
}

func TestFZFArgsPreviewAllItemsWithBat(t *testing.T) {
	args := strings.Join(fzfArgs(Options{}), "\n")
	for _, want := range []string{"--ansi", "--with-nth=3,4,5", "--preview=", "export PATH", "source={2}", "label={4}", "item_path={5}", "command -v bat", "/opt/homebrew/bin/bat", "--file-name \"$item_path\""} {
		assert.Contains(t, args, want)
	}
	require.NotContains(t, args, "\npath=")
	assert.NotContains(t, args, "{2} != herdr")
}

func TestFZFPreviewCommandFindsSystemToolsWithMinimalPath(t *testing.T) {
	fakeBin := t.TempDir()
	bat := filepath.Join(fakeBin, "bat")
	require.NoError(t, os.WriteFile(bat, []byte("#!/bin/sh\ncat\n"), 0600))
	//nolint:gosec // the fake bat binary must be executable for this test.
	require.NoError(t, os.Chmod(bat, 0700))

	project := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(project, "note.txt"), []byte("preview\n"), 0600))
	script := strings.NewReplacer(
		"{2}", "zoxide",
		"{4}", "project",
		"{5}", project,
	).Replace(fzfPreviewCommand())
	run := func(t *testing.T, cmd *exec.Cmd) {
		t.Helper()
		cmd.Env = []string{"PATH=" + fakeBin, "HOME=" + t.TempDir()}
		out, err := cmd.CombinedOutput()
		require.NoError(t, err)
		got := string(out)
		require.NotContains(t, got, "not found")
		require.Contains(t, got, "session: project")
		require.Contains(t, got, "note.txt")
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
		idx, ok := fzfSelectionIndex(out, 2)
		assert.Falsef(t, ok, "idx=%d for %q", idx, out)
	}
}

package namer

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNameFromGitRemote(t *testing.T) {
	d := t.TempDir()
	run(t, d, "git", "init")
	run(t, d, "git", "remote", "add", "origin", "git@github.com:fullerzz/herdr-plugin-sesh.git")
	got := Namer{}.Name(context.Background(), d, 1)
	assert.Equal(t, "herdr-plugin-sesh", got)
}
func TestNameFromDirectoryLength(t *testing.T) {
	got := Namer{}.Name(context.Background(), "/tmp/parent/child", 2)
	assert.Equal(t, "parent/child", got)
}
func TestNameFromRepoWithoutRemote(t *testing.T) {
	d := filepath.Join(t.TempDir(), "repo")
	require.NoError(t, os.MkdirAll(d, 0700))
	run(t, d, "git", "init")
	got := Namer{}.Name(context.Background(), d, 1)
	assert.Equal(t, "repo", got)
}
func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	//nolint:gosec // tests execute fixed git commands.
	c := exec.Command(args[0], args[1:]...)
	c.Dir = dir
	out, err := c.CombinedOutput()
	require.NoErrorf(t, err, "%v: %s", args, out)
}

package namer

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestNameFromGitRemote(t *testing.T) {
	d := t.TempDir()
	run(t, d, "git", "init")
	run(t, d, "git", "remote", "add", "origin", "git@github.com:fullerzz/herdr-plugin-sesh.git")
	got := Namer{}.Name(context.Background(), d, 1)
	if got != "herdr-plugin-sesh" {
		t.Fatalf("got %q", got)
	}
}
func TestNameFromDirectoryLength(t *testing.T) {
	got := Namer{}.Name(context.Background(), "/tmp/parent/child", 2)
	if got != "parent/child" {
		t.Fatalf("got %q", got)
	}
}
func TestNameFromRepoWithoutRemote(t *testing.T) {
	d := filepath.Join(t.TempDir(), "repo")
	os.MkdirAll(d, 0755)
	run(t, d, "git", "init")
	got := Namer{}.Name(context.Background(), d, 1)
	if got != "repo" {
		t.Fatalf("got %q", got)
	}
}
func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	c := exec.Command(args[0], args[1:]...)
	c.Dir = dir
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("%v: %v %s", args, err, out)
	}
}

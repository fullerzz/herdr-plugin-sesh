package clone

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
)

type Request struct {
	Repo   string
	CmdDir string
	Dir    string
}

func Destination(r Request) string {
	if r.Dir != "" {
		return r.Dir
	}
	base := strings.TrimSuffix(filepath.Base(r.Repo), ".git")
	if base == "" || base == "." {
		base = "repo"
	}
	if r.CmdDir != "" {
		return filepath.Join(r.CmdDir, base)
	}
	return base
}
func Clone(ctx context.Context, r Request) (string, error) {
	dest := Destination(r)
	args := []string{"clone", r.Repo}
	if r.Dir != "" {
		args = append(args, r.Dir)
	}
	c := exec.CommandContext(ctx, "git", args...)
	if r.CmdDir != "" {
		c.Dir = r.CmdDir
	}
	return dest, c.Run()
}

package preview

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"forgejo.local/fullerzz/herdr-plugin-sesh/internal/config"
	"forgejo.local/fullerzz/herdr-plugin-sesh/internal/model"
)

func Render(ctx context.Context, s model.Session, fallbackCommand string) (string, error) {
	cmd := s.PreviewCommand
	if cmd == "" {
		cmd = fallbackCommand
	}
	if cmd != "" {
		return runShell(ctx, config.SubstitutePath(cmd, s.Path))
	}
	if s.WorkspaceID != "" {
		return fmt.Sprintf("workspace: %s\nid: %s\npath: %s\n", s.Name, s.WorkspaceID, s.Path), nil
	}
	return directoryFallback(s.Path)
}

func runShell(ctx context.Context, command string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	//nolint:gosec // preview commands are user-configured shell snippets by design.
	c := exec.CommandContext(ctx, "sh", "-lc", command)
	var out, errb bytes.Buffer
	c.Stdout = &out
	c.Stderr = &errb
	err := c.Run()
	if err != nil {
		return out.String() + errb.String(), err
	}
	return out.String(), nil
}

func directoryFallback(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("no path to preview")
	}
	ents, err := os.ReadDir(path)
	if err != nil {
		return "", err
	}
	lines := make([]string, 0, len(ents))
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() {
			name += "/"
		}
		lines = append(lines, name)
	}
	sort.Strings(lines)
	return filepath.Clean(path) + "\n" + strings.Join(lines, "\n") + "\n", nil
}

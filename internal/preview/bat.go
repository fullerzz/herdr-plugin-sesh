package preview

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
)

var batPreviewFiles = []string{"README.md", "README", "AGENTS.md", "go.mod", "package.json", "pyproject.toml"}

func RenderBat(ctx context.Context, s model.Session) (string, error) {
	if s.Path == "" {
		return "", fmt.Errorf("no item path available")
	}
	if st, err := os.Stat(s.Path); err != nil || !st.IsDir() {
		return "", fmt.Errorf("no item path available")
	}
	bat, ok := batPath()
	if !ok {
		return "", fmt.Errorf("bat not found in PATH or common install locations")
	}
	for _, name := range batPreviewFiles {
		file := filepath.Join(s.Path, name)
		if st, err := os.Stat(file); err == nil && st.Mode().IsRegular() {
			return runBat(ctx, bat, nil, "--color=always", "--style=numbers", "--line-range=:160", file)
		}
	}
	files, err := previewFileList(s.Path)
	if err != nil {
		return "", err
	}
	if files == "" {
		files = "No files found\n"
	}
	return runBat(ctx, bat, strings.NewReader(files), "--color=always", "--style=plain", "--language=txt", "--file-name", s.Path)
}

func batPath() (string, bool) {
	for _, name := range []string{"bat", "batcat"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, true
		}
	}
	candidates := []string{"/opt/homebrew/bin/bat", "/usr/local/bin/bat", "/usr/bin/batcat"}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates,
			filepath.Join(home, ".local", "bin", "bat"),
			filepath.Join(home, ".local", "share", "mise", "shims", "bat"),
		)
	}
	for _, candidate := range candidates {
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() && st.Mode()&0111 != 0 {
			return candidate, true
		}
	}
	return "", false
}

func runBat(ctx context.Context, bin string, stdin io.Reader, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	//nolint:gosec // bat is a fixed local preview executable resolved from PATH or common install locations.
	cmd := exec.CommandContext(ctx, bin, args...)
	if stdin != nil {
		cmd.Stdin = stdin
	}
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return out.String() + errb.String(), err
	}
	return out.String(), nil
}

func previewFileList(root string) (string, error) {
	files := []string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == "." {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" || strings.Count(rel, string(os.PathSeparator)) >= 2 {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.Count(rel, string(os.PathSeparator)) < 2 {
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(files)
	if len(files) > 80 {
		files = files[:80]
	}
	if len(files) == 0 {
		return "", err
	}
	return strings.Join(files, "\n") + "\n", err
}

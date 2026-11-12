package preview

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/fullerzz/herdr-plugin-sesh/internal/config"
	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
)

func RenderPane(ctx context.Context, s model.Session) (string, error) {
	if s.Source != "herdr" || s.WorkspaceID == "" {
		return "Pane preview is only available for running Herdr workspaces\n", nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return herdr.NewBackend().WorkspacePaneRead(ctx, s.WorkspaceID)
}

// Render previews s, reaching a remote session's machine through the backend
// herdr.NewBackend selects.
func Render(ctx context.Context, s model.Session, fallbackCommand string) (string, error) {
	var machines herdr.MachineClient
	if s.Machine != "" {
		machines, _ = herdr.NewBackend().(herdr.MachineClient)
	}
	return RenderWith(ctx, machines, s, fallbackCommand)
}

// RenderWith previews s, reaching a remote session's machine through
// machines. With nil machines, a backend without machines, a remote session's
// preview runs nothing and shows its metadata.
func RenderWith(ctx context.Context, machines herdr.MachineClient, s model.Session, fallbackCommand string) (string, error) {
	if s.Machine != "" {
		return renderRemote(ctx, machines, s), nil
	}
	if s.Path == "" {
		if s.WorkspaceID != "" {
			return fmt.Sprintf("workspace: %s\nid: %s\npath: %s\n", s.Name, s.WorkspaceID, s.Path), nil
		}
		return "No item path available\n", nil
	}

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

func remoteSummary(s model.Session) string {
	return fmt.Sprintf("workspace: %s\nmachine: %s\nid: %s\npath: %s\n", s.Name, s.Machine, s.WorkspaceID, s.Path)
}

func runShell(ctx context.Context, command string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	sentinel := exec.Command("sh", "-c", `kill -s STOP "$$"`)
	sentinel.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := sentinel.Start(); err != nil {
		return "", err
	}
	processGroupID := sentinel.Process.Pid
	killProcessGroup := func() error {
		err := syscall.Kill(-processGroupID, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	//nolint:gosec // preview commands are user-configured shell snippets by design.
	c := exec.CommandContext(ctx, "sh", "-lc", command)
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pgid: processGroupID}
	c.Cancel = killProcessGroup
	c.WaitDelay = 250 * time.Millisecond
	var out, errb bytes.Buffer
	c.Stdout = &out
	c.Stderr = &errb
	err := c.Run()
	killErr := killProcessGroup()
	if killErr != nil {
		_ = sentinel.Process.Kill()
	}
	_ = sentinel.Wait()
	output := out.String() + errb.String()
	if err != nil {
		return output, err
	}
	if killErr != nil {
		return output, killErr
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

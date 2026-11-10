package preview

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
)

// remoteTimeout bounds the whole remote preview. hc's own --timeout takes
// whole seconds and also ends the remote program, so the two stay equal.
const remoteTimeout = 3 * time.Second

// remoteRunner runs hc; tests replace it so they never need hc or a machine.
var remoteRunner herdr.Runner = herdr.ExecRunner{}

// renderRemote lists a remote space's folder and its git status on that
// machine through `hc exec`, in place of any configured preview command:
// those name local tools and run only for local workspaces. It never runs a
// local command or reads a local path. It falls back to the space's metadata
// when the listing fails, and keeps the listing when only git status fails.
func renderRemote(ctx context.Context, s model.Session) string {
	summary := remoteSummary(s)
	if s.Path == "" {
		return summary
	}
	dir := remoteDir(s.Path)
	ctx, cancel := context.WithTimeout(ctx, remoteTimeout)
	defer cancel()
	// ls -A -p -1 is POSIX, so it works without eza or GNU coreutils.
	listing, _, err := remoteExec(ctx, s.Machine, dir, nil, "ls", "-A", "-p", "-1")
	if err != nil {
		return summary + "remote preview unavailable: " + err.Error() + "\n"
	}
	out := fmt.Sprintf("%s (%s)\n%s", s.Path, s.Machine, listing)
	status, stderr, err := remoteExec(ctx, s.Machine, dir, []string{gitLocale}, "git", "status", "--short", "--branch")
	switch {
	case err == nil:
		if status != "" {
			out += "\n" + status
		}
	case gitAbsent(stderr):
		// A folder outside a repository, or a machine without git, has no
		// summary; only the listing is required.
	default:
		out += "\ngit status unavailable: " + err.Error() + "\n"
	}
	return out
}

// gitAbsent reports the two git failures that mean there is no summary to
// show: git's own "not a git repository" error, and hc failing to find the git
// program. hc reports other spawn failures, such as a folder that vanished
// after ls ("cannot start folder ...") or a permission error, with the same
// kind, so only the missing-program message counts.
func gitAbsent(stderr []byte) bool {
	if kind, message := herdr.HCError(stderr); kind == "spawn_failed" {
		return strings.HasPrefix(message, "cannot start git: No such file or directory")
	}
	return strings.Contains(string(stderr), "not a git repository")
}

// remoteDir maps a configured path to hc exec's --cwd. hc does not expand ~,
// resolves a relative --cwd against the remote home, and starts in that home
// when --cwd is omitted, so ~ becomes "" and ~/x becomes x.
func remoteDir(path string) string {
	if path == "~" {
		return ""
	}
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		return strings.TrimLeft(rest, "/")
	}
	return path
}

// remoteExec runs argv on machine in dir, or in its home when dir is empty.
// gitLocale keeps git's messages in English on any machine: gitAbsent tells
// "not a repository" from other failures by that text, as git exits 128 for
// every fatal error.
const gitLocale = "LC_ALL=C"

func remoteExec(ctx context.Context, machine, dir string, env []string, argv ...string) (string, []byte, error) {
	seconds := strconv.Itoa(int(remoteTimeout / time.Second))
	args := []string{"exec", "--on", machine}
	if dir != "" {
		args = append(args, "--cwd", dir)
	}
	for _, kv := range env {
		args = append(args, "--env", kv)
	}
	args = append(append(args, "--timeout", seconds, "--"), argv...)
	stdout, stderr, err := remoteRunner.Run(ctx, "hc", args...)
	if err == nil {
		return string(stdout), stderr, nil
	}
	// The expired deadline may be the caller's, shorter than remoteTimeout,
	// so the message names no duration.
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "", stderr, fmt.Errorf("%s on %s: preview deadline expired", argv[0], machine)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", stderr, ctxErr
	}
	return "", stderr, fmt.Errorf("%s on %s: %s", argv[0], machine, remoteFailure(stderr, err))
}

// remoteFailure prefers hc's own error message, then the remote program's
// first stderr line, then the exit status.
func remoteFailure(stderr []byte, err error) string {
	if _, message := herdr.HCError(stderr); message != "" {
		return message
	}
	if line, _, _ := strings.Cut(strings.TrimSpace(string(stderr)), "\n"); line != "" {
		return line
	}
	return err.Error()
}

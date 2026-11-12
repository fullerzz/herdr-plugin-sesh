package preview

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/fullerzz/herdr-plugin-sesh/internal/config"
	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
)

// remoteTimeout bounds the whole remote preview. hc's own --timeout takes
// whole seconds and also ends the remote program, so the two stay equal.
const remoteTimeout = 3 * time.Second

// remoteScript lists the folder and reads its git status in one remote call,
// so both share the single deadline. It prints a header line holding git's
// exit status and the listing's length in bytes, then the listing, then git's
// output: the header splits them whatever the file names hold. ls -A -p -1 is
// POSIX, so it works without eza or GNU coreutils; a failed listing ends the
// script with ls's status and message. LC_ALL=C keeps git's messages in
// English on any machine: gitAbsent tells "not a repository" from other
// failures by that text, as git exits 128 for every fatal error. The folder
// arrives as hc's --cwd, never as script text.
const remoteScript = `l=$(ls -A -p -1) || exit
s=$(LC_ALL=C git status --short --branch 2>&1)
c=$?
printf '%s %s\n%s%s' "$c" "$(printf %s "$l" | wc -c)" "$l" "$s"`

// renderRemote lists a remote space's folder and its git status on that
// machine through the backend's MachineExec, in place of any configured
// preview command: those name local tools and run only for local workspaces.
// It never runs a local command or reads a local path. It falls back to the
// space's metadata when the listing fails or the backend has no machines, and
// keeps the listing when only git status fails.
func renderRemote(ctx context.Context, machines herdr.MachineClient, s model.Session) string {
	summary := remoteSummary(s)
	if s.Path == "" {
		return summary
	}
	if strings.HasPrefix(s.Path, "~") && !config.HasHomePrefix(s.Path) {
		return summary + fmt.Sprintf("remote preview unavailable: path %q names another user's home; use an absolute path\n", s.Path)
	}
	if machines == nil {
		return summary + "remote previews need the Herdr app (herdr-app)\n"
	}
	ctx, cancel := context.WithTimeout(ctx, remoteTimeout)
	defer cancel()
	stdout, stderr, err := machines.MachineExec(ctx, s.Machine, herdr.MachineExecRequest{
		Dir: remoteDir(s.Path), Timeout: remoteTimeout, Argv: []string{"sh", "-c", remoteScript},
	})
	if err == nil {
		var res remoteResult
		if res, err = parseRemote(stdout); err == nil {
			return res.render(s)
		}
	}
	// The expired deadline may be the caller's, shorter than remoteTimeout,
	// so the message names no duration.
	switch {
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded):
		err = fmt.Errorf("ls on %s: preview deadline expired", s.Machine)
	case errors.Is(err, context.Canceled) || ctx.Err() != nil:
		err = context.Canceled
	default:
		err = fmt.Errorf("ls on %s: %s", s.Machine, remoteFailure(stderr, err))
	}
	return summary + "remote preview unavailable: " + err.Error() + "\n"
}

type remoteResult struct {
	listing, git string
	gitStatus    int
}

// parseRemote splits remoteScript's output at the byte length its header
// gives.
func parseRemote(stdout []byte) (remoteResult, error) {
	header, body, ok := strings.Cut(string(stdout), "\n")
	fields := strings.Fields(header)
	if !ok || len(fields) != 2 {
		return remoteResult{}, fmt.Errorf("unexpected preview output %q", header)
	}
	status, statusErr := strconv.Atoi(fields[0])
	n, lenErr := strconv.Atoi(fields[1])
	if statusErr != nil || lenErr != nil || n < 0 || n > len(body) {
		return remoteResult{}, fmt.Errorf("unexpected preview output %q", header)
	}
	return remoteResult{listing: body[:n], git: body[n:], gitStatus: status}, nil
}

func (r remoteResult) render(s model.Session) string {
	out := fmt.Sprintf("%s (%s)\n%s", s.Path, s.Machine, withNewline(r.listing))
	switch {
	case r.gitStatus == 0:
		if r.git != "" {
			out += "\n" + withNewline(r.git)
		}
	case gitAbsent(r.gitStatus, r.git):
		// A folder outside a repository, or a machine without git, has no
		// summary; only the listing is required.
	default:
		out += fmt.Sprintf("\ngit status unavailable: git on %s: %s\n", s.Machine, remoteFailure([]byte(r.git), fmt.Errorf("exit status %d", r.gitStatus)))
	}
	return out
}

// withNewline restores the final newline that sh's $(...) strips.
func withNewline(text string) string {
	if text == "" || strings.HasSuffix(text, "\n") {
		return text
	}
	return text + "\n"
}

// gitAbsent reports the two git failures that mean there is no summary to
// show: git's own "not a git repository" error, and sh not finding the git
// program (status 127). Any other failure, such as a git that cannot be run
// (126), means the status is unknown.
func gitAbsent(status int, output string) bool {
	return status == 127 || strings.Contains(output, "not a git repository")
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

// remoteFailure prefers hc's own error message, then the remote program's
// first stderr line, then the exit status.
func remoteFailure(stderr []byte, err error) string {
	if _, message := herdr.HCError(stderr); message != "" {
		return message
	}
	if line, _, _ := strings.Cut(strings.TrimSpace(string(stderr)), "\n"); line != "" {
		return line
	}
	// AppClient's error names the whole hc call, script included.
	if exitErr := (*exec.ExitError)(nil); errors.As(err, &exitErr) {
		return exitErr.Error()
	}
	return err.Error()
}

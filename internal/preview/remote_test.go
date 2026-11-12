package preview

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// scripted is remoteScript's output for a listing and git's output and exit
// status, both without the final newline that sh's $(...) strips.
func scripted(listing, git string, gitStatus int) []byte {
	return fmt.Appendf(nil, "%d %d\n%s%s", gitStatus, len(listing), listing, git)
}

func fakeMachines(reply func(ctx context.Context, r herdr.MachineExecRequest) ([]byte, []byte, error)) *herdr.FakeClient {
	return &herdr.FakeClient{MachineExecReply: func(ctx context.Context, _ string, r herdr.MachineExecRequest) ([]byte, []byte, error) {
		return reply(ctx, r)
	}}
}

func replying(stdout []byte) *herdr.FakeClient {
	return fakeMachines(func(context.Context, herdr.MachineExecRequest) ([]byte, []byte, error) { return stdout, nil, nil })
}

var remoteSpace = model.Session{Source: "herdr", Machine: "buntu26", Name: "api", Path: "/srv/api", WorkspaceID: "buntu26/sp1"}

const remoteSummaryText = "workspace: api\nmachine: buntu26\nid: buntu26/sp1\npath: /srv/api\n"

func TestRemotePreviewListsFolderAndGitStatusInOneCall(t *testing.T) {
	f := replying(scripted(".git/\nREADME.md\nsrc/", "## main...origin/main\n M README.md", 0))
	text, err := RenderWith(context.Background(), f, remoteSpace, "eza -la {}")
	require.NoError(t, err)
	assert.Equal(t, "/srv/api (buntu26)\n.git/\nREADME.md\nsrc/\n\n## main...origin/main\n M README.md\n", text)
	calls := f.MachineExecCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "buntu26", calls[0].Machine)
	assert.Equal(t, herdr.MachineExecRequest{Dir: "/srv/api", Timeout: remoteTimeout, Argv: []string{"sh", "-c", remoteScript}}, calls[0].Request)
	assert.Contains(t, remoteScript, "ls -A -p -1")
	assert.Contains(t, remoteScript, "LC_ALL=C git status --short --branch")
}

func TestRemotePreviewOmitsGitOutsideRepositoryOrWithoutGit(t *testing.T) {
	for name, git := range map[string]struct {
		output string
		status int
	}{
		"outside repository": {"fatal: not a git repository (or any of the parent directories): .git", 128},
		"git not installed":  {"sh: 1: git: not found", 127},
	} {
		t.Run(name, func(t *testing.T) {
			text, err := RenderWith(context.Background(), replying(scripted("notes.txt", git.output, git.status)), remoteSpace, "")
			require.NoError(t, err)
			assert.Equal(t, "/srv/api (buntu26)\nnotes.txt\n", text)
		})
	}
}

// These mean the status is unknown, not that there is none.
func TestRemotePreviewReportsOtherGitFailuresAfterListing(t *testing.T) {
	for name, tc := range map[string]struct {
		output string
		status int
		want   string
	}{
		"permission denied": {"sh: 1: git: Permission denied", 126, "sh: 1: git: Permission denied"},
		"other fatal error": {"fatal: detected dubious ownership in repository at '/srv/api'\nhint: run git config", 128, "fatal: detected dubious ownership in repository at '/srv/api'"},
		"no message":        {"", 1, "exit status 1"},
	} {
		t.Run(name, func(t *testing.T) {
			text, err := RenderWith(context.Background(), replying(scripted("notes.txt", tc.output, tc.status)), remoteSpace, "")
			require.NoError(t, err)
			assert.Equal(t, "/srv/api (buntu26)\nnotes.txt\n\ngit status unavailable: git on buntu26: "+tc.want+"\n", text)
		})
	}
}

func TestRemotePreviewFallsBackToMetadataWhenMachineFails(t *testing.T) {
	for name, tc := range map[string]struct {
		stderr string
		err    error
		want   string
	}{
		"hc message":       {`{"error":{"hint":"","kind":"spawn_failed","message":"cannot start folder /srv/api: No such file or directory (os error 2)"}}` + "\n", errors.New("exit status 1"), "cannot start folder /srv/api: No such file or directory (os error 2)"},
		"transport":        {`{"error":{"hint":"","kind":"machine_disconnected","message":"machine buntu26 is not connected"}}` + "\n", errors.New("exit status 4"), "machine buntu26 is not connected"},
		"ls stderr":        {"ls: .: Permission denied\n", errors.New("exit status 1"), "ls: .: Permission denied"},
		"exit status only": {"", fmt.Errorf("hc exec --on buntu26 -- sh -c script: %w", &exec.ExitError{ProcessState: exitedWith2(t)}), "exit status 2"},
	} {
		t.Run(name, func(t *testing.T) {
			f := fakeMachines(func(context.Context, herdr.MachineExecRequest) ([]byte, []byte, error) {
				return nil, []byte(tc.stderr), tc.err
			})
			text, err := RenderWith(context.Background(), f, remoteSpace, "")
			require.NoError(t, err)
			assert.Equal(t, remoteSummaryText+"remote preview unavailable: ls on buntu26: "+tc.want+"\n", text)
		})
	}
}

func exitedWith2(t *testing.T) *os.ProcessState {
	t.Helper()
	cmd := exec.Command("/bin/sh", "-c", "exit 2")
	require.Error(t, cmd.Run())
	return cmd.ProcessState
}

func TestRemotePreviewFallsBackToMetadataOnTimeout(t *testing.T) {
	f := fakeMachines(func(ctx context.Context, _ herdr.MachineExecRequest) ([]byte, []byte, error) {
		<-ctx.Done()
		return nil, []byte("[hc] timed out\n"), ctx.Err()
	})
	// A caller deadline shorter than remoteTimeout must not be reported as 3s.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	text, err := RenderWith(ctx, f, remoteSpace, "")
	require.NoError(t, err)
	assert.Equal(t, remoteSummaryText+"remote preview unavailable: ls on buntu26: preview deadline expired\n", text)
}

func TestRemotePreviewStopsAtItsOwnTimeout(t *testing.T) {
	f := fakeMachines(func(ctx context.Context, _ herdr.MachineExecRequest) ([]byte, []byte, error) {
		<-ctx.Done()
		return nil, nil, ctx.Err()
	})
	ctx, cancel := context.WithTimeout(context.Background(), remoteTimeout*2)
	defer cancel()
	text, err := RenderWith(ctx, f, remoteSpace, "")
	require.NoError(t, err)
	assert.Equal(t, remoteSummaryText+"remote preview unavailable: ls on buntu26: preview deadline expired\n", text)
	require.NoError(t, ctx.Err(), "preview outlived its own timeout")
}

func TestRemotePreviewStopsWhenCanceled(t *testing.T) {
	f := fakeMachines(func(ctx context.Context, _ herdr.MachineExecRequest) ([]byte, []byte, error) {
		<-ctx.Done()
		return nil, nil, ctx.Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	text, err := RenderWith(ctx, f, remoteSpace, "")
	require.NoError(t, err)
	assert.Contains(t, text, "remote preview unavailable: context canceled\n")
}

// localTools makes git, ls, sh, eza and hc leave a marker when run here. PATH
// holds only these fakes, so they write the marker with shell redirection.
func localTools(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	marker := filepath.Join(t.TempDir(), "ran-locally")
	for _, name := range []string{"git", "ls", "sh", "eza", "hc"} {
		//nolint:gosec // fake local tools must be executable for this test.
		require.NoError(t, os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n: > '"+marker+"'\n"), 0700))
	}
	t.Setenv("PATH", bin)
	return marker
}

func TestRemotePreviewNeverRunsLocalCommandsOrReadsLocalPaths(t *testing.T) {
	marker := localTools(t)
	s := remoteSpace
	s.Path = t.TempDir()
	text, err := RenderWith(context.Background(), replying(scripted("remote.txt", "", 0)), s, "eza -la {}")
	require.NoError(t, err)
	assert.Equal(t, s.Path+" (buntu26)\nremote.txt\n", text)
	assert.NoFileExists(t, marker)
}

func TestRemotePreviewReplacesConfiguredCommandsWithBuiltInListing(t *testing.T) {
	f := replying(scripted("ls output", "git output", 0))
	s := remoteSpace
	s.PreviewCommand = "exit 97"
	text, err := RenderWith(context.Background(), f, s, "exit 98")
	require.NoError(t, err)
	assert.Equal(t, "/srv/api (buntu26)\nls output\n\ngit output\n", text)
	for _, call := range f.MachineExecCalls() {
		assert.NotContains(t, strings.Join(call.Request.Argv, " "), "exit 9")
	}
}

func TestRemotePreviewWithoutPathShowsMetadataOnly(t *testing.T) {
	f := replying([]byte("unexpected\n"))
	text, err := RenderWith(context.Background(), f, model.Session{Source: "herdr", Machine: "ser8", Name: "~", WorkspaceID: "ser8/sp1"}, "")
	require.NoError(t, err)
	assert.Equal(t, "workspace: ~\nmachine: ser8\nid: ser8/sp1\npath: \n", text)
	assert.Empty(t, f.MachineExecCalls())
}

// A configured workspace that is not open yet keeps its unexpanded ~ path. hc
// does not expand ~, so ~ omits --cwd (the remote home) and ~/x passes x,
// which hc resolves against that home.
func TestRemotePreviewResolvesTildeAgainstRemoteHome(t *testing.T) {
	for path, dir := range map[string]string{"~": "", "~/": "", "~/src/api": "src/api"} {
		t.Run(path, func(t *testing.T) {
			f := replying(scripted("notes.txt", "fatal: not a git repository", 128))
			text, err := RenderWith(context.Background(), f, model.Session{Source: "config", Machine: "buntu26", Name: "api", Path: path}, "")
			require.NoError(t, err)
			assert.Equal(t, path+" (buntu26)\nnotes.txt\n", text)
			calls := f.MachineExecCalls()
			require.Len(t, calls, 1)
			assert.Equal(t, dir, calls[0].Request.Dir)
		})
	}
}

// hc would resolve ~deploy/app under the connecting user's home.
func TestRemotePreviewRefusesAnotherUsersHome(t *testing.T) {
	f := replying(scripted("notes.txt", "", 0))
	s := model.Session{Source: "config", Machine: "buntu26", Name: "api", Path: "~deploy/app"}
	text, err := RenderWith(context.Background(), f, s, "")
	require.NoError(t, err)
	assert.Equal(t, "workspace: api\nmachine: buntu26\nid: \npath: ~deploy/app\nremote preview unavailable: path \"~deploy/app\" names another user's home; use an absolute path\n", text)
	assert.Empty(t, f.MachineExecCalls())
}

func TestRemotePreviewWithoutMachinesRunsNothing(t *testing.T) {
	want := remoteSummaryText + "remote previews need the Herdr app (herdr-app)\n"
	text, err := RenderWith(context.Background(), nil, remoteSpace, "eza -la {}")
	require.NoError(t, err)
	assert.Equal(t, want, text)

	// Render picks the backend itself; the Herdr TUI backend has no machines.
	marker := localTools(t)
	t.Setenv("HERDR_SESH_BACKEND", "herdr")
	text, err = Render(context.Background(), remoteSpace, "eza -la {}")
	require.NoError(t, err)
	assert.Equal(t, want, text)
	assert.NoFileExists(t, marker)
}

// runHere runs the remote argv on this machine, in r.Dir, as hc would there.
func runHere(path string) *herdr.FakeClient {
	return fakeMachines(func(ctx context.Context, r herdr.MachineExecRequest) ([]byte, []byte, error) {
		//nolint:gosec // the argv is remoteScript under test.
		cmd := exec.CommandContext(ctx, r.Argv[0], r.Argv[1:]...)
		cmd.Dir = r.Dir
		cmd.Env = append(os.Environ(), "PATH="+path, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
		var stdout, stderr strings.Builder
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		return []byte(stdout.String()), []byte(stderr.String()), err
	})
}

// The script itself runs under sh: a header line made to look like the
// script's own, a newline in a name and an empty folder all split correctly.
func TestRemoteScriptSeparatesListingFromGitStatus(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	path := os.Getenv("PATH")
	repo := t.TempDir()
	gitInit := exec.Command("git", "init", "-q", "-b", "main")
	gitInit.Dir = repo
	gitInit.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	require.NoError(t, gitInit.Run())
	require.NoError(t, os.WriteFile(filepath.Join(repo, "0 99"), nil, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "two\nlines"), nil, 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(repo, "src"), 0o700))
	s := remoteSpace
	s.Path = repo
	text, err := RenderWith(context.Background(), runHere(path), s, "")
	require.NoError(t, err)
	assert.Equal(t, repo+" (buntu26)\n.git/\n0 99\nsrc/\ntwo\nlines\n\n## No commits yet on main\n?? \"0 99\"\n?? \"two\\nlines\"\n", text)

	s.Path = t.TempDir()
	text, err = RenderWith(context.Background(), runHere(path), s, "")
	require.NoError(t, err)
	assert.Equal(t, s.Path+" (buntu26)\n", text, "an empty folder outside a repository shows the listing only")

	// Without git on PATH, sh reports status 127 and only the listing shows.
	bin := t.TempDir()
	for _, tool := range []string{"ls", "wc"} {
		found, err := exec.LookPath(tool)
		require.NoError(t, err)
		require.NoError(t, os.Symlink(found, filepath.Join(bin, tool)))
	}
	require.NoError(t, os.WriteFile(filepath.Join(s.Path, "notes.txt"), nil, 0o600))
	text, err = RenderWith(context.Background(), runHere(bin), s, "")
	require.NoError(t, err)
	assert.Equal(t, s.Path+" (buntu26)\nnotes.txt\n", text)
}

package preview

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRemote struct {
	mu    sync.Mutex
	calls [][]string
	run   func(ctx context.Context, argv []string) ([]byte, []byte, error)
}

func (f *fakeRemote) Run(ctx context.Context, bin string, args ...string) ([]byte, []byte, error) {
	f.mu.Lock()
	f.calls = append(f.calls, append([]string{bin}, args...))
	f.mu.Unlock()
	for i, arg := range args {
		if arg == "--" {
			return f.run(ctx, args[i+1:])
		}
	}
	return nil, nil, errors.New("no -- before argv")
}

func useRemote(t *testing.T, run func(ctx context.Context, argv []string) ([]byte, []byte, error)) *fakeRemote {
	t.Helper()
	f := &fakeRemote{run: run}
	previous := remoteRunner
	remoteRunner = f
	t.Cleanup(func() { remoteRunner = previous })
	return f
}

var remoteSpace = model.Session{Source: "herdr", Machine: "buntu26", Name: "api", Path: "/srv/api", WorkspaceID: "buntu26/sp1"}

func TestRemotePreviewListsFolderAndGitStatusOnMachine(t *testing.T) {
	f := useRemote(t, func(_ context.Context, argv []string) ([]byte, []byte, error) {
		switch argv[0] {
		case "ls":
			return []byte(".git/\nREADME.md\nsrc/\n"), nil, nil
		case "git":
			return []byte("## main...origin/main\n M README.md\n"), nil, nil
		}
		return nil, nil, errors.New("unexpected")
	})
	text, err := Render(context.Background(), remoteSpace, "eza -la {}")
	require.NoError(t, err)
	assert.Equal(t, "/srv/api (buntu26)\n.git/\nREADME.md\nsrc/\n\n## main...origin/main\n M README.md\n", text)
	require.Len(t, f.calls, 2)
	assert.Equal(t, []string{"hc", "exec", "--on", "buntu26", "--cwd", "/srv/api", "--timeout", "3", "--", "ls", "-A", "-p", "-1"}, f.calls[0])
	assert.Equal(t, []string{"hc", "exec", "--on", "buntu26", "--cwd", "/srv/api", "--env", "LC_ALL=C", "--timeout", "3", "--", "git", "status", "--short", "--branch"}, f.calls[1])
}

func TestRemotePreviewOmitsGitOutsideRepositoryOrWithoutGit(t *testing.T) {
	for name, stderr := range map[string]string{
		"outside repository": "fatal: not a git repository (or any of the parent directories): .git\n",
		"git not installed":  `{"error":{"hint":"","kind":"spawn_failed","message":"cannot start git: No such file or directory (os error 2)"}}` + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			useRemote(t, func(_ context.Context, argv []string) ([]byte, []byte, error) {
				if argv[0] == "git" {
					return nil, []byte(stderr), errors.New("exit status 1")
				}
				return []byte("notes.txt\n"), nil, nil
			})
			text, err := Render(context.Background(), remoteSpace, "")
			require.NoError(t, err)
			assert.Equal(t, "/srv/api (buntu26)\nnotes.txt\n", text)
		})
	}
}

// hc reports these with the same spawn_failed kind as a missing git program;
// they mean the status is unknown, not that there is none.
func TestRemotePreviewReportsGitSpawnFailuresOtherThanMissingGit(t *testing.T) {
	for name, message := range map[string]string{
		"folder vanished after ls": "cannot start folder /srv/api: No such file or directory (os error 2)",
		"permission denied":        "cannot start git: Permission denied (os error 13)",
	} {
		t.Run(name, func(t *testing.T) {
			useRemote(t, func(_ context.Context, argv []string) ([]byte, []byte, error) {
				if argv[0] == "git" {
					return nil, []byte(`{"error":{"hint":"","kind":"spawn_failed","message":"` + message + `"}}` + "\n"), errors.New("exit status 1")
				}
				return []byte("notes.txt\n"), nil, nil
			})
			text, err := Render(context.Background(), remoteSpace, "")
			require.NoError(t, err)
			assert.Equal(t, "/srv/api (buntu26)\nnotes.txt\n\ngit status unavailable: git on buntu26: "+message+"\n", text)
		})
	}
}

func TestRemotePreviewReportsGitTimeoutAfterListing(t *testing.T) {
	useRemote(t, func(ctx context.Context, argv []string) ([]byte, []byte, error) {
		if argv[0] == "git" {
			<-ctx.Done()
			return nil, []byte("[hc] timed out\n"), ctx.Err()
		}
		return []byte("notes.txt\n"), nil, nil
	})
	// A caller deadline shorter than remoteTimeout must not be reported as 3s.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	text, err := Render(ctx, remoteSpace, "")
	require.NoError(t, err)
	assert.Equal(t, "/srv/api (buntu26)\nnotes.txt\n\ngit status unavailable: git on buntu26: preview deadline expired\n", text)
}

func TestRemotePreviewReportsGitTransportFailureAfterListing(t *testing.T) {
	useRemote(t, func(_ context.Context, argv []string) ([]byte, []byte, error) {
		if argv[0] == "git" {
			return nil, []byte(`{"error":{"hint":"","kind":"machine_disconnected","message":"machine buntu26 is not connected"}}` + "\n"), errors.New("exit status 4")
		}
		return []byte("notes.txt\n"), nil, nil
	})
	text, err := Render(context.Background(), remoteSpace, "")
	require.NoError(t, err)
	assert.Equal(t, "/srv/api (buntu26)\nnotes.txt\n\ngit status unavailable: git on buntu26: machine buntu26 is not connected\n", text)
}

func TestRemotePreviewFallsBackToMetadataWhenMachineFails(t *testing.T) {
	useRemote(t, func(context.Context, []string) ([]byte, []byte, error) {
		return nil, []byte(`{"error":{"hint":"","kind":"spawn_failed","message":"cannot start folder /srv/api: No such file or directory (os error 2)"}}` + "\n"), errors.New("exit status 1")
	})
	text, err := Render(context.Background(), remoteSpace, "")
	require.NoError(t, err)
	assert.Equal(t, "workspace: api\nmachine: buntu26\nid: buntu26/sp1\npath: /srv/api\nremote preview unavailable: ls on buntu26: cannot start folder /srv/api: No such file or directory (os error 2)\n", text)
}

func TestRemotePreviewFallsBackToMetadataOnTimeout(t *testing.T) {
	useRemote(t, func(ctx context.Context, _ []string) ([]byte, []byte, error) {
		<-ctx.Done()
		return nil, nil, ctx.Err()
	})
	ctx, cancel := context.WithTimeout(context.Background(), remoteTimeout*2)
	defer cancel()
	text, err := Render(ctx, remoteSpace, "")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(text, "workspace: api\nmachine: buntu26\n"), text)
	assert.Contains(t, text, "remote preview unavailable: ls on buntu26: preview deadline expired\n")
	require.NoError(t, ctx.Err(), "preview outlived its own timeout")
}

func TestRemotePreviewStopsWhenCanceled(t *testing.T) {
	useRemote(t, func(ctx context.Context, _ []string) ([]byte, []byte, error) {
		<-ctx.Done()
		return nil, nil, ctx.Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	text, err := Render(ctx, remoteSpace, "")
	require.NoError(t, err)
	assert.Contains(t, text, "remote preview unavailable: context canceled\n")
}

func TestRemotePreviewNeverRunsLocalCommandsOrReadsLocalPaths(t *testing.T) {
	// Local git, ls and sh would leave a marker; the remote path must reach
	// the machine only through hc. PATH holds only these fakes, so they write
	// the marker with shell redirection instead of an external program.
	bin := t.TempDir()
	marker := filepath.Join(t.TempDir(), "ran-locally")
	for _, name := range []string{"git", "ls", "sh", "eza"} {
		//nolint:gosec // fake local tools must be executable for this test.
		require.NoError(t, os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n: > '"+marker+"'\n"), 0700))
	}
	t.Setenv("PATH", bin)
	f := useRemote(t, func(_ context.Context, argv []string) ([]byte, []byte, error) {
		return []byte(argv[0] + " output\n"), nil, nil
	})
	s := remoteSpace
	s.Path = t.TempDir()
	text, err := Render(context.Background(), s, "eza -la {}")
	require.NoError(t, err)
	assert.Contains(t, text, "ls output\n")
	for _, call := range f.calls {
		assert.Equal(t, "hc", call[0])
	}
	assert.NoFileExists(t, marker)
}

func TestRemotePreviewReplacesConfiguredCommandsWithBuiltInListing(t *testing.T) {
	f := useRemote(t, func(_ context.Context, argv []string) ([]byte, []byte, error) {
		return []byte(argv[0] + " output\n"), nil, nil
	})
	s := remoteSpace
	s.PreviewCommand = "exit 97"
	text, err := Render(context.Background(), s, "exit 98")
	require.NoError(t, err)
	assert.Equal(t, "/srv/api (buntu26)\nls output\n\ngit output\n", text)
	for _, call := range f.calls {
		assert.NotContains(t, call, "exit 97")
		assert.NotContains(t, call, "exit 98")
	}
}

func TestRemotePreviewWithoutPathShowsMetadataOnly(t *testing.T) {
	f := useRemote(t, func(context.Context, []string) ([]byte, []byte, error) {
		return []byte("unexpected\n"), nil, nil
	})
	text, err := Render(context.Background(), model.Session{Source: "herdr", Machine: "ser8", Name: "~", WorkspaceID: "ser8/sp1"}, "")
	require.NoError(t, err)
	assert.Equal(t, "workspace: ~\nmachine: ser8\nid: ser8/sp1\npath: \n", text)
	assert.Empty(t, f.calls)
}

// A configured workspace that is not open yet keeps its unexpanded ~ path. hc
// does not expand ~, so ~ omits --cwd (the remote home) and ~/x passes x,
// which hc resolves against that home.
func TestRemotePreviewResolvesTildeAgainstRemoteHome(t *testing.T) {
	for path, cwd := range map[string][]string{
		"~":         nil,
		"~/":        nil,
		"~/src/api": {"--cwd", "src/api"},
	} {
		t.Run(path, func(t *testing.T) {
			f := useRemote(t, func(_ context.Context, argv []string) ([]byte, []byte, error) {
				if argv[0] == "git" {
					return nil, []byte("fatal: not a git repository\n"), errors.New("exit status 128")
				}
				return []byte("notes.txt\n"), nil, nil
			})
			text, err := Render(context.Background(), model.Session{Source: "config", Machine: "buntu26", Name: "api", Path: path}, "")
			require.NoError(t, err)
			assert.Equal(t, path+" (buntu26)\nnotes.txt\n", text)
			require.Len(t, f.calls, 2)
			want := append(append([]string{"hc", "exec", "--on", "buntu26"}, cwd...), "--timeout", "3", "--", "ls", "-A", "-p", "-1")
			assert.Equal(t, want, f.calls[0])
		})
	}
}

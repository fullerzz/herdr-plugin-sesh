package connect

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fullerzz/herdr-plugin-sesh/internal/config"
	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/fullerzz/herdr-plugin-sesh/internal/sources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnectRemoteQualifiedIDOnlyFocusesExistingWorkspace(t *testing.T) {
	remote := model.Session{Source: "herdr", Machine: "buntu26", Name: "api", Path: "/src/api", WorkspaceID: "buntu26/sp1", StartupCommand: "must-not-run", WindowNames: []string{"dev"}}
	client := &herdr.FakeClient{}
	result, err := Connect(context.Background(), client, []model.Session{
		{Source: "config", Name: remote.WorkspaceID},
		{Source: "herdr", Name: "api", Path: remote.Path, WorkspaceID: "sp1"}, remote,
	}, remote.WorkspaceID, Options{Namer: func(context.Context, string) string {
		t.Fatal("remote path passed to local namer")
		return ""
	}})
	require.NoError(t, err)
	assert.Equal(t, remote, result.Session)
	assert.False(t, result.Created)
	assert.Equal(t, []string{remote.WorkspaceID}, client.FocusedWorkspaces)
	assert.Empty(t, client.CreatedWorkspaces)
	assert.Empty(t, client.CreatedTabs)
	assert.Empty(t, client.PaneRuns)

	remote.WorkspaceID = ""
	_, err = Connect(context.Background(), client, []model.Session{remote}, remote.Name, Options{})
	require.ErrorContains(t, err, "existing workspace ID")
	assert.Empty(t, client.CreatedWorkspaces)
}

func TestResolveNeverMatchesRemoteByNameOrPath(t *testing.T) {
	remote := model.Session{Source: "herdr", Machine: "buntu26", Name: "api", Path: "/src/api", WorkspaceID: "buntu26/sp1"}
	config := model.Session{Source: "config", Name: "api", Path: "/src/api"}

	// A remote space listed first must not shadow a local namesake.
	got, ok := Resolve([]model.Session{remote, config}, "api")
	require.True(t, ok)
	assert.Equal(t, config, got)
	got, ok = Resolve([]model.Session{remote, config}, "/src/api")
	require.True(t, ok)
	assert.Equal(t, config, got)

	_, ok = Resolve([]model.Session{remote}, "/src/api")
	assert.False(t, ok, "remote-only path match")
	_, ok = Resolve([]model.Session{remote}, "api")
	assert.False(t, ok, "remote-only name match")

	got, ok = Resolve([]model.Session{config, remote}, remote.WorkspaceID)
	require.True(t, ok)
	assert.Equal(t, remote, got)
}

func TestConnectLocalDirectoryIgnoresRemoteWithSamePath(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // Keep the created path out of the user's zoxide database.
	dir := t.TempDir()
	remote := model.Session{Source: "herdr", Machine: "buntu26", Name: "api", Path: dir, WorkspaceID: "buntu26/sp1"}
	client := &herdr.FakeClient{}
	result, err := Connect(context.Background(), client, []model.Session{remote}, dir, Options{NoFocus: true})
	require.NoError(t, err)
	assert.True(t, result.Created)
	assert.Empty(t, result.Session.Machine)
	assert.Equal(t, dir, result.Session.Path)
	assert.Empty(t, client.FocusedWorkspaces)
	require.Len(t, client.CreatedWorkspaces, 1)
	assert.Equal(t, dir, client.CreatedWorkspaces[0].CWD)
}

func TestConnectRemoteNameSuggestsQualifiedID(t *testing.T) {
	remote := model.Session{Machine: "buntu26", Name: "api", Path: "/remote/api", WorkspaceID: "buntu26/sp1"}
	client := &herdr.FakeClient{}
	for _, target := range []string{remote.Name, remote.Path} {
		_, err := Connect(t.Context(), client, []model.Session{
			remote, {Name: "other", Path: "/other"},
		}, target, Options{})
		require.ErrorContains(t, err, `connect with its workspace ID "buntu26/sp1"`)
	}
	assert.Empty(t, client.CreatedWorkspaces)
	assert.Empty(t, client.FocusedWorkspaces)
}

// zoxideSpy puts a zoxide on PATH that records each call in the returned file.
func zoxideSpy(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "zoxide.log")
	//nolint:gosec // the test's fake zoxide must be executable.
	require.NoError(t, os.WriteFile(filepath.Join(bin, "zoxide"), []byte("#!/bin/sh\necho \"$@\" >> '"+log+"'\n"), 0o755))
	t.Setenv("PATH", bin)
	return log
}

func TestConnectCreatesConfiguredRemoteWorkspaceOnItsMachine(t *testing.T) {
	zoxideLog := zoxideSpy(t)
	// The remote path also exists locally: neither way in may look at it.
	t.Chdir(t.TempDir())
	require.NoError(t, os.MkdirAll(filepath.Join("~", "src", "api"), 0o750))
	remote := model.Session{Source: "config", Machine: "buntu26", Name: "api", Path: "~/src/api", WindowConfigs: []model.WindowConfig{{
		Name: "dev", Path: "~/src/api",
		Panes: []model.PaneConfig{
			{Name: "server", Path: "~/src/api", Env: map[string]string{"PORT": "8080"}, Startup: "make serve", WaitFor: &model.PaneWait{Match: "listening", Timeout: time.Second}},
			{Name: "logs", SplitFrom: "server", Split: "down", Path: "/var/log", Env: map[string]string{"LEVEL": "debug"}, Startup: "tail -f app.log"},
		},
	}}}
	sharedPanes := remote.WindowConfigs[0].Panes
	client := &herdr.FakeClient{MachineHomes: map[string]string{"buntu26": "/home/zach"}}
	opts := Options{Namer: func(context.Context, string) string { t.Fatal("remote path passed to local namer"); return "" }}
	entries := map[string]func() (Result, error){
		"connect by name": func() (Result, error) {
			return Connect(context.Background(), client, []model.Session{remote}, "api", opts)
		},
		"picker selection": func() (Result, error) { return Open(context.Background(), client, remote, opts) },
	}
	for name, open := range entries {
		client.CreatedWorkspaces = nil
		client.CreatedTabs, client.Splits, client.Waits, client.PaneRuns = nil, nil, nil, nil
		require.DirExists(t, remote.Path)
		result, err := open()
		require.NoError(t, err, name)
		assert.True(t, result.Created)
		assert.Equal(t, "buntu26/new-workspace", result.Session.WorkspaceID)
		require.Len(t, client.CreatedWorkspaces, 1)
		req := client.CreatedWorkspaces[0]
		assert.Equal(t, "buntu26", req.Machine)
		assert.Equal(t, "/home/zach/src/api", req.CWD)
		// A root startup command keeps the initial tab, so the layout gets its own.
		require.Len(t, client.CreatedTabs, 1)
		tab := client.CreatedTabs[0]
		assert.Equal(t, "buntu26/new-workspace", tab.WorkspaceID)
		assert.Equal(t, "/home/zach/src/api", tab.CWD)
		assert.Equal(t, map[string]string{"PORT": "8080"}, tab.Env)
		require.Len(t, client.Splits, 1)
		assert.Equal(t, herdr.PaneSplitRequest{PaneID: "new-pane", Direction: "down", CWD: "/var/log", Env: map[string]string{"LEVEL": "debug"}}, client.Splits[0])
		assert.Equal(t, []string{"new-pane:make serve", "split-1:tail -f app.log"}, client.PaneRuns)
		assert.Equal(t, []string{"new-pane:listening"}, client.Waits)
	}
	assert.Equal(t, []string{"buntu26", "buntu26"}, client.HomeQueries)
	assert.Equal(t, "~/src/api", sharedPanes[0].Path, "candidates keep their unexpanded paths")
	assert.NoFileExists(t, zoxideLog, "remote paths never reach zoxide")
}

// A remote workspace's path is a path on its machine, so a target never
// matches it: the same path can be a directory here.
func TestConnectTargetPathNeverOpensConfiguredRemoteWorkspace(t *testing.T) {
	zoxideSpy(t)
	local := t.TempDir()
	remote := model.Session{Source: "config", Machine: "buntu26", Name: "rdocs", Path: local}
	open := model.Session{Source: "herdr", Machine: "buntu26", Name: "rdocs", Path: local, WorkspaceID: "buntu26/sp1", Configured: true}
	for name, candidate := range map[string]model.Session{"unopened": remote, "open": open} {
		t.Run(name+" local directory wins", func(t *testing.T) {
			client := &herdr.FakeClient{}
			result, err := Connect(t.Context(), client, []model.Session{candidate}, local, Options{NoFocus: true})
			require.NoError(t, err)
			require.Len(t, client.CreatedWorkspaces, 1)
			assert.Empty(t, client.CreatedWorkspaces[0].Machine)
			assert.Equal(t, local, client.CreatedWorkspaces[0].CWD)
			assert.Empty(t, result.Session.Machine)
		})
	}
	t.Run("remote-only path names the workspace instead", func(t *testing.T) {
		client := &herdr.FakeClient{}
		remote.Path = "/srv/remote-only"
		_, err := Connect(t.Context(), client, []model.Session{remote}, remote.Path, Options{NoFocus: true})
		require.EqualError(t, err, `"/srv/remote-only" is the path of workspace "rdocs" on machine "buntu26"; connect with its name`)
		assert.Empty(t, client.CreatedWorkspaces)
	})
	t.Run("local directory called like the workspace wins", func(t *testing.T) {
		t.Chdir(t.TempDir())
		require.NoError(t, os.Mkdir("rdocs", 0o750))
		client := &herdr.FakeClient{}
		_, err := Connect(t.Context(), client, []model.Session{remote}, "rdocs", Options{NoFocus: true})
		require.NoError(t, err)
		require.Len(t, client.CreatedWorkspaces, 1)
		assert.Empty(t, client.CreatedWorkspaces[0].Machine)
	})
}

// Relative remote paths start at the machine's home, as herdr-app's folder
// does, and reach every create as absolute paths: a split's shell enters its
// path from wherever the pane it splits is.
func TestConnectRemoteRelativePathsStartAtTheirMachinesHome(t *testing.T) {
	zoxideSpy(t)
	cfg := config.Config{
		SessionConfigs: []config.SessionConfig{{Name: "api", Machine: "buntu26", Path: "src/api", Windows: []string{"dev"}}},
		WindowConfigs: []model.WindowConfig{{Name: "dev", Panes: []model.PaneConfig{
			{Name: "root", Env: map[string]string{"PORT": "8080"}}, {Name: "web", SplitFrom: "root", Path: "web"},
		}}},
	}
	got, err := sources.ConfigSessions{Config: cfg, Home: "/Users/me"}.List(t.Context())
	require.NoError(t, err)
	client := &herdr.FakeClient{MachineHomes: map[string]string{"buntu26": "/home/zach"}}
	_, err = Connect(t.Context(), client, got.Ordered(), "api", Options{NoFocus: true})
	require.NoError(t, err)
	require.Len(t, client.CreatedWorkspaces, 1)
	assert.Equal(t, "/home/zach/src/api", client.CreatedWorkspaces[0].CWD)
	require.Len(t, client.Splits, 1)
	assert.Equal(t, "/home/zach/src/api/web", client.Splits[0].CWD)
}

// A relative tab or pane path may climb above the remote home; the home is
// joined before the path is cleaned.
func TestConnectRemoteRelativePathsClimbFromTheirMachinesHome(t *testing.T) {
	zoxideSpy(t)
	cfg := config.Config{
		SessionConfigs: []config.SessionConfig{{Name: "api", Machine: "buntu26", Path: "~/src", Windows: []string{"dev"}}},
		WindowConfigs: []model.WindowConfig{{Name: "dev", Path: "../..", Panes: []model.PaneConfig{
			{Name: "root"}, {Name: "home", SplitFrom: "root", Path: "zach"},
		}}},
	}
	got, err := sources.ConfigSessions{Config: cfg, Home: "/Users/me"}.List(t.Context())
	require.NoError(t, err)
	client := &herdr.FakeClient{MachineHomes: map[string]string{"buntu26": "/home/zach"}}
	_, err = Connect(t.Context(), client, got.Ordered(), "api", Options{NoFocus: true})
	require.NoError(t, err)
	require.Len(t, client.CreatedWorkspaces, 1)
	assert.Equal(t, "/home/zach/src", client.CreatedWorkspaces[0].CWD)
	require.Len(t, client.CreatedTabs, 1)
	assert.Equal(t, "/home", client.CreatedTabs[0].CWD)
	require.Len(t, client.Splits, 1)
	assert.Equal(t, "/home/zach", client.Splits[0].CWD)
}

func TestConnectRemoteWithAbsolutePathsSkipsHomeLookup(t *testing.T) {
	zoxideSpy(t)
	client := &herdr.FakeClient{}
	_, err := Connect(t.Context(), client, []model.Session{{Source: "config", Machine: "buntu26", Name: "api", Path: "/srv/api"}}, "api", Options{NoFocus: true})
	require.NoError(t, err)
	assert.Empty(t, client.HomeQueries)
	require.Len(t, client.CreatedWorkspaces, 1)
	assert.Equal(t, herdr.WorkspaceCreateRequest{Machine: "buntu26", CWD: "/srv/api", Label: "api"}, client.CreatedWorkspaces[0])
}

func TestConnectRemoteHomeFailureCreatesNothing(t *testing.T) {
	client := &herdr.FakeClient{}
	_, err := Connect(t.Context(), client, []model.Session{{Source: "config", Machine: "nosuchbox", Name: "api", Path: "~/api"}}, "api", Options{})
	require.ErrorContains(t, err, `unknown machine "nosuchbox"`)
	assert.Empty(t, client.CreatedWorkspaces)
}

func TestConnectRemoteConfigNeverShadowsLocalNamesake(t *testing.T) {
	zoxideLog := zoxideSpy(t)
	remote := model.Session{Source: "config", Machine: "buntu26", Name: "api", Path: "/srv/api"}
	local := model.Session{Source: "zoxide", Name: "api", Path: "/src/api"}
	client := &herdr.FakeClient{}
	result, err := Connect(t.Context(), client, []model.Session{remote, local}, "api", Options{NoFocus: true})
	require.NoError(t, err)
	assert.Empty(t, result.Session.Machine)
	require.Len(t, client.CreatedWorkspaces, 1)
	assert.Empty(t, client.CreatedWorkspaces[0].Machine)
	assert.Equal(t, "/src/api", client.CreatedWorkspaces[0].CWD)
	assert.FileExists(t, zoxideLog, "local creates still reach zoxide")
}

func TestConnectByNameFocusesOpenConfiguredRemoteSpace(t *testing.T) {
	open := model.Session{Source: "herdr", Machine: "buntu26", Name: "api", Path: "/srv/api", WorkspaceID: "buntu26/sp1", Configured: true}
	client := &herdr.FakeClient{}
	result, err := Connect(t.Context(), client, []model.Session{open}, "api", Options{})
	require.NoError(t, err)
	assert.False(t, result.Created)
	assert.Equal(t, []string{"buntu26/sp1"}, client.FocusedWorkspaces)
	assert.Empty(t, client.CreatedWorkspaces)
}

func TestConnectConfiguredRemoteNeedsMachineBackend(t *testing.T) {
	// Embedding only the Client interface hides FakeClient's MachineHome, as
	// the Herdr TUI backend has none.
	fake := &herdr.FakeClient{}
	client := struct{ herdr.Client }{fake}
	_, err := Connect(t.Context(), client, []model.Session{{Source: "config", Machine: "buntu26", Name: "api", Path: "/srv/api"}}, "api", Options{})
	require.ErrorIs(t, err, herdr.ErrNoMachines)
	require.ErrorContains(t, err, `workspace "api" on machine "buntu26"`)
	assert.Empty(t, fake.CreatedWorkspaces)
}

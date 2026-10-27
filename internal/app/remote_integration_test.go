package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fullerzz/herdr-plugin-sesh/internal/config"
	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/fullerzz/herdr-plugin-sesh/internal/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMain keeps tests from reading the developer's real remote.toml.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "herdr-sesh-remote-")
	if err == nil {
		path := filepath.Join(dir, "remote.toml")
		if err = os.WriteFile(path, []byte("enabled = false\n"), 0600); err == nil {
			err = os.Setenv("HERDR_SESH_REMOTE_CONFIG", path)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// fakeDaemon serves catalog to `accepts` connections on a short socket path,
// which macOS needs for Unix socket addresses, and enables remote.toml for it.
func fakeDaemon(t *testing.T, accepts int, catalog []remote.Machine) (<-chan remote.Request, <-chan error) {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "sesh-ipc-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "daemon.sock")
	listener, err := net.Listen("unix", socket)
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	configPath := filepath.Join(dir, "remote.toml")
	require.NoError(t, os.WriteFile(configPath, fmt.Appendf(nil, "enabled=true\nid='local'\nsocket=%q\n", socket), 0600))
	t.Setenv("HERDR_SESH_REMOTE_CONFIG", configPath)
	requests := make(chan remote.Request, accepts)
	done := make(chan error, 1)
	go func() {
		for range accepts {
			conn, e := listener.Accept()
			if e != nil {
				done <- e
				return
			}
			var request remote.Request
			e = json.NewDecoder(conn).Decode(&request)
			if e == nil {
				requests <- request
				e = json.NewEncoder(conn).Encode(remote.Response{Catalog: catalog})
			}
			_ = conn.Close()
			if e != nil {
				done <- e
				return
			}
		}
		done <- nil
	}()
	return requests, done
}

func TestRemoteCatalogDisabledAndUnavailable(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "remote.toml")
	t.Setenv("HERDR_SESH_REMOTE_CONFIG", configPath)
	require.NoError(t, os.WriteFile(configPath, []byte("enabled = false\n"), 0600))
	sessions, err := remoteSessions(context.Background())
	require.NoError(t, err)
	assert.Empty(t, sessions)
	socket := filepath.Join(dir, "missing.sock")
	require.NoError(t, os.WriteFile(configPath, fmt.Appendf(nil, "enabled=true\nid='local'\nsocket=%q\n", socket), 0600))
	sessions, err = remoteSessions(context.Background())
	require.ErrorContains(t, err, "remote catalog unavailable")
	assert.Empty(t, sessions)
}

func TestRemoteCatalogSnapshotAndSwitch(t *testing.T) {
	requests, done := fakeDaemon(t, 2, []remote.Machine{{ID: "peer", Session: "main", Stale: true, Workspaces: []remote.Workspace{{ID: "w1", Label: "same", CWD: "/remote"}}}})
	a := &App{Err: &bytes.Buffer{}}
	sessions, err := remoteSessions(context.Background())
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	s := sessions[0]
	assert.Equal(t, model.Session{Source: model.SourceRemote, Name: "same", Remote: &model.RemoteRef{Machine: "peer", Session: "main", WorkspaceID: "w1", CWD: "/remote", Offline: true}}, s)
	require.ErrorContains(t, a.switchRemote(context.Background(), s), "offline")
	s.Remote.Offline = false
	require.NoError(t, a.switchRemote(context.Background(), s))
	require.NoError(t, <-done)
	assert.Equal(t, "list", (<-requests).Op)
	request := <-requests
	assert.Equal(t, "switch", request.Op)
	assert.Equal(t, "peer", request.MachineID)
	assert.Equal(t, "w1", request.WorkspaceID)
}

func TestRemoteRowsSkipLocalSessionConfig(t *testing.T) {
	configureFakeSources(t, "")
	_, done := fakeDaemon(t, 1, []remote.Machine{{ID: "peer", Session: "main", Workspaces: []remote.Workspace{{ID: "w1", Label: "work", CWD: "/remote"}}}})
	cfg := config.Default()
	cfg.DefaultSessionConfig.StartupCommand = "local startup"
	cfg.DefaultSessionConfig.PreviewCommand = "local preview"
	a := &App{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}
	sessions, err := a.collectAllowUnavailableHerdr(context.Background(), cfg, "")
	require.NoError(t, err)
	require.NoError(t, <-done)
	require.NotEmpty(t, sessions)
	got := sessions[len(sessions)-1]
	require.NotNil(t, got.Remote)
	assert.Empty(t, got.StartupCommand)
	assert.Empty(t, got.PreviewCommand)
}

func TestPickerCollectionRoutesRemoteWarningsToCaller(t *testing.T) {
	configureFakeSources(t, "")
	configPath := filepath.Join(t.TempDir(), "remote.toml")
	require.NoError(t, os.WriteFile(configPath, fmt.Appendf(nil, "enabled=true\nid='local'\nsocket=%q\n", filepath.Join(t.TempDir(), "missing.sock")), 0600))
	t.Setenv("HERDR_SESH_REMOTE_CONFIG", configPath)
	var stderr bytes.Buffer
	var warnings []string
	a := &App{Err: &stderr}
	_, err := a.collectPickerWithClient(context.Background(), config.Default(), nil, herdr.NewCLIClient(), func(format string, args ...any) {
		warnings = append(warnings, fmt.Sprintf(format, args...))
	})
	require.NoError(t, err)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "remote catalog unavailable")
	assert.Empty(t, stderr.String(), "remote warnings must not write over the picker's alt-screen")
}

func TestListCachesLocalRowsAndFetchesRemoteRows(t *testing.T) {
	d := t.TempDir()
	configureFakeSources(t, "/first\n")
	_, done := fakeDaemon(t, 2, []remote.Machine{{ID: "peer", Session: "main", Workspaces: []remote.Workspace{{ID: "w1", Label: "work", CWD: "/remote"}}}})
	cfgPath := filepath.Join(d, "sesh.toml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("cache = true\n"), 0600))
	t.Setenv("HERDR_PLUGIN_STATE_DIR", filepath.Join(d, "state"))
	var out bytes.Buffer
	a := &App{Out: &out, Err: &bytes.Buffer{}}
	require.NoError(t, a.list(context.Background(), []string{"--config", cfgPath}))
	first := out.String()
	assert.Contains(t, first, "/first")
	assert.Contains(t, first, "work @ peer")
	t.Setenv("FAKE_ZOXIDE_OUTPUT", "/second\n")
	out.Reset()
	require.NoError(t, a.list(context.Background(), []string{"--config", cfgPath}))
	assert.Equal(t, first, out.String(), "second list must reuse cached local rows")
	assert.Equal(t, 1, strings.Count(out.String(), "work @ peer"))
	require.NoError(t, <-done, "remote rows must be fetched on every list")
}

func TestRemoteListPrintsQualifiedTarget(t *testing.T) {
	remoteSession := model.Session{Source: model.SourceRemote, Name: "work", Remote: &model.RemoteRef{Machine: "peer", Session: "main", WorkspaceID: "w1", CWD: "/remote", Offline: true}}
	var output bytes.Buffer
	a := &App{Out: &output}
	require.NoError(t, a.printSessions([]model.Session{remoteSession, {Source: "herdr", Name: "local", Path: "/local"}}, false))
	assert.Equal(t, "remote\twork @ peer (offline)\t/remote\t"+model.Key(remoteSession)+"\nherdr\tlocal\t/local\n", output.String())
}

func TestUnavailableRemotePreviewNeverExecutesLocalCommand(t *testing.T) {
	configureFakeSources(t, "")
	dir := t.TempDir()
	remoteConfig := filepath.Join(dir, "remote.toml")
	require.NoError(t, os.WriteFile(remoteConfig, []byte("enabled=false\n"), 0600))
	t.Setenv("HERDR_SESH_REMOTE_CONFIG", remoteConfig)
	marker := filepath.Join(dir, "executed")
	cfgPath := filepath.Join(dir, "sesh.toml")
	require.NoError(t, os.WriteFile(cfgPath, fmt.Appendf(nil, "[default_session]\npreview_command = %q\n", "touch "+marker), 0600))
	a := &App{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}
	require.ErrorContains(t, a.preview(context.Background(), []string{"--config", cfgPath, "remote:0123456789abcdef"}), "remote session unavailable")
	_, err := os.Stat(marker)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestRemoteBlacklistAppliesToPickerAndList(t *testing.T) {
	configureFakeSources(t, "")
	_, done := fakeDaemon(t, 3, []remote.Machine{{ID: "peer", Session: "main", Workspaces: []remote.Workspace{{ID: "w1", Label: "hidden"}, {ID: "w2", Label: "visible"}}}})
	cfgPath := filepath.Join(t.TempDir(), "sesh.toml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("blacklist = ['^hidden$']\n"), 0600))
	var output bytes.Buffer
	a := &App{Out: &output, Err: &bytes.Buffer{}}
	cfg, err := a.loadConfig(cfgPath)
	require.NoError(t, err)
	col, err := a.collectPicker(context.Background(), cfg, nil)
	require.NoError(t, err)
	require.Len(t, col.Sessions, 1)
	assert.Equal(t, "visible", col.Sessions[0].Name)
	for _, blacklisted := range []bool{false, true} {
		output.Reset()
		args := []string{"--config", cfgPath, "--json"}
		if blacklisted {
			args = append(args, "--blacklisted")
		}
		require.NoError(t, a.list(context.Background(), args))
		var sessions []model.Session
		require.NoError(t, json.Unmarshal(output.Bytes(), &sessions))
		require.Len(t, sessions, 1)
		want := "visible"
		if blacklisted {
			want = "hidden"
		}
		assert.Equal(t, want, sessions[0].Name)
	}
	require.NoError(t, <-done)
}

func TestExplicitRemoteTargetHonorsBlacklist(t *testing.T) {
	_, done := fakeDaemon(t, 1, []remote.Machine{{ID: "peer", Session: "main", Workspaces: []remote.Workspace{{ID: "w1", Label: "hidden"}}}})
	cfg := config.Default()
	cfg.Blacklist = []string{"^hidden$"}
	target := model.Key(model.Session{Source: model.SourceRemote, Remote: &model.RemoteRef{Machine: "peer", Session: "main", WorkspaceID: "w1"}})
	rows, err := (&App{Err: &bytes.Buffer{}}).collectAllowUnavailableHerdr(context.Background(), cfg, target)
	require.ErrorContains(t, err, "remote session unavailable")
	assert.Empty(t, rows)
	require.NoError(t, <-done)
}

func TestLocalRemotePrefixPreviewDoesNotContactRemoteCatalog(t *testing.T) {
	configureFakeSources(t, "")
	requests, _ := fakeDaemon(t, 1, nil)
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "sesh.toml")
	require.NoError(t, os.WriteFile(cfgPath, fmt.Appendf(nil, "version=1\n[[workspace]]\nname='remote:prod'\npath=%q\npreview='printf local-preview'\n", dir), 0600))
	var out, warnings bytes.Buffer
	a := &App{Out: &out, Err: &warnings}
	require.NoError(t, a.preview(context.Background(), []string{"--config", cfgPath, "remote:prod"}))
	assert.Equal(t, "local-preview", out.String())
	assert.Empty(t, warnings.String())
	select {
	case request := <-requests:
		t.Fatalf("local target contacted remote catalog: %+v", request)
	default:
	}
}

func TestLocalRemotePrefixConnectAndPreviewWhenRemotesDisabled(t *testing.T) {
	configureHerdrScript(t, `#!/bin/sh
case "$1 $2" in
"workspace list") printf '[{"id":"ws1","label":"remote:prod"}]\n' ;;
"workspace focus") ;;
*) exit 1 ;;
esac
`)
	dir := t.TempDir()
	remoteConfig := filepath.Join(dir, "remote.toml")
	require.NoError(t, os.WriteFile(remoteConfig, []byte("enabled=false\n"), 0600))
	t.Setenv("HERDR_SESH_REMOTE_CONFIG", remoteConfig)
	t.Setenv("HERDR_PLUGIN_STATE_DIR", dir)
	cfgPath := filepath.Join(dir, "herdr-sesh.toml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("version=1\n"), 0600))
	var out bytes.Buffer
	a := &App{Out: &out, Err: &bytes.Buffer{}}
	require.NoError(t, a.connect(context.Background(), []string{"--config", cfgPath, "remote:prod"}))
	assert.Equal(t, "remote:prod\n", out.String())
	out.Reset()
	require.NoError(t, a.preview(context.Background(), []string{"--config", cfgPath, "remote:prod"}))
	assert.Equal(t, "workspace: remote:prod\nid: ws1\npath: \n", out.String())
}

func TestLocalCollectionDoesNotContactRemoteCatalog(t *testing.T) {
	configureFakeSources(t, "")
	requests, _ := fakeDaemon(t, 1, nil)
	var warnings bytes.Buffer
	_, err := (&App{Err: &warnings}).collectAllowUnavailableHerdr(context.Background(), config.Default(), "local-name")
	require.NoError(t, err)
	assert.Empty(t, warnings.String())
	select {
	case request := <-requests:
		t.Fatalf("local collection contacted remote catalog: %+v", request)
	default:
	}
}

func TestPickerWarningsDeduplicateUntilFlush(t *testing.T) {
	var out bytes.Buffer
	b := &pickerBackend{app: &App{Err: &out}}
	for range 7 {
		b.warnf("remote catalog unavailable: %s", "connection refused")
	}
	b.warnf("remote catalog unavailable: %s", "connection timed out")
	require.Len(t, b.warnings, 2)
	b.flushWarnings()
	assert.Equal(t, 2, strings.Count(out.String(), "remote catalog unavailable"))
	assert.Contains(t, out.String(), "connection timed out")
	assert.Empty(t, b.warnings)
	b.warnf("remote catalog unavailable: %s", "connection refused")
	require.Len(t, b.warnings, 1)
	b.flushWarnings()
	assert.Equal(t, 3, strings.Count(out.String(), "remote catalog unavailable"))
}

func TestNativePickerInitializesBeforeRemoteCatalog(t *testing.T) {
	configureFakeSources(t, "")
	configureHerdrScript(t, `#!/bin/sh
case "$1 $2" in
"workspace list") printf '[{"id":"local","label":"local","cwd":"/local"}]\n' ;;
"pane list") printf '[]\n' ;;
*) exit 1 ;;
esac
`)
	requests, done := fakeDaemon(t, 1, []remote.Machine{{ID: "peer", Workspaces: []remote.Workspace{{ID: "w1", Label: "remote", CWD: "/remote"}}}})
	t.Setenv("HERDR_PLUGIN_STATE_DIR", t.TempDir())
	b := &pickerBackend{app: &App{}, ctx: context.Background(), client: herdr.NewCLIClient(), cfg: config.Default()}
	sessions, opts, err := b.initialize(context.Background())
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Equal(t, "local", sessions[0].WorkspaceID)
	select {
	case <-requests:
		t.Fatal("native initialization must not join the remote catalog")
	default:
	}
	require.NotNil(t, opts.LoadRemoteRows)
	peers, err := opts.LoadRemoteRows(context.Background())
	require.NoError(t, err)
	require.NoError(t, <-done)
	require.Len(t, peers, 1)
	assert.Equal(t, "peer", peers[0].Remote.Machine)
	assert.Equal(t, "list", (<-requests).Op)
}

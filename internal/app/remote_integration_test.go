package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/fullerzz/herdr-plugin-sesh/internal/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoteCatalogDisabledAndUnavailable(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "remote.toml")
	t.Setenv("HERDR_SESH_REMOTE_CONFIG", configPath)
	require.NoError(t, os.WriteFile(configPath, []byte("enabled = false\n"), 0600))
	var warnings bytes.Buffer
	a := &App{Err: &warnings}
	assert.Empty(t, a.remoteSessions(context.Background()))
	assert.Empty(t, warnings.String())
	socket := filepath.Join(dir, "missing.sock")
	require.NoError(t, os.WriteFile(configPath, fmt.Appendf(nil, "enabled=true\nid='local'\nsocket=%q\n", socket), 0600))
	assert.Empty(t, a.remoteSessions(context.Background()))
	assert.Contains(t, warnings.String(), "remote catalog unavailable")
}

func TestRemoteCatalogSnapshotAndSwitch(t *testing.T) {
	// A short socket path also works on macOS's limited Unix socket addresses.
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
	requests := make(chan remote.Request, 2)
	done := make(chan error, 1)
	go func() {
		for i := 0; i < 2; i++ {
			conn, e := listener.Accept()
			if e != nil {
				done <- e
				return
			}
			var request remote.Request
			e = json.NewDecoder(conn).Decode(&request)
			if e == nil {
				requests <- request
				e = json.NewEncoder(conn).Encode(remote.Response{Catalog: []remote.Machine{{ID: "peer", Session: "main", Stale: true, Workspaces: []remote.Workspace{{ID: "w1", Label: "same", CWD: "/remote"}}}}})
			}
			_ = conn.Close()
			if e != nil {
				done <- e
				return
			}
		}
		done <- nil
	}()
	a := &App{Err: &bytes.Buffer{}}
	sessions := a.remoteSessions(context.Background())
	require.Len(t, sessions, 1)
	s := sessions[0]
	assert.Equal(t, model.SourceRemote, s.Source)
	assert.True(t, s.RemoteOffline)
	assert.Equal(t, "main", s.RemoteSession)
	require.ErrorContains(t, a.switchRemote(context.Background(), s), "offline")
	s.RemoteOffline = false
	require.NoError(t, a.switchRemote(context.Background(), s))
	require.NoError(t, <-done)
	assert.Equal(t, "list", (<-requests).Op)
	request := <-requests
	assert.Equal(t, "switch", request.Op)
	assert.Equal(t, "peer", request.MachineID)
	assert.Equal(t, "w1", request.WorkspaceID)
	assert.Equal(t, model.Key(s), pickerTarget(s))
	assert.Empty(t, localSessions(sessions))
}

func TestRemoteListPrintsQualifiedTarget(t *testing.T) {
	remoteSession := model.Session{Source: model.SourceRemote, Name: "work", Path: "/remote", WorkspaceID: "w1", RemoteMachine: "peer", RemoteSession: "main", RemoteOffline: true}
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
	dir, err := os.MkdirTemp("/tmp", "sesh-filter-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	listener, err := net.Listen("unix", filepath.Join(dir, "daemon.sock"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	cfgPath := filepath.Join(dir, "sesh.toml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("blacklist = ['^hidden$']\n"), 0600))
	remotePath := filepath.Join(dir, "remote.toml")
	require.NoError(t, os.WriteFile(remotePath, fmt.Appendf(nil, "enabled=true\nid='local'\nsocket=%q\n", filepath.Join(dir, "daemon.sock")), 0600))
	t.Setenv("HERDR_SESH_REMOTE_CONFIG", remotePath)
	done := make(chan error, 1)
	go func() {
		for i := 0; i < 3; i++ {
			conn, e := listener.Accept()
			if e != nil {
				done <- e
				return
			}
			var request remote.Request
			e = json.NewDecoder(conn).Decode(&request)
			if e == nil {
				e = json.NewEncoder(conn).Encode(remote.Response{Catalog: []remote.Machine{{ID: "peer", Session: "main", Workspaces: []remote.Workspace{{ID: "w1", Label: "hidden"}, {ID: "w2", Label: "visible"}}}}})
			}
			_ = conn.Close()
			if e != nil {
				done <- e
				return
			}
		}
		done <- nil
	}()
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

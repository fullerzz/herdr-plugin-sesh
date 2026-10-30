package remote

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeClient struct {
	mu      sync.Mutex
	focused string
}

func (f *fakeClient) WorkspaceList(context.Context) ([]herdr.Workspace, error) {
	return []herdr.Workspace{{ID: "ws1", Label: "one"}}, nil
}
func (f *fakeClient) WorkspaceFocus(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.focused = id
	return nil
}
func testConfig() Config {
	return Config{Enabled: true, ID: "local", Peers: []Peer{{ID: "other", SSH: "other"}}}
}
func TestCatalogKeepsFailedPeersStaleAndExportLocal(t *testing.T) {
	s := NewServer(testConfig(), &fakeClient{})
	fail := false
	s.PeerCall = func(context.Context, Peer, Request) (Response, error) {
		if fail {
			return Response{}, errors.New("offline")
		}
		return Response{Catalog: []Machine{{ID: "other", Workspaces: []Workspace{{ID: "remote1"}}}}}, nil
	}
	s.refreshLocal(context.Background())
	s.refreshPeer(context.Background(), 0)
	r := s.Handle(context.Background(), Request{Op: "list"})
	require.Len(t, r.Catalog, 2)
	assert.False(t, r.Catalog[1].Stale)
	fail = true
	s.refreshLocal(context.Background())
	s.refreshPeer(context.Background(), 0)
	r = s.Handle(context.Background(), Request{Op: "list"})
	assert.True(t, r.Catalog[1].Stale)
	require.Len(t, r.Catalog[1].Workspaces, 1)
	r = s.Handle(context.Background(), Request{Op: "export"})
	require.Len(t, r.Catalog, 1)
	assert.Equal(t, "local", r.Catalog[0].ID)
}
func TestFocusChecksLiveInventory(t *testing.T) {
	f := &fakeClient{}
	s := NewServer(testConfig(), f)
	ctx := context.Background()
	assert.NotEmpty(t, s.Handle(ctx, Request{Op: "focus", WorkspaceID: "missing"}).Error)
	assert.NotEmpty(t, s.Handle(ctx, Request{Op: "focus", WorkspaceID: "--bad"}).Error)
	assert.Empty(t, s.Handle(ctx, Request{Op: "focus", WorkspaceID: "ws1"}).Error)
	assert.Equal(t, "ws1", f.focused)
}
func TestUnixAPIAndSocketLifecycle(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "sesh-rpc-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	require.NoError(t, os.Chmod(dir, 0700)) // #nosec G302 -- private socket directory requires traversal.
	c := Config{Enabled: true, ID: "local", Socket: filepath.Join(dir, "daemon.sock")}
	s := NewServer(c, &fakeClient{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx) }()
	defer cancel()
	require.Eventually(t, func() bool { _, e := Call(context.Background(), c, Request{Op: "list"}); return e == nil }, time.Second, 10*time.Millisecond)
	require.Error(t, NewServer(c, &fakeClient{}).Serve(context.Background()))
	_, e := Call(context.Background(), c, Request{Op: "unknown"})
	require.Error(t, e)
	cancel()
	require.NoError(t, <-done)
	_, e = os.Lstat(c.Socket)
	require.ErrorIs(t, e, os.ErrNotExist)
	require.NoError(t, os.WriteFile(c.Socket, []byte("keep"), 0600))
	require.Error(t, s.Serve(context.Background()))
	b, e := os.ReadFile(c.Socket)
	require.NoError(t, e)
	assert.Equal(t, "keep", string(b))
}
func TestConfigOptInAndValidation(t *testing.T) {
	p := filepath.Join(t.TempDir(), "remote.toml")
	t.Setenv("HERDR_SESH_REMOTE_CONFIG", p)
	_, e := LoadConfig()
	require.Error(t, e)
	require.NoError(t, os.WriteFile(p, []byte("enabled = false\n"), 0600))
	c, e := LoadConfig()
	require.NoError(t, e)
	assert.False(t, c.Enabled)
	for _, c := range []Config{{Enabled: true}, {Enabled: true, ID: "here", Peers: []Peer{{ID: "here", SSH: "host"}}}, {Enabled: true, ID: "here", Peers: []Peer{{ID: "there", SSH: "-oProxyCommand=bad"}}}, {Enabled: true, ID: "here", Socket: "relative"}} {
		require.Error(t, c.Validate())
	}
}
func TestSSHUsesFixedCommand(t *testing.T) {
	assert.Equal(t, []string{
		"-o", "BatchMode=yes", "-o", "ForwardAgent=no", "-o", "ConnectTimeout=5",
		"-o", "ServerAliveInterval=10", "-o", "ServerAliveCountMax=3",
		"--", "user@host", "herdr-sesh", "daemon", "rpc", "--stream",
	}, SSHArgs(Peer{SSH: "user@host", Session: "$(touch bad)"}))
	_, e := NewSSHClient(Peer{ID: "evil", SSH: "-evil"}).Call(context.Background(), Request{Op: "list"})
	require.ErrorContains(t, e, "invalid SSH destination")
}

func TestConfigDefaultsAndUnknownKeys(t *testing.T) {
	p := filepath.Join(t.TempDir(), "remote.toml")
	t.Setenv("HERDR_SESH_REMOTE_CONFIG", p)
	require.NoError(t, os.WriteFile(p, []byte("enabled=true\nid='local'\n[[peers]]\nid='other'\nssh='other'\n"), 0600))
	c, err := LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, "default", c.HerdrSession)
	require.Len(t, c.Peers, 1)
	assert.Equal(t, "default", c.Peers[0].Session)
	require.NoError(t, os.WriteFile(p, []byte("enabled=true\nid='local'\nenabeld=true\n"), 0600))
	_, err = LoadConfig()
	require.Error(t, err)
}
func TestExpiredCatalogCannotBeSwitched(t *testing.T) {
	c := testConfig()
	c.Peers = nil
	s := NewServer(c, &fakeClient{})
	s.refreshLocal(context.Background())
	s.catalog[0].UpdatedAt = time.Now().Add(-time.Minute)
	r := s.Handle(context.Background(), Request{Op: "export"})
	require.Len(t, r.Catalog, 1)
	assert.True(t, r.Catalog[0].Stale)
}
func TestPeerCannotReturnTransitiveOrWrongSession(t *testing.T) {
	for _, catalog := range [][]Machine{{{ID: "other", Session: "wrong"}}, {{ID: "other"}, {ID: "third"}}} {
		s := NewServer(testConfig(), &fakeClient{})
		s.PeerCall = func(context.Context, Peer, Request) (Response, error) { return Response{Catalog: catalog}, nil }
		s.refreshLocal(context.Background())
		s.refreshPeer(context.Background(), 0)
		r := s.Handle(context.Background(), Request{Op: "list"})
		require.Len(t, r.Catalog, 2)
		assert.True(t, r.Catalog[1].Stale)
		assert.Empty(t, r.Catalog[1].Workspaces)
	}
}
func TestRejectsSymlinkSocketDirectory(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "sesh-safe-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	require.NoError(t, os.Symlink(dir, filepath.Join(dir, "link")))
	c := Config{Enabled: true, ID: "here", Socket: filepath.Join(dir, "link", "rpc.sock")}
	require.Error(t, NewServer(c, &fakeClient{}).Serve(context.Background()))
}

func TestRecoversStaleSocket(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "sesh-stale-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	c := Config{Enabled: true, ID: "local", Socket: filepath.Join(dir, "daemon.sock")}
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: c.Socket, Net: "unix"})
	require.NoError(t, err)
	l.SetUnlinkOnClose(false)
	require.NoError(t, l.Close())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- NewServer(c, &fakeClient{}).Serve(ctx) }()
	require.Eventually(t, func() bool { _, err := Call(context.Background(), c, Request{Op: "list"}); return err == nil }, time.Second, 10*time.Millisecond)
	cancel()
	require.NoError(t, <-done)
}

func TestSocketRecoveryPreservesLiveListenerAndSymlink(t *testing.T) {
	for _, kind := range []string{"live", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			dir, err := os.MkdirTemp("/tmp", "sesh-preserve-")
			require.NoError(t, err)
			t.Cleanup(func() { _ = os.RemoveAll(dir) })
			c := Config{Enabled: true, ID: "local", Socket: filepath.Join(dir, "daemon.sock")}
			if kind == "live" {
				l, err := net.ListenUnix("unix", &net.UnixAddr{Name: c.Socket, Net: "unix"})
				require.NoError(t, err)
				l.SetUnlinkOnClose(false)
				defer func() { _ = l.Close() }()
			} else {
				require.NoError(t, os.Symlink("missing", c.Socket))
			}
			before, err := os.Lstat(c.Socket)
			require.NoError(t, err)
			require.Error(t, NewServer(c, &fakeClient{}).Serve(context.Background()))
			after, err := os.Lstat(c.Socket)
			require.NoError(t, err)
			assert.True(t, os.SameFile(before, after))
		})
	}
}

func TestSocketLockPersistsAndRejectsUnsafePaths(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "rpc.sock")
	first, err := lockSocket(p)
	require.NoError(t, err)
	_, err = lockSocket(p)
	require.Error(t, err)
	before, err := os.Lstat(p + ".lock")
	require.NoError(t, err)
	require.NoError(t, first.Close())
	second, err := lockSocket(p)
	require.NoError(t, err)
	require.NoError(t, second.Close())
	after, err := os.Lstat(p + ".lock")
	require.NoError(t, err)
	assert.True(t, os.SameFile(before, after))
	require.NoError(t, os.Remove(p+".lock"))
	require.NoError(t, os.Symlink("missing", p+".lock"))
	_, err = lockSocket(p)
	require.Error(t, err)
	_, err = os.Lstat(p + ".lock")
	require.NoError(t, err)
}

type inventoryClient struct{ workspaces []herdr.Workspace }

func (c inventoryClient) WorkspaceList(context.Context) ([]herdr.Workspace, error) {
	return c.workspaces, nil
}
func (inventoryClient) WorkspaceFocus(context.Context, string) error { return nil }

func TestCatalogSharesOnlySanitizedWireWorkspaces(t *testing.T) {
	local := []herdr.Workspace{{ID: "ws1", Label: "one", CWD: "/src", AgentStatus: "idle", ForegroundCWD: "/secret", Tokens: map[string]string{"token": "secret"}}}
	s := NewServer(testConfig(), inventoryClient{workspaces: local})
	s.PeerCall = func(context.Context, Peer, Request) (Response, error) {
		return Response{Catalog: []Machine{{ID: "other", Workspaces: []Workspace{
			{ID: "--evil", Label: "dropped"},
			{ID: "w1", Label: "\x1b]0;title\x07red\x1b[31m", CWD: "/tmp\n/x", AgentStatus: "busy\r"},
		}}}}, nil
	}
	ctx := context.Background()
	s.refreshLocal(ctx)
	s.refreshPeer(ctx, 0)
	export := s.Handle(ctx, Request{Op: "export"})
	b, err := json.Marshal(export)
	require.NoError(t, err)
	assert.NotContains(t, string(b), "secret")
	require.Len(t, export.Catalog, 1)
	assert.Equal(t, []Workspace{{ID: "ws1", Label: "one", CWD: "/src", AgentStatus: "idle"}}, export.Catalog[0].Workspaces)
	listed := s.Handle(ctx, Request{Op: "list"})
	require.Len(t, listed.Catalog, 2)
	assert.Equal(t, []Workspace{{ID: "w1", Label: "]0;titlered[31m", CWD: "/tmp/x", AgentStatus: "busy"}}, listed.Catalog[1].Workspaces)
}

func TestRefreshRecordsAndLogsPeerAvailability(t *testing.T) {
	s := NewServer(testConfig(), &fakeClient{})
	var logs []string
	s.Logf = func(format string, args ...any) { logs = append(logs, fmt.Sprintf(format, args...)) }
	var peerErr error
	s.PeerCall = func(context.Context, Peer, Request) (Response, error) {
		if peerErr != nil {
			return Response{}, peerErr
		}
		return Response{Catalog: []Machine{{ID: "other"}}}, nil
	}
	ctx := context.Background()
	peerErr = errors.New("peer other: Connection refused")
	s.refreshLocal(ctx)
	s.refreshPeer(ctx, 0)
	s.refreshLocal(ctx)
	s.refreshPeer(ctx, 0)
	listed := s.Handle(ctx, Request{Op: "list"})
	require.Len(t, listed.Catalog, 2)
	assert.True(t, listed.Catalog[1].Stale)
	assert.Equal(t, "peer other: Connection refused", listed.Catalog[1].LastError)
	peerErr = nil
	s.refreshLocal(ctx)
	s.refreshPeer(ctx, 0)
	listed = s.Handle(ctx, Request{Op: "list"})
	assert.Empty(t, listed.Catalog[1].LastError)
	assert.Equal(t, []string{"machine other unavailable: peer other: Connection refused", "machine other available"}, logs)
}

func TestServeRefreshesLocalInventoryWhilePeersAreSlow(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "sesh-refresh-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	cfg := Config{Enabled: true, ID: "local", Socket: filepath.Join(dir, "daemon.sock")}
	for i := range 24 {
		id := fmt.Sprintf("peer%d", i)
		cfg.Peers = append(cfg.Peers, Peer{ID: id, SSH: id})
	}
	s := NewServer(cfg, &fakeClient{})
	s.PeerCall = func(ctx context.Context, _ Peer, _ Request) (Response, error) {
		<-ctx.Done()
		return Response{}, ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			assert.NoError(t, err)
		case <-time.After(3 * time.Second):
			t.Error("daemon did not stop after cancellation")
		}
	})
	var first time.Time
	require.Eventually(t, func() bool {
		catalog := s.Handle(ctx, Request{Op: "export"}).Catalog
		first = catalog[0].UpdatedAt
		return !catalog[0].Stale
	}, time.Second, 10*time.Millisecond)
	// A complete peer batch takes at least 12 seconds; local refresh must not wait.
	require.Eventually(t, func() bool {
		local := s.Handle(ctx, Request{Op: "export"}).Catalog[0]
		return !local.Stale && local.UpdatedAt.After(first)
	}, 5*time.Second, 10*time.Millisecond)
}

func TestExportPeerPreservesStaleErrorAfterIdentityValidation(t *testing.T) {
	s := NewServer(testConfig(), &fakeClient{})
	machine := Machine{ID: "other", Stale: true, LastError: "Herdr\nnot running" + strings.Repeat("x", 600)}
	s.PeerCall = func(context.Context, Peer, Request) (Response, error) {
		return Response{Catalog: []Machine{machine}}, nil
	}
	_, err := s.exportPeer(context.Background(), s.cfg.Peers[0])
	require.Error(t, err)
	assert.Equal(t, clean(machine.LastError, 512), err.Error())
	machine.ID = "Other\x1b"
	_, err = s.exportPeer(context.Background(), s.cfg.Peers[0])
	assert.EqualError(t, err, `invalid peer catalog: peer reports machine "Other\x1b" session "", configured as machine "other" session ""`)
}

func TestPeerIdentityMismatchKeepsEveryValueVisible(t *testing.T) {
	long := func(last string) string { return strings.Repeat("a", 127) + last }
	for _, tc := range []struct{ name, id, session string }{
		{"control only", "other\x00", "default\x1b"},
		{"max length identifiers", long("b"), long("c")},
		{"escape expansion", strings.Repeat("\u00ad", 64), strings.Repeat("\x00", 128)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{Enabled: true, ID: "local", Peers: []Peer{{ID: long("x"), SSH: "other", Session: long("y")}}}
			require.NoError(t, cfg.Validate())
			s := NewServer(cfg, &fakeClient{})
			s.PeerCall = func(context.Context, Peer, Request) (Response, error) {
				return Response{Catalog: []Machine{{ID: tc.id, Session: tc.session}}}, nil
			}
			s.refreshPeer(context.Background(), 0)
			peer := s.Handle(context.Background(), Request{Op: "list"}).Catalog[1]
			assert.True(t, peer.Stale)
			assert.Contains(t, peer.LastError, "peer reports machine "+peerValue(tc.id)+" session "+peerValue(tc.session))
			assert.True(t, strings.HasSuffix(peer.LastError, `configured as machine "`+long("x")+`" session "`+long("y")+`"`), peer.LastError)
			assert.NotContains(t, peer.LastError, "\x00")
			assert.NotContains(t, peer.LastError, "\x1b")
		})
	}
	assert.Equal(t, `"other\x00"`, peerValue("other\x00"))
}

type shutdownClient struct{ entered, cancelled, release chan struct{} }

func (c *shutdownClient) WorkspaceList(ctx context.Context) ([]herdr.Workspace, error) {
	close(c.entered)
	<-ctx.Done()
	close(c.cancelled)
	<-c.release
	return nil, ctx.Err()
}
func (c *shutdownClient) WorkspaceFocus(context.Context, string) error { return nil }

func TestServeRetainsSocketOwnershipUntilWorkersStop(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "sesh-shutdown-")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(dir) }()
	cfg := Config{Enabled: true, ID: "local", Socket: filepath.Join(dir, "d.sock")}
	client := &shutdownClient{make(chan struct{}), make(chan struct{}), make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- NewServer(cfg, client).Serve(ctx) }()
	defer func() { cancel(); close(client.release); require.NoError(t, <-done) }()
	select {
	case <-client.entered:
	case <-time.After(time.Second):
		t.Fatal("server did not start")
	}
	cancel()
	select {
	case <-client.cancelled:
	case <-time.After(time.Second):
		t.Fatal("worker did not cancel")
	}
	// The cancellation worker closes the listener; wait until its shutdown path runs.
	require.Eventually(t, func() bool {
		conn, e := net.Dial("unix", cfg.Socket)
		if e == nil {
			_ = conn.Close()
		}
		return e != nil
	}, time.Second, time.Millisecond)
	lock, err := lockSocket(cfg.Socket)
	if lock != nil {
		defer func() { _ = lock.Close() }()
	}
	require.Error(t, err, "ownership must remain locked while a worker is still stopping")
	assert.FileExists(t, cfg.Socket)
}

package remote

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
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
		return Response{Catalog: []Machine{{ID: "other", Workspaces: []herdr.Workspace{{ID: "remote1"}}}}}, nil
	}
	s.refresh(context.Background())
	r := s.Handle(context.Background(), Request{Op: "list"})
	require.Len(t, r.Catalog, 2)
	assert.False(t, r.Catalog[1].Stale)
	fail = true
	s.refresh(context.Background())
	r = s.Handle(context.Background(), Request{Op: "list"})
	assert.True(t, r.Catalog[1].Stale)
	require.Len(t, r.Catalog[1].Workspaces, 1)
	r = s.Handle(context.Background(), Request{Op: "export"})
	require.Len(t, r.Catalog, 1)
	assert.Equal(t, "local", r.Catalog[0].ID)
}
func TestControllerLeaseAndTargetValidation(t *testing.T) {
	c := testConfig()
	c.Peers = nil
	s := NewServer(c, &fakeClient{})
	s.refresh(context.Background())
	ctx := context.Background()
	token := "01234567890123456789012345678901"
	assert.Empty(t, s.Handle(ctx, Request{Op: "register", Token: token}).Error)
	assert.NotEmpty(t, s.Handle(ctx, Request{Op: "register", Token: token + "x"}).Error)
	assert.NotEmpty(t, s.Handle(ctx, Request{Op: "switch", MachineID: "local", WorkspaceID: "missing"}).Error)
	assert.Empty(t, s.Handle(ctx, Request{Op: "switch", MachineID: "local", WorkspaceID: "ws1"}).Error)
	r := s.Handle(ctx, Request{Op: "poll", Token: token})
	require.NotNil(t, r.Switch)
	assert.Equal(t, "ws1", r.Switch.WorkspaceID)
	assert.Equal(t, r.Switch, s.Handle(ctx, Request{Op: "poll", Token: token}).Switch)
	s.expires = time.Now().Add(-time.Second)
	assert.NotEmpty(t, s.Handle(ctx, Request{Op: "switch", MachineID: "local", WorkspaceID: "ws1"}).Error)
	assert.Empty(t, s.Handle(ctx, Request{Op: "register", Token: token + "x"}).Error)
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
	assert.Equal(t, []string{"-o", "BatchMode=yes", "-o", "ForwardAgent=no", "-o", "ConnectTimeout=2", "--", "user@host", "herdr-sesh", "daemon", "rpc"}, SSHArgs(Peer{SSH: "user@host", Session: "$(touch bad)"}))
	_, e := SSHCall(context.Background(), Peer{SSH: "-evil"}, Request{})
	require.Error(t, e)
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
	s.refresh(context.Background())
	s.catalog[0].UpdatedAt = time.Now().Add(-time.Minute)
	r := s.Handle(context.Background(), Request{Op: "export"})
	require.Len(t, r.Catalog, 1)
	assert.True(t, r.Catalog[0].Stale)
}
func TestPeerCannotReturnTransitiveOrWrongSession(t *testing.T) {
	for _, catalog := range [][]Machine{{{ID: "other", Session: "wrong"}}, {{ID: "other"}, {ID: "third"}}} {
		s := NewServer(testConfig(), &fakeClient{})
		s.PeerCall = func(context.Context, Peer, Request) (Response, error) { return Response{Catalog: catalog}, nil }
		s.refresh(context.Background())
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

func TestPollRetriesUnacknowledgedSwitch(t *testing.T) {
	c := testConfig()
	c.Peers = nil
	s := NewServer(c, &fakeClient{})
	s.refresh(context.Background())
	ctx := context.Background()
	token := "01234567890123456789012345678901"
	require.Empty(t, s.Handle(ctx, Request{Op: "register", Token: token}).Error)
	require.Empty(t, s.Handle(ctx, Request{Op: "switch", MachineID: "local", WorkspaceID: "ws1"}).Error)
	first := s.Handle(ctx, Request{Op: "poll", Token: token})
	require.NotNil(t, first.Switch)
	require.Equal(t, first.Switch, s.Handle(ctx, Request{Op: "poll", Token: token}).Switch)
	require.Empty(t, s.Handle(ctx, Request{Op: "register", Token: token}).Error)
	require.Equal(t, first.Switch, s.Handle(ctx, Request{Op: "poll", Token: token}).Switch)
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

func TestAcknowledgmentsAndLeaseReplacement(t *testing.T) {
	c := testConfig()
	c.Peers = nil
	s := NewServer(c, &fakeClient{})
	ctx := context.Background()
	s.refresh(ctx)
	token := "01234567890123456789012345678901"
	register := func(token string) {
		t.Helper()
		require.Empty(t, s.Handle(ctx, Request{Op: "register", Token: token}).Error)
	}
	submit := func() *Target {
		t.Helper()
		require.Empty(t, s.Handle(ctx, Request{Op: "switch", MachineID: "local", WorkspaceID: "ws1"}).Error)
		target := s.Handle(ctx, Request{Op: "poll", Token: s.token}).Switch
		require.NotNil(t, target)
		require.NotEmpty(t, target.RequestID)
		return target
	}
	ack := func(token, id string) Response { return s.Handle(ctx, Request{Op: "ack", Token: token, RequestID: id}) }
	register(token)
	first := submit()
	assert.NotEmpty(t, s.Handle(ctx, Request{Op: "switch", MachineID: "local", WorkspaceID: "ws1"}).Error)
	assert.NotEmpty(t, ack(token, "").Error)
	require.Empty(t, ack(token, first.RequestID).Error)
	require.Empty(t, ack(token, first.RequestID).Error)
	assert.Nil(t, s.Handle(ctx, Request{Op: "poll", Token: token}).Switch)
	second := submit()
	assert.NotEqual(t, first.RequestID, second.RequestID)
	require.Empty(t, ack(token, first.RequestID).Error)
	assert.Equal(t, second, s.Handle(ctx, Request{Op: "poll", Token: token}).Switch)
	s.expires = time.Now().Add(-time.Second)
	register(token + "new")
	third := submit()
	assert.NotEmpty(t, s.Handle(ctx, Request{Op: "poll", Token: token}).Error)
	assert.NotEmpty(t, ack(token, third.RequestID).Error)
	assert.Equal(t, third, s.Handle(ctx, Request{Op: "poll", Token: token + "new"}).Switch)
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

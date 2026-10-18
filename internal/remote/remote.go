package remote

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
)

const LeaseDuration = 15 * time.Second
const maxMessage = 4 << 20

type Request struct {
	Op          string `json:"op"`
	RequestID   string `json:"request_id,omitempty"`
	Token       string `json:"token,omitempty"`
	MachineID   string `json:"machine_id,omitempty"`
	WorkspaceID string `json:"workspace_id,omitempty"`
}
type Response struct {
	Catalog []Machine `json:"catalog,omitempty"`
	Switch  *Target   `json:"switch,omitempty"`
	Error   string    `json:"error,omitempty"`
}
type Machine struct {
	ID         string            `json:"id"`
	Label      string            `json:"label"`
	Session    string            `json:"session"`
	Workspaces []herdr.Workspace `json:"workspaces"`
	Stale      bool              `json:"stale"`
	UpdatedAt  time.Time         `json:"updated_at"`
}
type Target struct {
	RequestID   string `json:"request_id"`
	MachineID   string `json:"machine_id"`
	WorkspaceID string `json:"workspace_id"`
}
type Client interface {
	WorkspaceList(context.Context) ([]herdr.Workspace, error)
	WorkspaceFocus(context.Context, string) error
}
type Server struct {
	cfg      Config
	client   Client
	mu       sync.Mutex
	catalog  []Machine
	token    string
	expires  time.Time
	pending  *Target
	PeerCall func(context.Context, Peer, Request) (Response, error)
}

func NewServer(c Config, client Client) *Server {
	s := &Server{cfg: c, client: client, PeerCall: SSHCall}
	s.catalog = []Machine{{ID: c.ID, Label: c.Label, Session: c.HerdrSession, Stale: true}}
	for _, p := range c.Peers {
		s.catalog = append(s.catalog, Machine{ID: p.ID, Label: p.Label, Session: p.Session, Stale: true})
	}
	return s
}
func (s *Server) refresh(ctx context.Context) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i := range s.catalog {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			c, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			var ws []herdr.Workspace
			var err error
			if i == 0 {
				ws, err = s.client.WorkspaceList(c)
			} else {
				var r Response
				r, err = s.PeerCall(c, s.cfg.Peers[i-1], Request{Op: "export"})
				if err == nil {
					if len(r.Catalog) != 1 || r.Catalog[0].ID != s.cfg.Peers[i-1].ID || r.Catalog[0].Session != s.cfg.Peers[i-1].Session || r.Catalog[0].Stale {
						err = errors.New("invalid peer catalog")
					} else {
						ws = r.Catalog[0].Workspaces
					}
				}
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			s.catalog[i].Stale = err != nil
			if err == nil {
				s.catalog[i].Workspaces = ws
				s.catalog[i].UpdatedAt = time.Now()
			}
		}(i)
	}
	wg.Wait()
}
func (s *Server) expireCatalog() {
	for i := range s.catalog {
		if time.Since(s.catalog[i].UpdatedAt) > 10*time.Second {
			s.catalog[i].Stale = true
		}
	}
}
func (s *Server) focusWorkspace(ctx context.Context, id string) Response {
	fail := func(e string) Response { return Response{Error: e} }
	if !identifier.MatchString(id) {
		return fail("invalid workspace id")
	}
	ws, e := s.client.WorkspaceList(ctx)
	if e != nil {
		return fail(e.Error())
	}
	for _, w := range ws {
		if w.ID == id {
			if e = s.client.WorkspaceFocus(ctx, w.ID); e != nil {
				return fail(e.Error())
			}
			return Response{}
		}
	}
	return fail("workspace no longer exists")
}

func (s *Server) Handle(ctx context.Context, r Request) Response {
	fail := func(e string) Response { return Response{Error: e} }
	if r.Op == "focus" {
		return s.focusWorkspace(ctx, r.WorkspaceID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireCatalog()
	switch r.Op {
	case "list", "export":
		n := len(s.catalog)
		if r.Op == "export" {
			n = 1
		}
		b, _ := json.Marshal(s.catalog[:n])
		var c []Machine
		_ = json.Unmarshal(b, &c)
		return Response{Catalog: c}
	case "register":
		if len(r.Token) < 32 || len(r.Token) > 256 {
			return fail("invalid controller token")
		}
		if s.token != "" && time.Now().Before(s.expires) && s.token != r.Token {
			return fail("another controller is active")
		}
		if s.token != r.Token || !time.Now().Before(s.expires) {
			s.pending = nil
		}
		s.token = r.Token
		s.expires = time.Now().Add(LeaseDuration)
	case "poll", "unregister", "ack":
		if s.token == "" || s.token != r.Token || time.Now().After(s.expires) {
			return fail("controller lease expired")
		}
		if r.Op == "ack" {
			if r.RequestID == "" {
				return fail("missing request id")
			}
			if s.pending != nil && s.pending.RequestID == r.RequestID {
				s.pending = nil
			}
			return Response{}
		}
		if r.Op == "unregister" {
			s.token = ""
			s.pending = nil
			return Response{}
		}
		s.expires = time.Now().Add(LeaseDuration)
		if s.pending == nil {
			return Response{}
		}
		t := *s.pending
		return Response{Switch: &t}
	case "switch":
		return s.queueSwitch(r)
	default:
		return fail("unknown operation")
	}
	return Response{}
}

// queueSwitch runs while Handle holds the catalog and controller lock.
func (s *Server) queueSwitch(r Request) Response {
	fail := func(e string) Response { return Response{Error: e} }
	if s.pending != nil && time.Now().Before(s.expires) {
		return fail("a switch is already pending")
	}
	if s.token == "" || time.Now().After(s.expires) {
		return fail("no active attach controller")
	}
	if !identifier.MatchString(r.WorkspaceID) {
		return fail("invalid workspace id")
	}
	for _, m := range s.catalog {
		if m.ID == r.MachineID && !m.Stale {
			for _, w := range m.Workspaces {
				if w.ID == r.WorkspaceID {
					s.pending = &Target{RequestID: rand.Text(), MachineID: m.ID, WorkspaceID: w.ID}
					return Response{}
				}
			}
		}
	}
	return fail("workspace unavailable")
}

func (s *Server) Serve(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	var workers sync.WaitGroup
	defer func() { cancel(); workers.Wait() }()
	if !s.cfg.Enabled {
		return errors.New("remote daemon is disabled")
	}
	if e := s.cfg.Validate(); e != nil {
		return e
	}
	p, e := SocketPath(s.cfg)
	if e != nil {
		return e
	}
	d := filepath.Dir(p)
	if e = os.MkdirAll(d, 0700); e != nil {
		return e
	}
	st, e := os.Lstat(d)
	if e != nil {
		return e
	}
	if !st.IsDir() || st.Mode().Perm() != 0700 {
		return errors.New("socket directory must be private (0700)")
	}
	if owner, ok := st.Sys().(*syscall.Stat_t); !ok || int64(owner.Uid) != int64(os.Getuid()) {
		return errors.New("socket directory must belong to the current user")
	}
	lock, e := lockSocket(p)
	if e != nil {
		return e
	}
	defer func() { _ = lock.Close() }()
	if e = recoverSocket(p); e != nil {
		return e
	}
	l, e := net.ListenUnix("unix", &net.UnixAddr{Name: p, Net: "unix"})
	if e != nil {
		return e
	}
	l.SetUnlinkOnClose(false)
	defer func() { _ = l.Close() }()
	owned, e := os.Lstat(p)
	if e != nil {
		return e
	}
	defer func() {
		current, err := os.Lstat(p)
		if err == nil && os.SameFile(owned, current) {
			_ = os.Remove(p)
		}
	}()
	if e = os.Chmod(p, 0600); e != nil {
		return e
	}
	workers.Add(2)
	go func() { defer workers.Done(); <-ctx.Done(); _ = l.Close() }()
	go func() {
		defer workers.Done()
		s.refresh(ctx)
		t := time.NewTicker(3 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.refresh(ctx)
			}
		}
	}()
	sem := make(chan struct{}, 32)
	for {
		conn, e := l.Accept()
		if e != nil {
			if ctx.Err() != nil {
				return nil
			}
			return e
		}
		select {
		case sem <- struct{}{}:
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer func() { <-sem }()
				defer func() { _ = conn.Close() }()
				stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
				defer stop()
				_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
				var r Request
				if e := json.NewDecoder(io.LimitReader(conn, 16<<10)).Decode(&r); e != nil {
					return
				}
				c, cancel := context.WithTimeout(ctx, 3*time.Second)
				defer cancel()
				_ = json.NewEncoder(conn).Encode(s.Handle(c, r))
			}()
		default:
			_ = conn.Close()
		}
	}
}
func Call(ctx context.Context, c Config, r Request) (Response, error) {
	if !c.Enabled {
		return Response{}, errors.New("remote daemon is disabled")
	}
	p, e := SocketPath(c)
	if e != nil {
		return Response{}, e
	}
	conn, e := (&net.Dialer{}).DialContext(ctx, "unix", p)
	if e != nil {
		return Response{}, e
	}
	defer func() { _ = conn.Close() }()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	deadline := time.Now().Add(3 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)
	if e = json.NewEncoder(conn).Encode(r); e != nil {
		return Response{}, e
	}
	var out Response
	e = json.NewDecoder(io.LimitReader(conn, maxMessage)).Decode(&out)
	if ctx.Err() != nil {
		return out, ctx.Err()
	}
	if e == nil && out.Error != "" {
		e = errors.New(out.Error)
	}
	return out, e
}
func SSHArgs(p Peer) []string {
	return []string{"-o", "BatchMode=yes", "-o", "ForwardAgent=no", "-o", "ConnectTimeout=2", "--", p.SSH, "herdr-sesh", "daemon", "rpc"}
}
func SSHCall(ctx context.Context, p Peer, r Request) (Response, error) {
	if !sshHost.MatchString(p.SSH) {
		return Response{}, errors.New("invalid SSH destination")
	}
	b, e := json.Marshal(r)
	if e != nil {
		return Response{}, e
	}
	cmd := exec.CommandContext(ctx, "ssh", SSHArgs(p)...) // #nosec G204 -- validated SSH destination and fixed remote command.
	cmd.WaitDelay = time.Second
	cmd.Stdin = bytes.NewReader(b)
	var out limitedBuffer
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	if e = cmd.Run(); e != nil {
		return Response{}, fmt.Errorf("peer %s: %w", p.ID, e)
	}
	var response Response
	if e = json.Unmarshal(out.Bytes(), &response); e != nil {
		return response, e
	}
	if response.Error != "" {
		return response, errors.New(response.Error)
	}
	return response, nil
}

type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > maxMessage {
		return 0, errors.New("RPC response too large")
	}
	return b.Buffer.Write(p)
}

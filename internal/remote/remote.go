package remote

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
)

// ProtocolVersion changes whenever stream peers can no longer interoperate.
const ProtocolVersion = 2
const maxMessage = 4 << 20
const maxRequest = 16 << 10

type Request struct {
	ID          uint64 `json:"id,omitempty"`
	Op          string `json:"op"`
	Version     int    `json:"version,omitempty"`
	MachineID   string `json:"machine_id,omitempty"`
	WorkspaceID string `json:"workspace_id,omitempty"`
}
type Response struct {
	ID      uint64    `json:"id,omitempty"`
	Version int       `json:"version,omitempty"`
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
	MachineID   string `json:"machine_id"`
	WorkspaceID string `json:"workspace_id"`
}
type Client interface {
	WorkspaceList(context.Context) ([]herdr.Workspace, error)
	WorkspaceFocus(context.Context, string) error
}

// controller is the attach wrapper subscribed on one daemon stream.
type controller struct {
	targets chan Target
	done    chan struct{}
}

type Server struct {
	cfg        Config
	client     Client
	mu         sync.Mutex
	catalog    []Machine
	controller *controller
	peers      map[string]*StreamClient
	PeerCall   func(context.Context, Peer, Request) (Response, error)
	// IdleTimeout closes streams that send nothing, including half-open SSH relays.
	IdleTimeout time.Duration
}

func NewServer(c Config, client Client) *Server {
	s := &Server{cfg: c, client: client, peers: map[string]*StreamClient{}, IdleTimeout: 30 * time.Second}
	s.PeerCall = func(ctx context.Context, p Peer, r Request) (Response, error) { return s.peers[p.ID].Call(ctx, r) }
	s.catalog = []Machine{{ID: c.ID, Label: c.Label, Session: c.HerdrSession, Stale: true}}
	for _, p := range c.Peers {
		s.catalog = append(s.catalog, Machine{ID: p.ID, Label: p.Label, Session: p.Session, Stale: true})
		s.peers[p.ID] = NewSSHClient(p)
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

// Handle serves one request. Controller subscriptions need a stream; see serveConn.
func (s *Server) Handle(ctx context.Context, r Request) Response {
	fail := func(e string) Response { return Response{Error: e} }
	switch r.Op {
	case "hello":
		if r.Version != ProtocolVersion {
			return fail(fmt.Sprintf("protocol version mismatch: client speaks v%d, daemon speaks v%d; install the same herdr-sesh version on both machines", r.Version, ProtocolVersion))
		}
		return Response{Version: ProtocolVersion}
	case "ping":
		return Response{}
	case "focus":
		return s.focusWorkspace(ctx, r.WorkspaceID)
	case "subscribe", "unsubscribe":
		return fail(r.Op + " requires a daemon stream")
	case "register", "poll", "ack", "unregister":
		return fail("controller leases were replaced by stream subscriptions; install the same herdr-sesh version on both machines")
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
	case "switch":
		return s.queueSwitch(r)
	default:
		return fail("unknown operation")
	}
}

// queueSwitch runs while Handle holds the catalog and controller lock.
func (s *Server) queueSwitch(r Request) Response {
	fail := func(e string) Response { return Response{Error: e} }
	if s.controller == nil {
		return fail("no active attach controller")
	}
	if !identifier.MatchString(r.WorkspaceID) {
		return fail("invalid workspace id")
	}
	for _, m := range s.catalog {
		if m.ID == r.MachineID && !m.Stale {
			for _, w := range m.Workspaces {
				if w.ID == r.WorkspaceID {
					select {
					case s.controller.targets <- Target{MachineID: m.ID, WorkspaceID: w.ID}:
						return Response{}
					default:
						return fail("a switch is already pending")
					}
				}
			}
		}
	}
	return fail("workspace unavailable")
}

func (s *Server) subscribe() (*controller, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.controller != nil {
		return nil, errors.New("another controller is active")
	}
	s.controller = &controller{targets: make(chan Target, 1), done: make(chan struct{})}
	return s.controller, nil
}

func (s *Server) release(c *controller) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.controller == c {
		s.controller = nil
		close(c.done)
	}
}

// serveConn handles newline-delimited requests until the client closes the
// connection. One-shot clients send a single request; streams multiplex by id.
func (s *Server) serveConn(ctx context.Context, conn net.Conn) {
	var workers sync.WaitGroup
	var sub *controller
	defer func() {
		if sub != nil {
			s.release(sub)
		}
		workers.Wait()
		_ = conn.Close()
	}()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	var wmu sync.Mutex
	enc := json.NewEncoder(conn)
	write := func(r Response) {
		wmu.Lock()
		defer wmu.Unlock()
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if enc.Encode(r) != nil {
			_ = conn.Close()
		}
	}
	reader := bufio.NewReader(conn)
	sem := make(chan struct{}, 8)
	deadline := 5 * time.Second
	for {
		_ = conn.SetReadDeadline(time.Now().Add(deadline))
		line, err := readLine(reader, maxRequest)
		if err != nil {
			return
		}
		deadline = s.IdleTimeout
		var r Request
		if err := json.Unmarshal(line, &r); err != nil {
			write(Response{Error: "invalid request"})
			return
		}
		switch r.Op {
		case "subscribe":
			if c := s.subscribeStream(r, sub, write); c != nil {
				sub = c
				workers.Add(1)
				go func(id uint64) {
					defer workers.Done()
					for {
						select {
						case t := <-c.targets:
							write(Response{ID: id, Switch: &t})
						case <-c.done:
							return
						}
					}
				}(r.ID)
			}
			continue
		case "unsubscribe":
			if sub != nil {
				s.release(sub)
				sub = nil
			}
			write(Response{ID: r.ID})
			continue
		}
		sem <- struct{}{}
		workers.Add(1)
		go func(r Request) {
			defer workers.Done()
			defer func() { <-sem }()
			c, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			resp := s.Handle(c, r)
			resp.ID = r.ID
			write(resp)
		}(r)
	}
}

// subscribeStream answers a subscribe request before any switch is pushed.
func (s *Server) subscribeStream(r Request, current *controller, write func(Response)) *controller {
	resp := Response{ID: r.ID}
	var c *controller
	var err error
	switch {
	case r.ID == 0:
		resp.Error = "subscribe requires a daemon stream"
	case current != nil:
		resp.Error = "stream is already subscribed"
	default:
		if c, err = s.subscribe(); err != nil {
			resp.Error = err.Error()
		}
	}
	write(resp)
	return c
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
	defer func() {
		for _, c := range s.peers {
			_ = c.Close()
		}
	}()
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
				s.serveConn(ctx, conn)
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

// Relay connects an SSH session's stdio to the local daemon socket. Requests
// stay bounded per line; responses and pushes flow back until either side closes.
func Relay(ctx context.Context, c Config, in io.Reader, out io.Writer) error {
	p, err := SocketPath(c)
	if err != nil {
		return err
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", p)
	if err != nil {
		return err
	}
	unix, ok := conn.(*net.UnixConn)
	if !ok {
		_ = conn.Close()
		return errors.New("daemon socket is not a Unix connection")
	}
	defer func() { _ = unix.Close() }()
	stop := context.AfterFunc(ctx, func() { _ = unix.Close() })
	defer stop()
	down := make(chan error, 1)
	go func() {
		_, err := io.Copy(out, unix)
		down <- err
	}()
	up := make(chan error, 1)
	go func() {
		r := bufio.NewReader(in)
		for {
			line, err := readLine(r, maxRequest)
			if errors.Is(err, io.EOF) {
				// The SSH session ended; the daemon drops its subscription.
				up <- unix.CloseWrite()
				return
			}
			if err == nil {
				_, err = unix.Write(line)
			}
			if err != nil {
				up <- err
				return
			}
		}
	}()
	select {
	case err := <-down:
		return err
	case err := <-up:
		if err != nil {
			return err
		}
		return <-down
	}
}

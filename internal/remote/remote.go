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
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode"

	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
)

// ProtocolVersion changes whenever stream peers can no longer interoperate.
const ProtocolVersion = 3
const maxMessage = 4 << 20
const maxRequest = 16 << 10

type Request struct {
	Op          string `json:"op"`
	Version     int    `json:"version,omitempty"`
	MachineID   string `json:"machine_id,omitempty"`
	Session     string `json:"session,omitempty"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	// Machines lists the machine IDs a subscribing controller can attach to.
	Machines []string `json:"machines,omitempty"`
	// Failure reports why a delivered switch did not complete.
	Failure string `json:"failure,omitempty"`
}
type Response struct {
	Version int       `json:"version,omitempty"`
	Catalog []Machine `json:"catalog,omitempty"`
	// SwitchError is the controller's last reported switch failure.
	SwitchError string `json:"switch_error,omitempty"`
	Error       string `json:"error,omitempty"`
}
type Machine struct {
	ID         string      `json:"id"`
	Label      string      `json:"label"`
	Session    string      `json:"session"`
	Workspaces []Workspace `json:"workspaces"`
	Stale      bool        `json:"stale"`
	UpdatedAt  time.Time   `json:"updated_at"`
	LastError  string      `json:"last_error,omitempty"`
}

// Workspace is the metadata one machine shares with its peers.
type Workspace struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	CWD         string `json:"cwd"`
	AgentStatus string `json:"agent_status"`
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
	// conn is the daemon connection that registered the controller.
	conn     uint64
	targets  chan Target
	done     chan struct{}
	machines map[string]bool
	// busy holds a delivered switch until the controller completes it.
	busy bool
}

type Server struct {
	cfg         Config
	client      Client
	mu          sync.Mutex
	catalog     []Machine
	controller  *controller
	switchError string
	peers       map[string]*StreamClient
	PeerCall    func(context.Context, Peer, Request) (Response, error)
	// IdleTimeout closes streams that send nothing, including half-open SSH relays.
	IdleTimeout time.Duration
	// Logf, when set, reports catalog availability, stream, and switch lifecycle events.
	Logf  func(format string, args ...any)
	conns atomic.Uint64
}

func (s *Server) logf(format string, args ...any) {
	if s.Logf != nil {
		s.Logf(format, args...)
	}
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
func (s *Server) refreshLocal(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	local, err := s.client.WorkspaceList(ctx)
	s.record(0, fromHerdr(local), err)
}

func (s *Server) refreshPeer(ctx context.Context, i int) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	ws, err := s.exportPeer(ctx, s.cfg.Peers[i])
	s.record(i+1, ws, err)
}

func (s *Server) refreshCatalog(ctx context.Context) {
	var workers sync.WaitGroup
	defer workers.Wait()
	refreshes := make([]func(context.Context), 1, 1+len(s.cfg.Peers))
	refreshes[0] = s.refreshLocal
	for i := range s.cfg.Peers {
		refreshes = append(refreshes, func(ctx context.Context) { s.refreshPeer(ctx, i) })
	}
	// One loop per machine keeps slow peers from delaying healthy catalogs.
	for _, refresh := range refreshes {
		workers.Go(func() {
			refresh(ctx)
			t := time.NewTicker(3 * time.Second)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					refresh(ctx)
				}
			}
		})
	}
}

func (s *Server) exportPeer(ctx context.Context, p Peer) ([]Workspace, error) {
	r, err := s.PeerCall(ctx, p, Request{Op: "export"})
	if err != nil {
		return nil, err
	}
	if len(r.Catalog) != 1 {
		return nil, fmt.Errorf("invalid peer catalog: want 1 machine, got %d", len(r.Catalog))
	}
	// Machine IDs are compared exactly, so a casing typo in either config rejects the peer.
	if got := r.Catalog[0]; got.ID != p.ID || got.Session != p.Session {
		return nil, fmt.Errorf("invalid peer catalog: peer reports machine %s session %s, configured as machine %q session %q",
			peerValue(got.ID), peerValue(got.Session), p.ID, p.Session)
	}
	if r.Catalog[0].Stale {
		if message := clean(r.Catalog[0].LastError, 512); message != "" {
			return nil, errors.New(message)
		}
		return nil, errors.New("peer catalog is stale")
	}
	return sanitizeWorkspaces(r.Catalog[0].Workspaces), nil
}

// peerValue quotes peer-reported identity text so every differing byte stays
// visible. Valid identifiers print whole; other text is escaped to ASCII from
// at most 32 input bytes, so each value stays under 140 bytes.
func peerValue(text string) string {
	if identifier.MatchString(text) {
		return strconv.Quote(text)
	}
	if len(text) > 32 {
		return strconv.QuoteToASCII(text[:32]) + "..."
	}
	return strconv.QuoteToASCII(text)
}

// maxLastError fits an identity mismatch naming four 128-byte identifiers.
const maxLastError = 1024

// record stores one refresh result and logs availability changes.
func (s *Server) record(i int, ws []Workspace, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := &s.catalog[i]
	previous := m.LastError
	m.Stale = err != nil
	if err != nil {
		m.LastError = clean(err.Error(), maxLastError)
		if m.LastError != previous {
			s.logf("machine %s unavailable: %s", m.ID, m.LastError)
		}
		return
	}
	m.Workspaces, m.UpdatedAt, m.LastError = ws, time.Now(), ""
	if previous != "" {
		s.logf("machine %s available", m.ID)
	}
}

func fromHerdr(ws []herdr.Workspace) []Workspace {
	out := make([]Workspace, 0, len(ws))
	for _, w := range ws {
		out = append(out, Workspace{ID: w.ID, Label: w.Label, CWD: w.CWD, AgentStatus: w.AgentStatus})
	}
	return sanitizeWorkspaces(out)
}

// sanitizeWorkspaces drops unaddressable workspaces and terminal control text.
func sanitizeWorkspaces(ws []Workspace) []Workspace {
	out := make([]Workspace, 0, len(ws))
	for _, w := range ws {
		if !identifier.MatchString(w.ID) {
			continue
		}
		out = append(out, Workspace{ID: w.ID, Label: clean(w.Label, 256), CWD: clean(w.CWD, 4096), AgentStatus: clean(w.AgentStatus, 64)})
	}
	return out
}

// clean removes control characters and bounds text taken from another process.
func clean(text string, limit int) string {
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, text)
	if len(text) > limit {
		text = strings.ToValidUTF8(text[:limit], "")
	}
	return text
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
	case "subscribe", "unsubscribe", "complete":
		return fail(r.Op + " requires a daemon stream")
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
		c := slices.Clone(s.catalog[:n])
		for i := range c {
			c[i].Workspaces = slices.Clone(c[i].Workspaces)
		}
		if r.Op == "export" {
			return Response{Catalog: c}
		}
		return Response{Catalog: c, SwitchError: s.switchError}
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
	if !s.controller.machines[r.MachineID] {
		return fail(fmt.Sprintf("machine %q is not in the attached wrapper's remote.toml allowlist", r.MachineID))
	}
	if s.controller.busy {
		return fail("a switch is already in progress")
	}
	for _, m := range s.catalog {
		if m.ID == r.MachineID && !m.Stale {
			for _, w := range m.Workspaces {
				if w.ID == r.WorkspaceID {
					// busy guarantees the one-slot channel is empty.
					s.controller.targets <- Target{MachineID: m.ID, WorkspaceID: w.ID}
					s.controller.busy = true
					s.logf("switch to %s/%s queued for controller on conn %d", m.ID, w.ID, s.controller.conn)
					return Response{}
				}
			}
		}
	}
	return fail("workspace unavailable")
}

func (s *Server) subscribe(conn uint64, r Request) (*controller, error) {
	if r.MachineID == "" || r.Session == "" || r.MachineID != s.cfg.ID || r.Session != s.cfg.HerdrSession {
		return nil, errors.New("daemon machine or session does not match this client's configuration")
	}
	machines := r.Machines
	if len(machines) == 0 || len(machines) > 256 {
		return nil, errors.New("subscribe requires the controller's reachable machines")
	}
	allowed := map[string]bool{}
	for _, id := range machines {
		if !identifier.MatchString(id) {
			return nil, errors.New("invalid machine id")
		}
		allowed[id] = true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.controller != nil {
		return nil, fmt.Errorf("another controller is active on conn %d", s.controller.conn)
	}
	s.controller = &controller{conn: conn, targets: make(chan Target, 1), done: make(chan struct{}), machines: allowed}
	s.logf("controller registered on conn %d for machines %s", conn, strings.Join(machines, ","))
	return s.controller, nil
}

// complete ends the controller's in-flight switch and records its outcome.
func (s *Server) complete(c *controller, failure string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.controller != c {
		s.logf("conn %d completed a switch after its controller was released", c.conn)
		return
	}
	c.busy = false
	s.switchError = clean(failure, 512)
	if s.switchError != "" {
		s.logf("controller on conn %d failed its switch: %s", c.conn, s.switchError)
		return
	}
	s.logf("controller on conn %d completed its switch", c.conn)
}

// release ends the controller's registration and any in-flight switch in the
// same step, so no new switch can be queued to a controller that is leaving.
// An unsubscribe mid-switch reports the handoff outcome atomically; a dropped
// stream leaves that outcome unknown.
func (s *Server) release(c *controller, unsubscribed bool, failure string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.controller != c {
		return
	}
	how := "disconnected"
	if unsubscribed {
		how = "unsubscribed"
	}
	if c.busy {
		s.switchError = clean(failure, 512)
		if !unsubscribed {
			s.switchError = "attach controller disconnected during a switch"
		}
		how += " during a switch"
	}
	s.logf("controller on conn %d %s", c.conn, how)
	s.controller = nil
	close(c.done)
}

// connState is one client connection: a JSON-RPC stream or a one-shot local request.
type connState struct {
	s  *Server
	id uint64
	// rpc marks a JSON-RPC stream; other connections carry one plain request.
	rpc     bool
	conn    net.Conn
	wmu     sync.Mutex
	enc     *json.Encoder
	workers sync.WaitGroup
	sem     chan struct{}
	sub     *controller
}

// write reports whether v was sent; a failed write closes the connection.
func (c *connState) write(v any) bool {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if err := c.enc.Encode(v); err != nil {
		c.s.logf("conn %d write failed, closing: %v", c.id, err)
		_ = c.conn.Close()
		return false
	}
	return true
}

// spawn runs a request concurrently so slow Herdr calls do not block the stream.
func (c *connState) spawn(ctx context.Context, r Request, reply func(Response)) {
	c.sem <- struct{}{}
	c.workers.Add(1)
	go func() {
		defer c.workers.Done()
		defer func() { <-c.sem }()
		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		start := time.Now()
		resp := c.s.Handle(ctx, r)
		c.logRequest(r, resp, time.Since(start))
		reply(resp)
	}()
}

// routineOps repeat every few seconds; they are logged only when they fail or stall.
var routineOps = map[string]bool{"ping": true, "list": true, "export": true}

func (c *connState) logRequest(r Request, resp Response, elapsed time.Duration) {
	slow := elapsed > time.Second
	if routineOps[r.Op] && resp.Error == "" && !slow {
		return
	}
	outcome := "ok"
	if resp.Error != "" {
		outcome = "error: " + resp.Error
	}
	c.s.logf("conn %d %s%s -> %s (%s)", c.id, r.Op, requestDetail(r), outcome, elapsed.Round(time.Millisecond))
}

func requestDetail(r Request) string {
	var b strings.Builder
	if r.MachineID != "" {
		fmt.Fprintf(&b, " machine=%s", clean(r.MachineID, 64))
	}
	if r.WorkspaceID != "" {
		fmt.Fprintf(&b, " workspace=%s", clean(r.WorkspaceID, 64))
	}
	if r.Op == "hello" {
		fmt.Fprintf(&b, " version=%d", r.Version)
	}
	return b.String()
}

// closeReason names why a connection's read loop ended.
func closeReason(err error) string {
	switch {
	case errors.Is(err, io.EOF):
		return "closed by client"
	case errors.Is(err, os.ErrDeadlineExceeded):
		return "idle timeout"
	case errors.Is(err, net.ErrClosed):
		return "closed by daemon"
	case err == nil:
		return "invalid request"
	}
	return err.Error()
}

// serveConn handles newline-delimited messages until the client closes the
// connection. Streams speak JSON-RPC 2.0; one-shot local requests from Call send
// one plain request.
func (s *Server) serveConn(ctx context.Context, conn net.Conn) {
	c := &connState{s: s, id: s.conns.Add(1), conn: conn, enc: json.NewEncoder(conn), sem: make(chan struct{}, 8)}
	opened := time.Now()
	var readErr error
	defer func() {
		if c.sub != nil {
			s.release(c.sub, false, "")
		}
		c.workers.Wait()
		_ = conn.Close()
		if c.rpc {
			s.logf("conn %d stream closed after %s: %s", c.id, time.Since(opened).Round(time.Millisecond), closeReason(readErr))
		}
	}()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	reader := bufio.NewReader(conn)
	deadline := 5 * time.Second
	for {
		_ = conn.SetReadDeadline(time.Now().Add(deadline))
		line, err := readLine(reader, maxRequest)
		if err != nil {
			readErr = err
			return
		}
		deadline = s.IdleTimeout
		if !c.rpc && isRPC(line) {
			c.rpc = true
			s.logf("conn %d stream opened", c.id)
		}
		if c.rpc {
			c.serveRPC(ctx, line)
			continue
		}
		var r Request
		if err := json.Unmarshal(line, &r); err != nil {
			c.write(Response{Error: "invalid request"})
			return
		}
		c.spawn(ctx, r, func(resp Response) { c.write(resp) })
	}
}

func (c *connState) serveRPC(ctx context.Context, line []byte) {
	r, id, failure := decodeRPC(line)
	switch {
	case failure != nil:
		c.s.logf("conn %d rejected message: %s", c.id, failure.Message)
		c.write(rpcFailure(id, failure.Code, failure.Message))
	case id == nil:
		// JSON-RPC notifications never receive a reply.
	case !rpcMethods[r.Op]:
		c.s.logf("conn %d unknown method %s", c.id, clean(r.Op, 64))
		c.write(rpcFailure(id, codeMethodNotFound, "method not found"))
	case r.Op == "subscribe":
		c.subscribe(id, r)
	case r.Op == "complete":
		if c.sub == nil {
			c.s.logf("conn %d complete rejected: stream is not subscribed", c.id)
			c.write(rpcFailure(id, codeDaemonError, "stream is not subscribed"))
			return
		}
		c.s.complete(c.sub, r.Failure)
		c.write(rpcReply(id, r.Op, Response{}))
	case r.Op == "unsubscribe":
		if c.sub != nil {
			c.s.release(c.sub, true, r.Failure)
			c.sub = nil
		}
		c.write(rpcReply(id, r.Op, Response{}))
	default:
		c.spawn(ctx, r, func(resp Response) { c.write(rpcReply(id, r.Op, resp)) })
	}
}

// subscribe answers before any switch is pushed on this stream.
func (c *connState) subscribe(id json.RawMessage, r Request) {
	if c.sub != nil {
		c.s.logf("conn %d subscribe rejected: stream is already subscribed", c.id)
		c.write(rpcFailure(id, codeDaemonError, "stream is already subscribed"))
		return
	}
	sub, err := c.s.subscribe(c.id, r)
	if err != nil {
		c.s.logf("conn %d subscribe rejected: %v", c.id, err)
		c.write(rpcFailure(id, codeDaemonError, err.Error()))
		return
	}
	c.write(rpcReply(id, "subscribe", Response{}))
	c.sub = sub
	c.workers.Add(1)
	go func() {
		defer c.workers.Done()
		for {
			select {
			case t := <-sub.targets:
				if c.write(rpcNotification{JSONRPC: jsonrpcVersion, Method: switchNotification, Params: t}) {
					c.s.logf("conn %d switch to %s/%s pushed to controller", c.id, t.MachineID, t.WorkspaceID)
				}
			case <-sub.done:
				return
			}
		}
	}()
}

func (s *Server) Serve(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	var workers sync.WaitGroup
	defer cancel()
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
	peers := make([]string, 0, len(s.cfg.Peers))
	for _, peer := range s.cfg.Peers {
		peers = append(peers, peer.ID)
	}
	s.logf("serving machine %s (herdr session %s) on %s; peers: %s", s.cfg.ID, s.cfg.HerdrSession, p, strings.Join(peers, ","))
	defer s.logf("stopped")
	// Close transports to release blocked writes before waiting; keep socket ownership until workers stop.
	defer func() {
		cancel()
		for _, c := range s.peers {
			_ = c.Close()
		}
		workers.Wait()
	}()
	workers.Add(2)
	go func() { defer workers.Done(); <-ctx.Done(); _ = l.Close() }()
	go func() { defer workers.Done(); s.refreshCatalog(ctx) }()
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

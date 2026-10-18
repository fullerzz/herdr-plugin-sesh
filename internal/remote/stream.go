package remote

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unicode"
)

// Dialer opens one transport stream to a daemon.
type Dialer func(context.Context) (io.ReadWriteCloser, error)

const (
	minBackoff  = 250 * time.Millisecond
	maxBackoff  = 30 * time.Second
	dialTimeout = 20 * time.Second
)

var errClientClosed = errors.New("remote client closed")

// StreamClient keeps one multiplexed stream to a daemon and redials it with backoff.
type StreamClient struct {
	name string
	dial Dialer
	// PingInterval keeps the daemon's idle deadline from closing a quiet stream.
	PingInterval time.Duration

	mu      sync.Mutex
	current *stream
	dialing chan struct{}
	dialErr error
	retryAt time.Time
	backoff time.Duration
	closed  bool
}

func NewStreamClient(name string, dial Dialer) *StreamClient {
	return &StreamClient{name: name, dial: dial, PingInterval: 10 * time.Second}
}

// NewUnixClient streams to this machine's daemon socket.
func NewUnixClient(c Config) *StreamClient {
	return NewStreamClient(c.ID, func(ctx context.Context) (io.ReadWriteCloser, error) {
		p, err := SocketPath(c)
		if err != nil {
			return nil, err
		}
		return (&net.Dialer{}).DialContext(ctx, "unix", p)
	})
}

// NewSSHClient streams to a peer daemon through one long-lived ssh process.
func NewSSHClient(p Peer) *StreamClient {
	return NewStreamClient(p.ID, func(context.Context) (io.ReadWriteCloser, error) { return dialSSH(p) })
}

func (c *StreamClient) Call(ctx context.Context, r Request) (Response, error) {
	s, err := c.stream(ctx)
	if err != nil {
		return Response{}, err
	}
	return s.call(ctx, r)
}

// Subscribe registers this stream as the machine's attach controller.
// The daemon releases the controller when the stream closes.
func (c *StreamClient) Subscribe(ctx context.Context) (*Subscription, error) {
	s, err := c.stream(ctx)
	if err != nil {
		return nil, err
	}
	return s.subscribe(ctx)
}

func (c *StreamClient) Close() error {
	c.mu.Lock()
	c.closed = true
	s := c.current
	c.current = nil
	c.mu.Unlock()
	if s != nil {
		s.fail(errClientClosed)
	}
	return nil
}

func (c *StreamClient) stream(ctx context.Context) (*stream, error) {
	for {
		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			return nil, errClientClosed
		}
		if c.current != nil && c.current.alive() {
			s := c.current
			c.mu.Unlock()
			return s, nil
		}
		if c.dialing == nil {
			if wait := time.Until(c.retryAt); wait > 0 {
				err := fmt.Errorf("peer %s unavailable, retrying in %s: %w", c.name, wait.Round(time.Millisecond), c.dialErr)
				c.mu.Unlock()
				return nil, err
			}
			c.dialing = make(chan struct{})
			go c.connect(context.WithoutCancel(ctx), c.dialing)
		}
		wait := c.dialing
		c.mu.Unlock()
		select {
		case <-wait:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		c.mu.Lock()
		err := c.dialErr
		c.mu.Unlock()
		if err != nil {
			// Report the dial this caller waited for, not the backoff it caused.
			return nil, err
		}
	}
}

// connect dials independently of any caller deadline, so a slow SSH handshake
// can finish for the next caller instead of being restarted.
func (c *StreamClient) connect(base context.Context, done chan struct{}) {
	ctx, cancel := context.WithTimeout(base, dialTimeout)
	defer cancel()
	s, err := c.handshake(ctx)
	c.mu.Lock()
	defer c.mu.Unlock()
	defer close(done)
	c.dialing = nil
	if err == nil && c.closed {
		s.fail(errClientClosed)
		err = errClientClosed
	}
	if err != nil {
		c.dialErr = err
		c.backoff = min(max(2*c.backoff, minBackoff), maxBackoff)
		c.retryAt = time.Now().Add(c.backoff)
		return
	}
	c.backoff, c.dialErr, c.current = 0, nil, s
	go s.keepAlive(base, c.PingInterval)
}

func (c *StreamClient) handshake(ctx context.Context) (*stream, error) {
	rwc, err := c.dial(ctx)
	if err != nil {
		return nil, fmt.Errorf("peer %s: %w", c.name, err)
	}
	s := newStream(rwc)
	r, err := s.call(ctx, Request{Op: "hello", Version: ProtocolVersion})
	if err == nil && r.Version != ProtocolVersion {
		err = fmt.Errorf("daemon speaks protocol v%d", r.Version)
	}
	if err != nil {
		s.fail(err)
		if d, ok := rwc.(interface{ diagnostic() string }); ok {
			if msg := d.diagnostic(); msg != "" {
				err = fmt.Errorf("%w (remote: %s)", err, msg)
			}
		}
		return nil, fmt.Errorf("peer %s: protocol v%d handshake failed; install the same herdr-sesh version on both machines and restart their daemons: %w", c.name, ProtocolVersion, err)
	}
	return s, nil
}

// stream multiplexes JSON-RPC 2.0 requests by id over one transport.
type stream struct {
	rwc  io.ReadWriteCloser
	wmu  sync.Mutex
	done chan struct{}

	mu      sync.Mutex
	next    uint64
	pending map[uint64]chan Response
	// sub receives switch notifications; a stream holds at most one controller.
	sub chan Target
	err error
}

func newStream(rwc io.ReadWriteCloser) *stream {
	s := &stream{rwc: rwc, done: make(chan struct{}), pending: map[uint64]chan Response{}}
	go s.readLoop()
	return s
}

func (s *stream) alive() bool {
	select {
	case <-s.done:
		return false
	default:
		return true
	}
}

func (s *stream) fail(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return
	}
	s.err = err
	close(s.done)
	_ = s.rwc.Close()
	if s.sub != nil {
		close(s.sub)
		s.sub = nil
	}
}

func (s *stream) failure() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

func (s *stream) readLoop() {
	r := bufio.NewReader(s.rwc)
	for {
		line, err := readLine(r, maxMessage)
		if err != nil {
			if errors.Is(err, io.EOF) {
				err = io.ErrUnexpectedEOF
			}
			s.fail(fmt.Errorf("read daemon stream: %w", err))
			return
		}
		if err := s.dispatch(line); err != nil {
			s.fail(err)
			return
		}
	}
}

// dispatch routes one daemon message to its caller or to the subscription.
func (s *stream) dispatch(line []byte) error {
	var m rpcMessage
	if err := json.Unmarshal(line, &m); err != nil || m.JSONRPC != jsonrpcVersion {
		// Daemons that predate streams answer once in the plain encoding.
		var legacy Response
		if json.Unmarshal(line, &legacy) == nil && legacy.Error != "" {
			return fmt.Errorf("daemon does not speak JSON-RPC 2.0: %s", legacy.Error)
		}
		return errors.New("daemon does not speak JSON-RPC 2.0")
	}
	if m.ID == nil {
		if m.Method == switchNotification {
			var t Target
			if err := json.Unmarshal(m.Params, &t); err != nil {
				return fmt.Errorf("decode switch notification: %w", err)
			}
			s.push(t)
		}
		return nil
	}
	var id uint64
	if err := json.Unmarshal(m.ID, &id); err != nil {
		if m.Error != nil {
			return fmt.Errorf("daemon rejected stream: %s", m.Error.Message)
		}
		return errors.New("daemon replied with an unknown id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if ch, ok := s.pending[id]; ok {
		delete(s.pending, id)
		ch <- rpcResponseOf(m)
	}
	return nil
}

func (s *stream) push(t Target) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sub == nil {
		return
	}
	select {
	case s.sub <- t:
	default:
	}
}

func (s *stream) send(ctx context.Context, r Request, sub chan Target) (Response, error) {
	ch := make(chan Response, 1)
	s.mu.Lock()
	if s.err != nil {
		err := s.err
		s.mu.Unlock()
		return Response{}, err
	}
	if sub != nil {
		if s.sub != nil {
			s.mu.Unlock()
			return Response{}, errors.New("stream is already subscribed")
		}
		// Register before sending: a notification may follow the reply immediately.
		s.sub = sub
	}
	s.next++
	id := s.next
	s.pending[id] = ch
	s.mu.Unlock()
	forget := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.pending, id)
		if sub != nil && s.sub == sub {
			close(sub)
			s.sub = nil
		}
	}
	b, err := json.Marshal(newRPCRequest(id, r))
	if err != nil {
		forget()
		return Response{}, err
	}
	s.wmu.Lock()
	_, err = s.rwc.Write(append(b, '\n'))
	s.wmu.Unlock()
	if err != nil {
		// The reader usually knows why the daemon went away; give it a moment.
		select {
		case <-s.done:
		case <-time.After(time.Second):
			s.fail(fmt.Errorf("write daemon stream: %w", err))
		}
		forget()
		return Response{}, s.failure()
	}
	select {
	case resp := <-ch:
		if resp.Error != "" {
			forget()
			return resp, errors.New(resp.Error)
		}
		return resp, nil
	case <-s.done:
		forget()
		return Response{}, s.failure()
	case <-ctx.Done():
		forget()
		return Response{}, ctx.Err()
	}
}

func (s *stream) call(ctx context.Context, r Request) (Response, error) {
	return s.send(ctx, r, nil)
}

func (s *stream) subscribe(ctx context.Context) (*Subscription, error) {
	ch := make(chan Target, 4)
	if _, err := s.send(ctx, Request{Op: "subscribe"}, ch); err != nil {
		return nil, err
	}
	return &Subscription{stream: s, targets: ch}, nil
}

func (s *stream) keepAlive(base context.Context, interval time.Duration) {
	if interval <= 0 {
		return
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-t.C:
			ctx, cancel := context.WithTimeout(base, interval)
			_, err := s.call(ctx, Request{Op: "ping"})
			cancel()
			if err != nil {
				s.fail(fmt.Errorf("daemon stream keepalive: %w", err))
				return
			}
		}
	}
}

// Subscription receives switch targets pushed to this machine's controller.
type Subscription struct {
	stream  *stream
	targets chan Target
}

// Targets is closed when the stream, and with it the controller, is lost.
func (s *Subscription) Targets() <-chan Target { return s.targets }

func (s *Subscription) Err() error { return s.stream.failure() }

// Close releases the controller while keeping the shared stream open.
func (s *Subscription) Close() {
	s.stream.mu.Lock()
	ok := s.stream.sub == s.targets
	if ok {
		close(s.targets)
		s.stream.sub = nil
	}
	s.stream.mu.Unlock()
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := s.stream.call(ctx, Request{Op: "unsubscribe"}); err != nil {
		// Closing the stream is the authoritative release.
		s.stream.fail(fmt.Errorf("release controller: %w", err))
	}
}

func readLine(r *bufio.Reader, limit int) ([]byte, error) {
	var line []byte
	for {
		chunk, err := r.ReadSlice('\n')
		if len(line)+len(chunk) > limit {
			return nil, fmt.Errorf("message exceeds %d bytes", limit)
		}
		line = append(line, chunk...)
		switch {
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF) && len(bytes.TrimSpace(line)) > 0:
			return line, nil
		case err != nil:
			return nil, err
		case len(bytes.TrimSpace(line)) == 0:
			line = line[:0]
			continue
		}
		return line, nil
	}
}

func SSHArgs(p Peer) []string {
	return []string{
		"-o", "BatchMode=yes", "-o", "ForwardAgent=no", "-o", "ConnectTimeout=5",
		"-o", "ServerAliveInterval=10", "-o", "ServerAliveCountMax=3",
		"--", p.SSH, "herdr-sesh", "daemon", "rpc", "--stream",
	}
}

// sshConn is a stdio stream to `herdr-sesh daemon rpc --stream` on a peer.
type sshConn struct {
	cmd    *exec.Cmd
	stdin  *os.File
	stdout *os.File
	stderr limitedBuffer
	waited chan struct{}
	once   sync.Once
}

func dialSSH(p Peer) (io.ReadWriteCloser, error) {
	if !sshHost.MatchString(p.SSH) {
		return nil, errors.New("invalid SSH destination")
	}
	inR, inW, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		_, _ = inR.Close(), inW.Close()
		return nil, err
	}
	c := &sshConn{stdin: inW, stdout: outR, waited: make(chan struct{})}
	c.stderr.limit = 4 << 10
	c.cmd = exec.Command("ssh", SSHArgs(p)...) // #nosec G204 -- validated SSH destination and fixed remote command.
	c.cmd.Stdin, c.cmd.Stdout, c.cmd.Stderr = inR, outW, &c.stderr
	err = c.cmd.Start()
	_, _ = inR.Close(), outW.Close()
	if err != nil {
		_, _ = inW.Close(), outR.Close()
		return nil, err
	}
	go func() { _ = c.cmd.Wait(); close(c.waited) }()
	return c, nil
}

func (c *sshConn) Read(p []byte) (int, error)  { return c.stdout.Read(p) }
func (c *sshConn) Write(p []byte) (int, error) { return c.stdin.Write(p) }

// Close ends the remote command by closing its stdin, then stops ssh.
func (c *sshConn) Close() error {
	c.once.Do(func() {
		_ = c.stdin.Close()
		select {
		case <-c.waited:
		case <-time.After(time.Second):
			_ = c.cmd.Process.Kill()
			<-c.waited
		}
		_ = c.stdout.Close()
	})
	return nil
}

// diagnostic reports the ssh or remote error output once ssh has exited.
func (c *sshConn) diagnostic() string {
	select {
	case <-c.waited:
	case <-time.After(time.Second):
		return ""
	}
	msg := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, c.stderr.String())
	msg = strings.Join(strings.Fields(msg), " ")
	if len(msg) > 512 {
		msg = msg[:512]
	}
	return msg
}

type limitedBuffer struct {
	bytes.Buffer

	limit int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if room := b.limit - b.Len(); room < len(p) {
		_, _ = b.Buffer.Write(p[:max(room, 0)])
		return len(p), nil
	}
	return b.Buffer.Write(p)
}

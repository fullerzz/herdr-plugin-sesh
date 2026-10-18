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

// stream multiplexes newline-delimited requests by id over one transport.
type stream struct {
	rwc  io.ReadWriteCloser
	wmu  sync.Mutex
	done chan struct{}

	mu      sync.Mutex
	next    uint64
	pending map[uint64]chan Response
	subs    map[uint64]chan Target
	err     error
}

func newStream(rwc io.ReadWriteCloser) *stream {
	s := &stream{rwc: rwc, done: make(chan struct{}), pending: map[uint64]chan Response{}, subs: map[uint64]chan Target{}}
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
	for id, ch := range s.subs {
		close(ch)
		delete(s.subs, id)
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
		var resp Response
		if err := json.Unmarshal(line, &resp); err != nil {
			s.fail(fmt.Errorf("decode daemon stream: %w", err))
			return
		}
		if resp.ID == 0 {
			// Daemons without stream support answer once without an id.
			s.fail(fmt.Errorf("daemon does not support streams: %s", resp.Error))
			return
		}
		s.route(resp)
	}
}

func (s *stream) route(resp Response) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ch, ok := s.pending[resp.ID]; ok {
		delete(s.pending, resp.ID)
		ch <- resp
		return
	}
	if ch, ok := s.subs[resp.ID]; ok && resp.Switch != nil {
		select {
		case ch <- *resp.Switch:
		default:
		}
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
	s.next++
	r.ID = s.next
	s.pending[r.ID] = ch
	if sub != nil {
		// Register before sending: a push may follow the response immediately.
		s.subs[r.ID] = sub
	}
	s.mu.Unlock()
	forget := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.pending, r.ID)
		if _, ok := s.subs[r.ID]; ok && sub != nil {
			delete(s.subs, r.ID)
			close(sub)
		}
	}
	b, err := json.Marshal(r)
	if err != nil {
		forget()
		return Response{}, err
	}
	s.wmu.Lock()
	_, err = s.rwc.Write(append(b, '\n'))
	s.wmu.Unlock()
	if err != nil {
		s.fail(fmt.Errorf("write daemon stream: %w", err))
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
	r, err := s.send(ctx, Request{Op: "subscribe"}, ch)
	if err != nil {
		return nil, err
	}
	return &Subscription{stream: s, id: r.ID, targets: ch}, nil
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
	id      uint64
	targets chan Target
}

// Targets is closed when the stream, and with it the controller, is lost.
func (s *Subscription) Targets() <-chan Target { return s.targets }

func (s *Subscription) Err() error { return s.stream.failure() }

// Close releases the controller while keeping the shared stream open.
func (s *Subscription) Close() {
	s.stream.mu.Lock()
	_, ok := s.stream.subs[s.id]
	if ok {
		delete(s.stream.subs, s.id)
		close(s.targets)
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

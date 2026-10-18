package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/fullerzz/herdr-plugin-sesh/internal/remote"
)

type attachEndpoint struct {
	id   string
	peer *remote.Peer
}

// attachSubscription is this wrapper's controller registration on one machine.
type attachSubscription struct {
	targets <-chan remote.Target
	err     func() error
	close   func()
}

type attachTransport interface {
	Call(context.Context, attachEndpoint, remote.Request) (remote.Response, error)
	Subscribe(context.Context, attachEndpoint) (attachSubscription, error)
}

// streamTransport keeps one daemon stream per machine for the wrapper's lifetime.
type streamTransport struct {
	cfg     remote.Config
	mu      sync.Mutex
	clients map[string]*remote.StreamClient
}

func (t *streamTransport) client(endpoint attachEndpoint) *remote.StreamClient {
	t.mu.Lock()
	defer t.mu.Unlock()
	if c := t.clients[endpoint.id]; c != nil {
		return c
	}
	c := remote.NewUnixClient(t.cfg)
	if endpoint.peer != nil {
		c = remote.NewSSHClient(*endpoint.peer)
	}
	t.clients[endpoint.id] = c
	return c
}

func (t *streamTransport) Call(ctx context.Context, endpoint attachEndpoint, req remote.Request) (remote.Response, error) {
	return t.client(endpoint).Call(ctx, req)
}

func (t *streamTransport) Subscribe(ctx context.Context, endpoint attachEndpoint) (attachSubscription, error) {
	sub, err := t.client(endpoint).Subscribe(ctx)
	if err != nil {
		return attachSubscription{}, err
	}
	return attachSubscription{targets: sub.Targets(), err: sub.Err, close: sub.Close}, nil
}

func (t *streamTransport) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, c := range t.clients {
		_ = c.Close()
	}
}

func (a *App) attach(ctx context.Context, args []string) error {
	if len(args) > 1 {
		return errors.New("usage: herdr-sesh attach [machine-id]")
	}
	if os.Getenv("HERDR_ENV") != "" || os.Getenv("HERDR_PANE_ID") != "" {
		return errors.New("run herdr-sesh attach from your outer terminal, outside Herdr")
	}
	if !term.IsTerminal(os.Stdin.Fd()) || !term.IsTerminal(os.Stdout.Fd()) {
		return errors.New("herdr-sesh attach requires an interactive terminal")
	}
	cfg, err := remote.LoadConfig()
	if err != nil {
		return err
	}
	if !cfg.Enabled {
		return errors.New("remote workspace support is disabled; configure remote.toml explicitly")
	}
	id := cfg.ID
	for _, arg := range args {
		id = arg
	}
	endpoint, err := resolveAttachEndpoint(cfg, id)
	if err != nil {
		return err
	}
	state, err := term.GetState(os.Stdin.Fd())
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	transport := &streamTransport{cfg: cfg, clients: map[string]*remote.StreamClient{}}
	defer transport.Close()
	bin := os.Getenv("HERDR_BIN_PATH")
	if bin == "" {
		bin = "herdr"
	}
	return runAttach(ctx, cfg, endpoint, transport, bin, os.Stdin, os.Stdout, a.Err, func() error {
		return term.Restore(os.Stdin.Fd(), state)
	})
}

func resolveAttachEndpoint(cfg remote.Config, id string) (attachEndpoint, error) {
	if id == cfg.ID {
		return attachEndpoint{id: id}, nil
	}
	for _, peer := range cfg.Peers {
		if peer.ID == id {
			return attachEndpoint{id: id, peer: &peer}, nil
		}
	}
	return attachEndpoint{}, fmt.Errorf("machine %q is not in this client's remote.toml allowlist", id)
}

func attachArgs(cfg remote.Config, endpoint attachEndpoint) []string {
	if endpoint.peer != nil {
		return []string{"--remote", endpoint.peer.SSH, "--session", endpoint.peer.Session}
	}
	return []string{"--session", cfg.HerdrSession}
}

func attachEnvironment(environ []string) []string {
	result := make([]string, 0, len(environ))
	for _, entry := range environ {
		if !strings.HasPrefix(entry, "HERDR_") || strings.HasPrefix(entry, "HERDR_SESH_REMOTE_CONFIG=") {
			result = append(result, entry)
		}
	}
	return result
}

// resubscribeDelays bounds how long a lost controller stream may take to recover.
var resubscribeDelays = []time.Duration{0, time.Second, 2 * time.Second, 4 * time.Second}

func runAttach(ctx context.Context, cfg remote.Config, current attachEndpoint, transport attachTransport, bin string, in io.Reader, out, errOut io.Writer, restore func() error) (result error) {
	// SSH streams may still be completing their first handshake.
	request := func(ctx context.Context, endpoint attachEndpoint, req remote.Request) (remote.Response, error) {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		return transport.Call(ctx, endpoint, req)
	}
	register := func(endpoint attachEndpoint) (attachSubscription, error) {
		exported, err := request(ctx, endpoint, remote.Request{Op: "export"})
		if err != nil {
			return attachSubscription{}, err
		}
		session := cfg.HerdrSession
		if endpoint.peer != nil {
			session = endpoint.peer.Session
		}
		if len(exported.Catalog) != 1 || exported.Catalog[0].ID != endpoint.id || exported.Catalog[0].Session != session {
			return attachSubscription{}, errors.New("daemon machine or session does not match this client's configuration")
		}
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		return transport.Subscribe(ctx, endpoint)
	}
	resubscribe := func(endpoint attachEndpoint, lost error) (attachSubscription, error) {
		err := lost
		for _, delay := range resubscribeDelays {
			select {
			case <-ctx.Done():
				return attachSubscription{}, ctx.Err()
			case <-time.After(delay):
			}
			var sub attachSubscription
			if sub, err = register(endpoint); err == nil {
				return sub, nil
			}
		}
		return attachSubscription{}, err
	}
	sub, err := register(current)
	if err != nil {
		return fmt.Errorf("register attachment controller: %w", err)
	}
	defer func() { sub.close() }()
	defer func() { result = errors.Join(result, restore()) }()
	for {
		// Only this child is replaced. Herdr's session server and pane processes survive.
		//nolint:gosec // bin is a user-selected Herdr executable; arguments are never shell-expanded.
		child := exec.Command(bin, attachArgs(cfg, current)...)
		child.Env = attachEnvironment(os.Environ())
		child.Stdin, child.Stdout, child.Stderr = in, out, errOut
		if err := child.Start(); err != nil {
			return err
		}
		done := make(chan error, 1)
		go func() { done <- child.Wait() }()
		stopChild := func() error {
			return errors.Join(stopAttachment(child, done, 2*time.Second), restore())
		}
		switching := false
		for !switching {
			select {
			case err := <-done:
				return err
			case <-ctx.Done():
				return errors.Join(ctx.Err(), stopChild())
			case target, ok := <-sub.targets:
				if !ok {
					next, err := resubscribe(current, sub.err())
					if err != nil {
						return errors.Join(fmt.Errorf("attachment controller lost: %w", err), stopChild())
					}
					sub = next
					continue
				}
				next, err := resolveAttachEndpoint(cfg, target.MachineID)
				nextSub := sub
				if err == nil && next.id != current.id {
					nextSub, err = register(next)
				}
				if err != nil {
					_, _ = fmt.Fprintf(errOut, "\r\nRemote switch failed: %v\r\n", err)
					continue
				}
				_, err = request(ctx, next, remote.Request{Op: "focus", WorkspaceID: target.WorkspaceID})
				if err != nil {
					if next.id != current.id {
						nextSub.close()
					}
					_, _ = fmt.Fprintf(errOut, "\r\nRemote switch failed: %v\r\n", err)
					continue
				}
				if next.id == current.id {
					continue
				}
				if err := stopChild(); err != nil {
					nextSub.close()
					return err
				}
				sub.close()
				sub, current = nextSub, next
				switching = true
			}
		}
	}
}

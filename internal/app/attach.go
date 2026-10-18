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
// The zero value is a lost registration: it delivers nothing.
type attachSubscription struct {
	targets  <-chan remote.Target
	err      func() error
	close    func()
	complete func(context.Context, error) error
}

func (s attachSubscription) release() {
	if s.close != nil {
		s.close()
	}
}

// finish tells the daemon a delivered switch is over, so it accepts the next.
func (s attachSubscription) finish(ctx context.Context, failure error) {
	if s.complete == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// A lost stream releases the controller, which also ends the switch.
	_ = s.complete(ctx, failure)
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
	// The daemon rejects switches this wrapper could not attach to.
	machines := make([]string, 0, 1+len(t.cfg.Peers))
	machines = append(machines, t.cfg.ID)
	for _, peer := range t.cfg.Peers {
		machines = append(machines, peer.ID)
	}
	sub, err := t.client(endpoint).Subscribe(ctx, machines)
	if err != nil {
		return attachSubscription{}, err
	}
	return attachSubscription{targets: sub.Targets(), err: sub.Err, close: sub.Close, complete: sub.Complete}, nil
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
	// A closed terminal sends SIGHUP; stop the client and release the controller.
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
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

// herdrContextVariables are set by Herdr inside panes, hooks, and plugins. They
// would point a new client at the wrong server, session, or pane.
var herdrContextVariables = map[string]bool{
	"HERDR_ENV": true, "HERDR_SESSION": true, "HERDR_SOCKET_PATH": true, "HERDR_CLIENT_SOCKET_PATH": true,
	"HERDR_WORKSPACE_ID": true, "HERDR_TAB_ID": true, "HERDR_PANE_ID": true, "HERDR_PANE_RUNTIME_ID": true,
	"HERDR_ACTION": true, "HERDR_HOOK_INPUT_FILE": true,
}

// attachEnvironment keeps user configuration such as HERDR_CONFIG_PATH and drops
// only the variables Herdr injects into its own processes.
func attachEnvironment(environ []string) []string {
	result := make([]string, 0, len(environ))
	for _, entry := range environ {
		name, _, _ := strings.Cut(entry, "=")
		if herdrContextVariables[name] || strings.HasPrefix(name, "HERDR_PLUGIN_") || strings.HasPrefix(name, "HERDR_ACTIVE_") {
			continue
		}
		result = append(result, entry)
	}
	return result
}

// resubscribeDelays paces controller recovery; the last delay repeats.
var resubscribeDelays = []time.Duration{0, time.Second, 2 * time.Second, 5 * time.Second, 15 * time.Second, 30 * time.Second}

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
	// Losing the controller only disables switching; the attached client stays.
	retryCtx, cancelRetry := context.WithCancel(ctx)
	defer cancelRetry()
	resubscribed := make(chan attachSubscription)
	resubscribe := func(endpoint attachEndpoint) {
		go func() {
			for attempt := 0; ; attempt++ {
				select {
				case <-retryCtx.Done():
					return
				case <-time.After(resubscribeDelays[min(attempt, len(resubscribeDelays)-1)]):
				}
				sub, err := register(endpoint)
				if err != nil {
					continue
				}
				select {
				case resubscribed <- sub:
				case <-retryCtx.Done():
					sub.release()
				}
				return
			}
		}()
	}
	sub, err := register(current)
	if err != nil {
		return fmt.Errorf("register attachment controller: %w", err)
	}
	defer func() { sub.release() }()
	defer func() { result = errors.Join(result, restore()) }()
	report := func(err error) { _, _ = fmt.Fprintf(errOut, "\r\nRemote switch failed: %v\r\n", err) }
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
			case next := <-resubscribed:
				sub = next
			case target, ok := <-sub.targets:
				if !ok {
					_, _ = fmt.Fprintf(errOut, "\r\nRemote switching unavailable, reconnecting in the background: %v\r\n", sub.err())
					sub = attachSubscription{}
					resubscribe(current)
					continue
				}
				next, err := resolveAttachEndpoint(cfg, target.MachineID)
				nextSub := sub
				if err == nil && next.id != current.id {
					nextSub, err = register(next)
				}
				if err == nil {
					_, err = request(ctx, next, remote.Request{Op: "focus", WorkspaceID: target.WorkspaceID})
					if err != nil && next.id != current.id {
						nextSub.release()
					}
				}
				if err != nil || next.id == current.id {
					if err != nil {
						report(err)
					}
					sub.finish(ctx, err)
					continue
				}
				if err := stopChild(); err != nil {
					nextSub.release()
					return err
				}
				// Releasing the old controller also ends its in-flight switch.
				sub.release()
				sub, current = nextSub, next
				switching = true
			}
		}
	}
}

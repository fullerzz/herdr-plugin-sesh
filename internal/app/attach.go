package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/fullerzz/herdr-plugin-sesh/internal/remote"
)

type attachEndpoint struct {
	id   string
	peer *remote.Peer
}

type attachCall func(context.Context, attachEndpoint, remote.Request) (remote.Response, error)

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
	call := func(ctx context.Context, endpoint attachEndpoint, req remote.Request) (remote.Response, error) {
		if endpoint.peer == nil {
			return remote.Call(ctx, cfg, req)
		}
		return remote.SSHCall(ctx, *endpoint.peer, req)
	}
	bin := os.Getenv("HERDR_BIN_PATH")
	if bin == "" {
		bin = "herdr"
	}
	return runAttach(ctx, cfg, endpoint, call, bin, os.Stdin, os.Stdout, a.Err, func() error {
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

func runAttach(ctx context.Context, cfg remote.Config, current attachEndpoint, call attachCall, bin string, in io.Reader, out, errOut io.Writer, restore func() error) (result error) {
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return err
	}
	token := hex.EncodeToString(secret[:])
	request := func(ctx context.Context, endpoint attachEndpoint, req remote.Request) (remote.Response, error) {
		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		return call(ctx, endpoint, req)
	}
	unregister := func(endpoint attachEndpoint) {
		_, _ = request(context.WithoutCancel(ctx), endpoint, remote.Request{Op: "unregister", Token: token})
	}
	register := func(endpoint attachEndpoint) error {
		exported, err := request(ctx, endpoint, remote.Request{Op: "export"})
		if err != nil {
			return err
		}
		session := cfg.HerdrSession
		if endpoint.peer != nil {
			session = endpoint.peer.Session
		}
		if len(exported.Catalog) != 1 || exported.Catalog[0].ID != endpoint.id || exported.Catalog[0].Session != session {
			return errors.New("daemon machine or session does not match this client's configuration")
		}
		_, err = request(ctx, endpoint, remote.Request{Op: "register", Token: token})
		return err
	}
	if err := register(current); err != nil {
		return fmt.Errorf("register attachment controller: %w", err)
	}
	defer func() { unregister(current) }()
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
		pollCtx, cancelPoll := context.WithCancel(ctx)
		updates := make(chan remote.Target)
		failures := make(chan error, 1)
		pollDone := make(chan struct{})
		endpoint := current
		go func() {
			defer close(pollDone)
			pollAttachment(pollCtx, endpoint, token, request, updates, failures)
		}()
		stopPolling := func() {
			cancelPoll()
			<-pollDone
		}
		stopChild := func() error {
			return errors.Join(stopAttachment(child, done, 2*time.Second), restore())
		}
		switching := false
		for !switching {
			select {
			case err := <-done:
				stopPolling()
				return err
			case <-ctx.Done():
				stopPolling()
				return errors.Join(ctx.Err(), stopChild())
			case err := <-failures:
				stopPolling()
				return errors.Join(fmt.Errorf("attachment controller lost: %w", err), stopChild())
			case target := <-updates:
				next, err := resolveAttachEndpoint(cfg, target.MachineID)
				if err == nil && next.id != current.id {
					err = register(next)
				}
				if err != nil {
					_, _ = fmt.Fprintf(errOut, "\r\nRemote switch failed: %v\r\n", err)
					continue
				}
				_, err = request(ctx, next, remote.Request{Op: "focus", WorkspaceID: target.WorkspaceID})
				if err != nil {
					if next.id != current.id {
						unregister(next)
					}
					_, _ = fmt.Fprintf(errOut, "\r\nRemote switch failed: %v\r\n", err)
					continue
				}
				if next.id == current.id {
					continue
				}
				stopPolling()
				if err := stopChild(); err != nil {
					unregister(next)
					return err
				}
				unregister(current)
				current = next
				switching = true
			}
		}
	}
}

func pollAttachment(ctx context.Context, endpoint attachEndpoint, token string, call attachCall, updates chan<- remote.Target, failures chan<- error) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	failed := 0
	deliveredID := ""
	for {
		response, err := call(ctx, endpoint, remote.Request{Op: "poll", Token: token})
		if ctx.Err() != nil {
			return
		}
		if err == nil && response.Switch != nil {
			target := *response.Switch
			if target.RequestID == "" {
				err = errors.New("daemon returned a switch without a request ID")
			} else {
				if target.RequestID != deliveredID {
					select {
					case updates <- target:
						deliveredID = target.RequestID
					case <-ctx.Done():
						return
					}
				}
				// A lost acknowledgement may replay the request, but never the action.
				_, err = call(ctx, endpoint, remote.Request{Op: "ack", Token: token, RequestID: target.RequestID})
			}
		}
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			failed++
			if failed >= 3 {
				failures <- err
				return
			}
		} else {
			failed = 0
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

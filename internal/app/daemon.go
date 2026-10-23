package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"slices"
	"syscall"
	"time"

	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
	"github.com/fullerzz/herdr-plugin-sesh/internal/remote"
)

func (a *App) daemon(ctx context.Context, args []string) error {
	// rpc is the fixed SSH entry point; workspace metadata never becomes shell code.
	stream := slices.Equal(args, []string{"rpc", "--stream"})
	if !stream && (len(args) != 1 || args[0] == "rpc") {
		return errors.New("usage: herdr-sesh daemon serve|list|rpc --stream")
	}
	cfg, err := remote.LoadConfig()
	if err != nil {
		return err
	}
	if !cfg.Enabled {
		return errors.New("remote workspace support is disabled; configure remote.toml explicitly")
	}
	switch args[0] {
	case "serve":
		ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
		defer stop()
		client := herdr.NewCLIClient()
		client.Runner = daemonHerdrRunner{session: cfg.HerdrSession}
		server := remote.NewServer(cfg, client)
		server.Logf = log.New(a.Err, "herdr-sesh daemon: ", log.LstdFlags|log.Lmicroseconds|log.Lmsgprefix).Printf
		return server.Serve(ctx)
	case "list":
		ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		response, err := remote.Call(ctx, cfg, remote.Request{Op: "list"})
		if err != nil {
			return err
		}
		return json.NewEncoder(a.Out).Encode(response)
	case "rpc":
		return remote.Relay(ctx, cfg, os.Stdin, a.Out)
	default:
		return fmt.Errorf("unknown daemon command %q", args[0])
	}
}

type daemonHerdrRunner struct{ session string }

func (r daemonHerdrRunner) Run(ctx context.Context, bin string, args ...string) ([]byte, []byte, error) {
	args = append([]string{"--session", r.session}, args...)
	//nolint:gosec // The executable is user configured; the session is a validated argument.
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = attachEnvironment(os.Environ())
	cmd.WaitDelay = time.Second
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	err := cmd.Run()
	return out.Bytes(), stderr.Bytes(), err
}

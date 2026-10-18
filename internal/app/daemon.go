package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
	"github.com/fullerzz/herdr-plugin-sesh/internal/remote"
)

func (a *App) daemon(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: herdr-sesh daemon serve|rpc|list")
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
		ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
		defer stop()
		client := herdr.NewCLIClient()
		client.Runner = daemonHerdrRunner{session: cfg.HerdrSession}
		return remote.NewServer(cfg, client).Serve(ctx)
	case "list":
		ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		response, err := remote.Call(ctx, cfg, remote.Request{Op: "list"})
		if err != nil {
			return err
		}
		return json.NewEncoder(a.Out).Encode(response)
	case "rpc":
		return daemonRPC(ctx, cfg, os.Stdin, a.Out)
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

// RPC is a fixed SSH entry point; workspace metadata never becomes shell code.
func daemonRPC(ctx context.Context, cfg remote.Config, in io.Reader, out io.Writer) error {
	const maxRequest = 16 * 1024
	data, err := io.ReadAll(io.LimitReader(in, maxRequest+1))
	if err != nil {
		return err
	}
	if len(data) > maxRequest {
		return errors.New("daemon request exceeds 16 KiB")
	}
	var request remote.Request
	if err := json.Unmarshal(data, &request); err != nil {
		return fmt.Errorf("decode daemon request: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	response, err := remote.Call(ctx, cfg, request)
	if err != nil {
		response = remote.Response{Error: err.Error()}
	}
	return json.NewEncoder(out).Encode(response)
}

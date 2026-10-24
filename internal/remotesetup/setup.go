package remotesetup

import (
	"cmp"
	"context"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"

	"charm.land/huh/v2"
	"github.com/fullerzz/herdr-plugin-sesh/internal/remote"
)

// Configure collects configuration without writing files or contacting peers.
func Configure(ctx context.Context, initial remote.Config, in *os.File, out io.Writer) (remote.Config, error) {
	hostname, _ := os.Hostname()
	accessible, _ := strconv.ParseBool(os.Getenv("ACCESSIBLE"))
	accessible = accessible || os.Getenv("TERM") == "dumb"
	input := &accessibleInput{reader: in, ctx: ctx}
	return configure(initial, hostname, func(fields ...huh.Field) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		form := huh.NewForm(huh.NewGroup(fields...)).WithInput(in).WithOutput(out)
		if accessible {
			form.WithAccessible(true).WithInput(input)
		}
		if err := form.RunWithContext(ctx); err != nil {
			return err
		}
		if input.err != nil {
			return input.err
		}
		return ctx.Err()
	})
}

func configure(initial remote.Config, hostname string, run func(...huh.Field) error) (remote.Config, error) {
	cfg := initial
	cfg.Peers = slices.Clone(initial.Peers)
	cfg.Enabled = true
	defaultID := cmp.Or(cfg.ID, machineID(hostname))
	cfg.ID = defaultID
	cfg.HerdrSession = cmp.Or(cfg.HerdrSession, "default")
	for i := range cfg.Peers {
		cfg.Peers[i].Session = cmp.Or(cfg.Peers[i].Session, "default")
	}
	if err := run(
		huh.NewInput().Title("Machine ID").Description("A stable, unique name for this machine.").Value(&cfg.ID).
			Validate(func(value string) error {
				return (remote.Config{Enabled: true, ID: cmp.Or(value, defaultID)}).Validate()
			}),
		huh.NewInput().Title("Herdr session").Value(&cfg.HerdrSession).Validate(validateSession),
	); err != nil {
		return remote.Config{}, err
	}
	cfg.ID = cmp.Or(cfg.ID, defaultID)
	cfg.HerdrSession = cmp.Or(cfg.HerdrSession, "default")
	for {
		var add bool
		if err := run(huh.NewConfirm().Title("Add an SSH peer?").Description("Use an existing SSH alias or user@host. No connection is made during setup.").Value(&add)); err != nil {
			return remote.Config{}, err
		}
		if !add {
			if err := cfg.Validate(); err != nil {
				return remote.Config{}, err
			}
			return cfg, nil
		}
		peer := remote.Peer{Session: "default"}
		if err := run(
			huh.NewInput().Title("Peer machine ID").Value(&peer.ID).Validate(func(value string) error {
				candidate := cfg
				candidate.Peers = append(slices.Clone(cfg.Peers), remote.Peer{ID: value, SSH: "host"})
				return candidate.Validate()
			}),
			huh.NewInput().Title("SSH alias or user@host").Value(&peer.SSH).Validate(func(value string) error {
				return (remote.Config{Enabled: true, ID: cfg.ID, Peers: []remote.Peer{{ID: peer.ID, SSH: value}}}).Validate()
			}),
			huh.NewInput().Title("Peer Herdr session").Value(&peer.Session).Validate(validateSession),
		); err != nil {
			return remote.Config{}, err
		}
		peer.Session = cmp.Or(peer.Session, "default")
		cfg.Peers = append(cfg.Peers, peer)
	}
}

func validateSession(value string) error {
	return (remote.Config{Enabled: true, ID: "machine", HerdrSession: value}).Validate()
}

func machineID(hostname string) string {
	id := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-' {
			return r
		}
		return '-'
	}, hostname)
	id = strings.TrimLeft(id, "._-")
	if len(id) > 128 {
		id = id[:128]
	}
	return cmp.Or(id, "machine")
}

// Huh's accessible prompts swallow read errors and buffer past each answer.
type accessibleInput struct {
	reader io.Reader
	ctx    context.Context
	err    error
}

func (r *accessibleInput) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		r.err = err
		return 0, err
	}
	if len(p) > 1 {
		p = p[:1]
	}
	n, err := r.reader.Read(p)
	if err != nil {
		r.err = err
	}
	return n, err
}

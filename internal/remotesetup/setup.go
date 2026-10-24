package remotesetup

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/fullerzz/herdr-plugin-sesh/internal/remote"
	"github.com/fullerzz/herdr-plugin-sesh/internal/uitheme"
)

// Configure collects configuration without writing files or contacting peers.
func Configure(ctx context.Context, initial remote.Config, palette uitheme.Palette, in *os.File, out io.Writer) (remote.Config, error) {
	hostname, _ := os.Hostname()
	accessible, _ := strconv.ParseBool(os.Getenv("ACCESSIBLE"))
	accessible = accessible || os.Getenv("TERM") == "dumb"
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		palette = uitheme.Palette{}
	}
	input := &accessibleInput{reader: in, ctx: ctx}
	theme := formTheme(palette)
	return configure(initial, hostname, palette, out, func(fields ...huh.Field) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		form := huh.NewForm(huh.NewGroup(fields...)).WithTheme(theme).WithInput(in).WithOutput(out)
		if accessible {
			form.WithAccessible(true).WithInput(input).WithOutput(colorprofile.NewWriter(out, os.Environ()))
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

func configure(initial remote.Config, hostname string, palette uitheme.Palette, out io.Writer, run func(...huh.Field) error) (remote.Config, error) {
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
		newInput("Machine ID", "A stable, unique name for this machine.", &cfg.ID,
			func(value string) error {
				return (remote.Config{Enabled: true, ID: cmp.Or(value, defaultID)}).Validate()
			}),
		newInput("Herdr session", "", &cfg.HerdrSession, validateSession),
	); err != nil {
		return remote.Config{}, err
	}
	cfg.ID = cmp.Or(cfg.ID, defaultID)
	cfg.HerdrSession = cmp.Or(cfg.HerdrSession, "default")
	for {
		var add bool
		confirm := huh.NewConfirm().Title("Add an SSH peer before continuing?").
			Description("Use an existing SSH alias or user@host. No connection is made during setup.").
			Affirmative("Add machine").Negative("Continue").Value(&add)
		if len(cfg.Peers) > 0 {
			confirm.Title("Add another SSH peer before continuing?").Affirmative("Add another machine")
		}
		if err := run(confirm); err != nil {
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
			newInput("Peer machine ID", "", &peer.ID, func(value string) error {
				candidate := cfg
				candidate.Peers = append(slices.Clone(cfg.Peers), remote.Peer{ID: value, SSH: "host"})
				return candidate.Validate()
			}),
			newInput("SSH alias or user@host", "", &peer.SSH, func(value string) error {
				return (remote.Config{Enabled: true, ID: cfg.ID, Peers: []remote.Peer{{ID: peer.ID, SSH: value}}}).Validate()
			}),
			newInput("Peer Herdr session", "", &peer.Session, validateSession),
		); err != nil {
			return remote.Config{}, err
		}
		peer.Session = cmp.Or(peer.Session, "default")
		cfg.Peers = append(cfg.Peers, peer)
		message := fmt.Sprintf("Peer %q added to setup (SSH: %s).", peer.ID, peer.SSH)
		if _, err := lipgloss.Fprintln(out, lipgloss.NewStyle().Foreground(palette.Green).Bold(true).Render(message)); err != nil {
			return remote.Config{}, err
		}
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

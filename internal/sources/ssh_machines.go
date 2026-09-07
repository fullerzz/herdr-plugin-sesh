package sources

import (
	"context"

	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
)

// SSHMachines contains validated catalog entries from herdr machine list.
type SSHMachines []herdr.Machine

func (SSHMachines) Name() string { return "ssh" }

func (machines SSHMachines) List(context.Context) (model.Sessions, error) {
	out := model.NewSessions()
	for _, machine := range machines {
		out.Add(model.Session{Source: "ssh", Name: machine.Label, SSH: &model.SSHMachine{
			ID: machine.ID, Target: machine.Target, RemoteSession: machine.Session, Enabled: machine.Enabled,
		}})
	}
	return out, nil
}

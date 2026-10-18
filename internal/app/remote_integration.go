package app

import (
	"context"
	"errors"
	"time"

	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/fullerzz/herdr-plugin-sesh/internal/remote"
)

func (a *App) remoteSessions(ctx context.Context) []model.Session {
	cfg, err := remote.LoadConfig()
	if err != nil {
		a.warnf("remote catalog configuration: %v", err)
		return nil
	}
	if !cfg.Enabled {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	response, err := remote.Call(ctx, cfg, remote.Request{Op: "list"})
	if err != nil {
		a.warnf("remote catalog unavailable: %v", err)
		return nil
	}
	var sessions []model.Session
	for _, machine := range response.Catalog {
		if machine.ID == cfg.ID {
			continue
		}
		for _, workspace := range machine.Workspaces {
			sessions = append(sessions, model.Session{Source: model.SourceRemote, Name: workspace.Label, Path: workspace.CWD, WorkspaceID: workspace.ID, RemoteMachine: machine.ID, RemoteSession: machine.Session, RemoteOffline: machine.Stale, AgentStatus: workspace.AgentStatus})
		}
	}
	return sessions
}

func (a *App) switchRemote(ctx context.Context, session model.Session) error {
	if session.RemoteOffline {
		return errors.New("remote machine is offline; refresh the catalog before switching")
	}
	cfg, err := remote.LoadConfig()
	if err != nil {
		return err
	}
	if !cfg.Enabled {
		return errors.New("remote sessions are disabled")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_, err = remote.Call(ctx, cfg, remote.Request{Op: "switch", MachineID: session.RemoteMachine, WorkspaceID: session.WorkspaceID})
	return err
}

func localSessions(sessions []model.Session) []model.Session {
	local := make([]model.Session, 0, len(sessions))
	for _, session := range sessions {
		if session.Source != model.SourceRemote {
			local = append(local, session)
		}
	}
	return local
}

// remoteSource keeps remote rows in the same filtering pipeline as local sources.
type remoteSource struct{ app *App }

func (remoteSource) Name() string { return model.SourceRemote }
func (s remoteSource) List(ctx context.Context) (model.Sessions, error) {
	sessions := model.NewSessions()
	for _, session := range s.app.remoteSessions(ctx) {
		sessions.Add(session)
	}
	return sessions, nil
}

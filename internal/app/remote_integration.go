package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/fullerzz/herdr-plugin-sesh/internal/remote"
	"github.com/fullerzz/herdr-plugin-sesh/internal/sources"
)

// remoteSessions returns peer workspace rows. Disabled support yields no rows
// and no error; an error explains why an enabled catalog is unavailable.
func remoteSessions(ctx context.Context) ([]model.Session, error) {
	cfg, err := remote.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("remote catalog configuration: %w", err)
	}
	if !cfg.Enabled {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	response, err := remote.Call(ctx, cfg, remote.Request{Op: "list"})
	if err != nil {
		return nil, fmt.Errorf("remote catalog unavailable: %w", err)
	}
	var sessions []model.Session
	for _, machine := range response.Catalog {
		if machine.ID == cfg.ID {
			continue
		}
		for _, workspace := range machine.Workspaces {
			sessions = append(sessions, model.Session{
				Source:      model.SourceRemote,
				Name:        workspace.Label,
				AgentStatus: workspace.AgentStatus,
				Remote: &model.RemoteRef{
					Machine:     machine.ID,
					Session:     machine.Session,
					WorkspaceID: workspace.ID,
					CWD:         workspace.CWD,
					Offline:     machine.Stale,
				},
			})
		}
	}
	return sessions, nil
}

// startRemoteRows fetches the remote catalog while local sources merge. Remote
// rows bypass the merge so local dedupe, caching, and session config never see
// them; the returned func waits and reports an unavailable catalog via warnf.
func startRemoteRows(ctx context.Context, blacklist []string, onlyBlacklisted bool) func(warnf func(string, ...any)) []model.Session {
	type result struct {
		sessions []model.Session
		err      error
	}
	done := make(chan result, 1)
	go func() {
		sessions, err := remoteSessions(ctx)
		done <- result{sources.FilterBlacklist(sessions, blacklist, onlyBlacklisted), err}
	}()
	return func(warnf func(string, ...any)) []model.Session {
		r := <-done
		if r.err != nil {
			warnf("%v", r.err)
		}
		return r.sessions
	}
}

func (a *App) switchRemote(ctx context.Context, session model.Session) error {
	if session.Remote.Offline {
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
	_, err = remote.Call(ctx, cfg, remote.Request{Op: "switch", MachineID: session.Remote.Machine, WorkspaceID: session.Remote.WorkspaceID})
	return err
}

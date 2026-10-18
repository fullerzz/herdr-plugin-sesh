package sources

import (
	"context"
	"testing"

	"github.com/fullerzz/herdr-plugin-sesh/internal/config"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoteMergeIdentityAndConfigIsolation(t *testing.T) {
	local := model.Session{Source: "herdr", Name: "same", Path: "/same", WorkspaceID: "w1"}
	peer := model.Session{Source: model.SourceRemote, Name: "same", Path: "/same", WorkspaceID: "w1", RemoteMachine: "peer", RemoteSession: "main"}
	other := peer
	other.RemoteMachine = "other"
	sessions, err := Merge(context.Background(), []Source{staticSource{"herdr", []model.Session{local}}, staticSource{model.SourceRemote, []model.Session{peer, other}}}, nil, nil, false, true)
	require.NoError(t, err)
	require.Len(t, sessions.Ordered(), 3)
	cfg := config.Default()
	cfg.DefaultSessionConfig.StartupCommand = "local startup"
	ApplyConfig(&sessions, cfg, "")
	assert.Equal(t, peer, sessions.Directory[model.Key(peer)])
	assert.Equal(t, other, sessions.Directory[model.Key(other)])
	assert.Equal(t, "local startup", sessions.Directory[model.Key(local)].StartupCommand)
}

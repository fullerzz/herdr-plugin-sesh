package connect

import (
	"context"
	"testing"

	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoteResolutionCannotShadowLocal(t *testing.T) {
	remote := model.Session{Source: model.SourceRemote, Name: "same", Path: "/same", WorkspaceID: "w1", RemoteMachine: "peer", RemoteSession: "main"}
	local := model.Session{Source: "herdr", Name: "same", Path: "/same", WorkspaceID: "w1"}
	for _, target := range []string{"same", "/same", "w1"} {
		got, ok := Resolve([]model.Session{remote, local}, target)
		require.True(t, ok)
		assert.Equal(t, local, got)
	}
	got, ok := Resolve([]model.Session{remote, local}, model.Key(remote))
	require.True(t, ok)
	assert.Equal(t, remote, got)
	t.Setenv("HERDR_BIN_PATH", "/does-not-exist")
	_, err := Connect(context.Background(), herdr.NewCLIClient(), []model.Session{remote}, model.Key(remote), Options{})
	require.ErrorContains(t, err, "attach wrapper")
}

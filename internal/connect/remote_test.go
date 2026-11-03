package connect

import (
	"context"
	"testing"

	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnectRemoteQualifiedIDOnlyFocusesExistingWorkspace(t *testing.T) {
	remote := model.Session{Source: "herdr", Machine: "buntu26", Name: "api", Path: "/src/api", WorkspaceID: "buntu26/sp1", StartupCommand: "must-not-run", WindowNames: []string{"dev"}}
	client := &herdr.FakeClient{}
	result, err := Connect(context.Background(), client, []model.Session{
		{Source: "config", Name: remote.WorkspaceID},
		{Source: "herdr", Name: "api", Path: remote.Path, WorkspaceID: "sp1"}, remote,
	}, remote.WorkspaceID, Options{Namer: func(context.Context, string) string {
		t.Fatal("remote path passed to local namer")
		return ""
	}})
	require.NoError(t, err)
	assert.Equal(t, remote, result.Session)
	assert.False(t, result.Created)
	assert.Equal(t, []string{remote.WorkspaceID}, client.FocusedWorkspaces)
	assert.Empty(t, client.CreatedWorkspaces)
	assert.Empty(t, client.CreatedTabs)
	assert.Empty(t, client.PaneRuns)

	remote.WorkspaceID = ""
	_, err = Connect(context.Background(), client, []model.Session{remote}, remote.Name, Options{})
	require.ErrorContains(t, err, "existing workspace ID")
	assert.Empty(t, client.CreatedWorkspaces)
}

func TestResolveNeverMatchesRemoteByNameOrPath(t *testing.T) {
	remote := model.Session{Source: "herdr", Machine: "buntu26", Name: "api", Path: "/src/api", WorkspaceID: "buntu26/sp1"}
	config := model.Session{Source: "config", Name: "api", Path: "/src/api"}

	// A remote space listed first must not shadow a local namesake.
	got, ok := Resolve([]model.Session{remote, config}, "api")
	require.True(t, ok)
	assert.Equal(t, config, got)
	got, ok = Resolve([]model.Session{remote, config}, "/src/api")
	require.True(t, ok)
	assert.Equal(t, config, got)

	_, ok = Resolve([]model.Session{remote}, "/src/api")
	assert.False(t, ok, "remote-only path match")
	_, ok = Resolve([]model.Session{remote}, "api")
	assert.False(t, ok, "remote-only name match")

	got, ok = Resolve([]model.Session{config, remote}, remote.WorkspaceID)
	require.True(t, ok)
	assert.Equal(t, remote, got)
}

func TestConnectLocalDirectoryIgnoresRemoteWithSamePath(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // Keep the created path out of the user's zoxide database.
	dir := t.TempDir()
	remote := model.Session{Source: "herdr", Machine: "buntu26", Name: "api", Path: dir, WorkspaceID: "buntu26/sp1"}
	client := &herdr.FakeClient{}
	result, err := Connect(context.Background(), client, []model.Session{remote}, dir, Options{NoFocus: true})
	require.NoError(t, err)
	assert.True(t, result.Created)
	assert.Empty(t, result.Session.Machine)
	assert.Equal(t, dir, result.Session.Path)
	assert.Empty(t, client.FocusedWorkspaces)
	require.Len(t, client.CreatedWorkspaces, 1)
	assert.Equal(t, dir, client.CreatedWorkspaces[0].CWD)
}

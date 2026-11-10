package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/fullerzz/herdr-plugin-sesh/internal/config"
	"github.com/fullerzz/herdr-plugin-sesh/internal/connect"
	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/fullerzz/herdr-plugin-sesh/internal/picker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type remotePickerRunner struct {
	selected []string
}

func (r *remotePickerRunner) Run(_ context.Context, bin string, args ...string) ([]byte, []byte, error) {
	if bin == "herdr-app" && len(args) == 1 && args[0] == "ls" {
		return []byte(`{"spaces":[
			{"id":"sp1","name":"api","folder":"/src/api","machine":null},
			{"id":"buntu26/sp1","name":"api","folder":"/src/api","machine":"buntu26"}
		]}`), nil, nil
	}
	if bin == "hc" && len(args) == 2 && args[0] == "ls" && args[1] == "--json" {
		return []byte(`{"terminals":[]}`), nil, nil
	}
	if bin == "herdr-app" && len(args) == 3 && args[0] == "do" && args[1] == "space.select" {
		r.selected = append(r.selected, args[2])
		return []byte(`{"ok":true}`), nil, nil
	}
	return nil, nil, errors.New("unexpected command")
}

func TestListTextLabelsRemoteSessionsOnly(t *testing.T) {
	var out bytes.Buffer
	a := &App{Out: &out}
	require.NoError(t, a.printSessions([]model.Session{
		{Source: "herdr", Name: "api", Path: "/src/api", WorkspaceID: "sp1"},
		{Source: "herdr", Machine: "buntu26", Name: "api", Path: "/src/api", WorkspaceID: "buntu26/sp1"},
		{Source: "herdr", Machine: "buntu26", Name: "~", WorkspaceID: "buntu26/sp2"},
		{Source: "config", Name: "notes"},
	}, false))
	assert.Equal(t, "herdr\tapi\t/src/api\nherdr\t[buntu26] api\t/src/api\nherdr\t[buntu26] ~\nconfig\tnotes\n", out.String())
}

func TestAppHistoryKeepsQualifiedRemoteIDsThroughSwitchAndPrune(t *testing.T) {
	t.Setenv("HERDR_SESH_BACKEND", "app")
	dir := t.TempDir()
	require.NoError(t, recordSwitch(dir, "sp1", "buntu26/sp1", 0))
	require.NoError(t, recordSwitch(dir, "buntu26/sp1", "ser8/sp1", 0))

	// buntu26/sp1 survives pruning by its qualified ID; a bare "sp1" live
	// space must not keep the closed remote "ser8/sp1".
	h, err := loadLastHistory(t.Context(), liveSpaces{ids: []string{"sp1", "buntu26/sp1"}}, dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"buntu26/sp1", "sp1"}, h.Workspaces)
	id, ok := h.PreviousWorkspace("sp1")
	require.True(t, ok)
	assert.Equal(t, "buntu26/sp1", id)
}

func TestAppPickerCollectsAndSelectsRemoteNamesakeByQualifiedID(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // Exclude the user's zoxide database.
	runner := &remotePickerRunner{}
	client := &herdr.AppClient{AppBin: "herdr-app", CoreBin: "hc", Runner: runner}
	cfg := config.Default()
	cfg.SessionConfigs = []config.SessionConfig{{Name: "api", Path: "/src/api"}}
	col, err := (&App{}).collectPickerWithClient(context.Background(), cfg, nil, client)
	require.NoError(t, err)
	require.NoError(t, col.HerdrErr)
	require.Len(t, col.Sessions, 2)
	require.Len(t, col.HerdrWorkspaces, 2)
	list := picker.New(col.Sessions)
	list.Filter("buntu26")
	selected, ok := list.Current()
	require.True(t, ok)
	assert.Equal(t, "buntu26/sp1", selected.WorkspaceID)
	result, err := connect.Open(context.Background(), client, selected, connect.Options{})
	require.NoError(t, err)
	assert.False(t, result.Created)
	assert.Equal(t, "buntu26/sp1", result.Session.WorkspaceID)
	assert.Equal(t, []string{`{"id":"buntu26/sp1"}`}, runner.selected)
}

// A connect or preview target may name a workspace on another machine, or be
// its path there, so collecting sessions for it must not run git on it here.
func TestCollectForRemoteTargetRunsNoLocalGit(t *testing.T) {
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "git.log")
	//nolint:gosec // the test's fake git must be executable.
	require.NoError(t, os.WriteFile(filepath.Join(bin, "git"), []byte("#!/bin/sh\necho \"$@\" >> '"+log+"'\nexit 1\n"), 0o755))
	t.Setenv("PATH", bin) // Also excludes the user's zoxide database.
	t.Chdir(t.TempDir())
	cfg := config.Default()
	cfg.SessionConfigs = []config.SessionConfig{{Name: "api", Machine: "buntu26", Path: "/remote-only/api"}}
	for _, target := range []string{"api", "/remote-only/api"} {
		sessions, err := (&App{}).collectAllowUnavailableHerdr(context.Background(), cfg, target)
		require.NoError(t, err)
		require.Len(t, sessions, 1)
		assert.Equal(t, "buntu26", sessions[0].Machine)
	}
	assert.NoFileExists(t, log, "git ran locally for a remote target")

	// The sentinel works: a local directory target is still named with git.
	local := t.TempDir()
	_, err := (&App{}).collectAllowUnavailableHerdr(context.Background(), cfg, local)
	require.NoError(t, err)
	assert.FileExists(t, log)
}

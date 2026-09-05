package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionJSONOmitsInternalPickerFields(t *testing.T) {
	s := Session{
		Source:      "config",
		Name:        "api",
		Path:        "/tmp/api",
		AgentStatus: "working",
		WindowConfigs: []WindowConfig{{
			Name: "dev",
		}},
		Worktree: WorktreeRelation{Linked: true, ParentWorkspaceID: "w-parent", ParentWorkspaceName: "parent"},
	}
	b, err := json.Marshal(s)
	require.NoError(t, err)
	got := string(b)
	require.Contains(t, got, `"source":"config"`)
	require.Contains(t, got, `"name":"api"`)
	require.NotContains(t, got, "WindowConfigs")
	require.NotContains(t, got, "window_configs")
	require.NotContains(t, got, "AgentStatus")
	require.NotContains(t, got, "agent_status")
	for _, internal := range []string{"Worktree", "worktree", "w-parent", "parent"} {
		require.NotContains(t, got, internal)
	}
}

func TestKeyIsStableAndSourceScoped(t *testing.T) {
	a := Session{Source: "config", Name: "api", Path: "/tmp/api"}
	b := Session{Source: "config", Name: "api", Path: "/tmp/api"}
	c := Session{Source: "zoxide", Name: "api", Path: "/tmp/api"}
	require.Equal(t, Key(b), Key(a))
	require.NotEqual(t, Key(c), Key(a))
	b.AgentStatus = "working"
	require.Equal(t, Key(b), Key(a))
	b.Worktree = WorktreeRelation{Linked: true, ParentWorkspaceID: "w-parent", ParentWorkspaceName: "parent"}
	assert.Equal(t, Key(b), Key(a))
}

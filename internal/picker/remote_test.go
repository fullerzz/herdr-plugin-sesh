package picker

import (
	"strings"
	"testing"

	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoteRowsKeepIdentityAndStatus(t *testing.T) {
	remote := model.Session{Source: model.SourceRemote, Name: "same", WorkspaceID: "w1", RemoteMachine: "peer", RemoteOffline: true, AgentStatus: "working"}
	m := New([]model.Session{{Source: "herdr", Name: "same", WorkspaceID: "w1"}, remote})
	m.UpdateAgentStatuses(map[string]string{"w1": "idle"})
	assert.Equal(t, "working", m.All[1].AgentStatus)
	m.Filter("peer")
	require.Len(t, m.Filtered, 1)
	assert.Equal(t, remote, m.Filtered[0])
	m.Filter("offline")
	require.Len(t, m.Filtered, 1)
	assert.Contains(t, fzfInput([]model.Session{remote}, false), "same @ peer (offline)")
	command := fzfPreviewCommand()
	assert.Less(t, strings.Index(command, `"$source" = remote`), strings.Index(command, `[ ! -d "$item_path" ]`))
	tm := newTeaModel([]model.Session{remote}, Options{})
	_, cmd := tm.closeSelectedWorkspace()
	assert.Nil(t, cmd)
}

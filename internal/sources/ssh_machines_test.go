package sources

import (
	"context"
	"testing"

	"github.com/fullerzz/herdr-plugin-sesh/internal/config"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMachineIdentityAndConfigIsolation(t *testing.T) {
	source := SSHMachines{
		{ID: "one", Label: "Build", Target: "zach@buntu26", Session: "default", Enabled: true},
		{ID: "two", Label: "Build", Target: "zach@ser8", Session: "agents"},
	}
	local := staticSource{"config", []model.Session{{Source: "config", Name: "Build", Path: "/local"}}}
	merged, err := Merge(context.Background(), []Source{local, source}, nil, nil, false, true)
	require.NoError(t, err)
	require.Len(t, merged.OrderedIndex, 3)
	session := merged.Ordered()[1]
	assert.Equal(t, "ssh-machine:one", model.Key(session))
	session.Name = "Renamed"
	assert.Equal(t, "ssh-machine:one", model.Key(session))
	cfg := config.Default()
	cfg.DefaultSessionConfig.StartupCommand = "must not execute"
	ApplyConfig(&merged, cfg, "")
	ssh := merged.Ordered()[1]
	assert.Empty(t, ssh.Path)
	assert.Empty(t, ssh.WorkspaceID)
	assert.Empty(t, ssh.StartupCommand)
	assert.Empty(t, ssh.PreviewCommand)
	assert.Empty(t, ssh.WindowConfigs)
	assert.Equal(t, &model.SSHMachine{ID: "one", Target: "zach@buntu26", RemoteSession: "default", Enabled: true}, ssh.SSH)
	// Catalog renames replace the same entry rather than creating a duplicate.
	ss := model.NewSessions()
	ss.Add(ssh)
	ss.Add(session)
	assert.Len(t, ss.Ordered(), 1)
}

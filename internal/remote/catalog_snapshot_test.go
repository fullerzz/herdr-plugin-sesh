package remote

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCatalogSnapshotIsolation(t *testing.T) {
	s := NewServer(testConfig(), &fakeClient{})
	s.catalog = []Machine{
		{ID: "local", Label: "Local", Session: "main", Workspaces: []Workspace{{ID: "ws1", Label: "One", CWD: "/tmp", AgentStatus: "idle"}}, UpdatedAt: time.Now()},
		{ID: "other", Label: "Other", Session: "peer", Workspaces: []Workspace{{ID: "ws2"}}, Stale: true, LastError: "offline"},
	}
	s.switchError = "switch failed"
	ctx := context.Background()
	listed := s.Handle(ctx, Request{Op: "list"})
	require.Len(t, listed.Catalog, 2)
	assert.Equal(t, s.catalog, listed.Catalog)
	assert.Equal(t, "switch failed", listed.SwitchError)

	exported := s.Handle(ctx, Request{Op: "export"})
	require.Len(t, exported.Catalog, 1)
	assert.Equal(t, s.catalog[:1], exported.Catalog)
	assert.Empty(t, exported.SwitchError)
	listed.Catalog[0].Label = "changed"
	listed.Catalog[0].Workspaces[0].Label = "changed"
	listed.Catalog[1].Workspaces[0].ID = "changed"
	exported.Catalog[0].Workspaces[0].CWD = "changed"
	assert.Equal(t, "Local", s.catalog[0].Label)
	assert.Equal(t, "One", s.catalog[0].Workspaces[0].Label)
	assert.Equal(t, "/tmp", s.catalog[0].Workspaces[0].CWD)
	assert.Equal(t, "ws2", s.catalog[1].Workspaces[0].ID)
	assert.Equal(t, s.catalog, s.Handle(ctx, Request{Op: "list"}).Catalog)

	s.record(0, []Workspace{{ID: "new"}}, nil)
	assert.Equal(t, "ws1", listed.Catalog[0].Workspaces[0].ID)
	assert.Equal(t, "ws1", exported.Catalog[0].Workspaces[0].ID)
}

func TestCatalogSnapshotPreservesEmptyWorkspaces(t *testing.T) {
	s := NewServer(testConfig(), &fakeClient{})
	s.catalog[1].Workspaces = []Workspace{}
	r := s.Handle(context.Background(), Request{Op: "list"})
	require.Len(t, r.Catalog, 2)
	assert.Nil(t, r.Catalog[0].Workspaces)
	assert.NotNil(t, r.Catalog[1].Workspaces)
	assert.Empty(t, r.Catalog[1].Workspaces)
}

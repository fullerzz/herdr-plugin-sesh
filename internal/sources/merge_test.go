package sources

import (
	"context"
	"testing"

	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type staticSource struct {
	name     string
	sessions []model.Session
}

func (s staticSource) Name() string { return s.name }
func (s staticSource) List(context.Context) (model.Sessions, error) {
	out := model.NewSessions()
	for _, x := range s.sessions {
		out.Add(x)
	}
	return out, nil
}
func TestMergeOrderBlacklistDedupe(t *testing.T) {
	srcs := []Source{staticSource{"zoxide", []model.Session{{Source: "zoxide", Name: "api", Path: "/z"}}}, staticSource{"config", []model.Session{{Source: "config", Name: "api", Path: "/c"}, {Source: "config", Name: "scratch", Path: "/s"}}}}
	got, err := Merge(context.Background(), srcs, []string{"config", "zoxide"}, []string{"scratch"}, false, true)
	require.NoError(t, err)
	os := got.Ordered()
	require.Len(t, os, 1)
	require.Equal(t, "config", os[0].Source)
	assert.Equal(t, "api", os[0].Name)
}
func TestParseZoxideLine(t *testing.T) {
	s, ok := ParseZoxideLine("42.5 /tmp/my app")
	require.True(t, ok)
	require.InDelta(t, 42.5, s.Score, 0)
	require.Equal(t, "/tmp/my app", s.Path)
	assert.Equal(t, "my app", s.Name)
}

func TestMergePlacesMachinesAfterHerdrBeforeDirectories(t *testing.T) {
	srcs := []Source{
		staticSource{"herdr", []model.Session{{Source: "herdr", Name: "running", WorkspaceID: "w1"}}},
		staticSource{"config", []model.Session{{Source: "config", Name: "configured", Path: "/configured"}}},
		staticSource{"zoxide", []model.Session{{Source: "zoxide", Name: "directory", Path: "/directory"}}},
		staticSource{"ssh", []model.Session{{Source: "ssh", Name: "remote", SSH: &model.SSHMachine{ID: "one", Enabled: true}}}},
	}
	for _, order := range [][]string{nil, {"herdr", "config", "zoxide", "dir"}} {
		got, err := Merge(context.Background(), srcs, order, nil, false, true)
		require.NoError(t, err)
		var names []string
		for _, session := range got.Ordered() {
			names = append(names, session.Name)
		}
		assert.Equal(t, []string{"running", "remote", "configured", "directory"}, names)
	}
}

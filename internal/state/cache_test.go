package state

import (
	"testing"
	"time"

	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionCacheUsesFreshEntries(t *testing.T) {
	d := t.TempDir()
	want := []model.Session{{Source: "config", Name: "api", Path: "/tmp/api"}}
	require.NoError(t, SaveSessionCache(d, "/tmp/sesh.toml", want, time.Now()))
	got, ok, err := LoadSessionCache(d, "/tmp/sesh.toml", 5*time.Second, time.Now())
	require.NoError(t, err)
	require.True(t, ok)
	require.Len(t, got, 1)
	assert.Equal(t, "api", got[0].Name)
}

func TestSessionCacheIgnoresStaleEntries(t *testing.T) {
	d := t.TempDir()
	require.NoError(t, SaveSessionCache(d, "", []model.Session{{Name: "old"}}, time.Unix(0, 0)))
	got, ok, err := LoadSessionCache(d, "", time.Second, time.Unix(10, 0))
	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, got)
}

func TestSessionCacheMissWhenMissing(t *testing.T) {
	got, ok, err := LoadSessionCache(t.TempDir(), "", time.Second, time.Now())
	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, got)
}

func TestSessionCacheNoopsWithoutStateDir(t *testing.T) {
	require.NoError(t, SaveSessionCache("", "", []model.Session{{Name: "api"}}, time.Now()))
	got, ok, err := LoadSessionCache("", "", time.Second, time.Now())
	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, got)
}

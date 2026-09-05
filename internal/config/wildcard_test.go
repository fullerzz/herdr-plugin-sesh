package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWildcardOneLevelAndRecursive(t *testing.T) {
	home := "/home/zach"
	require.True(t, MatchWildcard("~/projects/*", "/home/zach/projects/api", home), "one-level should match")
	require.False(t, MatchWildcard("~/projects/*", "/home/zach/projects/team/api", home), "one-level should not match nested")
	require.True(t, MatchWildcard("~/projects/**", "/home/zach/projects", home), "recursive should match base directory")
	assert.True(t, MatchWildcard("~/projects/**", "/home/zach/projects/team/api", home), "recursive should match nested")
}
func TestFindWildcardFirstWins(t *testing.T) {
	cfg := Config{WildcardConfigs: []WildcardConfig{{Pattern: "/tmp/**", StartupCommand: "first"}, {Pattern: "/tmp/app", StartupCommand: "second"}}}
	w, ok := FindWildcard(cfg, "/tmp/app", "")
	require.True(t, ok)
	assert.Equal(t, "first", w.StartupCommand)
}
func TestSubstitutePathShellQuotesSpaces(t *testing.T) {
	got := SubstitutePath("cd {} && pwd", "/tmp/has space")
	want := "cd '/tmp/has space' && pwd"
	assert.Equal(t, want, got)
}

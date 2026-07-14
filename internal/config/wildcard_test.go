package config

import "testing"

func TestWildcardOneLevelAndRecursive(t *testing.T) {
	home := "/home/zach"
	if !MatchWildcard("~/projects/*", "/home/zach/projects/api", home) {
		t.Fatal("one-level should match")
	}
	if MatchWildcard("~/projects/*", "/home/zach/projects/team/api", home) {
		t.Fatal("one-level should not match nested")
	}
	if !MatchWildcard("~/projects/**", "/home/zach/projects", home) {
		t.Fatal("recursive should match base directory")
	}
	if !MatchWildcard("~/projects/**", "/home/zach/projects/team/api", home) {
		t.Fatal("recursive should match nested")
	}
}
func TestFindWildcardFirstWins(t *testing.T) {
	cfg := Config{WildcardConfigs: []WildcardConfig{{Pattern: "/tmp/**", StartupCommand: "first"}, {Pattern: "/tmp/app", StartupCommand: "second"}}}
	w, ok := FindWildcard(cfg, "/tmp/app", "")
	if !ok || w.StartupCommand != "first" {
		t.Fatalf("bad wildcard %#v %v", w, ok)
	}
}
func TestSubstitutePathShellQuotesSpaces(t *testing.T) {
	got := SubstitutePath("cd {} && pwd", "/tmp/has space")
	want := "cd '/tmp/has space' && pwd"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

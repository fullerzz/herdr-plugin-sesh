package clone

import "testing"

func TestDestinationFromRepoURL(t *testing.T) {
	got := Destination(Request{Repo: "git@host:org/project.git", CmdDir: "/tmp"})
	if got != "/tmp/project" {
		t.Fatalf("got %q", got)
	}
}
func TestDestinationOverride(t *testing.T) {
	got := Destination(Request{Repo: "x", Dir: "/tmp/custom"})
	if got != "/tmp/custom" {
		t.Fatalf("got %q", got)
	}
}

package clone

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDestinationFromRepoURL(t *testing.T) {
	got := Destination(Request{Repo: "git@host:org/project.git", CmdDir: "/tmp"})
	assert.Equal(t, "/tmp/project", got)
}
func TestDestinationOverride(t *testing.T) {
	got := Destination(Request{Repo: "x", Dir: "/tmp/custom"})
	assert.Equal(t, "/tmp/custom", got)
}

func TestDestinationRelativeDirUsesCmdDir(t *testing.T) {
	got := Destination(Request{Repo: "x", CmdDir: "/tmp/work", Dir: "repo"})
	assert.Equal(t, "/tmp/work/repo", got)
}

func TestDestinationRelativeDirWithoutCmdDir(t *testing.T) {
	got := Destination(Request{Repo: "x", Dir: "repo"})
	assert.Equal(t, "repo", got)
}

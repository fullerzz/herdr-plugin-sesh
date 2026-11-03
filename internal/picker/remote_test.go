package picker

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoteMachineSearchAndDisplay(t *testing.T) {
	remote := model.Session{Source: "herdr", Machine: "buntu26", Name: "api", Path: "/src/api", WorkspaceID: "buntu26/sp1"}
	local := model.Session{Source: "herdr", Name: "api", Path: "/src/api", WorkspaceID: "sp1"}
	m := New([]model.Session{local, remote})
	m.Filter("BUNTU26")
	require.Equal(t, []model.Session{remote}, m.Filtered)
	assert.Contains(t, ansi.Strip(row(remote, true, 100, true, "")), "[buntu26] api")
	assert.NotContains(t, ansi.Strip(row(local, true, 100, true, "")), "[buntu26]")
	assert.Contains(t, fzfInput([]model.Session{remote}, false), "[buntu26] api")
	view := newTeaModel([]model.Session{remote}, Options{DisplayOptions: DisplayOptions{
		LastWorkspaceID: remote.WorkspaceID, HerdrWorkspaces: []model.Session{local, remote},
	}})
	label, path := view.lastWorkspaceStatus()
	assert.Equal(t, "[buntu26] api", label)
	assert.Equal(t, remote.Path, path)
	assert.Contains(t, ansi.Strip(view.previewTitle()), "[buntu26] api")
}

func TestRemoteMachineAndNameSearchTogether(t *testing.T) {
	remote := model.Session{Source: "herdr", Machine: "buntu26", Name: "api", Path: "/src/api", WorkspaceID: "buntu26/sp1"}
	local := model.Session{Source: "herdr", Name: "api", Path: "/src/api", WorkspaceID: "sp1"}
	apiDocs := model.Session{Source: "config", Name: "api-docs", Path: "/src/docs"}
	byPath := model.Session{Source: "zoxide", Name: "svc", Path: "/src/api/svc"}
	m := New([]model.Session{byPath, local, remote, apiDocs})
	m.Filter("buntu26 api")
	assert.Equal(t, []model.Session{remote}, m.Filtered)
	m.Filter("api")
	// Name matches stay ahead of path-only matches, in input order.
	assert.Equal(t, []model.Session{local, remote, apiDocs, byPath}, m.Filtered)
}

func TestRemoteSessionsSkipLocalHomeHeuristics(t *testing.T) {
	home := "/Users/zach"
	t.Setenv("HOME", home)
	remoteHome := model.Session{Source: "herdr", Machine: "buntu26", Name: "~", Path: home, WorkspaceID: "buntu26/sp2"}
	localHome := model.Session{Source: "herdr", Name: "~", Path: home, WorkspaceID: "sp2"}
	other := model.Session{Source: "config", Name: "homework", Path: "/src/homework"}
	m := New([]model.Session{other, remoteHome, localHome})
	m.Filter("home")
	assert.Equal(t, []model.Session{localHome, other}, m.Filtered, "a remote ~ is not the local home")
	m.Filter("~")
	assert.Equal(t, []model.Session{remoteHome, localHome}, m.Filtered, "remote ~ stays searchable literally")

	assert.True(t, isHomeSession(localHome))
	assert.False(t, isHomeSession(remoteHome))
	assert.Equal(t, home, displayPath(remoteHome))
	assert.Equal(t, "~", displayPath(localHome))
	assert.Contains(t, ansi.Strip(row(remoteHome, false, 120, true, "")), home)

	view := newTeaModel(nil, Options{DisplayOptions: DisplayOptions{
		LastWorkspaceID: remoteHome.WorkspaceID, HerdrWorkspaces: []model.Session{remoteHome},
	}})
	assert.Contains(t, ansi.Strip(view.lastWorkspaceText()), "[buntu26] ~  "+home)
}

// TestRealFZFMatchesRemoteMachine runs the installed fzf in --filter mode
// against the real fzfArgs and fzfInput.
func TestRealFZFMatchesRemoteMachine(t *testing.T) {
	fzf, err := exec.LookPath("fzf")
	if err != nil {
		t.Skip("fzf not installed")
	}
	local := model.Session{Source: "herdr", Name: "api", Path: "/src/api", WorkspaceID: "sp1"}
	buntu := model.Session{Source: "herdr", Machine: "buntu26", Name: "api", Path: "/src/api", WorkspaceID: "buntu26/sp1"}
	ser8 := model.Session{Source: "herdr", Machine: "ser8", Name: "web", WorkspaceID: "ser8/sp1"}
	items := []model.Session{local, buntu, ser8}
	for _, tc := range []struct {
		filter string
		want   model.Session
	}{
		{filter: "buntu26", want: buntu},
		{filter: "buntu26 api", want: buntu},
		{filter: "ser8", want: ser8},
		{filter: "/src/api", want: local},
	} {
		t.Run(tc.filter, func(t *testing.T) {
			t.Setenv("FZF_DEFAULT_OPTS", "--filter='"+tc.filter+"'")
			got, ok, err := RunFZF(t.Context(), items, Options{DisplayOptions: DisplayOptions{FZFCommand: fzf}})
			require.NoError(t, err)
			require.True(t, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestFZFRemotePreviewNeverReadsLocalFiles(t *testing.T) {
	script := strings.NewReplacer("{2}", "herdr", "{4}", "api", "{5}", "/does-not-exist", "{7}", "buntu26").Replace(fzfPreviewCommand())
	//nolint:gosec // The preview script and substituted values are fixed by this test.
	out, err := exec.Command("/bin/sh", "-c", script).CombinedOutput()
	require.NoError(t, err)
	assert.Contains(t, string(out), "machine: buntu26")
	assert.NotContains(t, string(out), "No item path")
}

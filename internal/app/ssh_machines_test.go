package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fullerzz/herdr-plugin-sesh/internal/config"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/fullerzz/herdr-plugin-sesh/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSavedMachinesAppearInAllCollections(t *testing.T) {
	configureHerdrScript(t, `#!/bin/sh
case "$*" in
"machine list --json") printf '%s' "$MACHINE_CATALOG" ;;
"workspace list") printf '[{"id":"w1","label":"buntu26","cwd":"/local"}]' ;;
"pane list --workspace w1") printf '[]' ;;
*) exit 1 ;;
esac
`)
	t.Setenv("MACHINE_CATALOG", `[{"id":"one","label":"buntu26","target":"zach@buntu26","session":"default","enabled":true,"selected":true},{"id":"two","label":"buntu26","target":"zach@ser8","session":"agents","enabled":false,"selected":false}]`)
	cfg := config.Default()
	a := &App{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}
	col, err := a.collectPicker(context.Background(), cfg)
	require.NoError(t, err)
	require.Len(t, col.Sessions, 3)
	assert.Len(t, col.HerdrWorkspaces, 1)
	other, err := a.collectAllowUnavailableHerdr(context.Background(), cfg, "")
	require.NoError(t, err)
	assert.Equal(t, col.Sessions, other)

	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("version = 1\n"), 0600))
	for _, command := range []string{"list", "picker"} {
		var out bytes.Buffer
		a.Out = &out
		require.NoError(t, a.Run(context.Background(), []string{command, "--json", "--config", cfgPath}))
		var got []model.Session
		require.NoError(t, json.Unmarshal(out.Bytes(), &got))
		assert.Equal(t, col.Sessions, got)
	}
}

func TestSavedMachinesStayFreshWithoutEnteringCache(t *testing.T) {
	configureHerdrScript(t, `#!/bin/sh
case "$*" in
"machine list --json") printf '%s' "$MACHINE_CATALOG" ;;
"workspace list") printf '[{"id":"w1","label":"local","cwd":"/local"}]' ;;
*) exit 1 ;;
esac
`)
	cfgPath := filepath.Join(t.TempDir(), "sesh.toml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("cache = true\n"), 0600))
	stateDir := t.TempDir()
	t.Setenv("HERDR_PLUGIN_STATE_DIR", stateDir)
	var out bytes.Buffer
	a := &App{Out: &out, Err: &bytes.Buffer{}}
	for _, tc := range []struct {
		catalog string
		name    string
	}{
		{`[]`, ""},
		{`[{"id":"one","label":"Build","target":"zach@buntu26","session":"agents","enabled":true}]`, "Build"},
		{`[{"id":"one","label":"Renamed","target":"zach@buntu26","session":"agents","enabled":false}]`, "Renamed"},
		{`[]`, ""},
	} {
		t.Setenv("MACHINE_CATALOG", tc.catalog)
		out.Reset()
		require.NoError(t, a.Run(context.Background(), []string{"list", "--json", "--config", cfgPath}))
		var got []model.Session
		require.NoError(t, json.Unmarshal(out.Bytes(), &got))
		if tc.name == "" {
			assert.Len(t, got, 1)
		} else {
			require.Len(t, got, 2)
			assert.Equal(t, tc.name, got[1].Name)
		}
		cached, ok, err := state.LoadSessionCache(stateDir, cfgPath, time.Minute, time.Now())
		require.NoError(t, err)
		require.True(t, ok)
		require.Len(t, cached, 1)
		assert.False(t, cached[0].IsSSH())
	}
}

func TestMachineListingFailureKeepsLocalSessions(t *testing.T) {
	configureHerdrScript(t, `#!/bin/sh
case "$*" in
"workspace list") printf '[{"id":"w1","label":"local","cwd":"/local"}]' ;;
"machine list --json") printf 'catalog unavailable' >&2; exit 1 ;;
*) exit 1 ;;
esac
`)
	var warnings bytes.Buffer
	a := &App{Out: &bytes.Buffer{}, Err: &warnings}
	col, err := a.collectPicker(context.Background(), config.Default())
	require.NoError(t, err)
	require.ErrorContains(t, col.MachineErr, "catalog unavailable")
	assert.Len(t, col.Sessions, 1)
	_, err = a.collectAllowUnavailableHerdr(context.Background(), config.Default(), "")
	require.NoError(t, err)
	assert.Contains(t, warnings.String(), "saved SSH machines unavailable")
}

func TestMachineConnectHasNoLocalSideEffects(t *testing.T) {
	configureHerdrScript(t, "#!/bin/sh\n: > \"$UNEXPECTED_HERDR_CALL\"\nexit 1\n")
	d := t.TempDir()
	marker := filepath.Join(d, "unexpected")
	t.Setenv("UNEXPECTED_HERDR_CALL", marker)
	t.Setenv("HERDR_PLUGIN_STATE_DIR", d)
	a := &App{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}
	err := a.Run(context.Background(), []string{"connect", "ssh-machine:one"})
	require.ErrorContains(t, err, "display-only")
	entries, err := os.ReadDir(d)
	require.NoError(t, err)
	assert.Empty(t, entries, "no command or history writes")
}

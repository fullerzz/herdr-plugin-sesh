package herdr

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMachineListReadsSavedProfiles(t *testing.T) {
	c := &CLIClient{Bin: "herdr", Runner: fixedRunner{stdout: []byte(`[{"id":"one","label":"Build","target":"zach@buntu26","session":"default","enabled":true,"selected":true,"future":"ignored"},{"id":"two","label":"Build","target":"zach@ser8","session":"agents","enabled":false}]`)}}
	got, err := c.MachineList(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []Machine{
		{ID: "one", Label: "Build", Target: "zach@buntu26", Session: "default", Enabled: true},
		{ID: "two", Label: "Build", Target: "zach@ser8", Session: "agents"},
	}, got)
}

func TestMachineListRejectsInvalidCatalog(t *testing.T) {
	valid := `{"id":"one","label":"Build","target":"zach@buntu26","session":"default","enabled":true}`
	for _, body := range []string{
		`null`, `{}`, `[{}]`, `[null]`, `[`,
		"[" + valid + "," + valid + "]",
		"[" + strings.Replace(valid, `"one"`, `""`, 1) + "]",
		"[" + strings.Replace(valid, `"one"`, `"has space"`, 1) + "]",
		"[" + strings.Replace(valid, `"Build"`, `"\u001b[2J"`, 1) + "]",
		"[" + strings.Replace(valid, `"Build"`, `"\n"`, 1) + "]",
		"[" + strings.Replace(valid, `"default"`, `""`, 1) + "]",
		"[" + strings.Replace(valid, `,"enabled":true`, "", 1) + "]",
		"[" + strings.Replace(valid, `true`, `"yes"`, 1) + "]",
		"[" + strings.Replace(valid, `Build`, strings.Repeat("x", 1025), 1) + "]",
	} {
		c := &CLIClient{Bin: "herdr", Runner: fixedRunner{stdout: []byte(body)}}
		got, err := c.MachineList(context.Background())
		require.Error(t, err)
		assert.Empty(t, got)
	}
}

func TestMachineListEmptyAndUnavailable(t *testing.T) {
	c := &CLIClient{Bin: "herdr", Runner: fixedRunner{stdout: []byte(`[]`)}}
	got, err := c.MachineList(context.Background())
	require.NoError(t, err)
	assert.Empty(t, got)
	c.Runner = fixedRunner{err: context.DeadlineExceeded}
	_, err = c.MachineList(context.Background())
	require.ErrorIs(t, err, context.DeadlineExceeded)
	c.Runner = fixedRunner{err: errors.New("unknown command")}
	_, err = c.MachineList(context.Background())
	require.ErrorContains(t, err, "machine list --json")
}

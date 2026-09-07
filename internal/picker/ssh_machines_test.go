package picker

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func savedMachine(t *testing.T) model.Session {
	t.Helper()
	var s model.Session
	require.NoError(t, json.Unmarshal([]byte(`{"source":"ssh","name":"Build machine","ssh":{"id":"one","target":"zach@buntu26","remote_session":"agents","enabled":true}}`), &s))
	return s
}

func TestSavedMachineDisplayAndStaticFZFPreview(t *testing.T) {
	s := savedMachine(t)
	line := ansi.Strip(row(s, true, 130, false, ""))
	assert.Contains(t, line, "zach@buntu26")
	input := fzfInput([]model.Session{s}, false, true)
	assert.Contains(t, input, "󰌘 ssh")
	withoutIcons := fzfInput([]model.Session{s}, false, false)
	assert.Contains(t, withoutIcons, "[ssh]")
	assert.NotContains(t, withoutIcons, "󰌘")
	assert.Contains(t, input, "zach@buntu26")
	assert.Contains(t, input, "agents")
	assert.Contains(t, input, "enabled")
	assert.Contains(t, input, "display-only")
	fields := strings.Split(strings.TrimSpace(input), "\t")
	require.GreaterOrEqual(t, len(fields), 6)
	// FZF shell-quotes substituted fields. Exercise the resulting shell
	// with command syntax in the label, rather than just inspecting text.
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
	script := strings.NewReplacer("{2}", "ssh", "{4}", quote("$(exit 42)"), "{5}", quote(fields[4]), "{7}", "one").Replace(fzfPreviewCommand())
	//nolint:gosec // Exercise the generated preview script with fixed, shell-quoted test data.
	out, err := exec.CommandContext(context.Background(), "/bin/sh", "-c", script).CombinedOutput()
	require.NoError(t, err)
	assert.Contains(t, string(out), "$(exit 42)")
	assert.Contains(t, string(out), "machine sidebar")
}

func TestSavedMachineSearchAndEnter(t *testing.T) {
	s := savedMachine(t)
	for _, query := range []string{"Build", "zach@buntu26", "agents"} {
		m := New([]model.Session{s})
		m.Filter(query)
		assert.Len(t, m.Filtered, 1, query)
	}
	m := newTeaModel([]model.Session{s}, Options{})
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	next := updated.(teaModel)
	assert.False(t, next.chosen)
	assert.Nil(t, cmd, "display-only rows must keep the picker open")
	m.closeWorkspace = func(context.Context, string) error { t.Fatal("machine must not close a workspace"); return nil }
	_, cmd = m.closeSelectedWorkspace()
	assert.Nil(t, cmd)
}

func TestFZFMachineEnterDoesNotAccept(t *testing.T) {
	var action string
	for _, arg := range fzfArgs(Options{}) {
		if strings.HasPrefix(arg, "--bind=enter:transform:") {
			action = strings.TrimPrefix(arg, "--bind=enter:transform:")
		}
	}
	require.NotEmpty(t, action)
	for _, source := range []string{"ssh", "herdr"} {
		script := strings.ReplaceAll(action, "{2}", source)
		//nolint:gosec // Execute the generated FZF action with fixed source categories.
		out, err := exec.CommandContext(context.Background(), "/bin/sh", "-c", script).Output()
		require.NoError(t, err)
		if source == "ssh" {
			assert.Empty(t, out)
		} else {
			assert.Equal(t, "accept", string(out))
		}
	}
}

func TestFZFRejectsMachineWhenAcceptBindingIsOverridden(t *testing.T) {
	fzf := filepath.Join(t.TempDir(), "fzf")
	//nolint:gosec // Fixed executable fixture simulates an external FZF selection.
	require.NoError(t, os.WriteFile(fzf, []byte("#!/bin/sh\ncat >/dev/null\nprintf '0\\n'\n"), 0700))
	_, ok, err := RunFZF(context.Background(), []model.Session{savedMachine(t)}, Options{FZFCommand: fzf})
	require.ErrorContains(t, err, "display-only")
	assert.False(t, ok)
}

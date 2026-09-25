package app

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fullerzz/herdr-plugin-sesh/internal/config"
	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
	"github.com/fullerzz/herdr-plugin-sesh/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigEditHelpDoesNotRequireHerdr(t *testing.T) {
	a := New()
	require.ErrorIs(t, a.Run(context.Background(), []string{"config", "edit", "--help"}), flag.ErrHelp)
}

func TestPluginOpenSettingsUsesSettingsOverlay(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	bin := filepath.Join(dir, "herdr")
	//nolint:gosec // Executable test fixture.
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" > \"$HERDR_FAKE_LOG\"\n"), 0700))
	t.Setenv("HERDR_BIN_PATH", bin)
	t.Setenv("HERDR_FAKE_LOG", log)
	require.NoError(t, New().Run(context.Background(), []string{"plugin", "open-settings"}))
	data, err := os.ReadFile(log) //nolint:gosec // Test-owned log.
	require.NoError(t, err)
	assert.Equal(t, "plugin pane open --plugin fullerzz.sesh --entrypoint settings --placement overlay\n", string(data))
}

func TestSettingsSaveInvalidatesSessionCache(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HERDR_PLUGIN_STATE_DIR", dir)
	path := state.SessionCachePath(dir)
	require.NoError(t, os.WriteFile(path, []byte("cached"), 0600))
	a := New()
	require.NoError(t, a.settingsSaved("config.toml"))
	_, err := os.Stat(path)
	require.ErrorIs(t, err, os.ErrNotExist)
	require.NoError(t, a.settingsSaved("config.toml"), "missing cache is already invalidated")
}

func TestSettingsReloadToleratesUnavailableHerdr(t *testing.T) {
	configureFakeSources(t, "")
	cfg := config.Default()
	workspace := ""
	lastHerdr := []model.Session{{Source: "herdr", Name: "api", Path: "/live/api", WorkspaceID: "w1"}}
	var warnings []string
	result, err := New().reloadPickerState(context.Background(), cfg, herdr.NewCLIClient(), &workspace, &lastHerdr, func(format string, args ...any) { warnings = append(warnings, fmt.Sprintf(format, args...)) }, true)
	require.NoError(t, err)
	assert.Contains(t, strings.Join(warnings, "\n"), "herdr workspaces unavailable")
	assert.Equal(t, lastHerdr, result.HerdrWorkspaces, "keeps last known workspace metadata")
	assert.Equal(t, lastHerdr, result.Sessions, "keeps last known workspaces listed")
}

func TestWorkspaceCloseReloadRejectsUnavailableHerdr(t *testing.T) {
	configureFakeSources(t, "")
	cfg := config.Default()
	workspace := ""
	_, err := New().reloadPickerState(context.Background(), cfg, herdr.NewCLIClient(), &workspace, new([]model.Session), func(string, ...any) {}, false)
	require.Error(t, err)
}

package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fullerzz/herdr-plugin-sesh/internal/state"
)

func TestVersionCommand(t *testing.T) {
	var out bytes.Buffer
	a := &App{Out: &out, Err: &bytes.Buffer{}}
	if err := a.Run(context.Background(), []string{"--version"}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "herdr-sesh 0.1.0-dev" {
		t.Fatalf("got %q", out.String())
	}
}

func TestConfigPathCommand(t *testing.T) {
	var out bytes.Buffer
	a := &App{Out: &out, Err: &bytes.Buffer{}}
	if err := a.Run(context.Background(), []string{"config", "path"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "sesh.toml") {
		t.Fatalf("got %q", out.String())
	}
}

func TestListIgnoresCorruptSessionCache(t *testing.T) {
	d := t.TempDir()
	cfgPath := filepath.Join(d, "sesh.toml")
	if err := os.WriteFile(cfgPath, []byte("cache = true\n[[session]]\nname = \"api\"\npath = \"/tmp/api\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(d, "state")
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "sessions.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_PLUGIN_STATE_DIR", stateDir)

	var out, errb bytes.Buffer
	a := &App{Out: &out, Err: &errb}
	if err := a.Run(context.Background(), []string{"list", "--json", "--config", cfgPath}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"name": "api"`) {
		t.Fatalf("output = %q", out.String())
	}
	if !strings.Contains(errb.String(), "warning: ignoring session cache") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestListWarnsWhenSessionCacheCannotBeSaved(t *testing.T) {
	d := t.TempDir()
	cfgPath := filepath.Join(d, "sesh.toml")
	if err := os.WriteFile(cfgPath, []byte("cache = true\n[[session]]\nname = \"api\"\npath = \"/tmp/api\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(d, "state-file")
	if err := os.WriteFile(statePath, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_PLUGIN_STATE_DIR", statePath)

	var out, errb bytes.Buffer
	a := &App{Out: &out, Err: &errb}
	if err := a.Run(context.Background(), []string{"list", "--json", "--config", cfgPath}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"name": "api"`) {
		t.Fatalf("output = %q", out.String())
	}
	if !strings.Contains(errb.String(), "warning: ignoring session cache") || !strings.Contains(errb.String(), "warning: could not save session cache") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestPickerJSONCommand(t *testing.T) {
	var out bytes.Buffer
	a := &App{Out: &out, Err: &bytes.Buffer{}}
	if err := a.Run(context.Background(), []string{"picker", "--json", "--config", filepath.Join("..", "..", "testdata", "sesh.toml")}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"name": "sesh"`) {
		t.Fatalf("output = %q", out.String())
	}
}

func TestPreviewCommandUsesExplicitConfig(t *testing.T) {
	d := t.TempDir()
	targetDir := filepath.Join(d, "target")
	if err := os.Mkdir(targetDir, 0700); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(d, "sesh.toml")
	if err := os.WriteFile(cfgPath, []byte("[default_session]\npreview_command = \"printf configured:%s {}\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	fakeBin := filepath.Join(d, "bin")
	if err := os.MkdirAll(fakeBin, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"herdr", "zoxide"} {
		//nolint:gosec // test creates local executable fixtures.
		if err := os.WriteFile(filepath.Join(fakeBin, name), []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	//nolint:gosec // test creates a local executable fixture.
	if err := os.WriteFile(filepath.Join(fakeBin, "eza"), []byte("#!/bin/sh\nprintf 'default:%s\\n' \"$*\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_BIN_PATH", filepath.Join(fakeBin, "herdr"))
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	var out bytes.Buffer
	a := &App{Out: &out, Err: &bytes.Buffer{}}
	if err := a.Run(context.Background(), []string{"preview", "--config", cfgPath, targetDir}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "configured:") || !strings.Contains(out.String(), targetDir) {
		t.Fatalf("output = %q", out.String())
	}
}

func TestLastFocusesPreviousWorkspaceAndRotatesHistory(t *testing.T) {
	d := t.TempDir()
	stateDir := filepath.Join(d, "state")
	if err := state.SaveHistory(stateDir, state.History{Workspaces: []string{"current", "previous", "older"}}); err != nil {
		t.Fatal(err)
	}
	fakeHerdr := filepath.Join(d, "herdr")
	logPath := filepath.Join(d, "herdr.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" > \"$HERDR_FAKE_LOG\"\n"
	//nolint:gosec // test creates a local executable fixture.
	if err := os.WriteFile(fakeHerdr, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_BIN_PATH", fakeHerdr)
	t.Setenv("HERDR_FAKE_LOG", logPath)
	t.Setenv("HERDR_PLUGIN_STATE_DIR", stateDir)
	t.Setenv("HERDR_WORKSPACE_ID", "current")

	a := &App{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}
	if err := a.Run(context.Background(), []string{"last"}); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // logPath is a test-owned temp file.
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(log)); got != "workspace focus previous" {
		t.Fatalf("herdr args = %q", got)
	}
	h, err := state.LoadHistory(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"previous", "current", "older"}
	if !reflect.DeepEqual(h.Workspaces, want) {
		t.Fatalf("workspaces=%#v want %#v", h.Workspaces, want)
	}
}

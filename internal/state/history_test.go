package state

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestHistoryRecordsMostRecentWithoutDuplicates(t *testing.T) {
	d := t.TempDir()
	if err := Record(d, "a"); err != nil {
		t.Fatal(err)
	}
	if err := Record(d, "b"); err != nil {
		t.Fatal(err)
	}
	if err := Record(d, "b"); err != nil {
		t.Fatal(err)
	}
	last, ok, err := Last(d)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || last != "a" {
		t.Fatalf("last=%q ok=%v", last, ok)
	}
}

func TestHistoryNoopsWithoutStateDir(t *testing.T) {
	if err := Record("", "ws1"); err != nil {
		t.Fatal(err)
	}
	if err := RecordSwitch("", "ws1", "ws2"); err != nil {
		t.Fatal(err)
	}
	last, ok, err := Last("")
	if err != nil {
		t.Fatal(err)
	}
	if ok || last != "" {
		t.Fatalf("last=%q ok=%v", last, ok)
	}
	previous, ok, err := Previous("", "ws1")
	if err != nil {
		t.Fatal(err)
	}
	if ok || previous != "" {
		t.Fatalf("previous=%q ok=%v", previous, ok)
	}
}

func TestRecordRecoversCorruptHistory(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "history.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Record(d, "ws1"); err != nil {
		t.Fatal(err)
	}
	h, err := LoadHistory(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Workspaces) != 1 || h.Workspaces[0] != "ws1" {
		t.Fatalf("history=%#v", h)
	}
}

func TestPreviousSkipsCurrentWorkspace(t *testing.T) {
	d := t.TempDir()
	if err := SaveHistory(d, History{Workspaces: []string{"current", "previous", "older"}}); err != nil {
		t.Fatal(err)
	}
	previous, ok, err := Previous(d, "current")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || previous != "previous" {
		t.Fatalf("previous=%q ok=%v", previous, ok)
	}
}

func TestRecordSwitchRotatesPreviousWorkspace(t *testing.T) {
	d := t.TempDir()
	if err := SaveHistory(d, History{Workspaces: []string{"current", "previous", "older"}}); err != nil {
		t.Fatal(err)
	}
	if err := RecordSwitch(d, "current", "previous"); err != nil {
		t.Fatal(err)
	}
	h, err := LoadHistory(d)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"previous", "current", "older"}
	if !reflect.DeepEqual(h.Workspaces, want) {
		t.Fatalf("workspaces=%#v want %#v", h.Workspaces, want)
	}
}

func TestRecordSwitchDeduplicatesAndCapsHistory(t *testing.T) {
	d := t.TempDir()
	workspaces := []string{"target", "from"}
	for i := 0; i < maxWorkspaces+20; i++ {
		workspaces = append(workspaces, fmt.Sprintf("ws-%02d", i))
	}
	if err := SaveHistory(d, History{Workspaces: workspaces}); err != nil {
		t.Fatal(err)
	}
	if err := RecordSwitch(d, "from", "target"); err != nil {
		t.Fatal(err)
	}
	h, err := LoadHistory(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Workspaces) != maxWorkspaces {
		t.Fatalf("len=%d want %d", len(h.Workspaces), maxWorkspaces)
	}
	if h.Workspaces[0] != "target" || h.Workspaces[1] != "from" {
		t.Fatalf("workspaces start=%#v", h.Workspaces[:2])
	}
	seen := map[string]bool{}
	for _, id := range h.Workspaces {
		if seen[id] {
			t.Fatalf("duplicate workspace %q in %#v", id, h.Workspaces)
		}
		seen[id] = true
	}
}

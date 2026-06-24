package state

import (
	"os"
	"path/filepath"
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
	last, ok, err := Last("")
	if err != nil {
		t.Fatal(err)
	}
	if ok || last != "" {
		t.Fatalf("last=%q ok=%v", last, ok)
	}
}

func TestRecordRecoversCorruptHistory(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "history.json"), []byte("{"), 0644); err != nil {
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

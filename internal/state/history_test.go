package state

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"
	"time"
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

func TestRecordKeepsCurrentHistoryFile(t *testing.T) {
	d := t.TempDir()
	if err := SaveHistory(d, History{Workspaces: []string{"current", "older"}}); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(Path(d))
	if err != nil {
		t.Fatal(err)
	}

	if err := Record(d, "current"); err != nil {
		t.Fatal(err)
	}

	after, err := os.Stat(Path(d))
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) {
		t.Fatal("Record rewrote history.json for the current workspace")
	}
}

func TestHistoryMutationsWaitForProcessLock(t *testing.T) {
	if os.Getenv("HERDR_SESH_HISTORY_LOCK_HELPER") == "1" {
		runHistoryLockHelper(t)
		return
	}

	for _, tc := range []struct {
		name   string
		action string
		setup  func(string) error
	}{
		{
			name:   "record",
			action: "record",
		},
		{
			name:   "record switch",
			action: "record-switch",
		},
		{
			name:   "remove workspace",
			action: "remove-workspace",
			setup: func(dir string) error {
				return SaveHistory(dir, History{Workspaces: []string{"remove"}})
			},
		},
		{
			name:   "save history",
			action: "save-history",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := t.TempDir()
			if tc.setup != nil {
				if err := tc.setup(d); err != nil {
					t.Fatal(err)
				}
			}
			assertHistoryMutationWaitsForLock(t, d, tc.action)
		})
	}
}

func assertHistoryMutationWaitsForLock(t *testing.T, dir, action string) {
	t.Helper()
	//nolint:gosec // Test lock path is derived from t.TempDir().
	lock, err := os.OpenFile(filepath.Join(dir, "history.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := lock.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	locked := true
	defer func() {
		if locked {
			_ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
		}
	}()

	//nolint:gosec // Test re-executes its own test binary.
	cmd := exec.Command(os.Args[0], "-test.run=^TestHistoryMutationsWaitForProcessLock$")
	cmd.Env = append(os.Environ(), "HERDR_SESH_HISTORY_LOCK_HELPER=1", "HERDR_SESH_HISTORY_LOCK_DIR="+dir, "HERDR_SESH_HISTORY_LOCK_ACTION="+action)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waitDone := make(chan struct{})
	var waitErr error
	go func() {
		waitErr = cmd.Wait()
		close(waitDone)
	}()
	t.Cleanup(func() {
		select {
		case <-waitDone:
			return
		default:
		}
		_ = cmd.Process.Kill()
		select {
		case <-waitDone:
		case <-time.After(time.Second):
			t.Error("history mutation helper did not exit after being killed")
		}
	})

	lines := make(chan string, 8)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
	}()
	select {
	case line := <-lines:
		if line != "ready" {
			t.Fatalf("helper readiness = %q", line)
		}
	case <-time.After(time.Second):
		t.Fatal("history mutation helper did not start")
	}

	select {
	case line := <-lines:
		t.Fatalf("history mutation bypassed held process lock: %q", line)
	case <-time.After(100 * time.Millisecond):
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	locked = false
	select {
	case <-waitDone:
		if waitErr != nil {
			t.Fatal(waitErr)
		}
	case <-time.After(2 * time.Second):
		_ = cmd.Process.Kill()
		select {
		case <-waitDone:
			t.Fatalf("history mutation helper did not exit after lock release: %v", waitErr)
		case <-time.After(time.Second):
			t.Fatal("history mutation helper did not exit after being killed")
		}
	}
}

func runHistoryLockHelper(t *testing.T) {
	t.Helper()
	dir := os.Getenv("HERDR_SESH_HISTORY_LOCK_DIR")
	action := os.Getenv("HERDR_SESH_HISTORY_LOCK_ACTION")
	fmt.Println("ready")
	var mutateErr error
	switch action {
	case "record":
		mutateErr = Record(dir, "record")
	case "record-switch":
		mutateErr = RecordSwitch(dir, "from", "to")
	case "remove-workspace":
		mutateErr = RemoveWorkspace(dir, "remove")
	case "save-history":
		mutateErr = SaveHistory(dir, History{Workspaces: []string{"saved"}})
	default:
		t.Fatalf("unknown helper action %q", action)
	}
	if mutateErr != nil {
		t.Fatal(mutateErr)
	}
	fmt.Println("done")
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

func TestRemoveWorkspacePrunesHistory(t *testing.T) {
	d := t.TempDir()
	if err := SaveHistory(d, History{Workspaces: []string{"current", "closed", "older"}}); err != nil {
		t.Fatal(err)
	}
	if err := RemoveWorkspace(d, "closed"); err != nil {
		t.Fatal(err)
	}
	h, err := LoadHistory(d)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"current", "older"}
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

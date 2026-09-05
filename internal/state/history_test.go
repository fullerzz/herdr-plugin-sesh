package state

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionHistoryDirIsolatesSockets(t *testing.T) {
	stateDir := t.TempDir()
	firstDir, err := SessionHistoryDir(stateDir, "/tmp/herdr-a.sock")
	require.NoError(t, err)
	secondDir, err := SessionHistoryDir(stateDir, "/tmp/herdr-b.sock")
	require.NoError(t, err)
	require.NotEqual(t, secondDir, firstDir)

	require.NoError(t, SaveHistory(firstDir, History{Workspaces: []string{"w1", "first-only"}}))
	require.NoError(t, SaveHistory(secondDir, History{Workspaces: []string{"w1", "second-only"}}))
	require.NoError(t, RecordSwitch(firstDir, "w1", "first-only"))
	require.NoError(t, RemoveWorkspace(secondDir, "w1"))

	first, err := LoadHistory(firstDir)
	require.NoError(t, err)
	second, err := LoadHistory(secondDir)
	require.NoError(t, err)
	require.Equal(t, []string{"first-only", "w1"}, first.Workspaces)
	assert.Equal(t, []string{"second-only"}, second.Workspaces)
}

func TestSessionHistoryDirMigratesOnlyDefaultSessionHistory(t *testing.T) {
	stateDir := t.TempDir()
	legacy := History{Workspaces: []string{"current", "previous"}}
	require.NoError(t, SaveHistory(stateDir, legacy))

	namedDir, err := SessionHistoryDir(stateDir, filepath.Join("/config", "herdr", "sessions", "work", "herdr.sock"))
	require.NoError(t, err)
	named, err := LoadHistory(namedDir)
	require.NoError(t, err)
	require.Empty(t, named.Workspaces)

	defaultDir, err := SessionHistoryDir(stateDir, filepath.Join("/config", "herdr", "herdr.sock"))
	require.NoError(t, err)
	migrated, err := LoadHistory(defaultDir)
	require.NoError(t, err)
	assert.Equal(t, legacy, migrated)
}

func TestSessionHistoryDirMigrationWaitsForDestinationLock(t *testing.T) {
	stateDir := t.TempDir()
	socketPath := filepath.Join(t.TempDir(), "herdr", "herdr.sock")
	sessionDir, err := SessionHistoryDir(stateDir, socketPath)
	require.NoError(t, err)
	legacy := History{Workspaces: []string{"legacy"}}
	require.NoError(t, SaveHistory(stateDir, legacy))
	require.NoError(t, os.MkdirAll(sessionDir, 0700))

	//nolint:gosec // Test lock path is derived from t.TempDir().
	lock, err := os.OpenFile(filepath.Join(sessionDir, "history.lock"), os.O_CREATE|os.O_RDWR, 0600)
	require.NoError(t, err)
	defer func() { _ = lock.Close() }()
	require.NoError(t, syscall.Flock(int(lock.Fd()), syscall.LOCK_EX))
	locked := true
	defer func() {
		if locked {
			_ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
		}
	}()

	done := make(chan error, 1)
	go func() {
		_, migrateErr := SessionHistoryDir(stateDir, socketPath)
		done <- migrateErr
	}()
	select {
	case err := <-done:
		require.FailNow(t, fmt.Sprintf("migration bypassed destination lock: %v", err))
	case <-time.After(100 * time.Millisecond):
	}

	concurrent := History{Workspaces: []string{"concurrent"}}
	require.NoError(t, writeJSONFile(Path(sessionDir), concurrent))
	require.NoError(t, syscall.Flock(int(lock.Fd()), syscall.LOCK_UN))
	locked = false
	require.NoError(t, <-done)

	got, err := LoadHistory(sessionDir)
	require.NoError(t, err)
	assert.Equal(t, concurrent, got)
}

func TestHistoryRecordsMostRecentWithoutDuplicates(t *testing.T) {
	d := t.TempDir()
	require.NoError(t, Record(d, "a"))
	require.NoError(t, Record(d, "b"))
	require.NoError(t, Record(d, "b"))
	last, ok, err := Last(d)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "a", last)
}

func TestRecordKeepsCurrentHistoryFile(t *testing.T) {
	d := t.TempDir()
	require.NoError(t, SaveHistory(d, History{Workspaces: []string{"current", "older"}}))
	before, err := os.Stat(Path(d))
	require.NoError(t, err)

	require.NoError(t, Record(d, "current"))

	after, err := os.Stat(Path(d))
	require.NoError(t, err)
	assert.True(t, os.SameFile(before, after), "Record rewrote history.json for the current workspace")
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
				require.NoError(t, tc.setup(d))
			}
			assertHistoryMutationWaitsForLock(t, d, tc.action)
		})
	}
}

func TestHistoryMutationTimesOutWaitingForProcessLock(t *testing.T) {
	d := t.TempDir()
	//nolint:gosec // Test lock path is derived from t.TempDir().
	lock, err := os.OpenFile(filepath.Join(d, "history.lock"), os.O_CREATE|os.O_RDWR, 0600)
	require.NoError(t, err)
	defer func() { _ = lock.Close() }()
	require.NoError(t, syscall.Flock(int(lock.Fd()), syscall.LOCK_EX))
	locked := true
	defer func() {
		if locked {
			_ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
		}
	}()

	done := make(chan error, 1)
	go func() { done <- Record(d, "blocked") }()
	select {
	case err := <-done:
		require.ErrorIs(t, err, ErrHistoryLockTimeout)
	case <-time.After(2 * time.Second):
		require.NoError(t, syscall.Flock(int(lock.Fd()), syscall.LOCK_UN))
		locked = false
		<-done
		require.FailNow(t, "history mutation remained blocked for two seconds")
	}
}

func assertHistoryMutationWaitsForLock(t *testing.T, dir, action string) {
	t.Helper()
	//nolint:gosec // Test lock path is derived from t.TempDir().
	lock, err := os.OpenFile(filepath.Join(dir, "history.lock"), os.O_CREATE|os.O_RDWR, 0600)
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, lock.Close())
	})
	require.NoError(t, syscall.Flock(int(lock.Fd()), syscall.LOCK_EX))
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
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
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
			assert.Fail(t, "history mutation helper did not exit after being killed")
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
		require.Equal(t, "ready", line)
	case <-time.After(time.Second):
		require.FailNow(t, "history mutation helper did not start")
	}

	select {
	case line := <-lines:
		require.FailNow(t, fmt.Sprintf("history mutation bypassed held process lock: %q", line))
	case <-time.After(100 * time.Millisecond):
	}
	require.NoError(t, syscall.Flock(int(lock.Fd()), syscall.LOCK_UN))
	locked = false
	select {
	case <-waitDone:
		require.NoError(t, waitErr)
	case <-time.After(2 * time.Second):
		_ = cmd.Process.Kill()
		select {
		case <-waitDone:
			require.FailNow(t, fmt.Sprintf("history mutation helper did not exit after lock release: %v", waitErr))
		case <-time.After(time.Second):
			require.FailNow(t, "history mutation helper did not exit after being killed")
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
		require.FailNow(t, fmt.Sprintf("unknown helper action %q", action))
	}
	require.NoError(t, mutateErr)
	fmt.Println("done")
}

func TestHistoryNoopsWithoutStateDir(t *testing.T) {
	require.NoError(t, Record("", "ws1"))
	require.NoError(t, RecordSwitch("", "ws1", "ws2"))
	last, ok, err := Last("")
	require.NoError(t, err)
	require.False(t, ok)
	require.Empty(t, last)
	previous, ok, err := Previous("", "ws1")
	require.NoError(t, err)
	require.False(t, ok)
	assert.Empty(t, previous)
}

func TestRecordRecoversCorruptHistory(t *testing.T) {
	d := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(d, "history.json"), []byte("{"), 0600))
	require.NoError(t, Record(d, "ws1"))
	h, err := LoadHistory(d)
	require.NoError(t, err)
	require.Len(t, h.Workspaces, 1)
	assert.Equal(t, "ws1", h.Workspaces[0])
}

func TestPreviousSkipsCurrentWorkspace(t *testing.T) {
	d := t.TempDir()
	require.NoError(t, SaveHistory(d, History{Workspaces: []string{"current", "previous", "older"}}))
	previous, ok, err := Previous(d, "current")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "previous", previous)
}

func TestRecordSwitchRotatesPreviousWorkspace(t *testing.T) {
	d := t.TempDir()
	require.NoError(t, SaveHistory(d, History{Workspaces: []string{"current", "previous", "older"}}))
	require.NoError(t, RecordSwitch(d, "current", "previous"))
	h, err := LoadHistory(d)
	require.NoError(t, err)
	want := []string{"previous", "current", "older"}
	assert.Equal(t, want, h.Workspaces)
}

func TestRemoveWorkspacePrunesHistory(t *testing.T) {
	d := t.TempDir()
	require.NoError(t, SaveHistory(d, History{Workspaces: []string{"current", "closed", "older"}}))
	require.NoError(t, RemoveWorkspace(d, "closed"))
	h, err := LoadHistory(d)
	require.NoError(t, err)
	want := []string{"current", "older"}
	assert.Equal(t, want, h.Workspaces)
}

func TestRecordSwitchDeduplicatesAndCapsHistory(t *testing.T) {
	d := t.TempDir()
	workspaces := []string{"target", "from"}
	for i := 0; i < maxWorkspaces+20; i++ {
		workspaces = append(workspaces, fmt.Sprintf("ws-%02d", i))
	}
	require.NoError(t, SaveHistory(d, History{Workspaces: workspaces}))
	require.NoError(t, RecordSwitch(d, "from", "target"))
	h, err := LoadHistory(d)
	require.NoError(t, err)
	require.Len(t, h.Workspaces, maxWorkspaces)
	require.Equal(t, "target", h.Workspaces[0])
	require.Equal(t, "from", h.Workspaces[1])
	seen := map[string]bool{}
	for _, id := range h.Workspaces {
		require.False(t, seen[id])
		seen[id] = true
	}
}

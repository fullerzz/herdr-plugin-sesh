package app

import (
	"context"
	"os"
	"testing"

	"github.com/fullerzz/herdr-plugin-sesh/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestHistoryWatcherPIDsFromProcLocks(t *testing.T) {
	for _, tt := range []struct {
		name  string
		locks string
		want  []int
		fail  bool
	}{
		{name: "no locks"},
		{
			name: "only matching exclusive flock owner",
			locks: `1: POSIX ADVISORY WRITE 111 08:01:900 0 EOF
2: FLOCK ADVISORY READ 222 08:01:900 0 EOF
3: FLOCK ADVISORY WRITE 333 08:02:900 0 EOF
4: FLOCK ADVISORY WRITE 444 08:01:901 0 EOF
5: FLOCK ADVISORY WRITE 555 08:01:900 0 EOF
5: -> FLOCK ADVISORY WRITE 666 08:01:900 0 EOF
`,
			want: []int{555},
		},
		{name: "invalid identity", locks: "1: FLOCK ADVISORY WRITE 123 invalid 0 EOF", fail: true},
		{name: "invalid pid", locks: "1: FLOCK ADVISORY WRITE nope 08:01:900 0 EOF", fail: true},
		{name: "process group pid", locks: "1: FLOCK ADVISORY WRITE 0 08:01:900 0 EOF", fail: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := historyWatcherPIDsFromProcLocks(tt.locks, unix.Mkdev(8, 1), 900)
			if tt.fail {
				require.Error(t, err)
				assert.Empty(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestHistoryWatcherPIDsFindsElectionHolder(t *testing.T) {
	dir := t.TempDir()
	release, acquired, err := state.TryHistoryWatcherLock(dir, "socket")
	require.NoError(t, err)
	require.True(t, acquired)
	defer func() { assert.NoError(t, release()) }()
	pids, err := historyWatcherPIDs(context.Background(), state.HistoryWatcherLockPath(dir, "socket"))
	require.NoError(t, err)
	assert.Equal(t, []int{os.Getpid()}, pids)
}

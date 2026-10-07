package app

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

func historyWatcherPIDs(ctx context.Context, lockPath string) ([]int, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var stat unix.Stat_t
	if err := unix.Stat(lockPath, &stat); err != nil {
		return nil, err
	}
	locks, err := os.ReadFile("/proc/locks")
	if err != nil {
		return nil, err
	}
	return historyWatcherPIDsFromProcLocks(string(locks), uint64(stat.Dev), stat.Ino)
}

// /proc/locks exposes flock owners even for older watchers without PID metadata.
func historyWatcherPIDsFromProcLocks(locks string, dev, inode uint64) ([]int, error) {
	var pids []int
	for line := range strings.SplitSeq(locks, "\n") {
		fields := strings.Fields(line)
		// Ignore other lock types and blocked waiters (whose second field is ->).
		if len(fields) < 8 || fields[1] != "FLOCK" || fields[3] != "WRITE" {
			continue
		}
		var major, minor uint32
		var lockInode uint64
		if _, err := fmt.Sscanf(fields[5], "%x:%x:%d", &major, &minor, &lockInode); err != nil {
			return nil, fmt.Errorf("decode /proc/locks file identity: %w", err)
		}
		if major != unix.Major(dev) || minor != unix.Minor(dev) || lockInode != inode {
			continue
		}
		pid, err := strconv.Atoi(fields[4])
		if err != nil || pid <= 0 {
			return nil, fmt.Errorf("invalid /proc/locks holder PID %q", fields[4])
		}
		pids = append(pids, pid)
	}
	return pids, nil
}

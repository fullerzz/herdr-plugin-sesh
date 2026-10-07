package app

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

func historyWatcherPIDs(ctx context.Context, lockPath string) ([]int, error) {
	// macOS ships lsof here; hook PATH need not include system utilities.
	//nolint:gosec // lockPath is derived from the plugin-owned state directory.
	return historyWatcherPIDsFromLSOF(exec.CommandContext(ctx, "/usr/sbin/lsof", "-t", "--", lockPath))
}

func historyWatcherPIDsFromLSOF(cmd *exec.Cmd) ([]int, error) {
	out, err := cmd.Output()
	// Only exit 1 with no output means no matching open files.
	var exitErr *exec.ExitError
	if err != nil && (!errors.As(err, &exitErr) || exitErr.ExitCode() != 1 || len(out) > 0) {
		return nil, err
	}
	var pids []int
	for _, field := range strings.Fields(string(out)) {
		pid, err := strconv.Atoi(field)
		if err != nil || pid <= 0 {
			return nil, fmt.Errorf("invalid lsof holder PID %q", field)
		}
		pids = append(pids, pid)
	}
	return pids, nil
}

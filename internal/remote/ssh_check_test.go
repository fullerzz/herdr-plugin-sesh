package remote

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fakeSSHCheck(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ssh"), []byte("#!/bin/sh\n"+script), 0700)) // #nosec G306 -- executable fixture in a test-owned directory.
	t.Setenv("PATH", dir)
	return dir
}

func TestCheckSSHUsesReadOnlyNoninteractiveCommand(t *testing.T) {
	dir := fakeSSHCheck(t, "printf '%s\\n' \"$@\" > \"$SSH_CHECK_ARGS\"\nread input && exit 1\nexit 0\n")
	argsPath := filepath.Join(dir, "args")
	t.Setenv("SSH_CHECK_ARGS", argsPath)
	require.NoError(t, CheckSSH(context.Background(), Peer{SSH: "user@host", Session: "ignored"}))
	args, err := os.ReadFile(argsPath) // #nosec G304 -- test-owned temporary fixture.
	require.NoError(t, err)
	assert.Equal(t, []string{
		"-T", "-n", "-o", "BatchMode=yes", "-o", "ForwardAgent=no",
		"-o", "StrictHostKeyChecking=yes", "-o", "UpdateHostKeys=no", "-o", "ConnectTimeout=5",
		"-o", "ConnectionAttempts=1", "-o", "ClearAllForwardings=yes",
		"-o", "PermitLocalCommand=no", "-o", "ControlPath=none", "--", "user@host", "true",
	}, strings.Split(strings.TrimSpace(string(args)), "\n"))
}

func TestCheckSSHReportsBoundedSanitizedFailure(t *testing.T) {
	for _, detail := range []string{"Permission denied (publickey).", "Host key verification failed.", "Connection refused"} {
		t.Run(detail, func(t *testing.T) {
			fakeSSHCheck(t, "printf '%s\\n' \"$SSH_CHECK_ERROR\" >&2\nexit 255\n")
			t.Setenv("SSH_CHECK_ERROR", "\x1b\n"+detail+"\t"+strings.Repeat("x", 5000))
			err := CheckSSH(context.Background(), Peer{SSH: "host"})
			require.ErrorContains(t, err, detail)
			assert.Contains(t, err.Error(), "exit status 255")
			assert.NotContains(t, err.Error(), "\x1b")
			assert.NotContains(t, err.Error(), "\n")
			assert.NotContains(t, err.Error(), "\t")
			assert.Less(t, len(err.Error()), 600)
		})
	}
}

func TestCheckSSHRejectsInvalidDestinationBeforeStarting(t *testing.T) {
	dir := fakeSSHCheck(t, "printf started > \"$SSH_CHECK_MARKER\"\n")
	marker := filepath.Join(dir, "marker")
	t.Setenv("SSH_CHECK_MARKER", marker)
	require.ErrorContains(t, CheckSSH(context.Background(), Peer{SSH: "-oProxyCommand=bad"}), "invalid SSH destination")
	_, err := os.Stat(marker)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestCheckSSHReportsMissingExecutable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	require.ErrorContains(t, CheckSSH(context.Background(), Peer{SSH: "host"}), "executable file not found")
}

func TestCheckSSHCancellationReapsProcess(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		t.Run(strconv.FormatBool(deadline), func(t *testing.T) {
			dir := fakeSSHCheck(t, "printf '%s' \"$$\" > \"$SSH_CHECK_PID\"\nexec /bin/sleep 30\n")
			pidPath := filepath.Join(dir, "pid")
			t.Setenv("SSH_CHECK_PID", pidPath)
			ctx, cancel := context.WithCancel(context.Background())
			wantErr := context.Canceled
			if deadline {
				cancel()
				ctx, cancel = context.WithTimeout(context.Background(), 200*time.Millisecond)
				wantErr = context.DeadlineExceeded
			}
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- CheckSSH(ctx, Peer{SSH: "host"}) }()
			require.Eventually(t, func() bool {
				pid, err := os.ReadFile(pidPath) // #nosec G304 -- test-owned temporary fixture.
				return err == nil && len(pid) > 0
			}, time.Second, time.Millisecond)
			if !deadline {
				cancel()
			}
			select {
			case err := <-done:
				require.ErrorIs(t, err, wantErr)
			case <-time.After(2 * time.Second):
				t.Fatal("SSH check did not stop after cancellation")
			}
			pidBytes, err := os.ReadFile(pidPath) // #nosec G304 -- test-owned temporary fixture.
			require.NoError(t, err)
			pid, err := strconv.Atoi(string(pidBytes))
			require.NoError(t, err)
			require.ErrorIs(t, syscall.Kill(pid, 0), syscall.ESRCH)
		})
	}
}

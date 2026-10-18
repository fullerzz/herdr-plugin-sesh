package app

import (
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAttachmentProcess(t *testing.T) {
	mode := os.Getenv("HERDR_SESH_TEST_ATTACHMENT")
	if mode == "" {
		return
	}
	dir := os.Getenv("HERDR_SESH_TEST_ATTACHMENT_DIR")
	stopped := make(chan os.Signal, 1)
	if mode == "stubborn" {
		signal.Ignore(syscall.SIGTERM)
	} else {
		signal.Notify(stopped, syscall.SIGTERM)
	}
	//nolint:gosec // Parent test passes its private temporary directory to this helper.
	if err := os.WriteFile(filepath.Join(dir, "ready"), nil, 0o600); err != nil {
		os.Exit(2)
	}
	if mode == "stubborn" {
		for {
			time.Sleep(time.Hour)
		}
	}
	<-stopped
	//nolint:gosec // Parent test passes its private temporary directory to this helper.
	if err := os.WriteFile(filepath.Join(dir, "cleaned"), nil, 0o600); err != nil {
		os.Exit(3)
	}
	os.Exit(0)
}

func startAttachmentProcess(t *testing.T, mode string) (*exec.Cmd, <-chan error, string) {
	t.Helper()
	dir := t.TempDir()
	//nolint:gosec // Re-execute this test binary with one fixed helper test.
	child := exec.Command(os.Args[0], "-test.run=^TestAttachmentProcess$")
	child.Env = append(os.Environ(), "HERDR_SESH_TEST_ATTACHMENT="+mode, "HERDR_SESH_TEST_ATTACHMENT_DIR="+dir)
	require.NoError(t, child.Start())
	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	t.Cleanup(func() {
		_ = child.Process.Kill()
	})
	require.Eventually(t, func() bool {
		_, err := os.Stat(filepath.Join(dir, "ready"))
		return err == nil
	}, 5*time.Second, 10*time.Millisecond)
	return child, done, dir
}

func TestStopAttachmentAllowsTerminalCleanup(t *testing.T) {
	child, done, dir := startAttachmentProcess(t, "graceful")
	require.NoError(t, stopAttachment(child, done, 2*time.Second))
	assert.FileExists(t, filepath.Join(dir, "cleaned"))
	require.NotNil(t, child.ProcessState)
	assert.True(t, child.ProcessState.Success())
}

func TestStopAttachmentKillsUnresponsiveChild(t *testing.T) {
	child, done, dir := startAttachmentProcess(t, "stubborn")
	started := time.Now()
	require.NoError(t, stopAttachment(child, done, 100*time.Millisecond))
	assert.Less(t, time.Since(started), 3*time.Second)
	assert.NoFileExists(t, filepath.Join(dir, "cleaned"))
	require.NotNil(t, child.ProcessState)
	status, ok := child.ProcessState.Sys().(syscall.WaitStatus)
	require.True(t, ok)
	assert.Equal(t, syscall.SIGKILL, status.Signal())
}

func TestStopAttachmentAlreadyExited(t *testing.T) {
	child := exec.Command("/bin/sh", "-c", "exit 0")
	require.NoError(t, child.Run())
	done := make(chan error, 1)
	done <- nil
	require.NoError(t, stopAttachment(child, done, time.Second))
}

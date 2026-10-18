package app

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func stopAttachment(child *exec.Cmd, done <-chan error, grace time.Duration) error {
	select {
	case <-done:
		return nil
	default:
	}
	// Herdr handles SIGTERM by restoring terminal modes before exiting.
	err := child.Process.Signal(syscall.SIGTERM)
	if errors.Is(err, os.ErrProcessDone) {
		err = nil
	}
	timer := time.NewTimer(grace)
	defer timer.Stop()
	select {
	case <-done:
		return err
	case <-timer.C:
		killErr := child.Process.Kill()
		if errors.Is(killErr, os.ErrProcessDone) {
			killErr = nil
		}
		<-done
		return errors.Join(err, killErr)
	}
}

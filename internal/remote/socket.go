package remote

import (
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// Keep the lock inode on disk: unlinking it permits independent concurrent locks.
func lockSocket(path string) (*os.File, error) {
	fd, err := unix.Open(path+".lock", unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0600)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path+".lock")
	st, err := f.Stat()
	if err == nil {
		owner, ok := st.Sys().(*syscall.Stat_t)
		if !st.Mode().IsRegular() || st.Mode().Perm() != 0600 || !ok || int(owner.Uid) != os.Getuid() || owner.Nlink != 1 {
			err = errors.New("socket lock must be a private regular file owned by the current user")
		}
	}
	if err == nil {
		err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB)
	}
	if err == nil {
		current, statErr := os.Lstat(path + ".lock")
		if statErr != nil {
			err = statErr
		} else if !os.SameFile(st, current) {
			err = errors.New("socket lock changed")
		}
	}
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("lock remote daemon: %w", err)
	}
	return f, nil
}

func recoverSocket(path string) error {
	st, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	owner, ok := st.Sys().(*syscall.Stat_t)
	if st.Mode()&os.ModeSocket == 0 || !ok || int(owner.Uid) != os.Getuid() {
		return errors.New("existing socket path is not a socket owned by the current user")
	}
	conn, err := net.DialTimeout("unix", path, 200*time.Millisecond)
	if err == nil {
		_ = conn.Close()
		return errors.New("socket already has a live listener")
	}
	// Only connection refusal proves a leftover socket; other failures are ambiguous.
	if !errors.Is(err, syscall.ECONNREFUSED) {
		return fmt.Errorf("check existing socket: %w", err)
	}
	current, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !os.SameFile(st, current) {
		return errors.New("socket changed during recovery")
	}
	return os.Remove(path)
}

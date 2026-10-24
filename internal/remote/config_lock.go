package remote

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// Keep the inode: unlinking would let concurrent writers acquire different locks.
func lockConfig(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0o600)
	if err != nil {
		return nil, fmt.Errorf("lock remote config: %w", err)
	}
	file := os.NewFile(uintptr(fd), path)
	info, err := file.Stat()
	if err == nil {
		owner, ok := info.Sys().(*syscall.Stat_t)
		if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || !ok || int(owner.Uid) != os.Getuid() || owner.Nlink != 1 {
			err = errors.New("remote config lock must be a private regular file owned by the current user")
		}
	}
	if err == nil {
		err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB)
		if errors.Is(err, unix.EWOULDBLOCK) {
			err = errors.New("remote config is busy; retry saving")
		}
	}
	if err == nil {
		current, statErr := os.Lstat(path) // #nosec G703 -- explicit config sidecar path; verify inode identity.
		if statErr != nil {
			err = statErr
		} else if !os.SameFile(info, current) {
			err = errors.New("remote config lock changed")
		}
	}
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("lock remote config: %w", err)
	}
	return file, nil
}

package remoteservice

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"syscall"

	"golang.org/x/sys/unix"
)

type transactionKey struct{}
type transaction struct {
	path   string
	active atomic.Bool
}

// WithLock protects an entire service/config transaction. Pass the callback's
// context to Enable and Disable, including recovery calls, to reuse the lock.
func (m *Manager) WithLock(ctx context.Context, operation func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if held, ok := ctx.Value(transactionKey{}).(*transaction); ok && held.path == m.lockPath && held.active.Load() {
		return operation(ctx)
	}
	file, err := lockService(m.lockPath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	held := &transaction{path: m.lockPath}
	held.active.Store(true)
	defer held.active.Store(false)
	if err = ctx.Err(); err != nil {
		return err
	}
	return operation(context.WithValue(ctx, transactionKey{}, held))
}

// Never unlink: all operations must continue to lock the same inode.
func lockService(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0o600)
	if err != nil {
		return nil, fmt.Errorf("lock remote service: %w", err)
	}
	file := os.NewFile(uintptr(fd), path)
	info, err := file.Stat()
	if err == nil {
		owner, ok := info.Sys().(*syscall.Stat_t)
		if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || !ok || int(owner.Uid) != os.Getuid() || owner.Nlink != 1 {
			err = errors.New("remote service lock must be a private regular file owned by the current user")
		}
	}
	if err == nil {
		err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB)
		if errors.Is(err, unix.EWOULDBLOCK) {
			err = errors.New("another remote service operation is in progress; retry when it finishes")
		}
	}
	if err == nil {
		current, statErr := os.Lstat(path) // #nosec G703 -- fixed home-derived lock path; inode identity is checked below.
		if statErr != nil {
			err = statErr
		} else if !os.SameFile(info, current) {
			err = errors.New("remote service lock changed")
		}
	}
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("lock remote service: %w", err)
	}
	return file, nil
}

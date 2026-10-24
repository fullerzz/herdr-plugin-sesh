package app

import "golang.org/x/sys/unix"

const (
	attachTermiosGet = unix.TIOCGETA
	attachTermiosSet = unix.TIOCSETA
)

// sstop is SSTOP from <sys/proc.h>.
const sstop = 4

func processStopped(pid int) bool {
	p, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	return err == nil && p.Proc.P_stat == sstop
}

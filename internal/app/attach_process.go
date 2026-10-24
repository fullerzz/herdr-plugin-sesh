package app

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/charmbracelet/x/term"
	"golang.org/x/sys/unix"
)

// stopGrace is how long a stopping client gets to restore terminal modes before SIGKILL.
const stopGrace = 2 * time.Second

// attachTerminal is the terminal the wrapper shares with each Herdr client. A
// nil *attachTerminal, or one without a file, runs clients without job control.
type attachTerminal struct {
	file    *os.File
	restore func() error
}

// newAttachTerminal saves f's current modes for every hand-back.
func newAttachTerminal(f *os.File) (*attachTerminal, error) {
	state, err := term.GetState(f.Fd())
	if err != nil {
		return nil, err
	}
	return &attachTerminal{file: f, restore: func() error { return term.Restore(f.Fd(), state) }}, nil
}

// handBack makes the wrapper's process group the foreground job again, then
// restores the wrapper's terminal modes. Restoring first would change modes
// under whichever group still owns the terminal.
func (t *attachTerminal) handBack() error {
	if t == nil {
		return nil
	}
	err := t.foreground(unix.Getpgrp())
	if t.restore != nil {
		err = errors.Join(err, t.restore())
	}
	return err
}

// foreground gives the terminal to process group pgid. A background process may
// do that only with SIGTTOU blocked or ignored, and ignoring it here would leak
// into every later child, so a short-lived child joins pgid and makes the change:
// Go performs it in the child with all signals blocked.
func (t *attachTerminal) foreground(pgid int) error {
	if t == nil || t.file == nil {
		return nil
	}
	bin, err := exec.LookPath("true")
	if err != nil {
		return err
	}
	cmd := exec.Command(bin) //nolint:gosec // true(1) from PATH runs with no arguments.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pgid: pgid, Foreground: true, Ctty: int(t.file.Fd())}
	return cmd.Run()
}

// attachJob is one Herdr client running as its own process group. `herdr
// --remote` is a launcher whose client is a separate child process, so stopping
// only the started process would orphan a client that still owns the terminal.
type attachJob struct {
	cmd    *exec.Cmd
	term   *attachTerminal
	exited chan struct{}
	err    error
}

// startAttachJob starts cmd as a new process group. With a terminal, the group
// is the foreground job and reads the terminal as its stdin.
func startAttachJob(cmd *exec.Cmd, tty *attachTerminal) (*attachJob, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if tty != nil && tty.file != nil {
		state, err := unix.IoctlGetTermios(int(tty.file.Fd()), attachTermiosGet)
		if err != nil {
			return nil, err
		}
		// The background wrapper must write live diagnostics; handBack restores TOSTOP.
		if state.Lflag&unix.TOSTOP != 0 {
			state.Lflag &^= unix.TOSTOP
			if err := unix.IoctlSetTermios(int(tty.file.Fd()), attachTermiosSet, state); err != nil {
				return nil, errors.Join(err, tty.handBack())
			}
		}
		cmd.Stdin = tty.file
		// With Foreground, Ctty is the terminal's descriptor in this process.
		cmd.SysProcAttr.Foreground, cmd.SysProcAttr.Ctty = true, int(tty.file.Fd())
	}
	// Registered before Start so a stop right after exec still wakes the watcher.
	children := make(chan os.Signal, 1)
	signal.Notify(children, syscall.SIGCHLD)
	if err := cmd.Start(); err != nil {
		signal.Stop(children)
		// The child may have taken the terminal before exec failed.
		return nil, errors.Join(err, tty.handBack())
	}
	j := &attachJob{cmd: cmd, term: tty, exited: make(chan struct{})}
	go func() { j.err = cmd.Wait(); close(j.exited) }()
	go j.resumeWhenStopped(children)
	return j, nil
}

// resumeWhenStopped continues the job whenever job control stops it, such as
// Ctrl-Z at an SSH prompt before Herdr enters raw mode. The wrapper's shell
// never sees that stop, so nothing else could continue the job, and the
// terminal would belong to a stopped group.
func (j *attachJob) resumeWhenStopped(children chan os.Signal) {
	defer signal.Stop(children)
	pgid := j.cmd.Process.Pid
	for {
		select {
		case <-j.exited:
			return
		case <-children:
			if processStopped(pgid) {
				// Retaking the terminal also undoes a stop on a background read.
				_ = j.term.foreground(pgid)
				_ = signalGroup(pgid, syscall.SIGCONT)
			}
		}
	}
}

// stop ends every process in the job, then hands the terminal back. It is the
// job's only teardown: a launcher that exits on its own can leave its client
// running in the group.
func (j *attachJob) stop(grace time.Duration) error {
	pgid := j.cmd.Process.Pid
	var err error
	// Once its last member is reaped, the group ID can be reused; signal only a live group.
	if groupExists(pgid) {
		// Herdr restores terminal modes on SIGTERM; SIGCONT lets a stopped member act on it.
		err = errors.Join(signalGroup(pgid, syscall.SIGTERM), signalGroup(pgid, syscall.SIGCONT))
		if !j.waitGroup(pgid, time.Now().Add(grace)) {
			err = errors.Join(err, signalGroup(pgid, syscall.SIGKILL))
			if !j.waitGroup(pgid, time.Now().Add(time.Second)) {
				err = errors.Join(err, errors.New("herdr client processes did not exit"))
			}
		}
	}
	return errors.Join(err, j.term.handBack())
}

// waitGroup reports whether the job's leader and every other member exited by deadline.
func (j *attachJob) waitGroup(pgid int, deadline time.Time) bool {
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	select {
	case <-j.exited:
	case <-timer.C:
		return false
	}
	for groupExists(pgid) {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(10 * time.Millisecond)
	}
	return true
}

func groupExists(pgid int) bool {
	return !errors.Is(syscall.Kill(-pgid, 0), syscall.ESRCH)
}

func signalGroup(pgid int, sig syscall.Signal) error {
	if err := syscall.Kill(-pgid, sig); !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}

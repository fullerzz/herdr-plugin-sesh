package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/fullerzz/herdr-plugin-sesh/internal/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestAttachmentProcess(t *testing.T) {
	mode := os.Getenv("HERDR_SESH_TEST_ATTACHMENT")
	if mode == "" {
		return
	}
	// Helpers run in their own process groups, beyond go test's interrupt.
	time.AfterFunc(30*time.Second, func() { os.Exit(4) })
	dir := os.Getenv("HERDR_SESH_TEST_ATTACHMENT_DIR")
	switch mode {
	case "launcher":
		// Like `herdr --remote`: the client is a child, and SIGTERM ends only the launcher.
		_ = attachmentHelper("client").Run()
		os.Exit(0)
	case "orphaning":
		// A launcher that dies before its client, as on a signal it does not handle.
		_ = attachmentHelper("client").Start()
		_ = waitForFile(filepath.Join(dir, "ready"))
		os.Exit(0)
	case "terminal", "attach", "tostop", "tostop-wrapper":
		check := foregroundJobRoundTrip
		switch mode {
		case "attach":
			check = attachRoundTrip
		case "tostop":
			check = tostopRoundTrip
		case "tostop-wrapper":
			check = func(dir string) error { return attachTerminalRoundTrip(dir, true) }
		}
		result := "ok"
		if err := check(dir); err != nil {
			result = err.Error()
		}
		//nolint:gosec // Parent test passes its private temporary directory to this helper.
		_ = os.WriteFile(filepath.Join(dir, "result"), []byte(result), 0o600)
		os.Exit(0)
	}
	stopped := make(chan os.Signal, 1)
	if mode == "stubborn" {
		signal.Ignore(syscall.SIGTERM)
	} else {
		signal.Notify(stopped, syscall.SIGTERM)
	}
	if mode == "fgclient" {
		check := expectForeground(os.Stdin, "client", unix.Getpgrp())
		if signal.Ignored(syscall.SIGTTOU) {
			check = errors.New("client inherited ignored SIGTTOU")
		}
		appendClient(dir, check)
		<-stopped
		os.Exit(0)
	}
	continued := make(chan os.Signal, 1)
	signal.Notify(continued, syscall.SIGCONT)
	go func() {
		<-continued
		//nolint:gosec // Parent test passes its private temporary directory to this helper.
		_ = os.WriteFile(filepath.Join(dir, "continued"), nil, 0o600)
	}()
	//nolint:gosec // Parent test passes its private temporary directory to this helper.
	if err := os.WriteFile(filepath.Join(dir, "ready"), nil, 0o600); err != nil {
		os.Exit(2)
	}
	if mode == "stubborn" {
		select {}
	}
	<-stopped
	if mode == "client" {
		// Terminal cleanup outlasts the launcher's exit.
		time.Sleep(200 * time.Millisecond)
	}
	//nolint:gosec // Parent test passes its private temporary directory to this helper.
	if err := os.WriteFile(filepath.Join(dir, "cleaned"), nil, 0o600); err != nil {
		os.Exit(3)
	}
	os.Exit(0)
}

// attachmentHelper re-executes this test binary as TestAttachmentProcess in mode.
func attachmentHelper(mode string) *exec.Cmd {
	//nolint:gosec // Re-execute this test binary with one fixed helper test.
	cmd := exec.Command(os.Args[0], "-test.run=^TestAttachmentProcess$")
	cmd.Env = append(os.Environ(), "HERDR_SESH_TEST_ATTACHMENT="+mode)
	return cmd
}

// helperTerminal opens this process's terminal on a descriptor other than
// stdin, which then reads /dev/null: Foreground must use the terminal's own
// descriptor rather than assume it is fd 0.
func helperTerminal() (*attachTerminal, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	null, err := os.Open(os.DevNull)
	if err != nil {
		return nil, err
	}
	if err := unix.Dup2(int(null.Fd()), 0); err != nil {
		return nil, err
	}
	return newAttachTerminal(tty)
}

// foregroundJobRoundTrip runs a launcher job on this process's terminal and
// checks that the job owns the terminal until stop hands it back.
func foregroundJobRoundTrip(dir string) error {
	tty, err := helperTerminal()
	if err != nil {
		return err
	}
	job, err := startAttachJob(attachmentHelper("launcher"), tty)
	if err != nil {
		return err
	}
	// stop is idempotent, so this also ends the job on an early return.
	defer func() { _ = job.stop(time.Second) }()
	if err := waitForFile(filepath.Join(dir, "ready")); err != nil {
		return err
	}
	if err := expectForeground(tty.file, "job", job.cmd.Process.Pid); err != nil {
		return err
	}
	if err := job.stop(stopGrace); err != nil {
		return err
	}
	return expectWrapperOwnsTerminal(tty.file)
}

// attachRoundTrip switches runAttach between two clients on this process's
// terminal; each client records whether it owned the terminal.
func attachRoundTrip(dir string) error {
	return attachTerminalRoundTrip(dir, false)
}

func attachTerminalRoundTrip(dir string, tostop bool) error {
	tty, err := helperTerminal()
	if err != nil {
		return err
	}
	restores := 0
	restore := tty.restore
	tty.restore = func() error { restores++; return restore() }
	bin := filepath.Join(dir, "herdr")
	client := fmt.Sprintf("#!/bin/sh\nHERDR_SESH_TEST_ATTACHMENT=fgclient exec '%s' -test.run='^TestAttachmentProcess$'\n", os.Args[0])
	//nolint:gosec // Executable fixture lives in the test-owned temporary directory.
	if err := os.WriteFile(bin, []byte(client), 0o700); err != nil {
		return err
	}
	cfg := remote.Config{ID: "local", HerdrSession: "default", Peers: []remote.Peer{{ID: "remote", SSH: "host", Session: "work"}}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	transport := &fakeTransport{}
	subscriptions := 0
	transport.subscribe = func(endpoint attachEndpoint) (chan remote.Target, error) {
		ch := make(chan remote.Target, 2)
		if endpoint.id == "local" {
			subscriptions++
			if tostop && subscriptions == 1 {
				afterClients(ctx, dir, 1, func() { close(ch) })
			} else {
				afterClients(ctx, dir, 1, func() {
					if tostop {
						ch <- remote.Target{MachineID: "unconfigured"}
					}
					ch <- remote.Target{MachineID: "remote", WorkspaceID: "w2"}
				})
			}
		} else {
			afterClients(ctx, dir, 2, cancel)
		}
		return ch, nil
	}
	diagnostics := io.Discard
	liveDiagnostics := &attachmentDiagnostics{tty: tty.file}
	if tostop {
		diagnostics = liveDiagnostics
	}
	err = runAttach(ctx, cfg, attachEndpoint{id: "local"}, transport, bin, tty, io.Discard, diagnostics)
	if !errors.Is(err, context.Canceled) || err.Error() != context.Canceled.Error() {
		return fmt.Errorf("runAttach: %w", err)
	}
	if restores != 2 {
		return fmt.Errorf("terminal restored %d times, want once per client", restores)
	}
	if tostop {
		if liveDiagnostics.err != nil {
			return liveDiagnostics.err
		}
		if !strings.Contains(liveDiagnostics.messages, "Remote switching unavailable") || !strings.Contains(liveDiagnostics.messages, "Remote switch failed") {
			return errors.New("missing live attachment diagnostics")
		}
	}
	return expectWrapperOwnsTerminal(tty.file)
}

type attachmentDiagnostics struct {
	tty      *os.File
	messages string
	err      error
}

func (w *attachmentDiagnostics) Write(p []byte) (int, error) {
	fg, err := unix.IoctlGetInt(int(w.tty.Fd()), unix.TIOCGPGRP)
	if err != nil {
		w.err = err
	} else if fg == unix.Getpgrp() {
		w.err = errors.New("diagnostic delayed until the client gave up its terminal")
	}
	n, err := w.tty.Write(p)
	w.messages += string(p[:n])
	return n, err
}

// Keep the wrapper's group non-orphaned: shells normally remain its parent,
// and orphaned groups do not receive SIGTTOU for background terminal writes.
func tostopRoundTrip(dir string) error {
	stty := exec.Command("stty", "tostop")
	stty.Stdin = os.Stdin
	if err := stty.Run(); err != nil {
		return err
	}
	state, err := term.GetState(os.Stdin.Fd())
	if err != nil {
		return err
	}
	child := attachmentHelper("tostop-wrapper")
	child.Stdin, child.Stdout, child.Stderr = os.Stdin, os.Stdout, os.Stderr
	child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Foreground: true, Ctty: 0}
	if err := child.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	select {
	case err = <-done:
	case <-time.After(6 * time.Second):
		// A stopped wrapper cannot run its deferred client cleanup.
		if fg, e := unix.IoctlGetInt(0, unix.TIOCGPGRP); e == nil && fg != unix.Getpgrp() {
			_ = syscall.Kill(-fg, syscall.SIGKILL)
		}
		_ = child.Process.Kill()
		<-done
		err = errors.New("TOSTOP stopped attachment progress before reconnect, switch, and cleanup completed")
	}
	if foregroundErr := (&attachTerminal{file: os.Stdin}).foreground(unix.Getpgrp()); foregroundErr != nil {
		return foregroundErr
	}
	if err != nil {
		return err
	}
	restored, err := term.GetState(os.Stdin.Fd())
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(state, restored) {
		return errors.New("attachment changed terminal state with TOSTOP enabled")
	}
	// Preserve the inner helper's result for the parent test.
	result, err := os.ReadFile(filepath.Join(dir, "result")) //nolint:gosec // Test-owned directory.
	if err != nil {
		return err
	}
	if string(result) != "ok" {
		return errors.New(string(result))
	}
	return nil
}

func expectForeground(tty *os.File, owner string, pgid int) error {
	fg, err := unix.IoctlGetInt(int(tty.Fd()), unix.TIOCGPGRP)
	if err != nil {
		return err
	}
	if fg != pgid {
		return fmt.Errorf("foreground group %d, want %s %d", fg, owner, pgid)
	}
	return nil
}

func expectWrapperOwnsTerminal(tty *os.File) error {
	if err := expectForeground(tty, "wrapper", unix.Getpgrp()); err != nil {
		return err
	}
	if signal.Ignored(syscall.SIGTTOU) {
		return errors.New("SIGTTOU is ignored; every later client would inherit that")
	}
	return nil
}

// appendClient records one fake client's foreground check.
func appendClient(dir string, check error) {
	line := "foreground\n"
	if check != nil {
		line = check.Error() + "\n"
	}
	//nolint:gosec // Parent test passes its private temporary directory to this helper.
	f, err := os.OpenFile(filepath.Join(dir, "clients"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err == nil {
		_, _ = f.WriteString(line)
		_ = f.Close()
	}
}

// afterClients runs then once n fake clients have recorded their check.
func afterClients(ctx context.Context, dir string, n int, then func()) {
	go func() {
		for ctx.Err() == nil {
			//nolint:gosec // Parent test passes its private temporary directory to this helper.
			data, _ := os.ReadFile(filepath.Join(dir, "clients"))
			if strings.Count(string(data), "\n") >= n {
				then()
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
}

func waitForFile(path string) error {
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		//nolint:gosec // Callers pass paths in their test-owned temporary directory.
		if _, err := os.Stat(path); err == nil {
			return nil
		}
	}
	return fmt.Errorf("%s was not created", filepath.Base(path))
}

func startAttachmentProcess(t *testing.T, mode string, tty *attachTerminal) (*attachJob, string) {
	t.Helper()
	dir := t.TempDir()
	child := attachmentHelper(mode)
	child.Env = append(child.Env, "HERDR_SESH_TEST_ATTACHMENT_DIR="+dir)
	job, err := startAttachJob(child, tty)
	require.NoError(t, err)
	// stop signals only a live group, so this is a no-op after a successful stop.
	t.Cleanup(func() { _ = job.stop(0) })
	require.NoError(t, waitForFile(filepath.Join(dir, "ready")))
	return job, dir
}

func TestStopAttachmentAllowsTerminalCleanup(t *testing.T) {
	restores := 0
	job, dir := startAttachmentProcess(t, "graceful", &attachTerminal{restore: func() error { restores++; return nil }})
	require.NoError(t, job.stop(stopGrace))
	assert.FileExists(t, filepath.Join(dir, "cleaned"))
	require.NotNil(t, job.cmd.ProcessState)
	assert.True(t, job.cmd.ProcessState.Success())
	assert.Equal(t, 1, restores)
}

func TestStopAttachmentStopsLauncherClient(t *testing.T) {
	job, dir := startAttachmentProcess(t, "launcher", nil)
	require.NoError(t, job.stop(stopGrace))
	assert.FileExists(t, filepath.Join(dir, "cleaned"), "the client finished cleanup before stop returned")
	assert.False(t, groupExists(job.cmd.Process.Pid))
}

func TestSuspendedAttachmentIsResumed(t *testing.T) {
	job, dir := startAttachmentProcess(t, "graceful", nil)
	// Ctrl-Z before Herdr enters raw mode stops the whole foreground job.
	require.NoError(t, syscall.Kill(-job.cmd.Process.Pid, syscall.SIGTSTP))
	require.NoError(t, waitForFile(filepath.Join(dir, "continued")), "the stopped job was not continued")
	require.NoError(t, job.stop(stopGrace))
	assert.FileExists(t, filepath.Join(dir, "cleaned"))
}

func TestStopAttachmentKillsUnresponsiveChild(t *testing.T) {
	job, dir := startAttachmentProcess(t, "stubborn", nil)
	started := time.Now()
	require.NoError(t, job.stop(100*time.Millisecond))
	assert.Less(t, time.Since(started), 3*time.Second)
	assert.NoFileExists(t, filepath.Join(dir, "cleaned"))
	require.NotNil(t, job.cmd.ProcessState)
	status, ok := job.cmd.ProcessState.Sys().(syscall.WaitStatus)
	require.True(t, ok)
	assert.Equal(t, syscall.SIGKILL, status.Signal())
}

func TestStopAttachmentAlreadyExited(t *testing.T) {
	job, err := startAttachJob(exec.Command("/bin/sh", "-c", "exit 0"), nil)
	require.NoError(t, err)
	<-job.exited
	require.NoError(t, job.stop(time.Second))
}

func TestAttachEndsClientLeftByExitedLauncher(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HERDR_SESH_TEST_ATTACHMENT_DIR", dir)
	bin := filepath.Join(dir, "herdr")
	launcher := fmt.Sprintf("#!/bin/sh\nHERDR_SESH_TEST_ATTACHMENT=orphaning exec '%s' -test.run='^TestAttachmentProcess$'\n", os.Args[0])
	//nolint:gosec // Executable fixture lives in the test-owned temporary directory.
	require.NoError(t, os.WriteFile(bin, []byte(launcher), 0o700))
	cfg := remote.Config{ID: "local", HerdrSession: "default"}
	transport := &fakeTransport{subscribe: func(attachEndpoint) (chan remote.Target, error) {
		return make(chan remote.Target), nil
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	require.NoError(t, runAttach(ctx, cfg, attachEndpoint{id: "local"}, transport, bin, nil, io.Discard, io.Discard))
	assert.FileExists(t, filepath.Join(dir, "cleaned"), "the orphaned client was stopped before runAttach returned")
}

// runInTerminal runs TestAttachmentProcess in mode under script(1), which
// gives it a controlling terminal, and returns the helper's directory.
func runInTerminal(t *testing.T, mode string) string {
	t.Helper()
	script, err := exec.LookPath("script")
	if err != nil {
		t.Skip("script(1) provides the controlling terminal")
	}
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		//nolint:gosec // Fixed script(1) arguments run this test binary.
		cmd = exec.CommandContext(ctx, script, "-q", "/dev/null", os.Args[0], "-test.run=^TestAttachmentProcess$")
	case "linux":
		//nolint:gosec // Fixed script(1) arguments run this test binary.
		cmd = exec.CommandContext(ctx, script, "-q", "-e", "-c", fmt.Sprintf("'%s' -test.run='^TestAttachmentProcess$'", os.Args[0]), "/dev/null")
	default:
		t.Skip("script(1) arguments are known only for darwin and linux")
	}
	// util-linux script(1) runs the command with $SHELL.
	cmd.Env = append(os.Environ(), "SHELL=/bin/sh", "HERDR_SESH_TEST_ATTACHMENT="+mode, "HERDR_SESH_TEST_ATTACHMENT_DIR="+dir)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
	//nolint:gosec // Result is confined to the test-owned temporary directory.
	result, err := os.ReadFile(filepath.Join(dir, "result"))
	require.NoError(t, err, "%s", out)
	assert.Equal(t, "ok", string(result))
	return dir
}

func TestAttachJobOwnsTerminalUntilStopped(t *testing.T) {
	runInTerminal(t, "terminal")
}

func TestAttachHandsTerminalToEachClient(t *testing.T) {
	dir := runInTerminal(t, "attach")
	//nolint:gosec // Clients file is confined to the test-owned temporary directory.
	clients, err := os.ReadFile(filepath.Join(dir, "clients"))
	require.NoError(t, err)
	assert.Equal(t, "foreground\nforeground\n", string(clients))
}

func TestAttachWithTostopReconnectsSwitchesAndRestoresTerminal(t *testing.T) {
	dir := runInTerminal(t, "tostop")
	//nolint:gosec // Clients file is confined to the test-owned temporary directory.
	clients, err := os.ReadFile(filepath.Join(dir, "clients"))
	require.NoError(t, err)
	assert.Equal(t, "foreground\nforeground\n", string(clients))
}

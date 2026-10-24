package remoteservice

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceTransactionProcessExitReleasesLock(t *testing.T) {
	if home := os.Getenv("HERDR_SESH_LOCK_TEST_HOME"); home != "" {
		manager, err := newManager("linux", home, "", os.Getuid())
		require.NoError(t, err)
		if os.Getenv("HERDR_SESH_LOCK_TEST_BUSY") == "1" {
			require.ErrorContains(t, manager.WithLock(t.Context(), func(context.Context) error {
				t.Error("competing process entered transaction")
				return nil
			}), "another remote service operation")
			return
		}
		require.NoError(t, manager.WithLock(t.Context(), func(context.Context) error {
			os.Exit(0) // Exit while locked: no deferred close runs.
			return nil
		}))
		return
	}
	manager, _, opts := setup(t, "linux")
	command := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestServiceTransactionProcessExitReleasesLock$") // #nosec G204 G702 -- current test executable and fixed test selection.
	command.Env = append(os.Environ(), "HERDR_SESH_LOCK_TEST_HOME="+filepath.Dir(opts.ConfigPath))
	output, err := command.CombinedOutput()
	require.NoError(t, err, "%s", output)
	require.FileExists(t, manager.lockPath)
	require.NoError(t, manager.WithLock(t.Context(), func(ctx context.Context) error {
		competitor := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestServiceTransactionProcessExitReleasesLock$") // #nosec G204 G702 -- current test executable and fixed test selection.
		competitor.Env = append(command.Env, "HERDR_SESH_LOCK_TEST_BUSY=1")
		output, err := competitor.CombinedOutput()
		require.NoError(t, err, "%s", output)
		return nil
	}))
}

func TestStatusDoesNotCreateServiceLock(t *testing.T) {
	manager, _, _ := setup(t, "linux")
	_, err := manager.Status(t.Context())
	require.NoError(t, err)
	assert.NoFileExists(t, manager.lockPath)
}

func TestServiceTransactionExcludesOtherManagers(t *testing.T) {
	manager, fake, opts := setup(t, "linux")
	other, err := newManager("linux", filepath.Dir(opts.ConfigPath), filepath.Join(t.TempDir(), "xdg"), 501)
	require.NoError(t, err)
	other.run = fake.run
	var expired context.Context
	require.NoError(t, manager.WithLock(t.Context(), func(ctx context.Context) error {
		expired = ctx
		competitorCtx := context.WithValue(ctx, transactionKey{}, false)
		require.ErrorContains(t, other.Enable(competitorCtx, opts), "another remote service operation")
		require.ErrorContains(t, other.Disable(competitorCtx), "another remote service operation")
		assert.Empty(t, fake.calls)
		assert.NoFileExists(t, other.path)
		require.NoError(t, manager.Enable(ctx, opts))
		return nil
	}))
	require.NoError(t, other.WithLock(t.Context(), func(context.Context) error {
		return assertBusy(t, manager, expired)
	}))
	require.NoError(t, manager.Disable(t.Context()))
}

func assertBusy(t *testing.T, manager *Manager, ctx context.Context) error {
	t.Helper()
	require.ErrorContains(t, manager.WithLock(ctx, func(context.Context) error { t.Error("expired token bypassed lock"); return nil }), "another remote service operation")
	return nil
}

func TestServiceTransactionReleaseAndCancellation(t *testing.T) {
	manager, _, _ := setup(t, "linux")
	failure := errors.New("transaction failed")
	require.ErrorIs(t, manager.WithLock(t.Context(), func(context.Context) error { return failure }), failure)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, manager.WithLock(ctx, func(context.Context) error { t.Error("canceled callback ran"); return nil }), context.Canceled)
	require.NoError(t, manager.WithLock(t.Context(), func(context.Context) error { return nil }))
}

func TestServiceTransactionRejectsUnsafeLock(t *testing.T) {
	for _, kind := range []string{"symlink", "public", "directory", "hardlink"} {
		t.Run(kind, func(t *testing.T) {
			manager, _, opts := setup(t, "linux")
			path := filepath.Join(filepath.Dir(opts.ConfigPath), ".herdr-sesh-service.lock")
			switch kind {
			case "symlink":
				require.NoError(t, os.Symlink(opts.Executable, path))
			case "public":
				require.NoError(t, os.WriteFile(path, nil, 0o600))
				require.NoError(t, os.Chmod(path, 0o644)) // #nosec G302 -- deliberately unsafe lock fixture.
			case "directory":
				require.NoError(t, os.Mkdir(path, 0o700))
			case "hardlink":
				require.NoError(t, os.Link(opts.Executable, path))
			}
			require.Error(t, manager.WithLock(t.Context(), func(context.Context) error { t.Error("unsafe callback ran"); return nil }))
		})
	}
}

func TestActivationRecoveryKeepsTransactionLocked(t *testing.T) {
	manager, fake, opts := setup(t, "linux")
	other, err := newManager("linux", filepath.Dir(opts.ConfigPath), "", 501)
	require.NoError(t, err)
	other.run = fake.run
	original := manager.run
	var competingErrors []error
	manager.run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if len(args) > 1 && (args[1] == "enable" || args[1] == "disable") {
			competitorCtx := context.WithValue(ctx, transactionKey{}, false)
			competingErrors = append(competingErrors, other.Enable(competitorCtx, opts), other.Disable(competitorCtx))
			if args[1] == "enable" {
				return nil, errors.New("activation failed")
			}
		}
		return original(ctx, name, args...)
	}
	require.ErrorContains(t, manager.Enable(t.Context(), opts), "service recovery completed")
	require.Len(t, competingErrors, 4)
	for _, err := range competingErrors {
		require.ErrorContains(t, err, "another remote service operation")
	}
	assert.NoFileExists(t, manager.path)
	manager.run = original
	require.NoError(t, manager.Enable(t.Context(), opts))
	assert.True(t, fake.running)
}

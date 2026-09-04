package state

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

type History struct {
	Workspaces []string `json:"workspaces"`
}

var ErrHistoryLockTimeout = errors.New("timed out waiting for history lock")

const (
	maxWorkspaces       = 50
	historyLockTimeout  = 250 * time.Millisecond
	historyLockInterval = 10 * time.Millisecond
)

func Path(dir string) string { return filepath.Join(dir, "history.json") }

// SessionHistoryDir resolves a socket-scoped history directory. Existing
// unscoped history belongs to the default session and is copied on first use;
// named sessions never inherit it.
func SessionHistoryDir(dir, socketPath string) (string, error) {
	if dir == "" || socketPath == "" {
		return dir, nil
	}
	sum := sha256.Sum256([]byte(filepath.Clean(socketPath)))
	sessionDir := filepath.Join(dir, "history", fmt.Sprintf("%x", sum))
	if !isDefaultSessionSocket(socketPath) {
		return sessionDir, nil
	}
	if _, err := os.Stat(Path(sessionDir)); err == nil {
		return sessionDir, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if _, err := os.Stat(Path(dir)); os.IsNotExist(err) {
		return sessionDir, nil
	} else if err != nil {
		return "", err
	}

	err := withHistoryLock(dir, func() error {
		return withHistoryLock(sessionDir, func() error {
			if _, err := os.Stat(Path(sessionDir)); err == nil {
				return nil
			} else if !os.IsNotExist(err) {
				return err
			}
			contents, err := os.ReadFile(Path(dir))
			if err != nil {
				return err
			}
			return writeFile(Path(sessionDir), contents)
		})
	})
	if err != nil {
		return "", err
	}
	return sessionDir, nil
}

func isDefaultSessionSocket(socketPath string) bool {
	clean := filepath.Clean(socketPath)
	parent := filepath.Dir(clean)
	if filepath.Base(clean) != "herdr.sock" || filepath.Base(parent) != "herdr" {
		return false
	}
	return filepath.Base(filepath.Dir(parent)) != "sessions"
}

// TryHistoryWatcherLock permits one event subscriber per Herdr socket.
func TryHistoryWatcherLock(dir, socketPath string) (release func() error, acquired bool, err error) {
	if dir == "" {
		return nil, false, errors.New("HERDR_PLUGIN_STATE_DIR is required")
	}
	//nolint:gosec // dir is the trusted plugin-owned state directory supplied to this API.
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, false, err
	}
	lockName := fmt.Sprintf("history-watch-%x.lock", sha256.Sum256([]byte(socketPath)))
	//nolint:gosec // dir is the trusted plugin-owned state directory supplied to this API.
	lock, err := os.OpenFile(filepath.Join(dir, lockName), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, false, err
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, false, lock.Close()
		}
		return nil, false, errors.Join(err, lock.Close())
	}
	return func() error {
		return errors.Join(syscall.Flock(int(lock.Fd()), syscall.LOCK_UN), lock.Close())
	}, true, nil
}

func LoadHistory(dir string) (History, error) {
	var h History
	if dir == "" {
		return h, nil
	}
	b, err := os.ReadFile(Path(dir))
	if os.IsNotExist(err) {
		return h, nil
	}
	if err != nil {
		return h, err
	}
	return h, json.Unmarshal(b, &h)
}

func SaveHistory(dir string, h History) error {
	if dir == "" {
		return nil
	}
	return withHistoryLock(dir, func() error {
		return writeJSONFile(Path(dir), h)
	})
}

func Record(dir, workspaceID string) error {
	if dir == "" || workspaceID == "" {
		return nil
	}
	return withHistoryLock(dir, func() error {
		h, err := loadHistoryForWrite(dir)
		if err != nil {
			return err
		}
		if len(h.Workspaces) > 0 && h.Workspaces[0] == workspaceID {
			return nil
		}
		h.Workspaces = dedupeWorkspaces([]string{workspaceID}, h.Workspaces)
		return writeJSONFile(Path(dir), h)
	})
}

func RecordSwitch(dir, fromWorkspaceID, toWorkspaceID string) error {
	if dir == "" || toWorkspaceID == "" {
		return nil
	}
	return withHistoryLock(dir, func() error {
		h, err := loadHistoryForWrite(dir)
		if err != nil {
			return err
		}
		h.Workspaces = dedupeWorkspaces([]string{toWorkspaceID, fromWorkspaceID}, h.Workspaces)
		return writeJSONFile(Path(dir), h)
	})
}

func RemoveWorkspace(dir, workspaceID string) error {
	if dir == "" || workspaceID == "" {
		return nil
	}
	return withHistoryLock(dir, func() error {
		h, err := loadHistoryForWrite(dir)
		if err != nil {
			return err
		}
		workspaces := h.Workspaces[:0]
		for _, id := range h.Workspaces {
			if id != workspaceID {
				workspaces = append(workspaces, id)
			}
		}
		h.Workspaces = workspaces
		return writeJSONFile(Path(dir), h)
	})
}

func withHistoryLock(dir string, fn func() error) (err error) {
	//nolint:gosec // dir is the trusted plugin-owned state directory supplied to this API.
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	//nolint:gosec // dir is the trusted plugin-owned state directory supplied to this API.
	lock, err := os.OpenFile(filepath.Join(dir, "history.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, lock.Close())
	}()
	deadline := time.Now().Add(historyLockTimeout)
	for {
		err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			return err
		}
		if time.Now().Add(historyLockInterval).After(deadline) {
			return fmt.Errorf("%w after %s", ErrHistoryLockTimeout, historyLockTimeout)
		}
		time.Sleep(historyLockInterval)
	}
	defer func() {
		err = errors.Join(err, syscall.Flock(int(lock.Fd()), syscall.LOCK_UN))
	}()
	return fn()
}

func loadHistoryForWrite(dir string) (History, error) {
	h, err := LoadHistory(dir)
	if err == nil {
		return h, nil
	}
	if !isJSONDecodeError(err) {
		return History{}, err
	}
	return History{}, nil
}

func dedupeWorkspaces(front []string, rest []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(front)+len(rest))
	for _, id := range append(front, rest...) {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
		if len(out) == maxWorkspaces {
			return out
		}
	}
	return out
}

func isJSONDecodeError(err error) bool {
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	return errors.As(err, &syntaxErr) || errors.As(err, &typeErr)
}

func Last(dir string) (string, bool, error) {
	h, err := LoadHistory(dir)
	if err != nil {
		return "", false, err
	}
	if len(h.Workspaces) < 2 {
		return "", false, nil
	}
	return h.Workspaces[1], true, nil
}

func Previous(dir, currentWorkspaceID string) (string, bool, error) {
	h, err := LoadHistory(dir)
	if err != nil {
		return "", false, err
	}
	for _, id := range h.Workspaces {
		if id != "" && id != currentWorkspaceID {
			return id, true, nil
		}
	}
	return "", false, nil
}

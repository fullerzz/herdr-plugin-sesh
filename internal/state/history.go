package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type History struct {
	Workspaces []string `json:"workspaces"`
}

const maxWorkspaces = 50

func Path(dir string) string { return filepath.Join(dir, "history.json") }

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
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	return writeJSONFile(Path(dir), h)
}

func Record(dir, workspaceID string) error {
	if workspaceID == "" {
		return nil
	}
	h, err := loadHistoryForWrite(dir)
	if err != nil {
		return err
	}
	h.Workspaces = dedupeWorkspaces([]string{workspaceID}, h.Workspaces)
	return SaveHistory(dir, h)
}

func RecordSwitch(dir, fromWorkspaceID, toWorkspaceID string) error {
	if toWorkspaceID == "" {
		return nil
	}
	h, err := loadHistoryForWrite(dir)
	if err != nil {
		return err
	}
	h.Workspaces = dedupeWorkspaces([]string{toWorkspaceID, fromWorkspaceID}, h.Workspaces)
	return SaveHistory(dir, h)
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

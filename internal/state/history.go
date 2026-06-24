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
	h, err := LoadHistory(dir)
	if err != nil {
		if !isJSONDecodeError(err) {
			return err
		}
		h = History{}
	}
	if len(h.Workspaces) > 0 && h.Workspaces[0] == workspaceID {
		return nil
	}
	filtered := []string{workspaceID}
	for _, id := range h.Workspaces {
		if id != workspaceID {
			filtered = append(filtered, id)
		}
	}
	if len(filtered) > 50 {
		filtered = filtered[:50]
	}
	h.Workspaces = filtered
	return SaveHistory(dir, h)
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

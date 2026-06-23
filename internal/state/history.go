package state

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type History struct {
	Workspaces []string `json:"workspaces"`
}

func Path(dir string) string { return filepath.Join(dir, "history.json") }

func LoadHistory(dir string) (History, error) {
	var h History
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
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(Path(dir), b, 0644)
}

func Record(dir, workspaceID string) error {
	if workspaceID == "" {
		return nil
	}
	h, err := LoadHistory(dir)
	if err != nil {
		return err
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

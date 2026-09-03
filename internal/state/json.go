package state

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
)

func writeJSONFile(path string, v any) error {
	var contents bytes.Buffer
	enc := json.NewEncoder(&contents)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}
	return writeFile(path, contents.Bytes())
}

func writeFile(path string, contents []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(contents); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0600); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

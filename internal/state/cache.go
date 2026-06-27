package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
)

type SessionCache struct {
	SavedAt  time.Time       `json:"saved_at"`
	Sessions []model.Session `json:"sessions"`
}

func SessionCachePath(dir string) string {
	return filepath.Join(dir, "sessions.json")
}

func SaveSessionCache(dir string, sessions []model.Session, now time.Time) error {
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	return writeJSONFile(SessionCachePath(dir), SessionCache{SavedAt: now, Sessions: sessions})
}

func LoadSessionCache(dir string, ttl time.Duration, now time.Time) ([]model.Session, bool, error) {
	if dir == "" || ttl <= 0 {
		return nil, false, nil
	}
	b, err := os.ReadFile(SessionCachePath(dir))
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var payload SessionCache
	if err := json.Unmarshal(b, &payload); err != nil {
		return nil, false, err
	}
	if now.Sub(payload.SavedAt) > ttl {
		return nil, false, nil
	}
	return payload.Sessions, true, nil
}

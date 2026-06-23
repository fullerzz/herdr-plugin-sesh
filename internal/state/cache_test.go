package state

import (
	"testing"
	"time"

	"forgejo.local/fullerzz/herdr-plugin-sesh/internal/model"
)

func TestSessionCacheUsesFreshEntries(t *testing.T) {
	d := t.TempDir()
	want := []model.Session{{Source: "config", Name: "api", Path: "/tmp/api"}}
	if err := SaveSessionCache(d, want, time.Now()); err != nil {
		t.Fatal(err)
	}
	got, ok, err := LoadSessionCache(d, 5*time.Second, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !ok || len(got) != 1 || got[0].Name != "api" {
		t.Fatalf("got=%#v ok=%v", got, ok)
	}
}

func TestSessionCacheIgnoresStaleEntries(t *testing.T) {
	d := t.TempDir()
	if err := SaveSessionCache(d, []model.Session{{Name: "old"}}, time.Unix(0, 0)); err != nil {
		t.Fatal(err)
	}
	got, ok, err := LoadSessionCache(d, time.Second, time.Unix(10, 0))
	if err != nil {
		t.Fatal(err)
	}
	if ok || got != nil {
		t.Fatalf("expected stale cache miss, got=%#v ok=%v", got, ok)
	}
}

func TestSessionCacheMissWhenMissing(t *testing.T) {
	got, ok, err := LoadSessionCache(t.TempDir(), time.Second, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if ok || got != nil {
		t.Fatalf("expected cache miss, got=%#v ok=%v", got, ok)
	}
}

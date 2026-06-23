package picker

import (
	"forgejo.local/fullerzz/herdr-plugin-sesh/internal/model"
	"testing"
)

func TestFilterAndCurrent(t *testing.T) {
	m := New([]model.Session{{Name: "api"}, {Name: "web"}})
	m.Filter("api")
	cur, ok := m.Current()
	if !ok || cur.Name != "api" {
		t.Fatalf("cur=%#v ok=%v", cur, ok)
	}
}
func TestSeparatorAwareMatch(t *testing.T) {
	if !Match("my-api.service", "api service", true) {
		t.Fatal("expected separator aware match")
	}
}

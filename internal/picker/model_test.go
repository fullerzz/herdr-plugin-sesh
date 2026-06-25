package picker

import (
	"testing"

	"forgejo.local/fullerzz/herdr-plugin-sesh/internal/model"
)

func TestFilterAndCurrent(t *testing.T) {
	m := New([]model.Session{{Name: "api"}, {Name: "web"}})
	m.Filter("api")
	cur, ok := m.Current()
	if !ok || cur.Name != "api" {
		t.Fatalf("cur=%#v ok=%v", cur, ok)
	}
}
func TestFilterDoesNotMutateSourceItems(t *testing.T) {
	m := New([]model.Session{{Name: "api"}, {Name: "web"}})
	m.Filter("web")
	m.Filter("")
	if len(m.Filtered) != 2 || m.Filtered[0].Name != "api" || m.Filtered[1].Name != "web" {
		t.Fatalf("filtered=%#v all=%#v", m.Filtered, m.All)
	}
}
func TestFilterResetsSelectionWhenQueryChanges(t *testing.T) {
	m := New([]model.Session{{Name: "api"}, {Name: "api-worker"}, {Name: "web"}})
	m.Move(1)
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

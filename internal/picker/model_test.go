package picker

import (
	"testing"

	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
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
func TestFilterSelectsHomeDirectoryWhenQueryIsHome(t *testing.T) {
	t.Setenv("HOME", "/Users/zach")
	m := New([]model.Session{
		{Name: "home-manager", Path: "/tmp/home-manager"},
		{Name: "~", Path: "/Users/zach"},
	})
	m.Filter("home")
	cur, ok := m.Current()
	if !ok || cur.Name != "~" {
		t.Fatalf("cur=%#v ok=%v", cur, ok)
	}
}

func TestUpdateWorkspaceSnapshotsReplacesPolicyFilteredEntries(t *testing.T) {
	m := New([]model.Session{
		{Source: "herdr", Name: "old", WorkspaceID: "w1"},
		{Source: "herdr", Name: "closed", WorkspaceID: "w2"},
		{Source: "config", Name: "local", Path: "/tmp/local"},
	})
	m.UpdateWorkspaceSnapshots([]model.Session{
		{Source: "herdr", Name: "renamed", WorkspaceID: "w1"},
		{Source: "herdr", Name: "new", WorkspaceID: "w3"},
		{Source: "config", Name: "local", Path: "/tmp/local"},
	})

	if len(m.All) != 3 || m.All[0].Name != "renamed" || m.All[1].Name != "new" || m.All[2].Source != "config" {
		t.Fatalf("workspace snapshot replacement=%#v", m.All)
	}
}
func TestSeparatorAwareMatch(t *testing.T) {
	if !Match("my-api.service", "api service", true) {
		t.Fatal("expected separator aware match")
	}
}

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
func TestFilterRanksNameMatchesBeforePathMatches(t *testing.T) {
	m := New([]model.Session{
		{Source: "zoxide", Path: "/work/api-unnamed"},
		{Source: "config", Name: "api-service", Path: "/work/service"},
		{Source: "herdr", Name: "api-both", Path: "/work/api-both"},
		{Source: "herdr", Name: "worker", Path: "/work/api-worker"},
		{Source: "config", Name: "api-web", Path: "/work/web"},
	})

	m.Filter("api")

	want := []string{"api-service", "api-both", "api-web", "", "worker"}
	if len(m.Filtered) != len(want) {
		t.Fatalf("filtered=%#v", m.Filtered)
	}
	for i, name := range want {
		if m.Filtered[i].Name != name {
			t.Fatalf("filtered[%d].Name=%q, want %q", i, m.Filtered[i].Name, name)
		}
	}
}
func TestFilterKeepsNameMatchAheadOfPathOnlyWorktreeParent(t *testing.T) {
	m := New([]model.Session{
		{Source: "herdr", Name: "backend", Path: "/work/api", WorkspaceID: "w-parent"},
		{Source: "herdr", Name: "api-fix", Path: "/work/api-fix", WorkspaceID: "w-child", Worktree: model.WorktreeRelation{Linked: true, ParentWorkspaceID: "w-parent"}},
	})

	m.Filter("api")

	want := []string{"api-fix", "backend"}
	if len(m.Filtered) != len(want) {
		t.Fatalf("filtered=%#v", m.Filtered)
	}
	for i, name := range want {
		if m.Filtered[i].Name != name {
			t.Fatalf("filtered[%d].Name=%q, want %q", i, m.Filtered[i].Name, name)
		}
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
func TestFilterRanksActualHomePathBeforeMisleadingHomeName(t *testing.T) {
	t.Setenv("HOME", "/Users/zachfuller")
	m := New([]model.Session{
		{Name: "~", Path: "/tmp/home-archive"},
		{Name: "home-manager", Path: "/tmp/manager"},
		{Name: "~", Path: "/Users/zachfuller"},
	})

	m.Filter("home")

	want := []string{"/Users/zachfuller", "/tmp/manager", "/tmp/home-archive"}
	if len(m.Filtered) != len(want) {
		t.Fatalf("filtered=%#v", m.Filtered)
	}
	for i, path := range want {
		if m.Filtered[i].Path != path {
			t.Fatalf("filtered[%d].Path=%q, want %q", i, m.Filtered[i].Path, path)
		}
	}
}

func TestFilterCanDisableHomePrioritization(t *testing.T) {
	t.Setenv("HOME", "/Users/zach")
	m := New([]model.Session{
		{Name: "path-before", Path: "/tmp/home-path"},
		{Name: "home-tools", Path: "/tmp/tools"},
		{Name: "home-manager", Path: "/tmp/manager"},
		{Name: "~", Path: "/Users/zach"},
		{Name: "path-after", Path: "/tmp/home-after"},
	})
	m.PrioritizeHome = false

	m.Filter("HOME")

	want := []string{"home-tools", "home-manager", "path-before", "~", "path-after"}
	if len(m.Filtered) != len(want) {
		t.Fatalf("filtered=%#v", m.Filtered)
	}
	for i, name := range want {
		if m.Filtered[i].Name != name {
			t.Fatalf("filtered[%d].Name=%q, want %q", i, m.Filtered[i].Name, name)
		}
	}
}
func TestSeparatorAwareMatch(t *testing.T) {
	if !Match("my-api.service", "api service", true) {
		t.Fatal("expected separator aware match")
	}
}

func TestSeparatorAwareMatchNormalizesQuerySeparators(t *testing.T) {
	for _, tc := range []struct {
		name  string
		hay   string
		query string
	}{
		{"query keeps the candidate's separator", "my-api.service", "api-service"},
		{"query separator differs from the candidate's", "my-api.service", "api/service"},
		{"query separator matches a path boundary", "/work/api-service", "work/api"},
		{"underscore query against a dashed candidate", "my-api-service", "api_service"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !Match(tc.hay, tc.query, true) {
				t.Fatalf("Match(%q, %q, true) = false, want true", tc.hay, tc.query)
			}
		})
	}
}

// Normalizing the query is strictly additive: the candidate never retains any
// of the four separators, so a query containing one could not match before.
func TestSeparatorAwareMatchIsAdditive(t *testing.T) {
	for _, tc := range []struct {
		hay   string
		query string
		want  bool
	}{
		{"my-api.service", "api", true},
		{"my-api.service", "nope", false},
		{"my-api.service", "api service", true},
		{"my-api.service", "service api", false},
		{"my-api.service", "", true},
	} {
		if got := Match(tc.hay, tc.query, true); got != tc.want {
			t.Fatalf("Match(%q, %q, true) = %v, want %v", tc.hay, tc.query, got, tc.want)
		}
	}
}

func TestSeparatorUnawareMatchLeavesQueryLiteral(t *testing.T) {
	if !Match("my-api.service", "api.service", false) {
		t.Fatal("expected literal match when separator awareness is off")
	}
	if Match("my-api.service", "api service", false) {
		t.Fatal("did not expect separator normalization when it is off")
	}
}

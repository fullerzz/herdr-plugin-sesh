package sources

import (
	"context"
	"forgejo.local/fullerzz/herdr-plugin-sesh/internal/model"
	"testing"
)

type staticSource struct {
	name     string
	sessions []model.Session
}

func (s staticSource) Name() string { return s.name }
func (s staticSource) List(context.Context) (model.Sessions, error) {
	out := model.NewSessions()
	for _, x := range s.sessions {
		out.Add(x)
	}
	return out, nil
}
func TestMergeOrderBlacklistDedupe(t *testing.T) {
	srcs := []Source{staticSource{"zoxide", []model.Session{{Source: "zoxide", Name: "api", Path: "/z"}}}, staticSource{"config", []model.Session{{Source: "config", Name: "api", Path: "/c"}, {Source: "config", Name: "scratch", Path: "/s"}}}}
	got, err := Merge(context.Background(), srcs, []string{"config", "zoxide"}, []string{"scratch"}, false, true)
	if err != nil {
		t.Fatal(err)
	}
	os := got.Ordered()
	if len(os) != 1 || os[0].Source != "config" || os[0].Name != "api" {
		t.Fatalf("bad merge %#v", os)
	}
}
func TestParseZoxideLine(t *testing.T) {
	s, ok := ParseZoxideLine("42.5 /tmp/my app")
	if !ok || s.Score != 42.5 || s.Path != "/tmp/my app" || s.Name != "my app" {
		t.Fatalf("bad parse %#v %v", s, ok)
	}
}

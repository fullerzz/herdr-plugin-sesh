package startup

import (
	"context"
	"testing"

	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
)

func TestApplyCreatesTabsAndRunsCommands(t *testing.T) {
	f := &herdr.FakeClient{}
	s := model.Session{
		Path: "/tmp/app", StartupCommand: "echo {}",
		WindowConfigs: []model.WindowConfig{{Name: "git", StartupScript: "git -C {} status"}},
	}
	if err := Apply(context.Background(), f, Plan{WorkspaceID: "ws1", Session: s}); err != nil {
		t.Fatal(err)
	}
	if len(f.CreatedTabs) != 1 || f.CreatedTabs[0].Label != "git" {
		t.Fatalf("tabs: %#v", f.CreatedTabs)
	}
	if len(f.PaneRuns) != 2 {
		t.Fatalf("pane runs: %#v", f.PaneRuns)
	}
}

func TestApplySkipsDisabledStartup(t *testing.T) {
	f := &herdr.FakeClient{}
	s := model.Session{Path: "/tmp/app", StartupCommand: "echo hi", DisableStartupCommand: true}
	if err := Apply(context.Background(), f, Plan{WorkspaceID: "ws1", Session: s}); err != nil {
		t.Fatal(err)
	}
	if len(f.PaneRuns) != 0 {
		t.Fatalf("unexpected pane runs: %#v", f.PaneRuns)
	}
}

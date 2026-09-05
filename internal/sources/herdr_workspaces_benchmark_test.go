package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fullerzz/herdr-plugin-sesh/internal/herdr"
	"github.com/stretchr/testify/require"
)

const (
	benchmarkWorkspaceCount = 100
	benchmarkHelperEnv      = "HERDR_SESH_BENCHMARK_HELPER"
	benchmarkHelperModeEnv  = "HERDR_SESH_BENCHMARK_HELPER_MODE"
)

type benchmarkHerdrRunner struct {
	workspaces []byte
	panes      []byte
	calls      int
}

func (r *benchmarkHerdrRunner) Run(_ context.Context, _ string, args ...string) ([]byte, []byte, error) {
	r.calls++
	if len(args) >= 2 && args[0] == "workspace" && args[1] == "list" {
		return r.workspaces, nil, nil
	}
	if len(args) >= 2 && args[0] == "pane" && args[1] == "list" {
		return r.panes, nil, nil
	}
	return nil, nil, fmt.Errorf("unexpected herdr arguments: %v", args)
}

type benchmarkCountingRunner struct {
	runner herdr.Runner
	calls  int
}

func (r *benchmarkCountingRunner) Run(ctx context.Context, bin string, args ...string) ([]byte, []byte, error) {
	r.calls++
	return r.runner.Run(ctx, bin, args...)
}

func TestMain(m *testing.M) {
	if os.Getenv(benchmarkHelperEnv) == "1" {
		if err := runBenchmarkHerdrHelper(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func BenchmarkHerdrWorkspacesList(b *testing.B) {
	for _, missingPath := range []bool{false, true} {
		b.Run(benchmarkPathCase(missingPath), func(b *testing.B) {
			workspaceJSON, paneJSON, err := benchmarkHerdrPayloads(missingPath)
			require.NoError(b, err)
			runner := &benchmarkHerdrRunner{workspaces: workspaceJSON, panes: paneJSON}
			source := HerdrWorkspaces{Client: &herdr.CLIClient{Bin: "herdr", Runner: runner, Timeout: time.Second}}
			runHerdrWorkspacesBenchmark(b, source, &runner.calls)
		})
	}
}

func BenchmarkHerdrWorkspacesListProcessBoundary(b *testing.B) {
	for _, missingPath := range []bool{false, true} {
		b.Run(benchmarkPathCase(missingPath), func(b *testing.B) {
			b.Setenv(benchmarkHelperEnv, "1")
			b.Setenv(benchmarkHelperModeEnv, benchmarkPathCase(missingPath))
			runner := &benchmarkCountingRunner{runner: herdr.ExecRunner{}}
			source := HerdrWorkspaces{Client: &herdr.CLIClient{Bin: os.Args[0], Runner: runner, Timeout: 5 * time.Second}}
			runHerdrWorkspacesBenchmark(b, source, &runner.calls)
		})
	}
}

func runHerdrWorkspacesBenchmark(b *testing.B, source HerdrWorkspaces, calls *int) {
	b.Helper()
	b.ReportAllocs()
	for b.Loop() {
		sessions, err := source.List(context.Background())
		// Keep assertion overhead off the measured success path.
		if err != nil {
			require.NoError(b, err)
		}
		if len(sessions.OrderedIndex) != benchmarkWorkspaceCount {
			require.Len(b, sessions.OrderedIndex, benchmarkWorkspaceCount)
		}
	}
	b.ReportMetric(float64(*calls)/float64(b.N), "commands/op")
}

func benchmarkPathCase(missingPath bool) string {
	if missingPath {
		return "PaneFallback"
	}
	return "CompletePaths"
}

func benchmarkHerdrPayloads(missingPath bool) ([]byte, []byte, error) {
	workspaces := make([]herdr.Workspace, benchmarkWorkspaceCount)
	panes := make([]herdr.Pane, benchmarkWorkspaceCount)
	statuses := []string{"", "idle", "working"}
	for i := range benchmarkWorkspaceCount {
		id := fmt.Sprintf("workspace-%03d", i)
		path := fmt.Sprintf("/repos/service-%03d", i)
		tabID := id + ":tab-1"
		workspaces[i] = herdr.Workspace{
			ID:            id,
			Label:         "service-" + id,
			CWD:           path,
			ForegroundCWD: path + "/src",
			ActiveTabID:   tabID,
			AgentStatus:   statuses[i%len(statuses)],
		}
		panes[i] = herdr.Pane{
			ID:            fmt.Sprintf("pane-%03d", i),
			WorkspaceID:   id,
			TabID:         tabID,
			CWD:           path,
			ForegroundCWD: path + "/src",
			Focused:       true,
		}
	}
	if missingPath {
		workspaces[len(workspaces)-1].CWD = ""
		workspaces[len(workspaces)-1].ForegroundCWD = ""
	}
	workspaceJSON, err := json.Marshal(map[string]any{"result": map[string]any{"workspaces": workspaces}})
	if err != nil {
		return nil, nil, err
	}
	paneJSON, err := json.Marshal(map[string]any{"result": map[string]any{"panes": panes}})
	if err != nil {
		return nil, nil, err
	}
	return workspaceJSON, paneJSON, nil
}

func runBenchmarkHerdrHelper() error {
	missingPath := os.Getenv(benchmarkHelperModeEnv) == benchmarkPathCase(true)
	workspaceJSON, paneJSON, err := benchmarkHerdrPayloads(missingPath)
	if err != nil {
		return err
	}
	var output []byte
	switch strings.Join(os.Args[1:], " ") {
	case "workspace list":
		output = workspaceJSON
	case "pane list":
		output = paneJSON
	default:
		return fmt.Errorf("unexpected helper arguments: %v", os.Args[1:])
	}
	_, err = os.Stdout.Write(output)
	return err
}

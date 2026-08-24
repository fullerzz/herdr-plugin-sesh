package picker

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/fullerzz/herdr-plugin-sesh/internal/model"
)

var benchmarkPickerOutput string

func BenchmarkFilterSessions(b *testing.B) {
	tests := []struct {
		name  string
		count int
		query string
		want  int
	}{
		{name: "Sessions100/MatchNone", count: 100, query: "missing", want: 0},
		{name: "Sessions100/MatchSome", count: 100, query: "service-00", want: 100},
		{name: "Sessions1000/MatchNone", count: 1000, query: "missing", want: 0},
		{name: "Sessions1000/MatchSome", count: 1000, query: "service-00", want: 100},
	}
	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			picker := New(benchmarkSessions(tt.count))
			picker.Filter(tt.query)
			if got := len(picker.Filtered); got != tt.want {
				b.Fatalf("filtered=%d, want %d", got, tt.want)
			}
			b.ReportAllocs()
			for b.Loop() {
				picker.Filter(tt.query)
			}
			if got := len(picker.Filtered); got != tt.want {
				b.Fatalf("filtered=%d, want %d", got, tt.want)
			}
		})
	}
}

func BenchmarkRenderPicker(b *testing.B) {
	for _, hidePreview := range []bool{false, true} {
		name := "DefaultPreview"
		if hidePreview {
			name = "HiddenPreview"
		}
		b.Run(name, func(b *testing.B) {
			picker := newTeaModel(benchmarkSessions(1000), Options{HidePreview: hidePreview, ShowIcons: true})
			picker.width = 120
			picker.height = 40
			picker.preview = "representative preview content"

			b.ReportAllocs()
			for b.Loop() {
				benchmarkPickerOutput = picker.View().Content
			}
		})
	}
}

func BenchmarkPreviewNavigationBurst(b *testing.B) {
	const previewCount = 8
	var active atomic.Pointer[previewBenchmarkWorkload]
	oldRenderPreview := renderPreview
	renderPreview = func(ctx context.Context, _ model.Session, _ string) (string, error) {
		return active.Load().render(ctx)
	}
	b.Cleanup(func() { renderPreview = oldRenderPreview })

	var canceled, completed int64
	b.ReportAllocs()
	for b.Loop() {
		workload := newPreviewBenchmarkWorkload(previewCount)
		active.Store(workload)
		picker := newTeaModel(benchmarkSessions(previewCount), Options{Context: context.Background()})
		picker.previewKey = ""
		done := make(chan struct{}, previewCount)
		for i := range previewCount {
			picker.list.Selected = i
			next, command := picker.refreshPreview()
			picker = next
			if command == nil {
				b.Fatalf("preview command %d is nil", i)
			}
			go func(command tea.Cmd) {
				_ = command()
				done <- struct{}{}
			}(command)
			<-workload.started
		}
		for range previewCount {
			<-done
		}
		canceled += workload.canceled.Load()
		completed += workload.completed.Load()
	}
	b.ReportMetric(float64(canceled)/float64(b.N), "canceled/op")
	b.ReportMetric(float64(completed)/float64(b.N), "completed/op")
}

type previewBenchmarkWorkload struct {
	started   chan struct{}
	gate      chan struct{}
	canceled  atomic.Int64
	completed atomic.Int64
}

func newPreviewBenchmarkWorkload(previewCount int) *previewBenchmarkWorkload {
	workload := &previewBenchmarkWorkload{
		started: make(chan struct{}, previewCount),
		gate:    make(chan struct{}, 1),
	}
	workload.gate <- struct{}{}
	return workload
}

func (w *previewBenchmarkWorkload) render(ctx context.Context) (string, error) {
	w.started <- struct{}{}
	select {
	case <-ctx.Done():
		w.canceled.Add(1)
		return "", ctx.Err()
	case <-w.gate:
	}
	defer func() { w.gate <- struct{}{} }()

	timer := time.NewTimer(time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		w.canceled.Add(1)
		return "", ctx.Err()
	case <-timer.C:
		w.completed.Add(1)
		return "preview", nil
	}
}

func benchmarkSessions(count int) []model.Session {
	sessions := make([]model.Session, count)
	statuses := []string{"", "idle", "working"}
	for i := range count {
		sessions[i] = model.Session{
			Source:      "herdr",
			Name:        fmt.Sprintf("service-%04d-api", i),
			Path:        fmt.Sprintf("/repos/service-%04d", i),
			WorkspaceID: fmt.Sprintf("workspace-%04d", i),
			AgentStatus: statuses[i%len(statuses)],
		}
	}
	return sessions
}

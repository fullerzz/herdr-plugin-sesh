package herdr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestWatchWorkspaceEventsReconcilesProtocol20ReplayBeforeSnapshot(t *testing.T) {
	listener, socketPath := listenTestSocket(t)
	serverDone := make(chan error, 1)
	go func() {
		if err := serveCompatiblePing(listener); err != nil {
			serverDone <- err
			return
		}
		stream, err := acceptRequest(listener, "events.subscribe")
		if err != nil {
			serverDone <- err
			return
		}
		defer func() { _ = stream.Close() }()
		enc := json.NewEncoder(stream)
		for _, message := range []any{
			map[string]any{"id": "herdr-sesh-history", "result": map[string]any{"type": "subscription_started"}},
			workspaceEventMessage("workspace_focused", "stale"),
			workspaceEventMessage("workspace_closed", "gone"),
			workspaceEventMessage("workspace_closed", "still-open"),
		} {
			if err := enc.Encode(message); err != nil {
				serverDone <- err
				return
			}
		}

		snapshot, err := acceptRequest(listener, "session.snapshot")
		if err != nil {
			serverDone <- err
			return
		}
		defer func() { _ = snapshot.Close() }()
		if err := enc.Encode(workspaceEventMessage("workspace_focused", "live")); err != nil {
			serverDone <- err
			return
		}
		serverDone <- json.NewEncoder(snapshot).Encode(snapshotResponse("snapshot", "snapshot", "still-open"))
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var got []string
	record := func(kind, id string) error {
		got = append(got, kind+":"+id)
		if kind == "focus" && id == "live" {
			cancel()
		}
		return nil
	}
	err := WatchWorkspaceEvents(ctx, socketPath,
		func(id string) error { return record("focus", id) },
		func(id string) error { return record("close", id) },
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("watch error=%v, want context cancellation", err)
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
	want := []string{"focus:stale", "close:gone", "focus:snapshot", "focus:live"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mutations=%#v want %#v", got, want)
	}
}

func TestWatchWorkspaceEventsWaitsForCompleteProtocol20ReplayBeforePublishingHistory(t *testing.T) {
	listener, socketPath := listenTestSocket(t)
	replayDone := make(chan struct{})
	serverDone := make(chan error, 1)
	go func() {
		if err := serveCompatiblePing(listener); err != nil {
			serverDone <- err
			return
		}
		stream, err := acceptRequest(listener, "events.subscribe")
		if err != nil {
			serverDone <- err
			return
		}
		defer func() { _ = stream.Close() }()
		enc := json.NewEncoder(stream)
		if err := enc.Encode(map[string]any{"id": "herdr-sesh-history", "result": map[string]any{"type": "subscription_started"}}); err != nil {
			serverDone <- err
			return
		}

		replayErr := make(chan error, 1)
		go func() {
			for i := 0; i < 10; i++ {
				if err := enc.Encode(workspaceEventMessage("workspace_focused", fmt.Sprintf("stale-%d", i))); err != nil {
					replayErr <- err
					return
				}
				time.Sleep(20 * time.Millisecond)
			}
			close(replayDone)
			replayErr <- nil
		}()

		snapshot, err := acceptRequest(listener, "session.snapshot")
		if err != nil {
			serverDone <- err
			return
		}
		if err := json.NewEncoder(snapshot).Encode(snapshotResponse("current", "current")); err != nil {
			_ = snapshot.Close()
			serverDone <- err
			return
		}
		_ = snapshot.Close()
		serverDone <- <-replayErr
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	record := func(id string) error {
		select {
		case <-replayDone:
		default:
			return errors.New("history published before retained replay finished")
		}
		if id == "current" {
			cancel()
		}
		return nil
	}
	err := WatchWorkspaceEvents(ctx, socketPath, record, func(string) error { return nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("watch error=%v, want context cancellation after complete replay", err)
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
}

func TestWatchWorkspaceEventsRejectsPreSubscriptionProtocol(t *testing.T) {
	listener, socketPath := listenTestSocket(t)
	serverDone := make(chan error, 1)
	go func() {
		conn, err := acceptRequest(listener, "ping")
		if err != nil {
			serverDone <- err
			return
		}
		defer func() { _ = conn.Close() }()
		serverDone <- json.NewEncoder(conn).Encode(map[string]any{
			"id": "herdr-sesh-history-ping",
			"result": map[string]any{
				"type": "pong", "version": "0.8.1", "protocol": 19,
			},
		})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := WatchWorkspaceEvents(ctx, socketPath, func(string) error { return nil }, func(string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "requires Herdr protocol 20") {
		t.Fatalf("watch error=%v, want protocol 20 requirement", err)
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
}

func TestWatchWorkspaceEventsNoHistoryKeepsSnapshotWindowEvent(t *testing.T) {
	listener, socketPath := listenTestSocket(t)
	serverDone := make(chan error, 1)
	go func() {
		if err := serveCompatiblePing(listener); err != nil {
			serverDone <- err
			return
		}
		stream, err := acceptRequest(listener, "events.subscribe")
		if err != nil {
			serverDone <- err
			return
		}
		defer func() { _ = stream.Close() }()
		enc := json.NewEncoder(stream)
		if err := enc.Encode(map[string]any{"id": "herdr-sesh-history", "result": map[string]any{"type": "subscription_started"}}); err != nil {
			serverDone <- err
			return
		}
		snapshot, err := acceptRequest(listener, "session.snapshot")
		if err != nil {
			serverDone <- err
			return
		}
		defer func() { _ = snapshot.Close() }()
		if err := enc.Encode(workspaceEventMessage("workspace_focused", "during-snapshot")); err != nil {
			serverDone <- err
			return
		}
		serverDone <- json.NewEncoder(snapshot).Encode(snapshotResponse("snapshot", "snapshot"))
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var got []string
	err := WatchWorkspaceEvents(ctx, socketPath,
		func(id string) error {
			got = append(got, id)
			if len(got) == 2 {
				cancel()
			}
			return nil
		},
		func(string) error { return nil },
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("watch error=%v, want context cancellation", err)
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
	if want := []string{"snapshot", "during-snapshot"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("focuses=%#v want %#v", got, want)
	}
}

func TestWatchWorkspaceEventsReconnectsAfterUnexpectedEOF(t *testing.T) {
	listener, socketPath := listenTestSocket(t)
	serverDone := make(chan error, 1)
	go func() {
		for attempt := 0; attempt < 2; attempt++ {
			if err := serveCompatiblePing(listener); err != nil {
				serverDone <- err
				return
			}
			stream, err := acceptRequest(listener, "events.subscribe")
			if err != nil {
				serverDone <- err
				return
			}
			enc := json.NewEncoder(stream)
			if err := enc.Encode(map[string]any{"id": "herdr-sesh-history", "result": map[string]any{"type": "subscription_started"}}); err != nil {
				_ = stream.Close()
				serverDone <- err
				return
			}
			snapshot, err := acceptRequest(listener, "session.snapshot")
			if err != nil {
				_ = stream.Close()
				serverDone <- err
				return
			}
			if err := json.NewEncoder(snapshot).Encode(snapshotResponse("")); err != nil {
				_ = snapshot.Close()
				_ = stream.Close()
				serverDone <- err
				return
			}
			_ = snapshot.Close()
			if attempt == 1 {
				if err := enc.Encode(workspaceEventMessage("workspace_focused", "after-reconnect")); err != nil {
					_ = stream.Close()
					serverDone <- err
					return
				}
				if err := enc.Encode(workspaceEventMessage("workspace_closed", "after-reconnect")); err != nil {
					_ = stream.Close()
					serverDone <- err
					return
				}
			}
			_ = stream.Close()
		}
		serverDone <- nil
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var got []string
	err := WatchWorkspaceEvents(ctx, socketPath,
		func(id string) error {
			got = append(got, "focus:"+id)
			return nil
		},
		func(id string) error {
			got = append(got, "close:"+id)
			cancel()
			return nil
		},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("watch error=%v, want context cancellation", err)
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
	want := []string{"focus:after-reconnect", "close:after-reconnect"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mutations=%#v want %#v", got, want)
	}
}

func listenTestSocket(t *testing.T) (net.Listener, string) {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "herdr-sesh-events-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socketPath := filepath.Join(dir, "herdr.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return listener, socketPath
}

func acceptRequest(listener net.Listener, method string) (net.Conn, error) {
	conn, err := listener.Accept()
	if err != nil {
		return nil, err
	}
	var request struct {
		Method string `json:"method"`
	}
	if err := json.NewDecoder(conn).Decode(&request); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if request.Method != method {
		_ = conn.Close()
		return nil, fmt.Errorf("method=%q want %q", request.Method, method)
	}
	return conn, nil
}

func serveCompatiblePing(listener net.Listener) error {
	conn, err := acceptRequest(listener, "ping")
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	return json.NewEncoder(conn).Encode(map[string]any{
		"id": "herdr-sesh-history-ping",
		"result": map[string]any{
			"type": "pong", "version": "0.8.2", "protocol": 20,
		},
	})
}

func workspaceEventMessage(event, workspaceID string) map[string]any {
	return map[string]any{"event": event, "data": map[string]any{"workspace_id": workspaceID}}
}

func snapshotResponse(focusedWorkspaceID string, workspaceIDs ...string) map[string]any {
	workspaces := make([]map[string]any, 0, len(workspaceIDs))
	for _, workspaceID := range workspaceIDs {
		workspaces = append(workspaces, map[string]any{"workspace_id": workspaceID})
	}
	return map[string]any{
		"id": "herdr-sesh-history-snapshot",
		"result": map[string]any{
			"type": "session_snapshot",
			"snapshot": map[string]any{
				"focused_workspace_id": focusedWorkspaceID,
				"workspaces":           workspaces,
			},
		},
	}
}

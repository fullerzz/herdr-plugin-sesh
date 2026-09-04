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

func TestWatchWorkspaceEventsReconcilesDelayedProtocol20Replay(t *testing.T) {
	listener, socketPath := listenTestSocket(t)
	serverDone := make(chan error, 1)
	go func() {
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
		if err := enc.Encode(workspaceEventMessage("workspace_focused", "older")); err != nil {
			_ = stream.Close()
			serverDone <- err
			return
		}
		if err := serveCompatiblePing(listener, 20); err != nil {
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
		if err := json.NewEncoder(snapshot).Encode(snapshotResponse("current", "current", "older", "previous")); err != nil {
			_ = snapshot.Close()
			_ = stream.Close()
			serverDone <- err
			return
		}
		_ = snapshot.Close()

		// Protocol 20 can deliver another retained event after the initial
		// snapshot with no replay-boundary marker.
		if err := enc.Encode(workspaceEventMessage("workspace_focused", "previous")); err != nil {
			_ = stream.Close()
			serverDone <- err
			return
		}
		if err := enc.Encode(workspaceEventMessage("workspace_closed", "current")); err != nil {
			_ = stream.Close()
			serverDone <- err
			return
		}
		_ = stream.Close()
		serverDone <- serveProtocol20Snapshots(listener, "current", "current", "older", "previous")
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var history []string
	reconnect, err := watchWorkspaceEventsOnce(ctx, socketPath,
		func(id string) error {
			history = recordHistoryVisit(history, id)
			return nil
		},
		func(id string) error {
			history = removeHistoryWorkspace(history, id)
			return nil
		},
	)
	_ = listener.Close()
	if !reconnect || err == nil {
		t.Fatalf("watch result=(%v, %v), want reconnect after closed stream", reconnect, err)
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
	want := []string{"current", "previous", "older"}
	if !reflect.DeepEqual(history, want) {
		t.Fatalf("history=%#v want %#v", history, want)
	}
}

func TestWatchWorkspaceEventsPreservesInterruptedProtocol20Replay(t *testing.T) {
	listener, socketPath := listenTestSocket(t)
	serverDone := make(chan error, 1)
	go func() {
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
		if err := serveCompatiblePing(listener, 20); err != nil {
			_ = stream.Close()
			serverDone <- err
			return
		}
		for _, event := range []map[string]any{
			workspaceEventMessage("workspace_focused", "older"),
			workspaceEventMessage("workspace_focused", "previous"),
			workspaceEventMessage("workspace_closed", "current"),
		} {
			if err := enc.Encode(event); err != nil {
				_ = stream.Close()
				serverDone <- err
				return
			}
		}
		_ = stream.Close()
		serverDone <- serveProtocol20Snapshots(listener, "current", "current", "older", "previous")
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var history []string
	reconnect, err := watchWorkspaceEventsOnce(ctx, socketPath,
		func(id string) error {
			history = recordHistoryVisit(history, id)
			return nil
		},
		func(id string) error {
			history = removeHistoryWorkspace(history, id)
			return nil
		},
	)
	_ = listener.Close()
	if !reconnect || err == nil {
		t.Fatalf("watch result=(%v, %v), want reconnect after closed stream", reconnect, err)
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
	want := []string{"current", "previous", "older"}
	if !reflect.DeepEqual(history, want) {
		t.Fatalf("history=%#v want %#v", history, want)
	}
}

func TestWatchWorkspaceEventsRejectsPreSubscriptionProtocol(t *testing.T) {
	listener, socketPath := listenTestSocket(t)
	serverDone := make(chan error, 1)
	go func() {
		stream, err := acceptRequest(listener, "events.subscribe")
		if err != nil {
			serverDone <- err
			return
		}
		defer func() { _ = stream.Close() }()
		if err := json.NewEncoder(stream).Encode(map[string]any{
			"id": "herdr-sesh-history", "result": map[string]any{"type": "subscription_started"},
		}); err != nil {
			serverDone <- err
			return
		}
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

func TestWatchWorkspaceEventsBuffersProtocol21EventsDuringProtocolProbe(t *testing.T) {
	listener, socketPath := listenTestSocket(t)
	serverDone := make(chan error, 1)
	go func() {
		first, method, err := acceptAnyRequest(listener)
		if err != nil {
			serverDone <- err
			return
		}
		var stream net.Conn
		switch method {
		case "events.subscribe":
			stream = first
			if err := json.NewEncoder(stream).Encode(map[string]any{
				"id": "herdr-sesh-history", "result": map[string]any{"type": "subscription_started"},
			}); err != nil {
				serverDone <- err
				return
			}
			ping, err := acceptRequest(listener, "ping")
			if err != nil {
				serverDone <- err
				return
			}
			for _, workspaceID := range []string{"B", "C"} {
				if err := json.NewEncoder(stream).Encode(workspaceEventMessage("workspace_focused", workspaceID)); err != nil {
					_ = ping.Close()
					serverDone <- err
					return
				}
			}
			if err := encodeCompatiblePing(ping, 21); err != nil {
				_ = ping.Close()
				serverDone <- err
				return
			}
			_ = ping.Close()
		case "ping":
			if err := encodeCompatiblePing(first, 21); err != nil {
				_ = first.Close()
				serverDone <- err
				return
			}
			_ = first.Close()
			stream, err = acceptRequest(listener, "events.subscribe")
			if err != nil {
				serverDone <- err
				return
			}
			if err := json.NewEncoder(stream).Encode(map[string]any{
				"id": "herdr-sesh-history", "result": map[string]any{"type": "subscription_started"},
			}); err != nil {
				serverDone <- err
				return
			}
		default:
			_ = first.Close()
			serverDone <- fmt.Errorf("first method=%q want events.subscribe or ping", method)
			return
		}
		defer func() { _ = stream.Close() }()

		snapshot, err := acceptRequest(listener, "session.snapshot")
		if err != nil {
			serverDone <- err
			return
		}
		serverDone <- json.NewEncoder(snapshot).Encode(snapshotResponse("C", "B", "C"))
		_ = snapshot.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	history := []string{"A"}
	reconnect, err := watchWorkspaceEventsOnce(ctx, socketPath,
		func(id string) error {
			history = recordHistoryVisit(history, id)
			return nil
		},
		func(id string) error {
			history = removeHistoryWorkspace(history, id)
			return nil
		},
	)
	if !reconnect || err == nil {
		t.Fatalf("watch result=(%v, %v), want reconnect after closed stream", reconnect, err)
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
	if want := []string{"C", "B", "A"}; !reflect.DeepEqual(history, want) {
		t.Fatalf("history=%#v want %#v", history, want)
	}
}

func TestWatchWorkspaceEventsNoHistoryKeepsSnapshotWindowEvent(t *testing.T) {
	listener, socketPath := listenTestSocket(t)
	serverDone := make(chan error, 1)
	go func() {
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
		if err := serveCompatiblePing(listener, 21); err != nil {
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
			if err := serveCompatiblePing(listener, 21); err != nil {
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
	conn, got, err := acceptAnyRequest(listener)
	if err != nil {
		return nil, err
	}
	if got != method {
		_ = conn.Close()
		return nil, fmt.Errorf("method=%q want %q", got, method)
	}
	return conn, nil
}

func acceptAnyRequest(listener net.Listener) (net.Conn, string, error) {
	conn, err := listener.Accept()
	if err != nil {
		return nil, "", err
	}
	var request struct {
		Method string `json:"method"`
	}
	if err := json.NewDecoder(conn).Decode(&request); err != nil {
		_ = conn.Close()
		return nil, "", err
	}
	return conn, request.Method, nil
}

func serveCompatiblePing(listener net.Listener, protocol int) error {
	conn, err := acceptRequest(listener, "ping")
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	return encodeCompatiblePing(conn, protocol)
}

func encodeCompatiblePing(conn net.Conn, protocol int) error {
	return json.NewEncoder(conn).Encode(map[string]any{
		"id": "herdr-sesh-history-ping",
		"result": map[string]any{
			"type": "pong", "version": "compatible", "protocol": protocol,
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

func serveProtocol20Snapshots(listener net.Listener, focusedWorkspaceID string, workspaceIDs ...string) error {
	for {
		conn, err := acceptRequest(listener, "session.snapshot")
		if errors.Is(err, net.ErrClosed) {
			return nil
		}
		if err != nil {
			return err
		}
		err = json.NewEncoder(conn).Encode(snapshotResponse(focusedWorkspaceID, workspaceIDs...))
		_ = conn.Close()
		if err != nil {
			return err
		}
	}
}

func recordHistoryVisit(history []string, workspaceID string) []string {
	next := []string{workspaceID}
	for _, existing := range history {
		if existing != workspaceID {
			next = append(next, existing)
		}
	}
	return next
}

func removeHistoryWorkspace(history []string, workspaceID string) []string {
	next := history[:0]
	for _, existing := range history {
		if existing != workspaceID {
			next = append(next, existing)
		}
	}
	return next
}

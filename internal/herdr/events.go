package herdr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

const (
	eventReconnectDelay = 100 * time.Millisecond
	replayQuietPeriod   = 25 * time.Millisecond
	replayDrainMax      = 150 * time.Millisecond
	eventBufferSize     = 1024
)

type eventSubscription struct {
	Type string `json:"type"`
}

type eventSubscribeRequest struct {
	ID     string `json:"id"`
	Method string `json:"method"`
	Params struct {
		Subscriptions []eventSubscription `json:"subscriptions"`
	} `json:"params"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type workspaceEvent struct {
	Event string `json:"event"`
	Data  struct {
		WorkspaceID string `json:"workspace_id"`
	} `json:"data"`
}

type sessionSnapshot struct {
	FocusedWorkspaceID string
	WorkspaceIDs       map[string]bool
}

func WatchWorkspaceEvents(ctx context.Context, socketPath string, onFocused, onClosed func(string) error) error {
	if socketPath == "" {
		return errors.New("HERDR_SOCKET_PATH is required")
	}
	for {
		reconnect, err := watchWorkspaceEventsOnce(ctx, socketPath, onFocused, onClosed)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !reconnect {
			return err
		}
		timer := time.NewTimer(eventReconnectDelay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func watchWorkspaceEventsOnce(ctx context.Context, socketPath string, onFocused, onClosed func(string) error) (bool, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
	if err != nil {
		return true, fmt.Errorf("connect to Herdr event stream: %w", err)
	}
	defer func() { _ = conn.Close() }()
	stopClose := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopClose()

	request := eventSubscribeRequest{ID: "herdr-sesh-history", Method: "events.subscribe"}
	request.Params.Subscriptions = []eventSubscription{{Type: "workspace.focused"}, {Type: "workspace.closed"}}
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return true, fmt.Errorf("subscribe to Herdr workspace events: %w", err)
	}

	decoder := json.NewDecoder(conn)
	var acknowledgement struct {
		Result struct {
			Type string `json:"type"`
		} `json:"result"`
		Error *apiError `json:"error"`
	}
	if err := decoder.Decode(&acknowledgement); err != nil {
		return true, fmt.Errorf("read Herdr event subscription acknowledgement: %w", err)
	}
	if acknowledgement.Error != nil {
		return false, fmt.Errorf("herdr event subscription failed: %s: %s", acknowledgement.Error.Code, acknowledgement.Error.Message)
	}
	if acknowledgement.Result.Type != "subscription_started" {
		return false, fmt.Errorf("unexpected Herdr event subscription acknowledgement %q", acknowledgement.Result.Type)
	}

	streamCtx, cancelStream := context.WithCancel(ctx)
	defer cancelStream()
	events := make(chan workspaceEvent, eventBufferSize)
	streamErrors := make(chan error, 1)
	go decodeWorkspaceEvents(streamCtx, decoder, events, streamErrors)

	replayed, err := drainRetainedReplay(ctx, events, streamErrors)
	if err != nil {
		return streamResult(ctx, err)
	}
	snapshot, err := loadSessionSnapshot(ctx, socketPath)
	if err != nil {
		return true, err
	}
	for _, event := range replayed {
		// A close for an ID that still exists in the fresh snapshot came
		// from Herdr 0.8.x retained history and must not prune current state.
		if event.Event == "workspace_closed" && snapshot.WorkspaceIDs[event.Data.WorkspaceID] {
			continue
		}
		if err := applyWorkspaceEvent(event, onFocused, onClosed); err != nil {
			return false, err
		}
	}
	if snapshot.FocusedWorkspaceID != "" {
		if err := onFocused(snapshot.FocusedWorkspaceID); err != nil {
			return false, err
		}
	}

	for {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case err := <-streamErrors:
			if drainErr := drainBufferedEvents(events, onFocused, onClosed); drainErr != nil {
				return false, drainErr
			}
			return streamResult(ctx, err)
		case event := <-events:
			if err := applyWorkspaceEvent(event, onFocused, onClosed); err != nil {
				return false, err
			}
		}
	}
}

func decodeWorkspaceEvents(ctx context.Context, decoder *json.Decoder, events chan<- workspaceEvent, streamErrors chan<- error) {
	for {
		var event workspaceEvent
		if err := decoder.Decode(&event); err != nil {
			select {
			case streamErrors <- err:
			case <-ctx.Done():
			}
			return
		}
		if event.Data.WorkspaceID == "" || (event.Event != "workspace_focused" && event.Event != "workspace_closed") {
			continue
		}
		select {
		case events <- event:
		case <-ctx.Done():
			return
		}
	}
}

func drainRetainedReplay(ctx context.Context, events <-chan workspaceEvent, streamErrors <-chan error) ([]workspaceEvent, error) {
	quiet := time.NewTimer(replayQuietPeriod)
	defer quiet.Stop()
	maximum := time.NewTimer(replayDrainMax)
	defer maximum.Stop()
	var replayed []workspaceEvent
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case err := <-streamErrors:
			return nil, err
		case event := <-events:
			replayed = append(replayed, event)
			if !quiet.Stop() {
				select {
				case <-quiet.C:
				default:
				}
			}
			quiet.Reset(replayQuietPeriod)
		case <-quiet.C:
			return replayed, nil
		case <-maximum.C:
			return replayed, nil
		}
	}
}

func drainBufferedEvents(events <-chan workspaceEvent, onFocused, onClosed func(string) error) error {
	for {
		select {
		case event := <-events:
			if err := applyWorkspaceEvent(event, onFocused, onClosed); err != nil {
				return err
			}
		default:
			return nil
		}
	}
}

func applyWorkspaceEvent(event workspaceEvent, onFocused, onClosed func(string) error) error {
	switch event.Event {
	case "workspace_focused":
		return onFocused(event.Data.WorkspaceID)
	case "workspace_closed":
		return onClosed(event.Data.WorkspaceID)
	default:
		return nil
	}
}

func streamResult(ctx context.Context, err error) (bool, error) {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return false, fmt.Errorf("watch Herdr workspace events: %w", ctxErr)
	}
	if errors.Is(err, io.EOF) {
		err = io.ErrUnexpectedEOF
	}
	return true, fmt.Errorf("read Herdr workspace event: %w", err)
}

func loadSessionSnapshot(ctx context.Context, socketPath string) (sessionSnapshot, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
	if err != nil {
		return sessionSnapshot{}, fmt.Errorf("connect to Herdr session snapshot: %w", err)
	}
	defer func() { _ = conn.Close() }()
	stopClose := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopClose()

	request := struct {
		ID     string   `json:"id"`
		Method string   `json:"method"`
		Params struct{} `json:"params"`
	}{ID: "herdr-sesh-history-snapshot", Method: "session.snapshot"}
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return sessionSnapshot{}, fmt.Errorf("request Herdr session snapshot: %w", err)
	}
	var response struct {
		Result struct {
			Type     string `json:"type"`
			Snapshot struct {
				FocusedWorkspaceID string `json:"focused_workspace_id"`
				Workspaces         []struct {
					WorkspaceID string `json:"workspace_id"`
				} `json:"workspaces"`
			} `json:"snapshot"`
		} `json:"result"`
		Error *apiError `json:"error"`
	}
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		return sessionSnapshot{}, fmt.Errorf("read Herdr session snapshot: %w", err)
	}
	if response.Error != nil {
		return sessionSnapshot{}, fmt.Errorf("herdr session snapshot failed: %s: %s", response.Error.Code, response.Error.Message)
	}
	if response.Result.Type != "session_snapshot" {
		return sessionSnapshot{}, fmt.Errorf("unexpected Herdr session snapshot response %q", response.Result.Type)
	}
	snapshot := sessionSnapshot{
		FocusedWorkspaceID: response.Result.Snapshot.FocusedWorkspaceID,
		WorkspaceIDs:       make(map[string]bool, len(response.Result.Snapshot.Workspaces)),
	}
	for _, workspace := range response.Result.Snapshot.Workspaces {
		snapshot.WorkspaceIDs[workspace.WorkspaceID] = true
	}
	return snapshot, nil
}

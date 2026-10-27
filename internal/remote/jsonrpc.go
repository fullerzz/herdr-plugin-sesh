package remote

import (
	"bytes"
	"encoding/json"
	"errors"
)

// Daemon streams use JSON-RPC 2.0, one message per line. One-shot local
// requests keep the plain Request and Response encoding.
const jsonrpcVersion = "2.0"

const (
	codeParseError      = -32700
	codeInvalidRequest  = -32600
	codeMethodNotFound  = -32601
	codeInvalidParams   = -32602
	codeDaemonError     = -32000
	codeVersionMismatch = -32001
)

// switchNotification is the method the daemon uses to push a switch target.
const switchNotification = "switch"

var rpcMethods = map[string]bool{
	"hello": true, "ping": true, "list": true, "export": true, "focus": true,
	"switch": true, "subscribe": true, "unsubscribe": true, "complete": true,
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return e.Message }

// rpcMessage decodes any JSON-RPC request, response, or notification.
type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcRequest struct {
	JSONRPC string     `json:"jsonrpc"`
	ID      uint64     `json:"id"`
	Method  string     `json:"method"`
	Params  *rpcParams `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  *rpcResult      `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcNotification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  Target `json:"params"`
}

type rpcParams struct {
	Version     int      `json:"version,omitempty"`
	MachineID   string   `json:"machine_id,omitempty"`
	Session     string   `json:"session,omitempty"`
	WorkspaceID string   `json:"workspace_id,omitempty"`
	Machines    []string `json:"machines,omitempty"`
	Failure     string   `json:"failure,omitempty"`
}

type rpcResult struct {
	Version     int       `json:"version,omitempty"`
	Catalog     []Machine `json:"catalog,omitempty"`
	SwitchError string    `json:"switch_error,omitempty"`
}

func newRPCRequest(id uint64, r Request) rpcRequest {
	m := rpcRequest{JSONRPC: jsonrpcVersion, ID: id, Method: r.Op}
	p := rpcParams{Version: r.Version, MachineID: r.MachineID, Session: r.Session, WorkspaceID: r.WorkspaceID, Machines: r.Machines, Failure: r.Failure}
	if p.Version != 0 || p.MachineID != "" || p.Session != "" || p.WorkspaceID != "" || len(p.Machines) > 0 || p.Failure != "" {
		m.Params = &p
	}
	return m
}

// isRPC reports whether a line opens a JSON-RPC conversation.
func isRPC(line []byte) bool {
	t := bytes.TrimSpace(line)
	if len(t) > 0 && t[0] == '[' {
		return true
	}
	var probe struct {
		JSONRPC *string `json:"jsonrpc"`
	}
	return json.Unmarshal(t, &probe) == nil && probe.JSONRPC != nil
}

// decodeRPC validates one request line. A nil id with no error is a notification.
func decodeRPC(line []byte) (Request, json.RawMessage, *rpcError) {
	t := bytes.TrimSpace(line)
	if len(t) > 0 && t[0] == '[' {
		return Request{}, nil, &rpcError{Code: codeInvalidRequest, Message: "batch requests are not supported"}
	}
	var m rpcMessage
	if err := json.Unmarshal(t, &m); err != nil {
		return Request{}, nil, &rpcError{Code: codeParseError, Message: "parse error"}
	}
	if m.ID != nil && !validID(m.ID) {
		return Request{}, nil, &rpcError{Code: codeInvalidRequest, Message: "invalid request id"}
	}
	if m.JSONRPC != jsonrpcVersion || m.Method == "" || m.Result != nil || m.Error != nil {
		return Request{}, m.ID, &rpcError{Code: codeInvalidRequest, Message: "invalid request"}
	}
	var p rpcParams
	if m.Params != nil {
		dec := json.NewDecoder(bytes.NewReader(m.Params))
		dec.DisallowUnknownFields()
		if raw := bytes.TrimSpace(m.Params); len(raw) == 0 || raw[0] != '{' || dec.Decode(&p) != nil {
			return Request{}, m.ID, &rpcError{Code: codeInvalidParams, Message: "invalid params"}
		}
	}
	return Request{Op: m.Method, Version: p.Version, MachineID: p.MachineID, Session: p.Session, WorkspaceID: p.WorkspaceID, Machines: p.Machines, Failure: p.Failure}, m.ID, nil
}

// validID accepts the JSON-RPC id forms: string, number, or null.
func validID(id json.RawMessage) bool {
	var v any
	if json.Unmarshal(id, &v) != nil {
		return false
	}
	switch v.(type) {
	case nil, string, float64:
		return true
	}
	return false
}

func rpcReply(id json.RawMessage, op string, r Response) rpcResponse {
	if r.Error != "" {
		code := codeDaemonError
		if op == "hello" {
			code = codeVersionMismatch
		}
		return rpcFailure(id, code, r.Error)
	}
	return rpcResponse{JSONRPC: jsonrpcVersion, ID: replyID(id), Result: &rpcResult{Version: r.Version, Catalog: r.Catalog, SwitchError: r.SwitchError}}
}

func rpcFailure(id json.RawMessage, code int, message string) rpcResponse {
	return rpcResponse{JSONRPC: jsonrpcVersion, ID: replyID(id), Error: &rpcError{Code: code, Message: message}}
}

func replyID(id json.RawMessage) json.RawMessage {
	if id == nil {
		return json.RawMessage("null")
	}
	return id
}

// rpcResponseOf converts a decoded reply into the package's Response. A daemon
// error is returned as an *rpcError with its text cleaned for terminal output.
func rpcResponseOf(m rpcMessage) (Response, error) {
	if m.Error != nil {
		e := &rpcError{Code: m.Error.Code, Message: clean(m.Error.Message, 512)}
		return Response{Error: e.Message}, e
	}
	var result rpcResult
	if err := json.Unmarshal(m.Result, &result); err != nil {
		return Response{Error: "invalid daemon result"}, errors.New("invalid daemon result")
	}
	return Response{Version: result.Version, Catalog: result.Catalog, SwitchError: result.SwitchError}, nil
}

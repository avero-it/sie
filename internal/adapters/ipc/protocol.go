// Package ipc is the control API of sie: every instance listens on a local
// socket and speaks newline-delimited JSON-RPC 2.0 on it, and `sie mcp`
// (the bridge, see mcp.go) turns that into MCP tools over stdio for an
// agent. Both ends live here so they share the wire types.
//
// The server never touches the editor directly: each request becomes a job
// for the editor's loop goroutine (the Submit function) and waits for the
// answer there.
package ipc

import (
	"encoding/json"

	"github.com/avero-it/sie/internal/core/ports"
)

// ProtocolVersion is bumped on incompatible changes of the socket API.
const ProtocolVersion = 1

// Methods of the socket API.
const (
	MethodHello            = "hello"
	MethodInfo             = "info"
	MethodAttach           = "attach"
	MethodDetach           = "detach"
	MethodGetState         = "get_state"
	MethodGetText          = "get_text"
	MethodGetSelection     = "get_selection"
	MethodGoto             = "goto"
	MethodSelect           = "select"
	MethodInsert           = "insert"
	MethodReplaceRange     = "replace_range"
	MethodReplaceSelection = "replace_selection"
	MethodFind             = "find"
	MethodValidate         = "validate"
	MethodRunCommand       = "run_command"
	MethodListCommands     = "list_commands"
)

// Methods lists every method, as announced by hello.
var Methods = []string{
	MethodHello, MethodInfo, MethodAttach, MethodDetach, MethodGetState, MethodGetText,
	MethodGetSelection, MethodGoto, MethodSelect, MethodInsert, MethodReplaceRange,
	MethodReplaceSelection, MethodFind, MethodValidate, MethodRunCommand, MethodListCommands,
}

// JSON-RPC error codes. Failures of the editor use ErrApplication and carry
// the stable code (ports.Code*) in the error's data.
const (
	ErrParse       = -32700
	ErrRequest     = -32600
	ErrNoMethod    = -32601
	ErrParams      = -32602
	ErrApplication = -32000
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// Error is a JSON-RPC error; Data.Code is the stable code of editor errors.
type Error struct {
	Code    int        `json:"code"`
	Message string     `json:"message"`
	Data    *ErrorData `json:"data,omitempty"`
}

// ErrorData carries the stable error code.
type ErrorData struct {
	Code string `json:"code"`
}

func (e *Error) Error() string {
	if e.Data != nil {
		return e.Data.Code + ": " + e.Message
	}
	return e.Message
}

// StableCode returns the stable code of err ("" if it has none).
func StableCode(err error) string {
	if e, ok := err.(*Error); ok && e.Data != nil {
		return e.Data.Code
	}
	return ""
}

// HelloParams identifies the client; the name is shown to the user when the
// client asks to attach.
type HelloParams struct {
	Client  string `json:"client"`
	Version string `json:"version,omitempty"`
}

// HelloResult describes the instance.
type HelloResult struct {
	SieVersion string   `json:"sie_version"`
	Protocol   int      `json:"protocol"`
	PID        int      `json:"pid"`
	Methods    []string `json:"methods"`
}

// InfoResult is the answer of info.
type InfoResult struct {
	ports.RemoteInfo
	PID int `json:"pid"`
}

// TextParams selects lines (1-based, inclusive); both 0 means everything.
type TextParams struct {
	StartLine int `json:"start_line,omitempty"`
	EndLine   int `json:"end_line,omitempty"`
}

// GotoParams is a position.
type GotoParams = ports.RemotePos

// SelectParams is a range.
type SelectParams = ports.RemoteRange

// InsertParams inserts Text at a position.
type InsertParams struct {
	ports.RemotePos
	Text     string  `json:"text"`
	Escaped  bool    `json:"escaped,omitempty"`
	Revision *uint64 `json:"revision"`
}

// ReplaceRangeParams replaces a range.
type ReplaceRangeParams struct {
	ports.RemoteRange
	Text     string  `json:"text"`
	Escaped  bool    `json:"escaped,omitempty"`
	Revision *uint64 `json:"revision"`
}

// ReplaceSelectionParams replaces the primary selection.
type ReplaceSelectionParams struct {
	Text     string  `json:"text"`
	Escaped  bool    `json:"escaped,omitempty"`
	Revision *uint64 `json:"revision"`
}

// EditResult is the answer of every edit.
type EditResult struct {
	Revision uint64 `json:"revision"`
}

// FindParams searches the buffer.
type FindParams struct {
	Pattern       string `json:"pattern"`
	CaseSensitive bool   `json:"case_sensitive,omitempty"`
	Limit         int    `json:"limit,omitempty"`
}

// RunParams runs a prompt line.
type RunParams struct {
	Command string `json:"command"`
}

// RunResult is the answer of run_command.
type RunResult struct {
	Note     string `json:"note"`
	Revision uint64 `json:"revision"`
}

// SelectionsResult is the answer of get_selection.
type SelectionsResult struct {
	Selections []ports.RemoteSel `json:"selections"`
	Revision   uint64            `json:"revision"`
}

// CommandsResult is the answer of list_commands.
type CommandsResult struct {
	Commands []ports.RemoteCommand `json:"commands"`
}

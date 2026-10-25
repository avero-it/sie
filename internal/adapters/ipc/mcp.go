//go:build !windows

package ipc

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// mcpProtocol is the MCP revision the bridge speaks; a client asking for
// another one is answered with this one and decides whether to continue.
const mcpProtocol = "2025-06-18"

// Bridge serves MCP over a pair of streams (stdin and stdout of `sie mcp`)
// and forwards the tool calls to the sie instances' sockets. It has no
// editing logic of its own.
type Bridge struct {
	Dir     string // socket directory, see DefaultDir
	Version string // reported as the server's version
	Timeout time.Duration

	cl     *Client
	client string // the MCP client's name, shown to the user when attaching
	inst   Instance
}

type rpcRequest struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// Run answers requests from in until it ends. Only protocol messages are
// written to out.
func (b *Bridge) Run(in io.Reader, out io.Writer) error {
	if b.Timeout <= 0 {
		b.Timeout = defaultAttach + 10*time.Second
	}
	defer b.drop()
	r := bufio.NewReaderSize(in, 64<<10)
	enc := json.NewEncoder(out)
	for {
		line, err := readLine(r)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			if err := enc.Encode(rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"),
				Error: &Error{Code: ErrParse, Message: "invalid JSON"}}); err != nil {
				return err
			}
			continue
		}
		if len(req.ID) == 0 { // a notification, e.g. notifications/initialized
			continue
		}
		resp := rpcResponse{JSONRPC: "2.0", ID: req.ID}
		resp.Result, resp.Error = b.handle(req)
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
}

func (b *Bridge) handle(req rpcRequest) (any, *Error) {
	switch req.Method {
	case "initialize":
		var p struct {
			ClientInfo struct {
				Name  string `json:"name"`
				Title string `json:"title"`
			} `json:"clientInfo"`
		}
		_ = json.Unmarshal(req.Params, &p)
		b.client = cleanName(firstNonEmpty(p.ClientInfo.Title, p.ClientInfo.Name))
		return map[string]any{
			"protocolVersion": mcpProtocol,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "sie", "version": b.Version},
			"instructions":    instructions,
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": tools}, nil
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, &Error{Code: ErrParams, Message: "invalid params"}
		}
		text, err := b.call(p.Name, p.Arguments)
		if err != nil {
			return toolResult(err.Error(), true), nil
		}
		return toolResult(text, false), nil
	}
	return nil, &Error{Code: ErrNoMethod, Message: "unknown method " + req.Method}
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

func toolResult(text string, isError bool) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
		"isError": isError,
	}
}

// toolMethods maps the tools that go straight to a socket method with the
// same arguments.
var toolMethods = map[string]string{
	"get_state": MethodGetState, "get_text": MethodGetText, "get_selection": MethodGetSelection,
	"goto": MethodGoto, "select": MethodSelect, "insert": MethodInsert,
	"replace_range": MethodReplaceRange, "replace_selection": MethodReplaceSelection,
	"find": MethodFind, "validate": MethodValidate, "run_command": MethodRunCommand,
	"list_commands": MethodListCommands,
}

func (b *Bridge) call(tool string, args json.RawMessage) (string, error) {
	switch tool {
	case "list_sessions":
		return b.listSessions()
	case "attach":
		return b.attach(args)
	case "detach":
		if b.cl == nil {
			return "no session", nil
		}
		err := b.cl.Call(MethodDetach, nil, nil, b.Timeout)
		b.drop()
		if err != nil && StableCode(err) != "not_authorized" {
			return "", err
		}
		return "detached", nil
	}
	method, ok := toolMethods[tool]
	if !ok {
		return "", fmt.Errorf("unknown tool %q", tool)
	}
	if b.cl == nil {
		// Asking the user for access is what attach does; do it for the
		// editor they used last.
		if _, err := b.attach(nil); err != nil {
			return "", err
		}
	}
	var params any
	if len(args) > 0 && string(args) != "null" {
		params = args
	}
	var out json.RawMessage
	if err := b.cl.Call(method, params, &out, b.Timeout); err != nil {
		return "", b.explain(err)
	}
	return string(out), nil
}

// explain turns a failure into advice for the agent, and forgets a session
// that no longer works.
func (b *Bridge) explain(err error) error {
	var e *Error
	if errors.As(err, &e) {
		if StableCode(e) == "not_authorized" {
			b.drop()
			return errors.New("not_authorized: the user detached this session (or sie was restarted). " +
				"Call attach again only if you should continue")
		}
		return e
	}
	b.drop()
	return fmt.Errorf("lost the connection to sie (%v); call attach again", err)
}

func (b *Bridge) drop() {
	if b.cl != nil {
		_ = b.cl.Close()
		b.cl = nil
	}
}

type sessionInfo struct {
	PID          int       `json:"pid"`
	File         string    `json:"file"`
	Language     string    `json:"language"`
	Dirty        bool      `json:"dirty"`
	LastActivity time.Time `json:"last_activity"`
	Attached     bool      `json:"attached"`
	Mine         bool      `json:"mine,omitempty"`
}

func (b *Bridge) listSessions() (string, error) {
	var out []sessionInfo
	for _, in := range Discover(b.Dir, 2*time.Second) {
		out = append(out, sessionInfo{PID: in.PID, File: in.Path, Language: in.Language, Dirty: in.Dirty,
			LastActivity: in.LastActivity, Attached: in.Attached, Mine: b.cl != nil && b.inst.PID == in.PID})
	}
	if out == nil {
		out = []sessionInfo{}
	}
	data, _ := json.Marshal(map[string]any{"sessions": out})
	return string(data), nil
}

func (b *Bridge) attach(args json.RawMessage) (string, error) {
	var p struct {
		PID int `json:"pid"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &p); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
	}
	insts := Discover(b.Dir, 2*time.Second)
	var pick *Instance
	for i := range insts {
		switch {
		case p.PID != 0 && insts[i].PID == p.PID:
			pick = &insts[i]
		case p.PID == 0 && !insts[i].Attached:
			if pick == nil { // most recent activity first
				pick = &insts[i]
			}
		}
	}
	switch {
	case pick == nil && p.PID != 0:
		return "", fmt.Errorf("no sie with pid %d is running", p.PID)
	case pick == nil && len(insts) == 0:
		return "", errors.New("no sie is running; ask the user to open a file with sie")
	case pick == nil:
		return "", errors.New("every running sie already has a client attached")
	}
	if b.cl != nil {
		if b.inst.PID == pick.PID {
			return b.state()
		}
		b.drop()
	}
	cl, err := Dial(pick.Socket, 2*time.Second)
	if err != nil {
		return "", fmt.Errorf("connect to sie %d: %w", pick.PID, err)
	}
	if err := cl.Call(MethodHello, HelloParams{Client: b.client}, nil, 5*time.Second); err != nil {
		_ = cl.Close()
		return "", err
	}
	if err := cl.Call(MethodAttach, nil, nil, b.Timeout); err != nil {
		_ = cl.Close()
		return "", err
	}
	b.cl, b.inst = cl, *pick
	return b.state()
}

func (b *Bridge) state() (string, error) {
	var out json.RawMessage
	if err := b.cl.Call(MethodGetState, nil, &out, b.Timeout); err != nil {
		return "", b.explain(err)
	}
	return string(out), nil
}

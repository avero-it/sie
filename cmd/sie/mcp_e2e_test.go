//go:build !windows

package main

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	fsadapter "github.com/avero-it/sie/internal/adapters/fs"
	"github.com/avero-it/sie/internal/adapters/ipc"
	"github.com/avero-it/sie/internal/adapters/keymap"
	"github.com/avero-it/sie/internal/adapters/syntax"
	"github.com/avero-it/sie/internal/adapters/validate"
	"github.com/avero-it/sie/internal/core/app"
	"github.com/avero-it/sie/internal/core/domain/buffer"
	"github.com/avero-it/sie/internal/core/ports"
)

// loopGoroutine owns the editor like the terminal's event loop does.
type loopGoroutine struct{ jobs chan func() }

func newLoopGoroutine(t *testing.T) *loopGoroutine {
	l := &loopGoroutine{jobs: make(chan func(), 64)}
	quit := make(chan struct{})
	t.Cleanup(func() { close(quit) })
	go func() {
		for {
			select {
			case j := <-l.jobs:
				j()
			case <-quit:
				return
			}
		}
	}()
	return l
}

func (l *loopGoroutine) submit(fn func()) { l.jobs <- fn }

func (l *loopGoroutine) do(fn func()) {
	done := make(chan struct{})
	l.jobs <- func() { fn(); close(done) }
	<-done
}

// mcpClient talks to a Bridge the way an agent does.
type mcpClient struct {
	t   *testing.T
	in  io.Writer
	out *bufio.Reader
	id  int
}

func (c *mcpClient) request(method string, params any) (json.RawMessage, *ipc.Error) {
	c.t.Helper()
	c.id++
	line, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": c.id, "method": method, "params": params})
	if _, err := c.in.Write(append(line, '\n')); err != nil {
		c.t.Fatal(err)
	}
	var resp struct {
		Result json.RawMessage `json:"result"`
		Error  *ipc.Error      `json:"error"`
	}
	data, err := c.out.ReadBytes('\n')
	if err != nil {
		c.t.Fatal(err)
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		c.t.Fatalf("bad reply %q: %v", data, err)
	}
	return resp.Result, resp.Error
}

// tool calls a tool and returns its text and whether it failed.
func (c *mcpClient) tool(name string, args any) (string, bool) {
	c.t.Helper()
	res, rpcErr := c.request("tools/call", map[string]any{"name": name, "arguments": args})
	if rpcErr != nil {
		c.t.Fatalf("%s: %v", name, rpcErr)
	}
	var r struct {
		Content []struct{ Text string } `json:"content"`
		IsError bool                    `json:"isError"`
	}
	if err := json.Unmarshal(res, &r); err != nil || len(r.Content) == 0 {
		c.t.Fatalf("%s: bad result %s", name, res)
	}
	return r.Content[0].Text, r.IsError
}

func (c *mcpClient) ok(name string, args any, out any) {
	c.t.Helper()
	text, failed := c.tool(name, args)
	if failed {
		c.t.Fatalf("%s failed: %s", name, text)
	}
	if out != nil {
		if err := json.Unmarshal([]byte(text), out); err != nil {
			c.t.Fatalf("%s: %v in %s", name, err, text)
		}
	}
}

func TestMCPEndToEnd(t *testing.T) {
	root, err := os.MkdirTemp("", "sie")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	file := filepath.Join(root, "a.json")
	if err := os.WriteFile(file, []byte("{\n  \"name\": \"sie\"\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	hl := syntax.New(func() {})
	t.Cleanup(hl.Close)
	editor, err := app.New(app.Config{Files: fsadapter.Store{}, Keymap: keymap.Default(), Validator: validate.Validator{},
		Highlighter: hl, SyntaxHighlighting: true,
		DefaultEOL: buffer.LF, TabWidth: 4, Version: "test", WorkDir: root})
	if err != nil {
		t.Fatal(err)
	}
	loop := newLoopGoroutine(t)
	loop.do(func() {
		editor.Resize(100, 20)
		if err := editor.Dispatch("file.open", file); err != nil {
			t.Error(err)
		}
	})

	dir := filepath.Join(root, "sock")
	srv, err := ipc.Listen(ipc.Options{Dir: dir, Remote: editor, Submit: loop.submit})
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve()
	t.Cleanup(func() { _ = srv.Close() })

	// Permissions: only this user.
	if fi, err := os.Stat(dir); err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("dir: %v %v", fi, err)
	}
	if fi, err := os.Stat(srv.Path()); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("socket: %v %v", fi, err)
	}

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	bridge := &ipc.Bridge{Dir: dir, Version: "test"}
	done := make(chan error, 1)
	go func() { done <- bridge.Run(inR, outW) }()
	t.Cleanup(func() { _ = inW.Close(); <-done })
	c := &mcpClient{t: t, in: inW, out: bufio.NewReader(outR)}

	if res, rpcErr := c.request("initialize", map[string]any{"protocolVersion": "2025-06-18",
		"clientInfo": map[string]any{"name": "claude-code", "title": "Claude Code"}}); rpcErr != nil || !strings.Contains(string(res), `"sie"`) {
		t.Fatalf("initialize = %s, %v", res, rpcErr)
	}
	if res, rpcErr := c.request("tools/list", nil); rpcErr != nil {
		t.Fatal(rpcErr)
	} else {
		var l struct{ Tools []struct{ Name string } }
		_ = json.Unmarshal(res, &l)
		names := map[string]bool{}
		for _, tl := range l.Tools {
			names[tl.Name] = true
		}
		for _, want := range []string{"attach", "get_text", "replace_range", "run_command", "list_sessions"} {
			if !names[want] {
				t.Errorf("tool %s missing", want)
			}
		}
		if names["save"] {
			t.Error("save must not be exposed")
		}
	}

	// Attach blocks until the user answers in the editor.
	attached := make(chan string, 1)
	go func() {
		text, failed := c.tool("attach", nil)
		if failed {
			text = "FAILED " + text
		}
		attached <- text
	}()
	waitFor(t, "the question", func() bool {
		var asking bool
		loop.do(func() {
			for _, r := range editor.View().Runs {
				if strings.Contains(r.Text, "Claude Code wants to control this buffer") {
					asking = true
				}
			}
		})
		return asking
	})
	select {
	case got := <-attached:
		t.Fatalf("attached before the user answered: %s", got)
	case <-time.After(50 * time.Millisecond):
	}
	loop.do(func() { editor.HandleKey(ports.KeyEvent{Key: ports.KeyRune, Rune: 'y'}) })
	var st ports.RemoteState
	select {
	case got := <-attached:
		if err := json.Unmarshal([]byte(got), &st); err != nil {
			t.Fatalf("attach result %q: %v", got, err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("attach never returned")
	}
	if st.Language != "JSON" || st.Lines != 4 || st.Dirty {
		t.Errorf("state = %+v", st)
	}
	loop.do(func() {
		if !strings.Contains(statusLine(editor), "● MCP") {
			t.Errorf("no MCP indicator: %q", statusLine(editor))
		}
	})

	// Read, edit, and the revision protects the user's typing.
	var txt ports.RemoteText
	c.ok("get_text", map[string]any{"start_line": 2, "end_line": 2}, &txt)
	if txt.Text != "  \"name\": \"sie\"\n" {
		t.Fatalf("text = %q", txt.Text)
	}
	edit := map[string]any{
		"start": map[string]any{"line": 2, "col": 12}, "end": map[string]any{"line": 2, "col": 15},
		"text": "agent", "revision": txt.Revision,
	}
	var res ipc.EditResult
	c.ok("replace_range", edit, &res)
	c.ok("get_text", nil, &txt)
	if txt.Text != "{\n  \"name\": \"agent\"\n}\n" || txt.Revision != res.Revision {
		t.Fatalf("after edit: %q rev %d vs %d", txt.Text, txt.Revision, res.Revision)
	}
	if text, failed := c.tool("replace_range", edit); !failed || !strings.Contains(text, "stale_revision") {
		t.Errorf("stale edit = %v %q", failed, text)
	}

	// The edit is one undo step for the user and is not saved.
	loop.do(func() { editor.HandleKey(ports.Ctrl('z')) })
	c.ok("get_text", nil, &txt)
	if txt.Text != "{\n  \"name\": \"sie\"\n}\n" {
		t.Errorf("after Ctrl+Z: %q", txt.Text)
	}
	if b, _ := os.ReadFile(file); string(b) != "{\n  \"name\": \"sie\"\n}\n" {
		t.Errorf("file on disk changed: %q", b)
	}

	// Other tools.
	var v ports.RemoteValidation
	c.ok("validate", nil, &v)
	if !v.Valid || v.Language != "JSON" {
		t.Errorf("validate = %+v", v)
	}
	var f ports.RemoteMatches
	c.ok("find", map[string]any{"pattern": "sie"}, &f)
	if len(f.Matches) != 1 || f.Matches[0].Start.Line != 2 {
		t.Errorf("find = %+v", f)
	}
	if text, failed := c.tool("run_command", map[string]any{"command": "save"}); !failed {
		t.Errorf("run_command save was allowed: %s", text)
	}
	var sess struct {
		Sessions []struct {
			PID      int
			Mine     bool
			Attached bool
		}
	}
	c.ok("list_sessions", nil, &sess)
	if len(sess.Sessions) != 1 || !sess.Sessions[0].Mine || !sess.Sessions[0].Attached || sess.Sessions[0].PID != os.Getpid() {
		t.Errorf("sessions = %+v", sess)
	}

	// The user takes the buffer back.
	loop.do(func() {
		if err := editor.Dispatch("mcp.detach"); err != nil {
			t.Error(err)
		}
	})
	if text, failed := c.tool("get_state", nil); !failed || !strings.Contains(text, "not_authorized") {
		t.Errorf("after detach = %v %q", failed, text)
	}
}

func statusLine(e *app.Editor) string {
	vm := e.View()
	var sb strings.Builder
	for _, r := range vm.Runs {
		if r.Y == vm.Height-1 {
			sb.WriteString(r.Text)
		}
	}
	return sb.String()
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

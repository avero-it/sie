package ipc

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/avero-it/sie/internal/core/ports"
)

// fakeRemote answers like an editor; only the methods the tests use are real.
type fakeRemote struct {
	ports.Remote
	mu       sync.Mutex
	session  uint64
	clients  []string
	answer   func(reply func(uint64, error)) // nil keeps the question open
	cancels  int
	replaced []string
	rev      uint64
}

func (f *fakeRemote) Info() ports.RemoteInfo {
	return ports.RemoteInfo{Version: "v-test", Path: "a.go", Language: "Go", Attached: f.session != 0}
}
func (f *fakeRemote) Attach(client string, reply func(uint64, error)) {
	f.clients = append(f.clients, client)
	if f.answer != nil {
		f.answer(func(id uint64, err error) {
			if err == nil {
				f.session = id
			}
			reply(id, err)
		})
	}
}
func (f *fakeRemote) CancelAttach()                { f.cancels++ }
func (f *fakeRemote) SessionActive(id uint64) bool { return id != 0 && id == f.session }
func (f *fakeRemote) Detach()                      { f.session = 0 }
func (f *fakeRemote) State() ports.RemoteState {
	return ports.RemoteState{Path: "a.go", Revision: f.rev}
}
func (f *fakeRemote) Replace(rev uint64, r ports.RemoteRange, t ports.RemoteTextIn) (uint64, error) {
	if rev != f.rev {
		return 0, &ports.RemoteError{Code: ports.CodeStaleRevision, Message: "stale"}
	}
	f.replaced = append(f.replaced, t.Text)
	f.rev++
	return f.rev, nil
}

// loopRunner runs submitted jobs one at a time, like the editor's loop.
type loopRunner struct {
	jobs chan func()
	stop chan struct{}
}

func newLoop() *loopRunner {
	l := &loopRunner{jobs: make(chan func(), 64), stop: make(chan struct{})}
	go func() {
		for {
			select {
			case j := <-l.jobs:
				j()
			case <-l.stop:
				return
			}
		}
	}()
	return l
}

// exec runs fn on the loop and waits for it, for inspecting the fake safely.
func (l *loopRunner) exec(fn func()) {
	done := make(chan struct{})
	l.jobs <- func() { fn(); close(done) }
	<-done
}

func (l *loopRunner) submit(fn func()) { l.jobs <- fn }

type env struct {
	t    *testing.T
	dir  string
	rem  *fakeRemote
	loop *loopRunner
	srv  *Server
}

func newEnv(t *testing.T) *env {
	t.Helper()
	root, err := os.MkdirTemp("", "sie")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	e := &env{t: t, dir: filepath.Join(root, "sock"), rem: &fakeRemote{}, loop: newLoop()}
	t.Cleanup(func() { close(e.loop.stop) })
	e.start(Options{})
	return e
}

func (e *env) start(o Options) {
	e.t.Helper()
	o.Dir, o.Remote, o.Submit = e.dir, e.rem, e.loop.submit
	srv, err := Listen(o)
	if err != nil {
		e.t.Fatal(err)
	}
	go srv.Serve()
	e.srv = srv
	e.t.Cleanup(func() { _ = srv.Close() })
}

func (e *env) dial() *Client {
	e.t.Helper()
	c, err := Dial(e.srv.Path(), time.Second)
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { _ = c.Close() })
	return c
}

func call(t *testing.T, c *Client, method string, params, out any) error {
	t.Helper()
	return c.Call(method, params, out, 5*time.Second)
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	if StableCode(err) != code {
		t.Fatalf("error = %v; want code %s", err, code)
	}
}

func grant(_ *fakeRemote, id uint64) func(func(uint64, error)) {
	return func(reply func(uint64, error)) { reply(id, nil) }
}

func TestSocketPermissions(t *testing.T) {
	e := newEnv(t)
	di, err := os.Stat(e.dir)
	if err != nil || di.Mode().Perm() != 0o700 {
		t.Errorf("dir mode = %v, %v", di.Mode(), err)
	}
	si, err := os.Stat(e.srv.Path())
	if err != nil || si.Mode().Perm() != 0o600 {
		t.Errorf("socket mode = %v, %v", si.Mode(), err)
	}
}

func TestLooseDirectoryRefused(t *testing.T) {
	e := newEnv(t)
	if err := os.Chmod(e.dir, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := Listen(Options{Dir: e.dir, PID: 4242, Remote: e.rem, Submit: e.loop.submit})
	if err == nil {
		t.Fatal("listened in a world-readable directory")
	}
}

func TestCloseRemovesSocket(t *testing.T) {
	e := newEnv(t)
	path := e.srv.Path()
	if err := e.srv.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("socket still there: %v", err)
	}
}

func TestStaleSocketsRemoved(t *testing.T) {
	e := newEnv(t)
	stale := filepath.Join(e.dir, "99999.sock")
	ln, err := net.Listen("unix", stale)
	if err != nil {
		t.Fatal(err)
	}
	ln.(*net.UnixListener).SetUnlinkOnClose(false)
	_ = ln.Close() // the file stays, nobody listens
	if _, err := os.Stat(stale); err != nil {
		t.Fatal(err)
	}
	found := Discover(e.dir, time.Second)
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale socket not removed: %v", err)
	}
	if len(found) != 1 || found[0].Path != "a.go" || found[0].PID != os.Getpid() {
		t.Errorf("found = %+v", found)
	}
}

func TestHelloAndInfoNeedNoSession(t *testing.T) {
	e := newEnv(t)
	c := e.dial()
	var h HelloResult
	if err := call(t, c, MethodHello, HelloParams{Client: "x"}, &h); err != nil {
		t.Fatal(err)
	}
	if h.Protocol != ProtocolVersion || h.SieVersion != "v-test" || len(h.Methods) != len(Methods) {
		t.Errorf("hello = %+v", h)
	}
	var info InfoResult
	if err := call(t, c, MethodInfo, nil, &info); err != nil || info.Language != "Go" {
		t.Errorf("info = %+v, %v", info, err)
	}
}

func TestUnauthorizedUntilAttached(t *testing.T) {
	e := newEnv(t)
	c := e.dial()
	for _, m := range []string{MethodGetState, MethodGetText, MethodGetSelection, MethodValidate, MethodListCommands, MethodDetach} {
		wantCode(t, call(t, c, m, nil, nil), ports.CodeNotAuthorized)
	}
	rev := uint64(0)
	wantCode(t, call(t, c, MethodInsert, InsertParams{Text: "x", Revision: &rev}, nil), ports.CodeNotAuthorized)
	if len(e.rem.replaced) != 0 {
		t.Error("an unauthorized edit was applied")
	}
}

func TestAttachFlow(t *testing.T) {
	e := newEnv(t)
	e.rem.answer = grant(e.rem, 7)
	c := e.dial()
	if err := call(t, c, MethodHello, HelloParams{Client: "Claude\x1b[31m Code\n"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := call(t, c, MethodAttach, nil, nil); err != nil {
		t.Fatal(err)
	}
	e.loop.exec(func() {
		if len(e.rem.clients) != 1 || e.rem.clients[0] != "Claude[31m Code" {
			t.Errorf("client name shown = %q", e.rem.clients)
		}
	})
	var st ports.RemoteState
	if err := call(t, c, MethodGetState, nil, &st); err != nil || st.Path != "a.go" {
		t.Errorf("state = %+v, %v", st, err)
	}
	// Another connection is not let in by the first one's session.
	wantCode(t, call(t, e.dial(), MethodGetState, nil, nil), ports.CodeNotAuthorized)
	// Detach, then nothing works.
	if err := call(t, c, MethodDetach, nil, nil); err != nil {
		t.Fatal(err)
	}
	wantCode(t, call(t, c, MethodGetState, nil, nil), ports.CodeNotAuthorized)
}

func TestAttachDenied(t *testing.T) {
	e := newEnv(t)
	e.rem.answer = func(reply func(uint64, error)) {
		reply(0, &ports.RemoteError{Code: ports.CodeDenied, Message: "no"})
	}
	wantCode(t, call(t, e.dial(), MethodAttach, nil, nil), ports.CodeDenied)
}

func TestAttachTimeoutCancelsQuestion(t *testing.T) {
	e := newEnv(t)
	e.srv.Close()
	e.start(Options{AttachTimeout: 50 * time.Millisecond})
	// The question stays open until CancelAttach; the fake answers then.
	var pending func(uint64, error)
	e.rem.answer = func(reply func(uint64, error)) { pending = reply }
	done := make(chan error, 1)
	c := e.dial()
	go func() { done <- c.Call(MethodAttach, nil, nil, 5*time.Second) }()
	deadline := time.After(2 * time.Second)
	for {
		var n int
		e.loop.exec(func() { n = e.rem.cancels })
		if n > 0 {
			e.loop.exec(func() { pending(0, &ports.RemoteError{Code: ports.CodeDenied, Message: "no answer"}) })
			break
		}
		select {
		case <-deadline:
			t.Fatal("question never withdrawn")
		case <-time.After(10 * time.Millisecond):
		}
	}
	wantCode(t, <-done, ports.CodeDenied)
}

func TestDisconnectDetaches(t *testing.T) {
	e := newEnv(t)
	e.rem.answer = grant(e.rem, 3)
	c := e.dial()
	if err := call(t, c, MethodAttach, nil, nil); err != nil {
		t.Fatal(err)
	}
	_ = c.Close()
	deadline := time.After(2 * time.Second)
	for {
		var s uint64
		e.loop.exec(func() { s = e.rem.session })
		if s == 0 {
			return
		}
		select {
		case <-deadline:
			t.Fatal("still attached after the client left")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestEditNeedsRevisionAndReportsStale(t *testing.T) {
	e := newEnv(t)
	e.rem.answer = grant(e.rem, 1)
	c := e.dial()
	if err := call(t, c, MethodAttach, nil, nil); err != nil {
		t.Fatal(err)
	}
	wantCode(t, call(t, c, MethodInsert, map[string]any{"line": 1, "col": 1, "text": "x"}, nil), ports.CodeInvalid)
	zero, five := uint64(0), uint64(5)
	var res EditResult
	if err := call(t, c, MethodInsert, InsertParams{Text: "x", Revision: &zero}, &res); err != nil || res.Revision != 1 {
		t.Fatalf("insert = %+v, %v", res, err)
	}
	wantCode(t, call(t, c, MethodReplaceRange, ReplaceRangeParams{Text: "y", Revision: &five}, nil), ports.CodeStaleRevision)
	e.loop.exec(func() {
		if len(e.rem.replaced) != 1 {
			t.Errorf("replaced = %q", e.rem.replaced)
		}
	})
}

func TestBusyLoopSkipsAbandonedJob(t *testing.T) {
	e := newEnv(t)
	e.srv.Close()
	e.start(Options{CallTimeout: 50 * time.Millisecond})
	e.rem.answer = grant(e.rem, 1)
	c := e.dial()
	if err := call(t, c, MethodAttach, nil, nil); err != nil {
		t.Fatal(err)
	}
	gate := make(chan struct{})
	e.loop.submit(func() { <-gate }) // blocks the loop in front of our request
	zero := uint64(0)
	wantCode(t, call(t, c, MethodInsert, InsertParams{Text: "late", Revision: &zero}, nil), ports.CodeBusy)
	close(gate)
	e.loop.exec(func() {})
	e.loop.exec(func() {
		if len(e.rem.replaced) != 0 {
			t.Errorf("an abandoned edit was applied later: %q", e.rem.replaced)
		}
	})
}

func TestProtocolErrors(t *testing.T) {
	e := newEnv(t)
	nc, err := net.Dial("unix", e.srv.Path())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	r := bufio.NewReader(nc)
	ask := func(line string) response {
		t.Helper()
		_ = nc.SetDeadline(time.Now().Add(5 * time.Second))
		if _, err := nc.Write([]byte(line + "\n")); err != nil {
			t.Fatal(err)
		}
		var resp response
		data, err := r.ReadBytes('\n')
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &resp); err != nil {
			t.Fatal(err)
		}
		return resp
	}
	if got := ask(`{nope`); got.Error == nil || got.Error.Code != ErrParse {
		t.Errorf("garbage = %+v", got.Error)
	}
	if got := ask(`{"jsonrpc":"2.0","id":1,"method":"nope"}`); got.Error == nil || got.Error.Code != ErrNoMethod {
		t.Errorf("unknown method = %+v", got.Error)
	}
	if got := ask(`{"jsonrpc":"1.0","id":2,"method":"info"}`); got.Error == nil || got.Error.Code != ErrRequest {
		t.Errorf("wrong version = %+v", got.Error)
	}
	if got := ask(`{"jsonrpc":"2.0","id":3,"method":"get_text","params":{"start_line":"x"}}`); got.Error == nil || got.Error.Code != ErrParams {
		t.Errorf("bad params = %+v", got.Error)
	}
	// A notification gets no answer; the next request still works.
	if _, err := nc.Write([]byte(`{"jsonrpc":"2.0","method":"info"}` + "\n")); err != nil {
		t.Fatal(err)
	}
	if got := ask(`{"jsonrpc":"2.0","id":4,"method":"info"}`); got.Error != nil || string(got.ID) != "4" {
		t.Errorf("after notification = %+v", got)
	}
}

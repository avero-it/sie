package ipc

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/avero-it/sie/internal/core/ports"
)

const (
	maxMessage     = 64 << 20 // largest request line
	maxConns       = 8
	maxClientName  = 40
	defaultAttach  = 60 * time.Second
	defaultCall    = 10 * time.Second
	writeTimeout   = 10 * time.Second
	socketSuffix   = ".sock"
	staleDialLimit = 300 * time.Millisecond
)

// Options configures a Server.
type Options struct {
	Dir    string // socket directory, see DefaultDir
	PID    int    // names the socket; 0 means this process
	Remote ports.Remote
	// Submit queues fn to run on the editor's loop goroutine (tui.Terminal.Submit).
	Submit func(fn func())

	AttachTimeout time.Duration // how long the user has to answer; <= 0 means 60 s
	CallTimeout   time.Duration // how long a request waits for the loop; <= 0 means 10 s
}

// DefaultDir is where instances put their sockets: ~/.sie/sock.
func DefaultDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	return filepath.Join(home, ".sie", "sock"), nil
}

// Server is the socket of one sie instance.
type Server struct {
	o    Options
	ln   net.Listener
	path string

	mu     sync.Mutex
	conns  map[*conn]struct{}
	closed bool
	wg     sync.WaitGroup
}

// Listen prepares the socket directory (mode 0700, ours), removes sockets
// of instances that no longer run and opens <pid>.sock with mode 0600.
func Listen(o Options) (*Server, error) {
	if o.Remote == nil || o.Submit == nil {
		return nil, errors.New("ipc: Remote and Submit are required")
	}
	if o.PID == 0 {
		o.PID = os.Getpid()
	}
	if o.AttachTimeout <= 0 {
		o.AttachTimeout = defaultAttach
	}
	if o.CallTimeout <= 0 {
		o.CallTimeout = defaultCall
	}
	if err := prepareDir(o.Dir); err != nil {
		return nil, err
	}
	RemoveStale(o.Dir)
	path := socketPath(o.Dir, o.PID)
	_ = os.Remove(path) // a leftover of an earlier process with the same PID
	ln, err := listenUnix(path)
	if err != nil {
		return nil, err
	}
	return &Server{o: o, ln: ln, path: path, conns: map[*conn]struct{}{}}, nil
}

func socketPath(dir string, pid int) string {
	return filepath.Join(dir, strconv.Itoa(pid)+socketSuffix)
}

// Path returns the socket's file name.
func (s *Server) Path() string { return s.path }

// Serve accepts connections until Close. Run it in its own goroutine.
func (s *Server) Serve() {
	for {
		nc, err := s.ln.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		if s.closed || len(s.conns) >= maxConns {
			s.mu.Unlock()
			_ = nc.Close()
			continue
		}
		c := &conn{s: s, nc: nc}
		s.conns[c] = struct{}{}
		s.wg.Add(1)
		s.mu.Unlock()
		go c.serve()
	}
}

// Close stops listening, drops every connection and removes the socket.
func (s *Server) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	conns := make([]*conn, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	s.mu.Unlock()
	err := s.ln.Close()
	for _, c := range conns {
		_ = c.nc.Close()
	}
	s.wg.Wait()
	if rerr := os.Remove(s.path); rerr != nil && !os.IsNotExist(rerr) && err == nil {
		err = rerr
	}
	return err
}

// conn is one client connection; its requests are handled one at a time.
type conn struct {
	s       *Server
	nc      net.Conn
	client  string
	session atomic.Uint64 // set when the user lets the client attach

	dead      atomic.Bool // the connection is gone
	attaching atomic.Bool // an attach question is open for this client
}

func (c *conn) serve() {
	defer func() {
		// A client that goes away lets go of the buffer, and withdraws its question.
		c.dead.Store(true)
		if c.attaching.Load() {
			c.s.o.Submit(c.s.o.Remote.CancelAttach)
		}
		if id := c.session.Load(); id != 0 {
			c.s.o.Submit(func() {
				if c.s.o.Remote.SessionActive(id) {
					c.s.o.Remote.Detach()
				}
			})
		}
		_ = c.nc.Close()
		c.s.mu.Lock()
		delete(c.s.conns, c)
		c.s.mu.Unlock()
		c.s.wg.Done()
	}()
	r := bufio.NewReaderSize(c.nc, 64<<10)
	for {
		line, err := readLine(r)
		if err != nil {
			return
		}
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		resp, ok := c.handle(line)
		if !ok {
			continue
		}
		_ = c.nc.SetWriteDeadline(time.Now().Add(writeTimeout))
		out, _ := json.Marshal(resp)
		if _, err := c.nc.Write(append(out, '\n')); err != nil {
			return
		}
	}
}

func readLine(r *bufio.Reader) ([]byte, error) {
	var line []byte
	for {
		part, err := r.ReadSlice('\n')
		line = append(line, part...)
		if len(line) > maxMessage {
			return nil, errors.New("message too large")
		}
		if err == nil {
			return line, nil
		}
		if !errors.Is(err, bufio.ErrBufferFull) {
			return nil, err
		}
	}
}

// handle answers one request line; ok is false for notifications.
func (c *conn) handle(line []byte) (response, bool) {
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		return response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &Error{Code: ErrParse, Message: "invalid JSON"}}, true
	}
	if len(req.ID) == 0 {
		return response{}, false
	}
	resp := response{JSONRPC: "2.0", ID: req.ID}
	if req.JSONRPC != "2.0" || req.Method == "" {
		resp.Error = &Error{Code: ErrRequest, Message: "not a JSON-RPC 2.0 request"}
		return resp, true
	}
	result, err := c.dispatch(req.Method, req.Params)
	if err != nil {
		resp.Error = toError(err)
		return resp, true
	}
	if result == nil {
		result = struct{}{}
	}
	raw, merr := json.Marshal(result)
	if merr != nil {
		resp.Error = &Error{Code: ErrApplication, Message: merr.Error(), Data: &ErrorData{Code: ports.CodeFailed}}
		return resp, true
	}
	resp.Result = raw
	return resp, true
}

func toError(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	var re *ports.RemoteError
	if errors.As(err, &re) {
		return &Error{Code: ErrApplication, Message: re.Message, Data: &ErrorData{Code: re.Code}}
	}
	return &Error{Code: ErrApplication, Message: err.Error(), Data: &ErrorData{Code: ports.CodeFailed}}
}

func badParams(format string, args ...any) error {
	return &Error{Code: ErrParams, Message: fmt.Sprintf(format, args...), Data: &ErrorData{Code: ports.CodeInvalid}}
}

func decode(raw json.RawMessage, v any) error {
	if len(raw) == 0 {
		raw = json.RawMessage("{}")
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return badParams("invalid params: %v", err)
	}
	return nil
}

func (c *conn) dispatch(method string, raw json.RawMessage) (any, error) {
	rem := c.s.o.Remote
	switch method {
	case MethodHello:
		var p HelloParams
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		c.client = cleanName(p.Client)
		return c.loop(false, func() (any, error) {
			return HelloResult{SieVersion: rem.Info().Version, Protocol: ProtocolVersion, PID: c.s.o.PID, Methods: Methods}, nil
		})
	case MethodInfo:
		return c.loop(false, func() (any, error) { return InfoResult{RemoteInfo: rem.Info(), PID: c.s.o.PID}, nil })
	case MethodAttach:
		return c.attach()
	case MethodDetach:
		return c.loop(true, func() (any, error) {
			rem.Detach()
			c.session.Store(0)
			return nil, nil
		})
	case MethodGetState:
		return c.loop(true, func() (any, error) { return rem.State(), nil })
	case MethodGetText:
		var p TextParams
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		return c.loop(true, func() (any, error) { return rem.Text(p.StartLine, p.EndLine) })
	case MethodGetSelection:
		return c.loop(true, func() (any, error) {
			return SelectionsResult{Selections: rem.Selections(), Revision: rem.State().Revision}, nil
		})
	case MethodGoto:
		var p GotoParams
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		return c.loop(true, func() (any, error) { return nil, rem.Goto(p) })
	case MethodSelect:
		var p SelectParams
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		return c.loop(true, func() (any, error) { return nil, rem.Select(p) })
	case MethodInsert:
		var p InsertParams
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		if p.Revision == nil {
			return nil, badParams("revision is required")
		}
		return c.loop(true, func() (any, error) {
			rev, err := rem.Replace(*p.Revision, ports.RemoteRange{Start: p.RemotePos, End: p.RemotePos},
				ports.RemoteTextIn{Text: p.Text, Escaped: p.Escaped})
			return editResult(rev, err)
		})
	case MethodReplaceRange:
		var p ReplaceRangeParams
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		if p.Revision == nil {
			return nil, badParams("revision is required")
		}
		return c.loop(true, func() (any, error) {
			rev, err := rem.Replace(*p.Revision, p.RemoteRange, ports.RemoteTextIn{Text: p.Text, Escaped: p.Escaped})
			return editResult(rev, err)
		})
	case MethodReplaceSelection:
		var p ReplaceSelectionParams
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		if p.Revision == nil {
			return nil, badParams("revision is required")
		}
		return c.loop(true, func() (any, error) {
			rev, err := rem.ReplaceSelection(*p.Revision, ports.RemoteTextIn{Text: p.Text, Escaped: p.Escaped})
			return editResult(rev, err)
		})
	case MethodFind:
		var p FindParams
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		return c.loop(true, func() (any, error) {
			return rem.Find(ports.RemoteFind{Pattern: p.Pattern, CaseSensitive: p.CaseSensitive, Limit: p.Limit})
		})
	case MethodValidate:
		return c.loop(true, func() (any, error) { return rem.Validate() })
	case MethodRunCommand:
		var p RunParams
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		return c.loop(true, func() (any, error) {
			note, err := rem.Run(p.Command)
			if err != nil {
				return nil, err
			}
			return RunResult{Note: note, Revision: rem.State().Revision}, nil
		})
	case MethodListCommands:
		return c.loop(true, func() (any, error) { return CommandsResult{Commands: rem.Commands()}, nil })
	}
	return nil, &Error{Code: ErrNoMethod, Message: "unknown method " + strconv.Quote(method)}
}

func editResult(rev uint64, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	return EditResult{Revision: rev}, nil
}

// loop runs fn on the editor's loop goroutine and waits for its result. It
// is skipped, with a busy error, if the loop does not get to it in time.
// With auth, fn only runs while this connection is the attached session.
func (c *conn) loop(auth bool, fn func() (any, error)) (any, error) {
	type result struct {
		v   any
		err error
	}
	const pending, running, abandoned = 0, 1, 2
	var state atomic.Int32
	done := make(chan result, 1)
	c.s.o.Submit(func() {
		if !state.CompareAndSwap(pending, running) {
			return // the client gave up: do not edit behind its back
		}
		if auth && !c.s.o.Remote.SessionActive(c.session.Load()) {
			done <- result{nil, notAuthorized()}
			return
		}
		v, err := fn()
		done <- result{v, err}
	})
	timer := time.NewTimer(c.s.o.CallTimeout)
	defer timer.Stop()
	select {
	case r := <-done:
		return r.v, r.err
	case <-timer.C:
		if state.CompareAndSwap(pending, abandoned) {
			return nil, &Error{Code: ErrApplication, Message: "the editor did not respond in time",
				Data: &ErrorData{Code: ports.CodeBusy}}
		}
		r := <-done // already running: it ends soon
		return r.v, r.err
	}
}

func notAuthorized() error {
	return &Error{Code: ErrApplication, Message: "not attached: call attach and let the user confirm it",
		Data: &ErrorData{Code: ports.CodeNotAuthorized}}
}

// attach asks the user, in the editor, to let this client in.
func (c *conn) attach() (any, error) {
	rem := c.s.o.Remote
	type result struct {
		id  uint64
		err error
	}
	reply := make(chan result, 1)
	client := c.client
	if client == "" {
		client = "A client"
	}
	if id := c.session.Load(); id != 0 {
		ok, _ := c.loop(false, func() (any, error) { return rem.SessionActive(id), nil })
		if ok == true {
			return nil, nil
		}
	}
	c.s.o.Submit(func() {
		rem.Attach(client, func(id uint64, err error) {
			c.attaching.Store(false)
			if err == nil {
				c.session.Store(id)
				if c.dead.Load() { // the client left while the user was deciding
					rem.Detach()
				}
			}
			reply <- result{id, err}
		})
	})
	c.attaching.Store(true)
	var r result
	select {
	case r = <-reply:
	case <-time.After(c.s.o.AttachTimeout):
		c.s.o.Submit(rem.CancelAttach)
		select {
		case r = <-reply:
		case <-time.After(c.s.o.CallTimeout):
			r.err = &Error{Code: ErrApplication, Message: "the user did not answer",
				Data: &ErrorData{Code: ports.CodeDenied}}
		}
	}
	return nil, r.err
}

// cleanName makes a client-supplied name safe to show in the editor.
func cleanName(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > maxClientName {
		s = string(r[:maxClientName]) + "…"
	}
	return s
}

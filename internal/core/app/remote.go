package app

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/avero-it/sie/internal/core/domain/editor"
	"github.com/avero-it/sie/internal/core/ports"
)

const (
	maxRemoteLines = 2000     // most lines Text returns, and the size above which a range is required
	maxRemoteSel   = 64 << 10 // bytes of a selection returned as text
	defaultFinds   = 100
	maxFinds       = 1000
)

// remoteState is the control API's side of the editor: who is attached and
// the attach question waiting for the user.
type remoteState struct {
	session uint64 // the attached session; 0 when none
	last    uint64 // last session ID handed out
	client  string
	always  bool // the user answered "always": later attaches need no question
	pending *attachRequest
	input   time.Time // last key, click or paste
}

type attachRequest struct {
	client   string
	reply    func(uint64, error)
	answered bool
	closed   bool // the prompt closed, whatever the answer
}

var _ ports.Remote = (*Editor)(nil)

func remoteErr(code, format string, args ...any) error {
	return &ports.RemoteError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Info implements ports.Remote.
func (e *Editor) Info() ports.RemoteInfo {
	return ports.RemoteInfo{
		Version:      e.version(),
		Path:         e.path,
		Language:     e.languageName(),
		Dirty:        e.doc.Modified(),
		LastActivity: e.remote.input,
		Attached:     e.remote.session != 0,
		Client:       e.remote.client,
	}
}

func (e *Editor) languageName() string {
	if e.hl.language == "" {
		return "Plain"
	}
	return e.hl.language
}

// Attach implements ports.Remote.
func (e *Editor) Attach(client string, reply func(uint64, error)) {
	r := &e.remote
	if client == "" {
		client = "A client"
	}
	switch {
	case r.session != 0:
		reply(0, remoteErr(ports.CodeBusy, "already attached to %s", r.client))
	case r.pending != nil:
		reply(0, remoteErr(ports.CodeBusy, "another client is waiting for confirmation"))
	case r.always:
		reply(e.grantSession(client), nil)
	case e.focus != focusEditor:
		reply(0, remoteErr(ports.CodeBusy, "the user is busy in a prompt or menu; try again"))
	default:
		req := &attachRequest{client: client, reply: reply}
		r.pending = req
		answer := func(ok, always bool) func() error {
			return func() error {
				req.answered = true
				switch {
				case !ok:
					e.finishAttach(0, remoteErr(ports.CodeDenied, "the user denied access"))
				default:
					r.always = always
					e.finishAttach(e.grantSession(client), nil)
				}
				return nil
			}
		}
		e.choose(client+" wants to control this buffer",
			choice{'y', "[y]es", answer(true, false)},
			choice{'n', "[n]o", answer(false, false)},
			choice{'a', "[a]lways", answer(true, true)})
		e.prompt.onClose = func() { req.closed = true }
	}
}

func (e *Editor) grantSession(client string) uint64 {
	r := &e.remote
	r.last++
	r.session, r.client = r.last, client
	e.info(client + " attached (Detach MCP: Ctrl+P D)")
	return r.session
}

func (e *Editor) finishAttach(session uint64, err error) {
	req := e.remote.pending
	if req == nil {
		return
	}
	e.remote.pending = nil
	req.reply(session, err)
}

// settleAttach answers a waiting attach whose question was dismissed
// without an answer (Esc, a click, another key).
func (e *Editor) settleAttach() {
	if req := e.remote.pending; req != nil && req.closed && !req.answered {
		e.finishAttach(0, remoteErr(ports.CodeDenied, "the user dismissed the question"))
	}
}

// CancelAttach implements ports.Remote.
func (e *Editor) CancelAttach() {
	req := e.remote.pending
	if req == nil {
		return
	}
	if e.focus == focusPrompt && e.prompt.mode == modeChoice {
		e.closePrompt()
	}
	e.finishAttach(0, remoteErr(ports.CodeDenied, "the user did not answer in time"))
}

// SessionActive implements ports.Remote.
func (e *Editor) SessionActive(session uint64) bool {
	return session != 0 && session == e.remote.session
}

// Detach implements ports.Remote.
func (e *Editor) Detach() {
	e.remote.session, e.remote.client = 0, ""
}

// cmdDetachMCP is the user's way to take the buffer back. It also forgets
// an "always" answer.
func (e *Editor) cmdDetachMCP([]string) error {
	e.remote.always = false
	if e.remote.session == 0 {
		e.note("no MCP session attached")
		return nil
	}
	client := e.remote.client
	e.Detach()
	e.note(client + " detached")
	return nil
}

// State implements ports.Remote.
func (e *Editor) State() ports.RemoteState {
	return ports.RemoteState{
		Path:      e.path,
		Language:  e.languageName(),
		Encoding:  "UTF-8",
		BOM:       e.doc.HasBOM(),
		LineEnd:   e.doc.LineEnding().String(),
		Dirty:     e.doc.Modified(),
		Lines:     e.doc.LineCount(),
		Bytes:     e.doc.Len(),
		Large:     e.file != nil,
		Revision:  e.doc.Version(),
		Selection: e.Selections(),
	}
}

// Text implements ports.Remote. The lines come back with LF endings,
// each followed by its line break except the last line of the document.
func (e *Editor) Text(startLine, endLine int) (ports.RemoteText, error) {
	n := e.doc.LineCount()
	if startLine == 0 && endLine == 0 {
		if e.file != nil || n > maxRemoteLines {
			return ports.RemoteText{}, remoteErr(ports.CodeTooLarge,
				"the buffer has %d lines; ask for a range of at most %d lines", n, maxRemoteLines)
		}
		startLine, endLine = 1, n
	}
	if endLine == 0 {
		endLine = startLine
	}
	switch {
	case startLine < 1 || startLine > n || endLine < startLine:
		return ports.RemoteText{}, remoteErr(ports.CodeOutOfRange, "lines %d-%d: the buffer has %d lines", startLine, endLine, n)
	case endLine-startLine+1 > maxRemoteLines:
		return ports.RemoteText{}, remoteErr(ports.CodeTooLarge, "at most %d lines per request", maxRemoteLines)
	}
	endLine = min(endLine, n)
	var b []byte
	for l := startLine; l <= endLine; l++ {
		b = append(b, e.doc.Line(l-1)...)
		if l < n {
			b = append(b, '\n')
		}
	}
	text, escaped := escapeInvalid(b)
	return ports.RemoteText{Text: text, Escaped: escaped, StartLine: startLine, EndLine: endLine,
		Lines: n, Revision: e.doc.Version()}, nil
}

// Selections implements ports.Remote.
func (e *Editor) Selections() []ports.RemoteSel {
	var out []ports.RemoteSel
	for i, s := range e.doc.Selections() {
		rs := ports.RemoteSel{Anchor: e.offPos(s.Anchor), Head: e.offPos(s.Head), Primary: i == 0}
		if !s.Empty() {
			start, end := s.Range()
			if end-start > maxRemoteSel {
				end, rs.Truncated = start+maxRemoteSel, true
			}
			data, _ := e.doc.Slice(start, end)
			rs.Text, rs.Escaped = escapeInvalid(data)
		}
		out = append(out, rs)
	}
	return out
}

// Goto implements ports.Remote.
func (e *Editor) Goto(p ports.RemotePos) error {
	off, err := e.posOffset(p)
	if err != nil {
		return err
	}
	e.doc.SetSelection(editor.Cursor(off))
	e.remoteTouched()
	return nil
}

// Select implements ports.Remote.
func (e *Editor) Select(r ports.RemoteRange) error {
	start, end, err := e.rangeOffsets(r)
	if err != nil {
		return err
	}
	e.doc.SetSelection(editor.Selection{Anchor: start, Head: end})
	e.remoteTouched()
	return nil
}

// Replace implements ports.Remote.
func (e *Editor) Replace(revision uint64, r ports.RemoteRange, text ports.RemoteTextIn) (uint64, error) {
	start, end, err := e.rangeOffsets(r)
	if err != nil {
		return 0, err
	}
	return e.replaceOffsets(revision, start, end, text)
}

// ReplaceSelection implements ports.Remote.
func (e *Editor) ReplaceSelection(revision uint64, text ports.RemoteTextIn) (uint64, error) {
	start, end := e.doc.Selection().Range()
	return e.replaceOffsets(revision, start, end, text)
}

func (e *Editor) replaceOffsets(revision uint64, start, end int, in ports.RemoteTextIn) (uint64, error) {
	if revision != e.doc.Version() {
		return 0, remoteErr(ports.CodeStaleRevision, "the buffer is at revision %d, not %d; read it again", e.doc.Version(), revision)
	}
	text, err := unescapeText(in)
	if err != nil {
		return 0, err
	}
	e.doc.BeginUndoGroup() // one API call is one undo step
	defer e.doc.EndUndoGroup()
	if _, err := e.doc.ReplaceRange(start, end, text); err != nil {
		return 0, remoteErr(ports.CodeFailed, "%v", err)
	}
	e.info(e.remote.client + " edited the buffer (Ctrl+Z undoes it)")
	e.remoteTouched()
	return e.doc.Version(), nil
}

// Find implements ports.Remote.
func (e *Editor) Find(q ports.RemoteFind) (ports.RemoteMatches, error) {
	if q.Pattern == "" {
		return ports.RemoteMatches{}, remoteErr(ports.CodeInvalid, "pattern is empty")
	}
	limit := q.Limit
	if limit <= 0 {
		limit = defaultFinds
	}
	limit = min(limit, maxFinds)
	opts := editor.SearchOptions{CaseSensitive: q.CaseSensitive}
	out := ports.RemoteMatches{Matches: []ports.RemoteRange{}, Revision: e.doc.Version()}
	for from := 0; ; {
		at, ok := e.doc.FindNext(q.Pattern, from, opts)
		if !ok {
			break
		}
		if len(out.Matches) == limit {
			out.Truncated = true
			break
		}
		end := at + len(q.Pattern)
		out.Matches = append(out.Matches, ports.RemoteRange{Start: e.offPos(at), End: e.offPos(end)})
		from = end
	}
	return out, nil
}

// Validate implements ports.Remote. Unlike Ctrl+T it leaves the cursor alone.
func (e *Editor) Validate() (ports.RemoteValidation, error) {
	lang, p, err := e.runValidate()
	if err != nil {
		return ports.RemoteValidation{}, remoteErr(ports.CodeFailed, "%v", err)
	}
	if p == nil {
		return ports.RemoteValidation{Valid: true, Language: lang}, nil
	}
	return ports.RemoteValidation{Language: lang, Line: p.Line, Col: p.Col, Message: p.Message}, nil
}

// remoteDenied lists what a client must not run through Run: anything that
// touches files or the process (saving is the user's decision), the
// editor's own UI, and interactive sessions.
func remoteDenied(id string) bool {
	return strings.HasPrefix(id, "file.") || strings.HasPrefix(id, "ui.") ||
		strings.HasPrefix(id, "mcp.") || id == "search.replace"
}

// Run implements ports.Remote.
func (e *Editor) Run(line string) (string, error) {
	c, args, ok := e.reg.ParseInline(strings.TrimSpace(line))
	if !ok {
		return "", remoteErr(ports.CodeInvalid, "unknown command %q", line)
	}
	if remoteDenied(c.ID) {
		return "", remoteErr(ports.CodeInvalid, "%q is not available to MCP clients", c.Title)
	}
	if len(args) < len(c.Args) {
		var prompts []string
		for _, a := range c.Args[len(args):] {
			prompts = append(prompts, a.Prompt)
		}
		return "", remoteErr(ports.CodeInvalid, "%q needs more arguments: %s", c.Title, strings.Join(prompts, ", "))
	}
	err := e.Dispatch(c.ID, args...)
	if e.focus != focusEditor { // it wanted to ask something
		if e.focus == focusPrompt {
			e.closePrompt()
		}
		e.focus = focusEditor
		e.clearMessage()
		return "", remoteErr(ports.CodeFailed, "%q needs an interactive answer and was not run", c.Title)
	}
	e.remoteTouched()
	if err != nil {
		return "", remoteErr(ports.CodeFailed, "%v", err)
	}
	return strings.TrimPrefix(e.msg.text, "> "), nil
}

// Commands implements ports.Remote.
func (e *Editor) Commands() []ports.RemoteCommand {
	var out []ports.RemoteCommand
	for _, c := range e.reg.All() {
		if c.Silent || remoteDenied(c.ID) {
			continue
		}
		rc := ports.RemoteCommand{ID: c.ID, Title: c.Title, Aliases: c.Aliases, Shortcut: e.shortcut(c.ID)}
		for _, a := range c.Args {
			rc.Args = append(rc.Args, a.Prompt)
		}
		out = append(out, rc)
	}
	return out
}

// remoteTouched runs after an API call changed something the user sees.
func (e *Editor) remoteTouched() {
	e.follow()
	e.afterInput()
}

// offPos converts a document offset to a 1-based position.
func (e *Editor) offPos(off int) ports.RemotePos {
	line, col := e.doc.GraphemePos(off)
	return ports.RemotePos{Line: line + 1, Col: col + 1}
}

// posOffset converts a position to a document offset. The position
// (lines+1, 1) is the end of the document, so a range can end after the
// last line.
func (e *Editor) posOffset(p ports.RemotePos) (int, error) {
	n := e.doc.LineCount()
	if p.Line == n+1 && p.Col == 1 && n > 0 {
		return e.doc.Len(), nil
	}
	off, ok := e.doc.OffsetAtGrapheme(p.Line-1, p.Col-1)
	if !ok {
		return 0, remoteErr(ports.CodeOutOfRange, "line %d, col %d does not exist", p.Line, p.Col)
	}
	return off, nil
}

func (e *Editor) rangeOffsets(r ports.RemoteRange) (start, end int, err error) {
	if start, err = e.posOffset(r.Start); err != nil {
		return 0, 0, err
	}
	if end, err = e.posOffset(r.End); err != nil {
		return 0, 0, err
	}
	if end < start {
		return 0, 0, remoteErr(ports.CodeOutOfRange, "the range ends before it starts")
	}
	return start, end, nil
}

// escapeInvalid returns b as text with LF line endings. Valid UTF-8 is
// returned as is. Otherwise the bytes are escaped (\xNN for an invalid
// byte, \\ for a backslash) so that unescapeText restores them exactly.
func escapeInvalid(b []byte) (string, bool) {
	b = []byte(strings.ReplaceAll(string(b), "\r\n", "\n"))
	if utf8.Valid(b) {
		return string(b), false
	}
	var sb strings.Builder
	for len(b) > 0 {
		r, size := utf8.DecodeRune(b)
		switch {
		case r == utf8.RuneError && size == 1:
			fmt.Fprintf(&sb, `\x%02x`, b[0])
		case r == '\\':
			sb.WriteString(`\\`)
		default:
			sb.Write(b[:size])
		}
		b = b[size:]
	}
	return sb.String(), true
}

// unescapeText undoes escapeInvalid when in.Escaped is set; otherwise the
// text is taken literally, and must be valid UTF-8.
func unescapeText(in ports.RemoteTextIn) (string, error) {
	if !in.Escaped {
		if !utf8.ValidString(in.Text) {
			return "", remoteErr(ports.CodeInvalid, "text is not valid UTF-8 (use the escaped form)")
		}
		return in.Text, nil
	}
	var sb strings.Builder
	s := in.Text
	for i := 0; i < len(s); {
		if s[i] != '\\' {
			sb.WriteByte(s[i])
			i++
			continue
		}
		switch {
		case strings.HasPrefix(s[i:], `\\`):
			sb.WriteByte('\\')
			i += 2
		case strings.HasPrefix(s[i:], `\x`) && i+4 <= len(s) && isHex(s[i+2]) && isHex(s[i+3]):
			sb.WriteByte(hexVal(s[i+2])<<4 | hexVal(s[i+3]))
			i += 4
		default:
			return "", errors.New("invalid escape in text: only \\\\ and \\xNN are allowed")
		}
	}
	return sb.String(), nil
}

func isHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

func hexVal(c byte) byte {
	switch {
	case c >= 'a':
		return c - 'a' + 10
	case c >= 'A':
		return c - 'A' + 10
	}
	return c - '0'
}

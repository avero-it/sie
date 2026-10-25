package app

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/avero-it/sie/internal/core/ports"
)

// attached opens a file and attaches a client, answering the question with
// key.
func attached(t *testing.T, content string) (*harness, uint64) {
	t.Helper()
	h := newHarness(t, map[string]string{"a.txt": content})
	if err := h.Dispatch("file.open", "a.txt"); err != nil {
		t.Fatal(err)
	}
	return h, attach(t, h, 'y')
}

func attach(t *testing.T, h *harness, answer rune) uint64 {
	t.Helper()
	h.Resize(100, 10)
	var session uint64
	var gotErr error
	called := false
	h.Attach("Claude Code", func(s uint64, err error) { session, gotErr, called = s, err, true })
	if called {
		t.Fatalf("attach answered before the user did: %v", gotErr)
	}
	h.wantRowContains(-2, "Claude Code wants to control this buffer")
	h.HandleKey(ports.KeyEvent{Key: ports.KeyRune, Rune: answer})
	if !called {
		t.Fatal("attach not answered")
	}
	if gotErr != nil {
		t.Fatalf("attach: %v", gotErr)
	}
	return session
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	var re *ports.RemoteError
	if !errors.As(err, &re) || re.Code != code {
		t.Fatalf("error = %v; want code %s", err, code)
	}
}

func TestAttachAnswers(t *testing.T) {
	tests := []struct {
		name string
		key  ports.KeyEvent
		code string
	}{
		{"no", ports.KeyEvent{Key: ports.KeyRune, Rune: 'n'}, ports.CodeDenied},
		{"esc", ports.KeyEvent{Key: ports.KeyEsc}, ports.CodeDenied},
		{"other key", ports.KeyEvent{Key: ports.KeyRune, Rune: 'x'}, ports.CodeDenied},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, nil)
			var err error
			called := false
			h.Attach("c", func(_ uint64, e error) { err, called = e, true })
			h.HandleKey(tt.key)
			if !called {
				t.Fatal("not answered")
			}
			wantCode(t, err, tt.code)
			if h.Info().Attached {
				t.Error("attached after refusal")
			}
		})
	}
}

func TestAttachCancelAndBusy(t *testing.T) {
	h := newHarness(t, nil)
	var err error
	h.Attach("c", func(_ uint64, e error) { err = e })
	var busy error
	h.Attach("d", func(_ uint64, e error) { busy = e })
	wantCode(t, busy, ports.CodeBusy)
	h.CancelAttach()
	wantCode(t, err, ports.CodeDenied)
	if h.focus != focusEditor {
		t.Error("question still open after cancel")
	}
}

func TestAttachWhileUserInPrompt(t *testing.T) {
	h := newHarness(t, nil)
	h.keys(ports.KeyEvent{Key: ports.KeyRune, Rune: 'p', Mod: ports.ModCtrl | ports.ModAlt})
	var err error
	h.Attach("c", func(_ uint64, e error) { err = e })
	wantCode(t, err, ports.CodeBusy)
}

func TestAttachAlwaysAndDetach(t *testing.T) {
	h := newHarness(t, nil)
	s1 := attach(t, h, 'a')
	if !h.SessionActive(s1) || !strings.Contains(h.row(-1), "● MCP") {
		t.Fatalf("not attached: %q", h.row(-1))
	}
	if err := h.Dispatch("mcp.detach"); err != nil {
		t.Fatal(err)
	}
	if h.SessionActive(s1) || strings.Contains(h.row(-1), "● MCP") {
		t.Fatal("still attached after detach")
	}
	// The user revoked it: the question comes back.
	called := false
	h.Attach("c", func(uint64, error) { called = true })
	if called {
		t.Fatal("always survived an explicit detach")
	}
}

func TestAttachAlwaysSkipsQuestion(t *testing.T) {
	h := newHarness(t, nil)
	attach(t, h, 'a')
	h.Detach() // the client left; the user did not revoke
	var s uint64
	h.Attach("c", func(id uint64, err error) { s = id })
	if s == 0 || !h.SessionActive(s) {
		t.Fatal("always did not skip the question")
	}
}

func TestTextAndState(t *testing.T) {
	h, _ := attached(t, "one\r\ntwo\r\nthree")
	st := h.State()
	if st.Lines != 3 || st.LineEnd != "CRLF" || st.Path != "a.txt" || st.Dirty {
		t.Errorf("state = %+v", st)
	}
	got, err := h.Text(2, 3)
	if err != nil || got.Text != "two\nthree" {
		t.Fatalf("Text(2,3) = %q, %v", got.Text, err)
	}
	got, _ = h.Text(0, 0)
	if got.Text != "one\ntwo\nthree" {
		t.Errorf("Text() = %q", got.Text)
	}
	_, err = h.Text(4, 4)
	wantCode(t, err, ports.CodeOutOfRange)
}

func TestTextTooLarge(t *testing.T) {
	h, _ := attached(t, strings.Repeat("x\n", maxRemoteLines+5))
	_, err := h.Text(0, 0)
	wantCode(t, err, ports.CodeTooLarge)
	_, err = h.Text(1, maxRemoteLines+1)
	wantCode(t, err, ports.CodeTooLarge)
	if got, err := h.Text(1, maxRemoteLines); err != nil || got.EndLine != maxRemoteLines {
		t.Errorf("max range: %v, %v", got.EndLine, err)
	}
}

func TestInvalidUTF8RoundTrip(t *testing.T) {
	h, _ := attached(t, "a\\b\xffc\n")
	got, err := h.Text(1, 1)
	if err != nil || !got.Escaped || got.Text != `a\\b\xffc`+"\n" {
		t.Fatalf("Text = %q escaped=%v err=%v", got.Text, got.Escaped, err)
	}
	// Writing the same text back changes nothing.
	r := ports.RemoteRange{Start: ports.RemotePos{Line: 1, Col: 1}, End: ports.RemotePos{Line: 2, Col: 1}}
	if _, err := h.Replace(h.State().Revision, r, ports.RemoteTextIn{Text: got.Text, Escaped: true}); err != nil {
		t.Fatal(err)
	}
	if h.doc.Text() != "a\\b\xffc\n" {
		t.Errorf("document = %q", h.doc.Text())
	}
	_, err = h.Replace(h.State().Revision, r, ports.RemoteTextIn{Text: "\xff"})
	wantCode(t, err, ports.CodeInvalid)
	_, err = h.Replace(h.State().Revision, r, ports.RemoteTextIn{Text: `\q`, Escaped: true})
	if err == nil {
		t.Error("bad escape accepted")
	}
}

func TestReplaceRevisionAndUndo(t *testing.T) {
	h, _ := attached(t, "héllo wörld\nsecond")
	rev := h.State().Revision
	r := ports.RemoteRange{Start: ports.RemotePos{Line: 1, Col: 7}, End: ports.RemotePos{Line: 1, Col: 12}}
	newRev, err := h.Replace(rev, r, ports.RemoteTextIn{Text: "there"})
	if err != nil {
		t.Fatal(err)
	}
	if newRev == rev || h.doc.Text() != "héllo there\nsecond" || !h.State().Dirty {
		t.Fatalf("after replace: %q rev %d->%d", h.doc.Text(), rev, newRev)
	}
	// A stale revision changes nothing.
	_, err = h.Replace(rev, r, ports.RemoteTextIn{Text: "X"})
	wantCode(t, err, ports.CodeStaleRevision)
	if h.doc.Text() != "héllo there\nsecond" {
		t.Errorf("stale edit applied: %q", h.doc.Text())
	}
	// A second API edit, then one Ctrl+Z undoes only the last one.
	if _, err := h.Replace(newRev, ports.RemoteRange{Start: ports.RemotePos{Line: 2, Col: 1}, End: ports.RemotePos{Line: 2, Col: 7}}, ports.RemoteTextIn{Text: "2nd"}); err != nil {
		t.Fatal(err)
	}
	h.HandleKey(ports.Ctrl('z'))
	if h.doc.Text() != "héllo there\nsecond" {
		t.Errorf("one undo = %q", h.doc.Text())
	}
	// The user typing bumps the revision.
	h.typeText("!")
	_, err = h.Replace(newRev, r, ports.RemoteTextIn{Text: "X"})
	wantCode(t, err, ports.CodeStaleRevision)
}

func TestInsertAtEndAndMultiLineEdit(t *testing.T) {
	h, _ := attached(t, "a\nb")
	end := ports.RemotePos{Line: 3, Col: 1} // past the last line
	if _, err := h.Replace(h.State().Revision, ports.RemoteRange{Start: end, End: end}, ports.RemoteTextIn{Text: "\nc"}); err != nil {
		t.Fatal(err)
	}
	if h.doc.Text() != "a\nb\nc" {
		t.Fatalf("text = %q", h.doc.Text())
	}
	// One call touching two lines is one undo step.
	r := ports.RemoteRange{Start: ports.RemotePos{Line: 1, Col: 1}, End: ports.RemotePos{Line: 3, Col: 1}}
	if _, err := h.Replace(h.State().Revision, r, ports.RemoteTextIn{Text: "x\ny\n"}); err != nil {
		t.Fatal(err)
	}
	h.HandleKey(ports.Ctrl('z'))
	if h.doc.Text() != "a\nb\nc" {
		t.Errorf("undo = %q", h.doc.Text())
	}
}

func TestOutOfRange(t *testing.T) {
	h, _ := attached(t, "ab")
	rev := h.State().Revision
	for _, p := range []ports.RemotePos{{Line: 0, Col: 1}, {Line: 1, Col: 0}, {Line: 1, Col: 4}, {Line: 2, Col: 2}} {
		_, err := h.Replace(rev, ports.RemoteRange{Start: p, End: p}, ports.RemoteTextIn{Text: "x"})
		wantCode(t, err, ports.CodeOutOfRange)
	}
	wantCode(t, h.Goto(ports.RemotePos{Line: 9, Col: 1}), ports.CodeOutOfRange)
	back := ports.RemoteRange{Start: ports.RemotePos{Line: 1, Col: 2}, End: ports.RemotePos{Line: 1, Col: 1}}
	wantCode(t, h.Select(back), ports.CodeOutOfRange)
}

func TestSelectionAndReplaceSelection(t *testing.T) {
	h, _ := attached(t, "alpha beta\ngamma")
	err := h.Select(ports.RemoteRange{Start: ports.RemotePos{Line: 1, Col: 7}, End: ports.RemotePos{Line: 1, Col: 11}})
	if err != nil {
		t.Fatal(err)
	}
	sel := h.Selections()
	if len(sel) != 1 || sel[0].Text != "beta" || !sel[0].Primary || sel[0].Head != (ports.RemotePos{Line: 1, Col: 11}) {
		t.Fatalf("selections = %+v", sel)
	}
	if _, err := h.ReplaceSelection(h.State().Revision, ports.RemoteTextIn{Text: "BETA"}); err != nil {
		t.Fatal(err)
	}
	if h.doc.Text() != "alpha BETA\ngamma" {
		t.Errorf("text = %q", h.doc.Text())
	}
	if err := h.Goto(ports.RemotePos{Line: 2, Col: 3}); err != nil {
		t.Fatal(err)
	}
	if sel := h.Selections(); sel[0].Head != (ports.RemotePos{Line: 2, Col: 3}) || sel[0].Text != "" {
		t.Errorf("after goto: %+v", sel)
	}
}

func TestFind(t *testing.T) {
	h, _ := attached(t, "Foo foo\nxfoo")
	got, err := h.Find(ports.RemoteFind{Pattern: "foo"})
	if err != nil || len(got.Matches) != 3 {
		t.Fatalf("matches = %+v, %v", got, err)
	}
	if m := got.Matches[2]; m.Start != (ports.RemotePos{Line: 2, Col: 2}) || m.End != (ports.RemotePos{Line: 2, Col: 5}) {
		t.Errorf("third match = %+v", m)
	}
	got, _ = h.Find(ports.RemoteFind{Pattern: "foo", CaseSensitive: true})
	if len(got.Matches) != 2 {
		t.Errorf("case sensitive = %d", len(got.Matches))
	}
	got, _ = h.Find(ports.RemoteFind{Pattern: "foo", Limit: 1})
	if len(got.Matches) != 1 || !got.Truncated {
		t.Errorf("limit: %+v", got)
	}
	_, err = h.Find(ports.RemoteFind{})
	wantCode(t, err, ports.CodeInvalid)
}

func TestRun(t *testing.T) {
	h, _ := attached(t, "one\ntwo\nthree")
	if _, err := h.Run("goto 2"); err != nil {
		t.Fatal(err)
	}
	if got := h.Selections()[0].Head.Line; got != 2 {
		t.Errorf("line = %d", got)
	}
	if _, err := h.Run("replace all two 2"); err != nil || h.doc.Text() != "one\n2\nthree" {
		t.Errorf("replace all: %v %q", err, h.doc.Text())
	}
	for _, line := range []string{"save", "quit", "open b.txt", "replace", "prompt", "mcp detach", "nonsense"} {
		_, err := h.Run(line)
		wantCode(t, err, ports.CodeInvalid)
	}
	_, err := h.Run("goto")
	wantCode(t, err, ports.CodeInvalid)
	if h.focus != focusEditor {
		t.Error("a prompt is left open")
	}
}

func TestCommandsHideForbidden(t *testing.T) {
	h := newHarness(t, nil)
	seen := map[string]bool{}
	for _, c := range h.Commands() {
		seen[c.ID] = true
	}
	for _, id := range []string{"file.save", "file.quit", "ui.prompt", "mcp.detach", "search.replace"} {
		if seen[id] {
			t.Errorf("%s is listed", id)
		}
	}
	if !seen["search.gotoLine"] || !seen["search.replaceAll"] {
		t.Error("allowed commands missing")
	}
}

func TestInfoLastActivity(t *testing.T) {
	h := newHarness(t, nil)
	var now = h.Info().LastActivity
	h.cfg.Now = func() time.Time { return now.Add(5e9) }
	h.HandleKey(ports.KeyEvent{Key: ports.KeyRune, Rune: 'x'})
	if !h.Info().LastActivity.After(now) {
		t.Error("last activity not updated by a key")
	}
}

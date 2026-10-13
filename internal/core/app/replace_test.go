package app

import (
	"strings"
	"testing"

	"github.com/avero-it/sie/internal/core/domain/editor"
	"github.com/avero-it/sie/internal/core/ports"
)

func rk(r rune) ports.KeyEvent { return ports.KeyEvent{Key: ports.KeyRune, Rune: r} }

// startReplace types text, puts the cursor at off and runs Replace.
func startReplace(t *testing.T, text string, off int, query, repl string) *harness {
	t.Helper()
	h := newHarness(t, nil)
	h.HandlePaste(text)
	h.doc.SetSelection(editor.Cursor(off))
	if err := h.Dispatch("search.replace", query, repl); err != nil {
		t.Fatal(err)
	}
	return h
}

func TestReplaceAsksPerMatch(t *testing.T) {
	h := startReplace(t, "a1 a2 a3", 0, "a", "b")
	h.wantRow(rowPrompt, `Replace "a" with "b"? (y/n/a/q)`)
	if h.doc.SelectedText() != "a" || h.doc.Selection().Head != 1 {
		t.Fatalf("first match not selected: %+v", h.doc.Selection())
	}
	h.keys(rk('y'), rk('n'), rk('y'))
	if got := h.doc.Text(); got != "b1 a2 b3" {
		t.Fatalf("text %q", got)
	}
	if h.focus != focusEditor {
		t.Error("session did not end after the last match")
	}
	h.wantRowPrefix(rowPrompt, "> Replace a b — 2 matches")
	h.keys(ports.Ctrl('z'))
	if got := h.doc.Text(); got != "a1 a2 a3" {
		t.Errorf("one undo should revert the session: %q", got)
	}
}

func TestReplaceWrapsAndStopsAtStart(t *testing.T) {
	// Start in the middle; replacement is longer, so offsets shift.
	h := startReplace(t, "x x x x", 4, "x", "yyy")
	h.keys(rk('a'))
	if got := h.doc.Text(); got != "yyy yyy yyy yyy" {
		t.Fatalf("text %q", got)
	}
	h.wantRowPrefix(rowPrompt, "> Replace x yyy — 4 matches")

	// With "n" for everything, each match is visited exactly once.
	h = startReplace(t, "x x x", 2, "x", "y")
	visits := 0
	for h.focus == focusPrompt && visits < 10 {
		visits++
		h.keys(rk('n'))
	}
	if visits != 3 {
		t.Errorf("visited %d matches, want 3", visits)
	}
}

func TestReplaceQuitAndAbort(t *testing.T) {
	h := startReplace(t, "a a a", 0, "a", "b")
	h.keys(rk('y'), rk('q'))
	if got := h.doc.Text(); got != "b a a" || h.focus != focusEditor {
		t.Fatalf("text %q focus %v", got, h.focus)
	}
	h.wantRowPrefix(rowPrompt, "> Replace a b — 1 match")

	// Leaving through Esc or a click must also close the undo group.
	for _, leave := range []func(h *harness){
		func(h *harness) { h.keys(key(ports.KeyEsc)) },
		func(h *harness) { h.click(10, rowText, 0) },
	} {
		h = startReplace(t, "a a", 0, "a", "b")
		h.keys(rk('y'))
		leave(h)
		h.typeText("Z") // must be a separate undo step
		h.keys(ports.Ctrl('z'))
		if got := h.doc.Text(); got != "b a" && got != "b a"[:3] {
			t.Errorf("after leaving and undoing typing: %q", got)
		}
		h.keys(ports.Ctrl('z'))
		if got := h.doc.Text(); got != "a a" {
			t.Errorf("session undo: %q", got)
		}
	}
}

func TestReplaceNoMatch(t *testing.T) {
	h := startReplace(t, "abc", 0, "zz", "y")
	if h.focus != focusEditor {
		t.Error("prompt opened without a match")
	}
	h.wantRowPrefix(rowPrompt, "> Replace zz y — 0 matches")
}

func TestReplaceFromPromptAndMenu(t *testing.T) {
	h := newHarness(t, nil)
	h.HandlePaste("foo bar foo")
	h.keys(ports.CtrlAlt('p'))
	h.typeText("find and replace")
	if it := h.prompt.items; len(it) == 0 || it[0].cmd.ID != "search.replace" {
		t.Fatalf("completion: %+v", it)
	}
	h.keys(key(ports.KeyEnter))
	h.wantRow(rowPrompt, "Replace:")

	var search Menu
	for _, m := range DefaultMenus() {
		if m.Title == "Search" {
			search = m
		}
	}
	if !strings.Contains(strings.Join(search.Items, ","), "search.replace,search.replaceAll") {
		t.Errorf("Search menu items: %v", search.Items)
	}
}

func TestNavigationKeys(t *testing.T) {
	alt := func(k ports.Key) ports.KeyEvent { return ports.KeyEvent{Key: k, Mod: ports.ModAlt} }
	shiftAlt := func(k ports.Key) ports.KeyEvent { return ports.KeyEvent{Key: k, Mod: ports.ModAlt | ports.ModShift} }
	cmd := func(k ports.Key) ports.KeyEvent { return ports.KeyEvent{Key: k, Mod: ports.ModSuper} }
	shiftCmd := func(k ports.Key) ports.KeyEvent { return ports.KeyEvent{Key: k, Mod: ports.ModSuper | ports.ModShift} }
	lines := strings.Repeat("line\n", 20) + "  foo bar baz"
	last := strings.LastIndex(lines, "\n") + 1

	tests := []struct {
		name     string
		at       int
		keys     []ports.KeyEvent
		head     int
		selected string
	}{
		{"Alt+Right word", last + 2, []ports.KeyEvent{alt(ports.KeyRight)}, last + 5, ""},
		{"Alt+Left word", last + 9, []ports.KeyEvent{alt(ports.KeyLeft)}, last + 6, ""},
		{"Shift+Alt+Right selects word", last + 2, []ports.KeyEvent{shiftAlt(ports.KeyRight)}, last + 5, "foo"},
		{"Alt+F (macOS Option+→)", last + 2, []ports.KeyEvent{{Key: ports.KeyRune, Rune: 'f', Mod: ports.ModAlt}}, last + 5, ""},
		{"Alt+Shift+B selects word left", last + 9, []ports.KeyEvent{{Key: ports.KeyRune, Rune: 'B', Mod: ports.ModAlt}}, last + 6, "bar"},
		{"Ctrl+Right still word", last + 2, []ports.KeyEvent{{Key: ports.KeyRight, Mod: ports.ModCtrl}}, last + 5, ""},
		{"Cmd+Left smart line start", last + 9, []ports.KeyEvent{cmd(ports.KeyLeft)}, last + 2, ""},
		{"Cmd+Right line end", last + 2, []ports.KeyEvent{cmd(ports.KeyRight)}, len(lines), ""},
		{"Shift+Cmd+Right selects to end", last + 6, []ports.KeyEvent{shiftCmd(ports.KeyRight)}, len(lines), "bar baz"},
		{"Cmd+Up document start", last + 4, []ports.KeyEvent{cmd(ports.KeyUp)}, 0, ""},
		{"Cmd+Down document end", 0, []ports.KeyEvent{cmd(ports.KeyDown)}, len(lines), ""},
		{"Alt+Down 5 lines", 0, []ports.KeyEvent{alt(ports.KeyDown)}, 5 * 5, ""},
		{"Alt+Up 5 lines", 10 * 5, []ports.KeyEvent{alt(ports.KeyUp)}, 5 * 5, ""},
		{"Shift+Alt+Down selects 5 lines", 0, []ports.KeyEvent{shiftAlt(ports.KeyDown)}, 5 * 5, strings.Repeat("line\n", 5)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, nil)
			h.HandlePaste(lines)
			h.doc.SetSelection(editor.Cursor(tt.at))
			h.keys(tt.keys...)
			if got := h.doc.Selection().Head; got != tt.head {
				t.Errorf("head %d, want %d", got, tt.head)
			}
			if got := h.doc.SelectedText(); got != tt.selected {
				t.Errorf("selected %q, want %q", got, tt.selected)
			}
		})
	}
}

func TestJumpLinesFromConfigAndCmdNeverTypes(t *testing.T) {
	e, err := New(Config{Files: &memFS{}, Keymap: keys, JumpLines: 2})
	if err != nil {
		t.Fatal(err)
	}
	e.Resize(40, 10)
	e.HandlePaste("a\nb\nc\nd")
	e.doc.SetSelection(editor.Cursor(0))
	e.HandleKey(ports.KeyEvent{Key: ports.KeyDown, Mod: ports.ModAlt})
	if line, _, _ := e.doc.CursorPos(); line != 2 {
		t.Errorf("line %d, want 2", line)
	}
	e.HandleKey(ports.KeyEvent{Key: ports.KeyRune, Rune: 's', Mod: ports.ModSuper})
	if e.doc.Text() != "a\nb\nc\nd" {
		t.Errorf("Cmd+S typed text: %q", e.doc.Text())
	}
}

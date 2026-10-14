package editor

import (
	"slices"
	"strings"
	"testing"

	"github.com/avero-it/sie/internal/core/domain/buffer"
)

// cursorsDoc builds a document from s, where every '|' is a cursor; the
// first one is the primary cursor.
func cursorsDoc(t *testing.T, s string, eol buffer.LineEnding) *Document {
	t.Helper()
	parts := strings.Split(s, "|")
	if len(parts) < 2 {
		t.Fatalf("cursorsDoc %q: no cursor", s)
	}
	text := strings.Join(parts, "")
	d := NewDocument([]byte(text), Options{TabWidth: 4, DefaultEOL: eol})
	var offs []int
	off := 0
	for _, p := range parts[:len(parts)-1] {
		off += len(p)
		offs = append(offs, off)
	}
	d.SetSelection(Cursor(offs[len(offs)-1]))
	for i := len(offs) - 2; i >= 0; i-- {
		d.AddCursor(offs[i])
	}
	if d.Selection().Head != offs[0] || d.CursorCount() != len(offs) {
		t.Fatalf("cursorsDoc %q: got %v", s, d.Selections())
	}
	return d
}

// showCursors renders the text with a '|' at every (collapsed) cursor.
func showCursors(t *testing.T, d *Document) string {
	t.Helper()
	var offs []int
	for _, s := range d.Selections() {
		if !s.Empty() {
			t.Fatalf("showCursors: selection %+v", s)
		}
		offs = append(offs, s.Head)
	}
	slices.Sort(offs)
	text := d.Text()
	var b strings.Builder
	prev := 0
	for _, o := range offs {
		b.WriteString(text[prev:o] + "|")
		prev = o
	}
	return b.String() + text[prev:]
}

func TestMultiCursorEdits(t *testing.T) {
	tests := []struct {
		name, in, want string
		eol            buffer.LineEnding
		op             func(d *Document) error
	}{
		{"type same line", "a|b|c", "ax|bx|c", buffer.LF, func(d *Document) error { return d.Insert("x") }},
		{"type lines", "|ab\n|cd\n|ef", "x|ab\nx|cd\nx|ef", buffer.LF, func(d *Document) error { return d.Insert("x") }},
		{"paste", "|a\n|b", "xy|a\nxy|b", buffer.LF, func(d *Document) error { return d.Insert("xy") }},
		{"backspace", "ab|cd|e", "a|c|e", buffer.LF, (*Document).Backspace},
		{"backspace joins", "a\n|b\n|c", "a|b|c", buffer.LF, (*Document).Backspace},
		{"backspace merges", "a|b|c", "|c", buffer.LF, (*Document).Backspace},
		{"delete", "|ab|cd", "|b|d", buffer.LF, (*Document).DeleteForward},
		{"delete merges", "|a|b", "|", buffer.LF, (*Document).DeleteForward},
		{"newline indent", "  a|b\n\tc|d", "  a\n  |b\n\tc\n\t|d", buffer.LF, (*Document).InsertNewline},
		{"newline crlf", "a|b\r\nc|d", "a\r\n|b\r\nc\r\n|d", buffer.CRLF, (*Document).InsertNewline},
		{"backspace crlf", "a\r\n|b\r\n|c", "a|b|c", buffer.CRLF, (*Document).Backspace},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := cursorsDoc(t, tt.in, tt.eol)
			if err := tt.op(d); err != nil {
				t.Fatal(err)
			}
			if got := showCursors(t, d); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMultiCursorUndo(t *testing.T) {
	d := cursorsDoc(t, "a|b\nc|d", buffer.LF)
	for _, s := range []string{"x", "y"} {
		if err := d.Insert(s); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.InsertNewline(); err != nil {
		t.Fatal(err)
	}
	if got, want := showCursors(t, d), "axy\n|b\ncxy\n|d"; got != want {
		t.Fatalf("typed: %q, want %q", got, want)
	}
	steps := []string{"axy|b\ncxy|d", "a|b\nc|d"} // newline, then the typing burst
	for _, want := range steps {
		if ok, err := d.Undo(); !ok || err != nil {
			t.Fatalf("undo: %v %v", ok, err)
		}
		if got := showCursors(t, d); got != want {
			t.Errorf("undo: %q, want %q", got, want)
		}
	}
	if d.CanUndo() {
		t.Error("more undo steps than expected")
	}
	for _, want := range []string{"axy|b\ncxy|d", "axy\n|b\ncxy\n|d"} {
		if ok, err := d.Redo(); !ok || err != nil {
			t.Fatalf("redo: %v %v", ok, err)
		}
		if got := showCursors(t, d); got != want {
			t.Errorf("redo: %q, want %q", got, want)
		}
	}
}

func TestMultiCursorMove(t *testing.T) {
	d := cursorsDoc(t, "|ab\n|cd\nef", buffer.LF)
	d.Move(MoveRight, false)
	if got, want := showCursors(t, d), "a|b\nc|d\nef"; got != want {
		t.Errorf("right: %q, want %q", got, want)
	}
	d.Move(MoveDown, false)
	if got, want := showCursors(t, d), "ab\nc|d\ne|f"; got != want {
		t.Errorf("down: %q, want %q", got, want)
	}
	d.MovePage(-5, false) // both reach the first line: merged
	if got, want := showCursors(t, d), "|ab\ncd\nef"; got != want {
		t.Errorf("page up: %q, want %q", got, want)
	}
}

func TestAddCursor(t *testing.T) {
	d := doc(t, "ab|cd")
	d.AddCursor(0)
	if got, want := showCursors(t, d), "|ab|cd"; got != want || d.Selection().Head != 0 {
		t.Fatalf("add: %q primary %d, want %q primary 0", got, d.Selection().Head, want)
	}
	d.AddCursor(2) // toggles the existing cursor off
	if got, want := showCursors(t, d), "|abcd"; got != want {
		t.Fatalf("toggle: %q, want %q", got, want)
	}
	d.AddCursor(0) // the only cursor stays
	if d.CursorCount() != 1 {
		t.Fatalf("count = %d, want 1", d.CursorCount())
	}
	d.AddCursor(4)
	d.SetSelection(Cursor(1))
	if d.CursorCount() != 1 {
		t.Errorf("SetSelection kept %d cursors", d.CursorCount())
	}
	d.AddCursor(4)
	if !d.ClearSecondary() || d.CursorCount() != 1 || d.Selection().Head != 4 {
		t.Errorf("ClearSecondary: %v", d.Selections())
	}
}

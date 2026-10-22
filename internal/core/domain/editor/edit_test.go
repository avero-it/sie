package editor

import (
	"bytes"
	"testing"

	"github.com/avero-it/sie/internal/core/domain/buffer"
)

func TestEdits(t *testing.T) {
	tests := []struct {
		name string
		in   string
		op   func(d *Document) error
		want string
	}{
		{"insert", "a|c", func(d *Document) error { return d.Insert("b") }, "ab|c"},
		{"insert before combining mark stays on boundary", "|́a", func(d *Document) error { return d.Insert("x") }, "x́|a"},
		{"insert replaces selection", "a^xx|d", func(d *Document) error { return d.Insert("bc") }, "abc|d"},
		{"newline auto-indents", "  ab|c", (*Document).InsertNewline, "  ab\n  |c"},
		{"newline in indent", " | x", (*Document).InsertNewline, " \n | x"},
		{"backspace", "ab|c", (*Document).Backspace, "a|c"},
		{"backspace at start", "|ab", (*Document).Backspace, "|ab"},
		{"backspace joins lines", "a\n|b", (*Document).Backspace, "a|b"},
		{"backspace removes CRLF whole", "a\r\n|b", (*Document).Backspace, "a|b"},
		{"backspace removes whole cluster", "é|x", (*Document).Backspace, "|x"},
		{"backspace deletes selection", "a^bc|d", (*Document).Backspace, "a|d"},
		{"delete forward", "a|bc", (*Document).DeleteForward, "a|c"},
		{"delete forward at end", "ab|", (*Document).DeleteForward, "ab|"},
		{"delete forward joins", "a|\nb", (*Document).DeleteForward, "a|b"},
		{"delete to line end", "ab|cd\nx", (*Document).DeleteToLineEnd, "ab|\nx"},
		{"delete to line end keeps CRLF", "a|b\r\nc", (*Document).DeleteToLineEnd, "a|\r\nc"},
		{"delete to line end at end joins", "ab|\nx", (*Document).DeleteToLineEnd, "ab|x"},
		{"delete to line end at CRLF end joins", "ab|\r\nx", (*Document).DeleteToLineEnd, "ab|x"},
		{"delete to line end at doc end is a no-op", "ab|", (*Document).DeleteToLineEnd, "ab|"},
		{"delete to line end ignores selection", "^a|bc", (*Document).DeleteToLineEnd, "a|"},
		{"delete line", "a\nb|b\nc", (*Document).DeleteLine, "a\n|c"},
		{"delete last line", "a\nb|b", (*Document).DeleteLine, "|a"},
		{"delete only line", "a|b", (*Document).DeleteLine, "|"},
		{"delete selected lines", "a\nb^b\nc|c\nd", (*Document).DeleteLine, "a\n|d"},
		{"delete lines ending at col 0", "^a\nb\n|c", (*Document).DeleteLine, "|c"},
		{"duplicate line", "a|b\nc", (*Document).DuplicateLine, "ab\na|b\nc"},
		{"duplicate last line", "x\na|b", (*Document).DuplicateLine, "x\nab\na|b"},
		{"duplicate block", "^a\nb|\nc", (*Document).DuplicateLine, "a\nb\n^a\nb|\nc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := doc(t, tt.in)
			before := show(d)
			if err := tt.op(d); err != nil {
				t.Fatal(err)
			}
			if got := show(d); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
			if _, err := d.Undo(); err != nil {
				t.Fatal(err)
			}
			if got := show(d); got != before {
				t.Errorf("undo: got %q, want %q", got, before)
			}
		})
	}
}

func TestInsertNormalizesLineEndings(t *testing.T) {
	d := NewDocument([]byte("a\r\nb"), Options{DefaultEOL: buffer.LF})
	d.SetSelection(Cursor(1))
	if err := d.Insert("x\ny\r\nz"); err != nil {
		t.Fatal(err)
	}
	if got, want := d.Text(), "ax\r\ny\r\nz\r\nb"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTypingCoalescesIntoOneUndo(t *testing.T) {
	d := doc(t, "|")
	for _, s := range []string{"h", "i", " ", "y", "o"} {
		if err := d.Insert(s); err != nil {
			t.Fatal(err)
		}
	}
	d.Move(MoveLeft, false)
	d.Move(MoveRight, false)
	_ = d.Insert("!")
	_, _ = d.Undo()
	if got := show(d); got != "hi yo|" {
		t.Fatalf("first undo: %q", got)
	}
	_, _ = d.Undo()
	if got := show(d); got != "|" {
		t.Fatalf("second undo: %q", got)
	}
	_, _ = d.Redo()
	if got := show(d); got != "hi yo|" {
		t.Fatalf("redo: %q", got)
	}
}

func TestPasteIsSeparateUndoStep(t *testing.T) {
	d := doc(t, "|")
	_ = d.Insert("a")
	_ = d.Insert("pasted")
	_, _ = d.Undo()
	if got := show(d); got != "a|" {
		t.Errorf("got %q, want %q", got, "a|")
	}
}

func TestTrimTrailingSpace(t *testing.T) {
	tests := []struct{ in, want string }{
		{"a  \nb\t\n|c ", "a\nb\n|c"},
		{"a  \n^b  \nc| \nd  ", "a  \n^b\nc|\nd  "},
		{"a \r\n|b", "a\r\n|b"},
	}
	for _, tt := range tests {
		d := doc(t, tt.in)
		if _, err := d.TrimTrailingSpace(); err != nil {
			t.Fatal(err)
		}
		if got := show(d); got != tt.want {
			t.Errorf("%q: got %q, want %q", tt.in, got, tt.want)
		}
		_, _ = d.Undo()
		if got := show(d); got != tt.in {
			t.Errorf("%q: one undo restores, got %q", tt.in, got)
		}
	}
}

func TestSelectLineExtends(t *testing.T) {
	d := doc(t, "a\nb|b\nc\nd")
	d.SelectLine()
	if got, want := show(d), "a\n^bb\n|c\nd"; got != want {
		t.Fatalf("first: got %q, want %q", got, want)
	}
	d.SelectLine()
	if got, want := show(d), "a\n^bb\nc\n|d"; got != want {
		t.Fatalf("second: got %q, want %q", got, want)
	}
	d.SelectLine()
	d.SelectLine()
	if got, want := show(d), "a\n^bb\nc\nd|"; got != want {
		t.Fatalf("at end: got %q, want %q", got, want)
	}
}

func TestSelectAllAndText(t *testing.T) {
	d := doc(t, "ab|c")
	d.SelectAll()
	if d.SelectedText() != "abc" {
		t.Errorf("SelectedText = %q", d.SelectedText())
	}
}

func TestModifiedAndBOMRoundTrip(t *testing.T) {
	content := append(buffer.UTF8BOM(), "x\r\ny"...)
	d := NewDocument(content, Options{})
	if !d.HasBOM() || d.LineEnding() != buffer.CRLF || d.Text() != "x\r\ny" {
		t.Fatalf("load: bom=%v eol=%v text=%q", d.HasBOM(), d.LineEnding(), d.Text())
	}
	if d.Modified() {
		t.Fatal("fresh document modified")
	}
	_ = d.Insert("!")
	if !d.Modified() {
		t.Fatal("not modified after insert")
	}
	_, _ = d.Undo()
	if d.Modified() || !bytes.Equal(d.Bytes(), content) {
		t.Fatalf("undo to original: modified=%v bytes=%q", d.Modified(), d.Bytes())
	}
}

func TestWordAndLineRange(t *testing.T) {
	d := NewDocument([]byte("foo  bar.baz\n\nend"), Options{})
	tests := []struct {
		off          int
		word         string
		wordLo, line int
	}{
		{1, "foo", 0, 0},
		{4, "  ", 3, 0},
		{6, "bar", 5, 0},
		{8, ".", 8, 0},
		{12, "baz", 9, 0}, // end of line: previous run
		{13, "", 13, 13},  // empty line
		{16, "end", 14, 14},
	}
	for _, tt := range tests {
		lo, hi := d.WordRange(tt.off)
		if got := d.Text()[lo:hi]; got != tt.word || lo != tt.wordLo && tt.word != "" {
			t.Errorf("WordRange(%d) = %q [%d,%d)", tt.off, got, lo, hi)
		}
		if lo, _ := d.LineRange(tt.off); lo != tt.line {
			t.Errorf("LineRange(%d) start = %d, want %d", tt.off, lo, tt.line)
		}
	}
}

func TestIndentCol(t *testing.T) {
	tests := []struct {
		line string
		col  int
		ok   bool
	}{
		{"", 0, false},
		{"    ", 0, false},
		{"foo", 0, true},
		{"    foo", 4, true},
		{"\tfoo", 4, true},
		{"  \tfoo", 4, true},
		{" 世", 1, true},
		{"  \x01", 2, true},
	}
	for _, tt := range tests {
		col, ok := IndentCol([]byte(tt.line), 4)
		if col != tt.col || ok != tt.ok {
			t.Errorf("IndentCol(%q) = %d,%v want %d,%v", tt.line, col, ok, tt.col, tt.ok)
		}
	}
}

func TestTrimLeadingSpace(t *testing.T) {
	tests := []struct{ in, want string }{
		{"  a\n\t b\n|c", "a\nb\n|c"},
		{"  a\n^ b\n |c", "  a\n^b\n|c"},
		{"  \n|x", "  \n|x"},
	}
	for _, tt := range tests {
		d := doc(t, tt.in)
		if _, err := d.TrimLeadingSpace(); err != nil {
			t.Fatal(err)
		}
		if got := show(d); got != tt.want {
			t.Errorf("%q: got %q, want %q", tt.in, got, tt.want)
		}
		_, _ = d.Undo()
		if got := show(d); got != tt.in {
			t.Errorf("%q: undo gave %q", tt.in, got)
		}
	}
}

func TestAlignToNeighbor(t *testing.T) {
	tests := []struct {
		in, want string
		next     bool
		wantErr  bool
	}{
		{"\t a\n |b", "\t a\n\t |b", false, false},
		{"  \n    a\n|b", "  \n    a\n    |b", false, false},
		{"|a\n\n  b", "  |a\n\n  b", true, false},
		{"|a\nb", "", false, true},
		{"a\n|b", "", true, true},
		{"x\n^  a\n  b|\n    c", "x\n^    a\n    b|\n    c", true, false},
	}
	for _, tt := range tests {
		d := doc(t, tt.in)
		op := d.AlignToPrevious
		if tt.next {
			op = d.AlignToNext
		}
		_, err := op()
		if (err != nil) != tt.wantErr {
			t.Errorf("%q: err %v", tt.in, err)
			continue
		}
		if tt.wantErr {
			continue
		}
		if got := show(d); got != tt.want {
			t.Errorf("%q: got %q, want %q", tt.in, got, tt.want)
		}
		_, _ = d.Undo()
		if got := show(d); got != tt.in {
			t.Errorf("%q: undo gave %q", tt.in, got)
		}
	}
}

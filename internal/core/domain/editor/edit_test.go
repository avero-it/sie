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

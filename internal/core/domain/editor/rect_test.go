package editor

import (
	"slices"
	"testing"
)

func rectDoc(text string) *Document {
	return NewDocument([]byte(text), Options{TabWidth: 4})
}

func TestSelectRectTexts(t *testing.T) {
	tests := []struct {
		name                 string
		text                 string
		aLine, aV, hLine, hV int
		want                 []string
	}{
		{"block", "abcdef\nghijkl\nmnopqr", 0, 1, 2, 3, []string{"bc", "hi", "no"}},
		{"up and left", "abcdef\nghijkl\nmnopqr", 2, 3, 0, 1, []string{"bc", "hi", "no"}},
		{"short line gets an empty cursor", "abcdef\nx\nghijkl", 0, 2, 2, 4, []string{"cd", "", "ij"}},
		{"past the end of every line", "ab\ncd", 0, 5, 1, 9, []string{"", ""}},
		{"zero width", "abc\ndef", 0, 1, 1, 1, []string{"", ""}},
		{"one line", "abcdef", 0, 1, 0, 4, []string{"bcd"}},
		{"tab and wide glyph on the edge", "a\tb\n日本語c", 0, 2, 1, 4, []string{"\t", "本"}},
		{"lines clamped", "ab\ncd", -3, 0, 99, 1, []string{"a", "c"}},
		{"CRLF", "abc\r\ndef\r\n", 0, 1, 1, 3, []string{"bc", "ef"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := rectDoc(tt.text)
			d.SelectRect(tt.aLine, tt.aV, tt.hLine, tt.hV)
			if got := d.SelectedTexts(); !slices.Equal(got, tt.want) {
				t.Errorf("SelectedTexts = %q, want %q", got, tt.want)
			}
			if d.CursorCount() != len(tt.want) {
				t.Errorf("CursorCount = %d, want %d", d.CursorCount(), len(tt.want))
			}
		})
	}
}

func TestSelectRectPrimaryAndDirection(t *testing.T) {
	d := rectDoc("abcdef\nghijkl\nmnopqr")
	// Dragged up and to the left: the primary selection is on the head
	// line and its cursor (Head) is on the left edge.
	d.SelectRect(2, 4, 0, 1)
	sel := d.Selection()
	if line, _, vcol := d.Position(sel.Head); line != 0 || vcol != 1 {
		t.Errorf("head at line %d col %d, want line 0 col 1", line, vcol)
	}
	if sel.Anchor <= sel.Head {
		t.Errorf("selection %+v should run right to left", sel)
	}
	// Dragged down and to the right: head on the right edge, last line.
	d.SelectRect(0, 1, 2, 4)
	sel = d.Selection()
	if line, _, vcol := d.Position(sel.Head); line != 2 || vcol != 4 {
		t.Errorf("head at line %d col %d, want line 2 col 4", line, vcol)
	}
}

func TestRectEdits(t *testing.T) {
	const text = "abcdef\nghijkl\nmnopqr"
	tests := []struct {
		name string
		do   func(d *Document) error
		want string
	}{
		{"type replaces the block", func(d *Document) error { return d.Insert("X") }, "aXdef\ngXjkl\nmXpqr"},
		{"backspace deletes the block", func(d *Document) error { return d.Backspace() }, "adef\ngjkl\nmpqr"},
		{"delete deletes the block", func(d *Document) error { return d.DeleteForward() }, "adef\ngjkl\nmpqr"},
		{"cut", func(d *Document) error { return d.DeleteSelection() }, "adef\ngjkl\nmpqr"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := rectDoc(text)
			d.SelectRect(0, 1, 2, 3)
			if err := tt.do(d); err != nil {
				t.Fatal(err)
			}
			if got := d.Text(); got != tt.want {
				t.Errorf("text = %q, want %q", got, tt.want)
			}
			// One undo step restores the text and the whole rectangle.
			if _, err := d.Undo(); err != nil {
				t.Fatal(err)
			}
			if got := d.Text(); got != text {
				t.Errorf("after undo: %q", got)
			}
			if got := d.SelectedTexts(); !slices.Equal(got, []string{"bc", "hi", "no"}) {
				t.Errorf("after undo, selected %q", got)
			}
		})
	}
}

func TestRectZeroWidthInsertsAColumn(t *testing.T) {
	d := rectDoc("abc\nx\ndef")
	d.SelectRect(0, 2, 2, 2)
	if err := d.Insert("-"); err != nil {
		t.Fatal(err)
	}
	// The short line gets the text at its end.
	if got, want := d.Text(), "ab-c\nx-\nde-f"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestExtendRect(t *testing.T) {
	d := rectDoc("abcdef\nghijkl\nmnopqr")
	d.SetSelection(Cursor(d.OffsetAt(0, 1)))
	d.ExtendRect(0, 2) // columns 1..3 on line 0
	if got := d.SelectedTexts(); !slices.Equal(got, []string{"bc"}) {
		t.Fatalf("after right: %q", got)
	}
	d.ExtendRect(2, 0) // down two lines, same corner column
	if got := d.SelectedTexts(); !slices.Equal(got, []string{"bc", "hi", "no"}) {
		t.Fatalf("after down: %q", got)
	}
	d.ExtendRect(-1, 0)
	if got := d.SelectedTexts(); !slices.Equal(got, []string{"bc", "hi"}) {
		t.Fatalf("after up: %q", got)
	}
	d.ExtendRect(0, -9) // the column stops at 0
	if got := d.SelectedTexts(); !slices.Equal(got, []string{"a", "g"}) {
		t.Fatalf("after far left: %q", got)
	}
}

func TestExtendRectKeepsVirtualColumn(t *testing.T) {
	// The corner column survives a short line in between.
	d := rectDoc("abcdef\nx\nghijkl")
	d.SetSelection(Cursor(d.OffsetAt(0, 3)))
	d.ExtendRect(1, 0)
	d.ExtendRect(1, 0)
	if _, _, vcol := d.CursorPos(); vcol != 3 {
		t.Errorf("head column %d, want 3", vcol)
	}
	if got := d.SelectedTexts(); !slices.Equal(got, []string{"", "", ""}) {
		t.Errorf("zero width rectangle: %q", got)
	}
	d.ExtendRect(0, 2)
	if got := d.SelectedTexts(); !slices.Equal(got, []string{"de", "", "jk"}) {
		t.Errorf("widened: %q", got)
	}
}

func TestRectInactiveAfterOtherChanges(t *testing.T) {
	d := rectDoc("abcdef\nghijkl\nmnopqr")
	d.SelectRect(0, 1, 2, 3)
	d.SetSelection(Cursor(d.OffsetAt(1, 5)))
	d.ExtendRect(0, 1) // starts a new rectangle at the cursor
	if got := d.SelectedTexts(); !slices.Equal(got, []string{"l"}) {
		t.Errorf("new rectangle: %q", got)
	}
	d.SelectRect(0, 1, 1, 2)
	if err := d.Insert("Z"); err != nil {
		t.Fatal(err)
	}
	d.ExtendRect(0, 1)
	if d.CursorCount() != 1 {
		t.Errorf("after an edit the old rectangle must not come back: %d cursors", d.CursorCount())
	}
}

func TestSelectedTextsInDocumentOrder(t *testing.T) {
	d := rectDoc("one two three")
	d.SetSelection(Selection{8, 13}) // three
	d.AddCursor(0)
	d.SetSelection(Selection{8, 13})
	d.extra = []Selection{{0, 3}, {4, 7}} // primary is last in the document
	if got := d.SelectedTexts(); !slices.Equal(got, []string{"one", "two", "three"}) {
		t.Errorf("SelectedTexts = %q", got)
	}
}

func TestInsertEach(t *testing.T) {
	d := rectDoc("a\nb\nc")
	d.SelectRect(0, 1, 2, 1)
	if err := d.InsertEach([]string{"1", "2", "3"}); err != nil {
		t.Fatal(err)
	}
	if got, want := d.Text(), "a1\nb2\nc3"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
	if _, err := d.Undo(); err != nil {
		t.Fatal(err)
	}
	if got := d.Text(); got != "a\nb\nc" {
		t.Errorf("one undo step: %q", got)
	}
	if err := d.InsertEach([]string{"1", "2"}); err == nil {
		t.Error("wrong number of parts accepted")
	}
}

func TestInsertEachWithPrimaryNotFirst(t *testing.T) {
	// Document order, not primary-first order, decides who gets which part.
	d := rectDoc("a b c")
	d.SetSelection(Cursor(1))
	d.AddCursor(3)
	d.AddCursor(5) // primary is the last one
	if err := d.InsertEach([]string{"1", "2", "3"}); err != nil {
		t.Fatal(err)
	}
	if got, want := d.Text(), "a1 b2 c3"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

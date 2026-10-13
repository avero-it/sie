package ui

import "testing"

func TestInput(t *testing.T) {
	var in Input
	in.Insert("ab")
	in.Left()
	in.Insert("é")
	if in.String() != "aéb" || in.CursorCells() != 2 {
		t.Fatalf("got %q cells=%d", in.String(), in.CursorCells())
	}
	in.Insert("日")
	if in.CursorCells() != 4 {
		t.Fatalf("wide cursor cells = %d", in.CursorCells())
	}
	in.Backspace()
	in.Backspace()
	if in.String() != "ab" {
		t.Fatalf("backspace: %q", in.String())
	}
	in.Home()
	in.Backspace() // no-op at start
	in.Delete()
	if in.String() != "b" {
		t.Fatalf("delete: %q", in.String())
	}
	in.End()
	in.Delete() // no-op at end
	in.Set("éx")
	in.Left()
	in.Left()
	if in.CursorCells() != 0 {
		t.Fatalf("left over combining cluster: cells=%d", in.CursorCells())
	}
	in.Right()
	if in.CursorCells() != 1 {
		t.Fatalf("right over combining cluster: cells=%d", in.CursorCells())
	}
}

func TestScore(t *testing.T) {
	if _, ok := Score("xyz", "Save file"); ok {
		t.Error("non-subsequence matched")
	}
	if s, ok := Score("", "anything"); !ok || s != 0 {
		t.Error("empty query should match everything with score 0")
	}
	better := [][3]string{
		// query, should rank higher, than
		{"sa", "Save", "Select all"},
		{"sa", "Select all", "Paste"},
		{"trim sel", "Trim spaces from selection", "Trim trailing spaces"},
		{"gl", "Go to line", "Toggle"},
	}
	for _, b := range better {
		hi, ok1 := Score(b[0], b[1])
		lo, ok2 := Score(b[0], b[2])
		if !ok1 || (ok2 && hi <= lo) {
			t.Errorf("Score(%q): %q=%d (ok %v) should beat %q=%d (ok %v)", b[0], b[1], hi, ok1, b[2], lo, ok2)
		}
	}
}

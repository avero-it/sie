package history

import (
	"testing"
)

// text is a naive Target used to keep history tests independent of buffer.
type text struct{ b []byte }

func (t *text) Insert(off int, s []byte) error {
	t.b = append(t.b[:off:off], append(append([]byte{}, s...), t.b[off:]...)...)
	return nil
}

func (t *text) Delete(off, n int) ([]byte, error) {
	removed := append([]byte{}, t.b[off:off+n]...)
	t.b = append(t.b[:off:off], t.b[off+n:]...)
	return removed, nil
}

// typeAt applies an insert to t and records it in h, using the cursor as state.
func typeAt(t *text, h *History[int], off int, s, key string) {
	_ = t.Insert(off, []byte(s))
	h.Record(Edit{Off: off, Inserted: []byte(s)}, off, off+len(s), key)
}

func TestUndoRedoGrouping(t *testing.T) {
	doc := &text{}
	h := New[int](0)
	typeAt(doc, h, 0, "a", "type")
	typeAt(doc, h, 1, "b", "type")
	h.Seal()
	typeAt(doc, h, 2, "c", "type")

	cur, ok, err := h.Undo(doc)
	if err != nil || !ok || string(doc.b) != "ab" || cur != 2 {
		t.Fatalf("undo 1: %q cur=%d ok=%v err=%v", doc.b, cur, ok, err)
	}
	cur, ok, _ = h.Undo(doc)
	if !ok || string(doc.b) != "" || cur != 0 {
		t.Fatalf("undo 2 (grouped): %q cur=%d", doc.b, cur)
	}
	if _, ok, _ := h.Undo(doc); ok {
		t.Fatal("undo on empty stack reported ok")
	}
	cur, ok, _ = h.Redo(doc)
	if !ok || string(doc.b) != "ab" || cur != 2 {
		t.Fatalf("redo: %q cur=%d", doc.b, cur)
	}
}

func TestReplaceEditRoundTrip(t *testing.T) {
	doc := &text{b: []byte("hello world")}
	h := New[int](0)
	removed, _ := doc.Delete(6, 5)
	_ = doc.Insert(6, []byte("there"))
	h.Record(Edit{Off: 6, Removed: removed, Inserted: []byte("there")}, 6, 11, "")

	_, _, _ = h.Undo(doc)
	if string(doc.b) != "hello world" {
		t.Fatalf("undo replace: %q", doc.b)
	}
	_, _, _ = h.Redo(doc)
	if string(doc.b) != "hello there" {
		t.Fatalf("redo replace: %q", doc.b)
	}
}

func TestRecordClearsRedo(t *testing.T) {
	doc := &text{}
	h := New[int](0)
	typeAt(doc, h, 0, "a", "")
	_, _, _ = h.Undo(doc)
	typeAt(doc, h, 0, "b", "")
	if h.CanRedo() {
		t.Fatal("redo stack not cleared by Record")
	}
}

func TestUndoSealsGroup(t *testing.T) {
	doc := &text{}
	h := New[int](0)
	typeAt(doc, h, 0, "a", "type")
	typeAt(doc, h, 1, "b", "type")
	_, _, _ = h.Undo(doc)
	_, _, _ = h.Redo(doc)
	typeAt(doc, h, 2, "c", "type")
	_, _, _ = h.Undo(doc)
	if string(doc.b) != "ab" {
		t.Fatalf("typing after redo merged into redone group: %q", doc.b)
	}
}

func TestModified(t *testing.T) {
	doc := &text{}
	h := New[int](0)
	if h.Modified() {
		t.Fatal("fresh history is modified")
	}
	typeAt(doc, h, 0, "a", "type")
	if !h.Modified() {
		t.Fatal("not modified after edit")
	}
	h.MarkSaved()
	if h.Modified() {
		t.Fatal("modified right after save")
	}
	typeAt(doc, h, 1, "b", "type") // must not merge into the saved group
	_, _, _ = h.Undo(doc)
	if h.Modified() || string(doc.b) != "a" {
		t.Fatalf("undo back to saved state: modified=%v %q", h.Modified(), doc.b)
	}
	_, _, _ = h.Undo(doc)
	if !h.Modified() {
		t.Fatal("undo past save point not modified")
	}
	typeAt(doc, h, 0, "x", "") // saved state now only reachable via discarded redo
	_, _, _ = h.Undo(doc)
	if !h.Modified() {
		t.Fatal("saved state should be unreachable after redo was discarded")
	}
}

func TestLimit(t *testing.T) {
	doc := &text{}
	h := New[int](2)
	typeAt(doc, h, 0, "a", "")
	typeAt(doc, h, 1, "b", "")
	typeAt(doc, h, 2, "c", "")
	for h.CanUndo() {
		_, _, _ = h.Undo(doc)
	}
	if string(doc.b) != "a" {
		t.Fatalf("after undoing all with limit 2: %q", doc.b)
	}
	if !h.Modified() {
		t.Fatal("initial (saved) state was dropped, must report modified")
	}
}

func TestExplicitGroup(t *testing.T) {
	doc := &text{}
	h := New[int](0)
	typeAt(doc, h, 0, "x", "type") // before the group: its own step
	h.BeginGroup()
	typeAt(doc, h, 1, "a", "")
	h.Seal() // ignored inside a group
	typeAt(doc, h, 2, "b", "other")
	h.EndGroup()
	typeAt(doc, h, 3, "c", "type") // after the group: must not merge into it
	_, _, _ = h.Undo(doc)
	if string(doc.b) != "xab" {
		t.Fatalf("undo after group: %q", doc.b)
	}
	_, _, _ = h.Undo(doc)
	if string(doc.b) != "x" {
		t.Fatalf("group undone in one step: %q", doc.b)
	}
	h.BeginGroup()
	h.EndGroup() // an empty group records nothing
	if _, _, _ = h.Undo(doc); string(doc.b) != "" {
		t.Fatalf("empty group changed history: %q", doc.b)
	}
}

func TestAmend(t *testing.T) {
	doc := &text{}
	h := New[int](0)
	h.Amend(7) // no group yet: nothing to amend
	typeAt(doc, h, 0, "a", "")
	h.Amend(9)
	if _, _, err := h.Undo(doc); err != nil {
		t.Fatal(err)
	}
	cur, ok, err := h.Redo(doc)
	if err != nil || !ok || cur != 9 {
		t.Fatalf("redo: cur=%d ok=%v err=%v, want 9", cur, ok, err)
	}
}

// textTarget is a minimal Target over a string.
type textTarget struct{ s string }

func (t *textTarget) Insert(off int, text []byte) error {
	t.s = t.s[:off] + string(text) + t.s[off:]
	return nil
}

func (t *textTarget) Delete(off, n int) ([]byte, error) {
	removed := []byte(t.s[off : off+n])
	t.s = t.s[:off] + t.s[off+n:]
	return removed, nil
}

func TestBudgetDropsOldestSteps(t *testing.T) {
	h := New[int](0)
	h.SetBudget(10)
	tg := &textTarget{}
	for i := range 5 { // 5 steps of 4 bytes: only the newest two fit in 10
		off := len(tg.s)
		_ = tg.Insert(off, []byte("abcd"))
		h.Record(Edit{Off: off, Inserted: []byte("abcd")}, i, i+1, "")
		h.Seal()
	}
	steps := 0
	for h.CanUndo() {
		if _, ok, err := h.Undo(tg); !ok || err != nil {
			t.Fatal(ok, err)
		}
		steps++
	}
	if steps != 2 || tg.s != "abcdabcdabcd"[:4*3] {
		t.Errorf("undid %d steps leaving %q, want 2 steps leaving 12 bytes", steps, tg.s)
	}
	if !h.Modified() {
		t.Error("document with dropped history should count as modified")
	}
}

func TestClearKeepsDocumentModified(t *testing.T) {
	h := New[int](0)
	tg := &textTarget{}
	_ = tg.Insert(0, []byte("x"))
	h.Record(Edit{Inserted: []byte("x")}, 0, 1, "")
	h.MarkSaved()
	h.Clear()
	if h.CanUndo() || h.CanRedo() || !h.Modified() {
		t.Errorf("after Clear: undo=%v redo=%v modified=%v", h.CanUndo(), h.CanRedo(), h.Modified())
	}
}

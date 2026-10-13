package editor

import "testing"

func TestVersionAndChanges(t *testing.T) {
	d := doc(t, "ab|c")
	if d.Version() != 0 {
		t.Fatalf("fresh version %d", d.Version())
	}
	_ = d.Insert("X")       // v1: insert 1 at 2
	d.Move(MoveLeft, false) // selection only: no version bump
	_ = d.Backspace()       // v2: delete 1 at 1
	if d.Version() != 2 {
		t.Fatalf("version %d, want 2", d.Version())
	}
	got, ok := d.ChangesSince(0)
	want := []Change{{1, 2, 0, 1}, {2, 1, 1, 0}}
	if !ok || len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("changes %+v ok=%v", got, ok)
	}
	if got, ok := d.ChangesSince(2); !ok || got != nil {
		t.Errorf("up to date: %+v %v", got, ok)
	}
	if _, ok := d.ChangesSince(9); ok {
		t.Error("future version accepted")
	}
	// Undo goes through the recorder: delete + insert are logged.
	_, _ = d.Undo()
	if got, _ := d.ChangesSince(2); len(got) == 0 || d.Version() <= 2 {
		t.Errorf("undo not logged: %+v version %d", got, d.Version())
	}
}

func TestChangeLogTruncation(t *testing.T) {
	d := doc(t, "|")
	for range changeLogSize + 5 {
		_ = d.Insert("x")
	}
	if _, ok := d.ChangesSince(0); ok {
		t.Error("log claims to reach version 0 after truncation")
	}
	if got, ok := d.ChangesSince(d.Version() - 3); !ok || len(got) != 3 {
		t.Errorf("recent changes: %d %v", len(got), ok)
	}
}

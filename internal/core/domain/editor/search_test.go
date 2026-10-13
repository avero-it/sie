package editor

import "testing"

func TestFind(t *testing.T) {
	cs := SearchOptions{CaseSensitive: true}
	ci := SearchOptions{}
	tests := []struct {
		name    string
		in      string
		query   string
		forward bool
		opts    SearchOptions
		want    string
		found   bool
	}{
		{"forward", "|foo bar foo", "foo", true, cs, "^foo| bar foo", true},
		{"forward skips selection", "^foo| bar foo", "foo", true, cs, "foo bar ^foo|", true},
		{"forward wraps", "foo bar ^foo|", "foo", true, cs, "^foo| bar foo", true},
		{"backward", "foo bar foo|", "foo", false, cs, "foo bar ^foo|", true},
		{"backward wraps", "^foo| bar foo", "foo", false, cs, "foo bar ^foo|", true},
		{"case sensitive misses", "|Foo", "foo", true, cs, "|Foo", false},
		{"case insensitive", "|x FOO", "foo", true, ci, "x ^FOO|", true},
		{"unicode fold", "|ÀB", "àb", true, ci, "^ÀB|", true},
		{"not found", "|abc", "zz", true, cs, "|abc", false},
		{"empty query", "|abc", "", true, cs, "|abc", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := doc(t, tt.in)
			found := d.Find(tt.query, tt.forward, tt.opts)
			if found != tt.found || show(d) != tt.want {
				t.Errorf("got %q found=%v, want %q found=%v", show(d), found, tt.want, tt.found)
			}
		})
	}
}

func TestReplaceAll(t *testing.T) {
	d := doc(t, "a|a.aa.aa")
	n, err := d.ReplaceAll("aa", "bbb", SearchOptions{CaseSensitive: true})
	if err != nil || n != 3 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if got := d.Text(); got != "bbb.bbb.bbb" {
		t.Fatalf("text %q", got)
	}
	if _, err := d.Undo(); err != nil {
		t.Fatal(err)
	}
	if got := show(d); got != "a|a.aa.aa" {
		t.Errorf("one undo restores all: %q", got)
	}
}

func TestReplaceCurrent(t *testing.T) {
	opts := SearchOptions{CaseSensitive: true}
	d := doc(t, "|x foo foo")
	if ok, _ := d.ReplaceCurrent("foo", "bar", opts); ok {
		t.Fatal("replaced without a matching selection")
	}
	d.Find("foo", true, opts)
	if ok, err := d.ReplaceCurrent("foo", "bar", opts); !ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if got, want := show(d), "x bar ^foo|"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFindNextAndReplaceRangeInGroup(t *testing.T) {
	d := doc(t, "|a x a x a")
	opts := SearchOptions{CaseSensitive: true}
	if at, ok := d.FindNext("a", 1, opts); !ok || at != 4 {
		t.Fatalf("FindNext = %d, %v", at, ok)
	}
	if _, ok := d.FindNext("a", 9, opts); ok {
		t.Fatal("FindNext wrapped")
	}
	d.BeginUndoGroup()
	n, err := d.ReplaceRange(0, 1, "bb\n")
	if err != nil || n != 3 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	d.SetSelection(Cursor(6)) // selection changes must not split the group
	if _, err := d.ReplaceRange(6, 7, "c"); err != nil {
		t.Fatal(err)
	}
	d.EndUndoGroup()
	if got := d.Text(); got != "bb\n x c x a" {
		t.Fatalf("text %q", got)
	}
	_, _ = d.Undo()
	if got := d.Text(); got != "a x a x a" {
		t.Errorf("one undo should revert the group: %q", got)
	}
	if _, err := d.ReplaceRange(5, 99, ""); err == nil {
		t.Error("out of range accepted")
	}
}

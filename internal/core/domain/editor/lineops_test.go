package editor

import "testing"

func TestIndentOutdent(t *testing.T) {
	tests := []struct {
		name, in, want string
		outdent        bool
	}{
		{"indent block", "^a\n\nb|", "^\ta\n\n\tb|", false},
		{"indent keeps mid-line anchor", "a^b\nc|d", "\ta^b\n\tc|d", false},
		{"outdent block tabs", "^\ta\n  b\n\t\tc|", "^a\nb\n\tc|", true},
		{"outdent block spaces", "^      a\n    b|", "^  a\nb|", true},
		{"tab left of cursor", "foo\t|bar", "foo|bar", true},
		{"mid-line tab keeps indent", "\tfoo\t|bar", "\tfoo|bar", true},
		{"spaces to tab stop", "foo    |bar", "foo |bar", true},
		{"spaces partial stop", "ab  |c", "ab|c", true},
		{"fewer spaces than stop", " |x", "|x", true},
		{"no whitespace: line outdent", "\tfo|o", "fo|o", true},
		{"no whitespace, spaces", "      fo|o", "  fo|o", true},
		{"nothing to do", "fo|o", "fo|o", true},
		{"single-line selection outdents line", "\t^fo|o", "^fo|o", true},
	}
	for _, tt := range tests {
		d := doc(t, tt.in)
		var err error
		if tt.outdent {
			err = d.Outdent()
		} else {
			err = d.Indent()
		}
		if err != nil {
			t.Fatal(err)
		}
		if got := show(d); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
		if tt.in != tt.want {
			_, _ = d.Undo()
			if got := show(d); got != tt.in {
				t.Errorf("%s: undo gave %q, want %q", tt.name, got, tt.in)
			}
		}
	}
}

func TestOutdentMultiCursor(t *testing.T) {
	d := doc(t, "\ta|\n\tb")
	d.AddCursor(len("\ta\n\tb"))
	if err := d.Outdent(); err != nil {
		t.Fatal(err)
	}
	if got := string(d.Text()); got != "a\nb" {
		t.Fatalf("got %q", got)
	}
	_, _ = d.Undo()
	if got := string(d.Text()); got != "\ta\n\tb" {
		t.Fatalf("undo gave %q", got)
	}
}

func TestSelectsLines(t *testing.T) {
	for in, want := range map[string]bool{"a|b": false, "^ab|": false, "^a\nb|": true, "^a\n|b": false} {
		if got := doc(t, in).SelectsLines(); got != want {
			t.Errorf("%q: got %v", in, got)
		}
	}
}

func TestDeleteWord(t *testing.T) {
	d := doc(t, "foo bar| baz")
	if err := d.DeleteWordBack(); err != nil {
		t.Fatal(err)
	}
	if got := show(d); got != "foo | baz" {
		t.Fatalf("back: %q", got)
	}
	if err := d.DeleteWordForward(); err != nil {
		t.Fatal(err)
	}
	if got := show(d); got != "foo |" {
		t.Fatalf("forward: %q", got)
	}
	_, _ = d.Undo()
	if got := show(d); got != "foo | baz" {
		t.Fatalf("undo: %q", got)
	}
}

func TestMoveLines(t *testing.T) {
	tests := []struct {
		in, want string
		up       bool
	}{
		{"a\nb|\nc", "a\nc\nb|", false},
		{"a\nb|\nc", "b|\na\nc", true},
		{"a\n^b\nc|\nd", "a\nd\n^b\nc|", false},
		{"|a\nb", "|a\nb", true},
		{"a\nb|", "a\nb|", false},
		{"a\nb|", "b|\na", true},
		{"a|\nb", "b\na|", false},
	}
	for _, tt := range tests {
		d := doc(t, tt.in)
		if err := d.MoveLines(tt.up); err != nil {
			t.Fatal(err)
		}
		if got := show(d); got != tt.want {
			t.Errorf("%q up=%v: got %q, want %q", tt.in, tt.up, got, tt.want)
		}
		if tt.in != tt.want {
			_, _ = d.Undo()
			if got := show(d); got != tt.in {
				t.Errorf("%q: undo gave %q", tt.in, got)
			}
		}
	}
}

func TestMoveLinesCRLF(t *testing.T) {
	d := NewDocument([]byte("a\r\nb\r\nc"), Options{TabWidth: 4})
	d.SetSelection(Cursor(3))
	if err := d.MoveLines(false); err != nil {
		t.Fatal(err)
	}
	if got := string(d.Text()); got != "a\r\nc\r\nb" {
		t.Fatalf("got %q", got)
	}
}

func TestSelectOccurrence(t *testing.T) {
	d := doc(t, "foo x foo y foo|")
	d.SetSelection(Selection{0, 3})
	if !d.SelectOccurrence(true) || d.CursorCount() != 2 {
		t.Fatalf("next: %v", d.Selections())
	}
	if d.Selection() != (Selection{6, 9}) {
		t.Fatalf("primary %v", d.Selection())
	}
	if !d.SelectOccurrence(true) || d.Selection() != (Selection{12, 15}) {
		t.Fatalf("third %v", d.Selections())
	}
	if d.SelectOccurrence(true) {
		t.Fatal("all occurrences are selected, nothing to add")
	}
	if d.CursorCount() != 3 {
		t.Fatalf("cursors %d", d.CursorCount())
	}

	d = doc(t, "foo x foo|")
	d.SetSelection(Selection{6, 9})
	if !d.SelectOccurrence(false) || d.Selection() != (Selection{0, 3}) {
		t.Fatalf("prev: %v", d.Selections())
	}

	d = doc(t, "foo ba|r")
	if !d.SelectOccurrence(true) || d.SelectedText() != "bar" || d.CursorCount() != 1 {
		t.Fatalf("empty selection should take the word, got %q", d.SelectedText())
	}
}

func TestToggleComment(t *testing.T) {
	tests := []struct {
		in, want  string
		commented bool
	}{
		{"a|\nb", "// a|\nb", true},
		{"  ^a\n\n    b|", "  ^// a\n\n  //   b|", true},
		{"^// a\n//b|", "^a\nb|", false},
		{"// a|\n//b", "a|\n//b", false},
		{"^// a\nb|", "^// // a\n// b|", true},
		{"\n|", "\n|", false},
	}
	for _, tt := range tests {
		d := doc(t, tt.in)
		got, err := d.ToggleComment("//")
		if err != nil {
			t.Fatal(err)
		}
		if got != tt.commented || show(d) != tt.want {
			t.Errorf("%q: got %q (%v), want %q (%v)", tt.in, show(d), got, tt.want, tt.commented)
		}
		if tt.in != tt.want {
			_, _ = d.Undo()
			if g := show(d); g != tt.in {
				t.Errorf("%q: undo gave %q", tt.in, g)
			}
		}
	}
}

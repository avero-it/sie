package editor

import "testing"

func TestMove(t *testing.T) {
	tests := []struct {
		name   string
		in     string
		motion Motion
		extend bool
		want   string
	}{
		{"right", "|ab", MoveRight, false, "a|b"},
		{"right at end of doc", "ab|", MoveRight, false, "ab|"},
		{"right over line break", "a|\nb", MoveRight, false, "a\n|b"},
		{"right over CRLF as one step", "a|\r\nb", MoveRight, false, "a\r\n|b"},
		{"right over grapheme cluster", "|éx", MoveRight, false, "é|x"},
		{"right over emoji", "|👩‍💻x", MoveRight, false, "👩‍💻|x"},
		{"left", "ab|", MoveLeft, false, "a|b"},
		{"left at doc start", "|ab", MoveLeft, false, "|ab"},
		{"left over CRLF", "a\r\n|b", MoveLeft, false, "a|\r\nb"},
		{"left collapses selection", "a^bc|d", MoveLeft, false, "a|bcd"},
		{"right collapses selection", "a|bc^d", MoveRight, false, "abc|d"},
		{"shift right extends", "a|bc", MoveRight, true, "a^b|c"},
		{"up keeps column", "abc\nab|c", MoveUp, false, "ab|c\nabc"},
		{"up clamps to short line", "a\nabc|", MoveUp, false, "a|\nabc"},
		{"up on first line goes to start", "ab|c", MoveUp, false, "|abc"},
		{"down on last line goes to end", "a|bc", MoveDown, false, "abc|"},
		{"down by screen column over tabs", "\tx|\n12345678", MoveDown, false, "\tx\n12345|678"},
		{"down from wide char", "日|本\nabcd", MoveDown, false, "日本\nab|cd"},
		{"word right skips word", "|foo bar", MoveWordRight, false, "foo| bar"},
		{"word right skips space then word", "foo| bar", MoveWordRight, false, "foo bar|"},
		{"word right stops at punctuation", "|foo.bar", MoveWordRight, false, "foo|.bar"},
		{"word right at line end", "foo|\nbar", MoveWordRight, false, "foo\n|bar"},
		{"word left", "foo bar|", MoveWordLeft, false, "foo |bar"},
		{"word left over space", "foo |bar", MoveWordLeft, false, "|foo bar"},
		{"word left at line start", "foo\n|bar", MoveWordLeft, false, "foo|\nbar"},
		{"smart home to indent", "  ab|c", MoveLineStart, false, "  |abc"},
		{"smart home toggles to col 0", "  |abc", MoveLineStart, false, "|  abc"},
		{"end of CRLF line", "|ab\r\nc", MoveLineEnd, false, "ab|\r\nc"},
		{"doc start", "a\nb|", MoveDocStart, false, "|a\nb"},
		{"doc end", "|a\nb", MoveDocEnd, true, "^a\nb|"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := doc(t, tt.in)
			d.Move(tt.motion, tt.extend)
			if got := show(d); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGoalColumnSurvivesShortLine(t *testing.T) {
	d := doc(t, "abcdef|\nab\nabcdef")
	d.Move(MoveDown, false)
	d.Move(MoveDown, false)
	if got, want := show(d), "abcdef\nab\nabcdef|"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMovePage(t *testing.T) {
	d := doc(t, "a|\nb\nc\nd")
	d.MovePage(2, false)
	if got, want := show(d), "a\nb\nc|\nd"; got != want {
		t.Errorf("page down: got %q, want %q", got, want)
	}
	d.MovePage(-10, false)
	if got, want := show(d), "|a\nb\nc\nd"; got != want {
		t.Errorf("page up: got %q, want %q", got, want)
	}
}

func TestSetSelectionSnaps(t *testing.T) {
	d := doc(t, "|é\r\nx")
	d.SetSelection(Cursor(2)) // inside the cluster "é" (bytes 0..3)
	if got := d.Selection().Head; got != 0 {
		t.Errorf("snap inside cluster = %d, want 0", got)
	}
	d.SetSelection(Cursor(4)) // between \r and \n
	if got := d.Selection().Head; got != 3 {
		t.Errorf("snap inside CRLF = %d, want 3", got)
	}
	d.SetSelection(Cursor(99))
	if got := d.Selection().Head; got != d.Len() {
		t.Errorf("snap past end = %d, want %d", got, d.Len())
	}
}

func TestPositionAndOffsetAt(t *testing.T) {
	d := doc(t, "ab\n\t日|x")
	line, col, vcol := d.CursorPos()
	if line != 1 || col != 4 || vcol != 6 {
		t.Errorf("CursorPos = %d,%d,%d; want 1,4,6", line, col, vcol)
	}
	if got := d.OffsetAt(1, 5); got != 4 { // right half of 日 → its start
		t.Errorf("OffsetAt(1,5) = %d, want 4", got)
	}
	if got := d.OffsetAt(9, 0); got != d.Len() {
		t.Errorf("OffsetAt past last line = %d, want %d", got, d.Len())
	}
}

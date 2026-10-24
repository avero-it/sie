package editor

import (
	"reflect"
	"strings"
	"testing"
)

func TestSegments(t *testing.T) {
	tests := []struct {
		name string
		line string
		cols int
		want [][2]int // glyph ranges
	}{
		{"empty", "", 5, [][2]int{{0, 0}}},
		{"fits", "abc", 5, [][2]int{{0, 3}}},
		{"word break", "aaa bbb ccc", 6, [][2]int{{0, 4}, {4, 8}, {8, 11}}},
		{"hard break", "abcdefghij", 4, [][2]int{{0, 4}, {4, 8}, {8, 10}}},
		{"exact fill adds an empty row", "abcd", 4, [][2]int{{0, 4}, {4, 4}}},
		{"wide glyph is not split", "ab日本", 3, [][2]int{{0, 2}, {2, 3}, {3, 4}}},
		{"no wrap below 2 cols", "abcdef", 1, [][2]int{{0, 6}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got [][2]int
			for _, s := range Segments(Layout([]byte(tt.line), 4), tt.cols) {
				got = append(got, [2]int{s.First, s.End})
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Segments(%q, %d) = %v, want %v", tt.line, tt.cols, got, tt.want)
			}
		})
	}
}

// Every grapheme boundary maps to a row cell and back to the same offset.
func TestWrapPosRoundTrip(t *testing.T) {
	text := "the quick brown fox jumps over the lazy dog, 日本語 and\ttabs"
	d := doc(t, "|"+text)
	for cols := 2; cols < 30; cols++ {
		for off := 0; ; off = nextBoundary([]byte(text), off) {
			line, seg, x := d.WrapPos(off, cols)
			if x < 0 || x > cols {
				t.Fatalf("cols %d off %d: x = %d", cols, off, x)
			}
			if got := d.OffsetAtRow(line, seg, x, cols); got != off {
				t.Fatalf("cols %d: off %d -> (%d,%d,%d) -> %d", cols, off, line, seg, x, got)
			}
			if off == len(text) {
				break
			}
		}
	}
}

func TestMoveRows(t *testing.T) {
	// Width 6 wraps to the rows "aaa ", "bbb ", "ccc"; the cursor starts at x=3 of "ccc".
	d := doc(t, "aaa bbb ccc|\nxy")
	for _, step := range []struct{ delta, want int }{
		{-1, 7}, // x=3 of "bbb "
		{-1, 3}, // x=3 of "aaa "
		{-1, 0}, // above the first row: document start
		{10, len("aaa bbb ccc\nxy")},
	} {
		d.MoveRows(step.delta, 6, false)
		if got := d.Selection().Head; got != step.want {
			t.Fatalf("MoveRows(%d): head = %d, want %d", step.delta, got, step.want)
		}
	}
}

func TestMoveRowsKeepsGoalAcrossLines(t *testing.T) {
	long := strings.Repeat("abcd ", 4)[:19] // rows "abcd abcd " and "abcd abcd" at width 10
	d := doc(t, long+"\nx\n"+long[:18]+"|") // the cursor is at x=8 of the second row
	for i, want := range [][3]int{{2, 0, 8}, {1, 0, 1}, {0, 1, 8}} {
		d.MoveRows(-1, 10, false)
		if line, seg, x := d.WrapPos(d.Selection().Head, 10); [3]int{line, seg, x} != want {
			t.Fatalf("move %d: (line, seg, x) = (%d, %d, %d), want %v", i+1, line, seg, x, want)
		}
	}
}

func TestViewportWrap(t *testing.T) {
	rows := func(l int) int { return []int{3, 1, 2, 1}[l] }
	v := Viewport{Rows: 3}
	v.FollowWrap(3, 0, 4, rows)
	if v.Top != 2 || v.Skip != 0 {
		t.Errorf("follow down: Top,Skip = %d,%d, want 2,0", v.Top, v.Skip)
	}
	v.FollowWrap(0, 1, 4, rows)
	if v.Top != 0 || v.Skip != 1 {
		t.Errorf("follow up: Top,Skip = %d,%d, want 0,1", v.Top, v.Skip)
	}
	v.ScrollRows(3, 4, rows)
	if v.Top != 2 || v.Skip != 0 {
		t.Errorf("scroll down 3: Top,Skip = %d,%d, want 2,0", v.Top, v.Skip)
	}
	v.ScrollRows(-100, 4, rows)
	if v.Top != 0 || v.Skip != 0 {
		t.Errorf("scroll to top: Top,Skip = %d,%d", v.Top, v.Skip)
	}
}

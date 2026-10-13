package editor

import "testing"

func TestViewportFollow(t *testing.T) {
	tests := []struct {
		name            string
		v               Viewport
		line, vcol      int
		wantTop, wantLf int
	}{
		{"visible, no scroll", Viewport{Rows: 10, Cols: 20}, 5, 5, 0, 0},
		{"below", Viewport{Rows: 10, Cols: 20}, 15, 0, 6, 0},
		{"above", Viewport{Top: 20, Rows: 10, Cols: 20}, 3, 0, 3, 0},
		{"below with scrolloff", Viewport{Rows: 10, Cols: 20, ScrollOff: 2}, 8, 0, 1, 0},
		{"above with scrolloff", Viewport{Top: 10, Rows: 10, Cols: 20, ScrollOff: 2}, 11, 0, 9, 0},
		{"scrolloff at top clamps", Viewport{Top: 3, Rows: 10, Cols: 20, ScrollOff: 5}, 0, 0, 0, 0},
		{"huge scrolloff is capped", Viewport{Rows: 4, Cols: 20, ScrollOff: 100}, 10, 0, 8, 0},
		{"right", Viewport{Rows: 10, Cols: 20}, 0, 25, 0, 6},
		{"left", Viewport{Left: 30, Rows: 10, Cols: 20}, 0, 5, 0, 5},
		{"right with side margin", Viewport{Rows: 10, Cols: 20, SideScrollOff: 3}, 0, 18, 0, 2},
		{"zero size is a no-op", Viewport{}, 50, 50, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := tt.v
			v.Follow(tt.line, tt.vcol)
			if v.Top != tt.wantTop || v.Left != tt.wantLf {
				t.Errorf("Top,Left = %d,%d; want %d,%d", v.Top, v.Left, tt.wantTop, tt.wantLf)
			}
		})
	}
}

func TestViewportScrollBy(t *testing.T) {
	v := Viewport{Rows: 10}
	v.ScrollBy(-3, 100)
	if v.Top != 0 {
		t.Errorf("scroll above start: %d", v.Top)
	}
	v.ScrollBy(500, 100)
	if v.Top != 99 {
		t.Errorf("scroll past end: %d", v.Top)
	}
}

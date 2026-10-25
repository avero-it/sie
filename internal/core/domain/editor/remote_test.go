package editor

import "testing"

func TestGraphemeConversion(t *testing.T) {
	d := doc(t, "ab\n|éx🇮🇹y\r\nz")
	tests := []struct {
		line, gcol int
		off        int
		ok         bool
	}{
		{0, 0, 0, true},
		{0, 2, 2, true},
		{0, 3, 0, false},
		{1, 0, 3, true},
		{1, 1, 3 + 3, true}, // e + combining accent is one grapheme
		{1, 3, 3 + 3 + 1 + 8, true},
		{1, 4, 3 + 3 + 1 + 8 + 1, true}, // end of line, before \r\n
		{1, 5, 0, false},
		{2, 1, 3 + 3 + 1 + 8 + 1 + 2 + 1, true},
		{3, 0, 0, false},
		{-1, 0, 0, false},
	}
	for _, tt := range tests {
		off, ok := d.OffsetAtGrapheme(tt.line, tt.gcol)
		if ok != tt.ok || (ok && off != tt.off) {
			t.Errorf("OffsetAtGrapheme(%d,%d) = %d,%v; want %d,%v", tt.line, tt.gcol, off, ok, tt.off, tt.ok)
			continue
		}
		if ok {
			if l, c := d.GraphemePos(off); l != tt.line || c != tt.gcol {
				t.Errorf("GraphemePos(%d) = %d,%d; want %d,%d", off, l, c, tt.line, tt.gcol)
			}
		}
	}
}

func TestSliceClamps(t *testing.T) {
	d := doc(t, "he|llo")
	got, err := d.Slice(-3, 99)
	if err != nil || string(got) != "hello" {
		t.Fatalf("Slice = %q, %v", got, err)
	}
}

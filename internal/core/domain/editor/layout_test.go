package editor

import (
	"testing"
)

func TestLayout(t *testing.T) {
	type g struct {
		col, width int
		disp       string
	}
	tests := []struct {
		name string
		line string
		want []g
	}{
		{"ascii", "ab", []g{{0, 1, "a"}, {1, 1, "b"}}},
		{"tab stops", "a\tb\t", []g{{0, 1, "a"}, {1, 3, "   "}, {4, 1, "b"}, {5, 3, "   "}}},
		{"wide", "日本", []g{{0, 2, "日"}, {2, 2, "本"}}},
		{"combining mark is one glyph", "éx", []g{{0, 1, "é"}, {1, 1, "x"}}},
		{"emoji zwj sequence", "👩‍💻!", []g{{0, 2, "👩‍💻"}, {2, 1, "!"}}},
		{"control", "a\x01", []g{{0, 1, "a"}, {1, 2, "^A"}}},
		{"del", "\x7f", []g{{0, 2, "^?"}}},
		{"lone cr", "a\r", []g{{0, 1, "a"}, {1, 2, "^M"}}},
		{"invalid utf8", "a\xffb", []g{{0, 1, "a"}, {1, 1, "�"}, {2, 1, "b"}}},
		{"zero width space", "a\u200bb", []g{{0, 1, "a"}, {1, 1, "¤"}, {2, 1, "b"}}},
		{"space is a faint dot", "a b", []g{{0, 1, "a"}, {1, 1, "·"}, {2, 1, "b"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line := []byte(tt.line)
			got := Layout(line, 4)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d glyphs %+v, want %d", len(got), got, len(tt.want))
			}
			for i, w := range tt.want {
				if got[i].Col != w.col || got[i].Width != w.width || got[i].Display(line) != w.disp {
					t.Errorf("glyph %d = col %d width %d %q; want %d %d %q",
						i, got[i].Col, got[i].Width, got[i].Display(line), w.col, w.width, w.disp)
				}
			}
		})
	}
}

func TestVisualColAndOffset(t *testing.T) {
	line := []byte("a\t日x")
	// cells: a=0, tab=1..3, 日=4..5, x=6
	cols := []struct{ byteCol, vcol int }{{0, 0}, {1, 1}, {2, 4}, {5, 6}, {6, 7}}
	for _, c := range cols {
		if got := VisualCol(line, c.byteCol, 4); got != c.vcol {
			t.Errorf("VisualCol(%d) = %d, want %d", c.byteCol, got, c.vcol)
		}
	}
	offs := []struct{ vcol, byteCol int }{{0, 0}, {2, 1}, {4, 2}, {5, 2}, {6, 5}, {7, 6}, {99, 6}}
	for _, o := range offs {
		if got := OffsetAtCol(line, o.vcol, 4); got != o.byteCol {
			t.Errorf("OffsetAtCol(%d) = %d, want %d", o.vcol, got, o.byteCol)
		}
	}
}

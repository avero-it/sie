package editor

import (
	"bytes"
	"slices"
	"testing"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

// refLayout is the original Layout: every cluster goes through uniseg. The
// ASCII fast path in eachCluster must give exactly the same glyphs.
func refLayout(line []byte, tabWidth int) []Glyph {
	var glyphs []Glyph
	off, col, state := 0, 0, -1
	rest := line
	for len(rest) > 0 {
		var cluster []byte
		var width int
		cluster, rest, width, state = uniseg.FirstGraphemeCluster(rest, state)
		g := Glyph{Off: off, Len: len(cluster), Col: col, Width: width, Kind: GlyphText}
		switch {
		case len(cluster) == 1 && cluster[0] == ' ':
			g.Kind = GlyphSpace
		case len(cluster) == 1 && cluster[0] == '\t':
			g.Kind = GlyphTab
			g.Width = tabWidth - col%tabWidth
		case len(cluster) == 1 && (cluster[0] < 0x20 || cluster[0] == 0x7f):
			g.Kind = GlyphControl
			g.Width = 2
		case !utf8.Valid(cluster):
			g.Kind = GlyphInvalid
			g.Width = 1
		case width == 0:
			g.Kind = GlyphControl
			g.Width = 1
		}
		glyphs = append(glyphs, g)
		off += len(cluster)
		col += g.Width
	}
	return glyphs
}

// refStarts returns the start of every cluster, per uniseg alone.
func refStarts(line []byte) []int {
	var starts []int
	for _, g := range refLayout(line, 4) {
		starts = append(starts, g.Off)
	}
	return starts
}

func refOffsetAtCol(glyphs []Glyph, vcol, n int) int {
	for _, g := range glyphs {
		if vcol < g.Col+g.Width {
			return g.Off
		}
	}
	return n
}

// checkLayout compares every helper built on eachCluster with the uniseg-only
// reference, at every byte and screen column of line.
func checkLayout(t *testing.T, line []byte) {
	t.Helper()
	const tw = 4
	want := refLayout(line, tw)
	if got := Layout(line, tw); !slices.Equal(got, want) {
		t.Fatalf("Layout(%q):\n got %+v\nwant %+v", line, got, want)
	}
	starts := refStarts(line)
	width := 0
	if len(want) > 0 {
		last := want[len(want)-1]
		width = last.Col + last.Width
	}
	for col := 0; col <= len(line); col++ {
		wantV := 0
		for _, g := range want {
			if g.Off >= col {
				break
			}
			wantV = g.Col + g.Width
		}
		if got := VisualCol(line, col, tw); got != wantV {
			t.Fatalf("VisualCol(%q, %d) = %d, want %d", line, col, got, wantV)
		}
		prev, next, snap := 0, len(line), 0
		for _, s := range starts {
			if s < col {
				prev = s
			}
			if s <= col {
				snap = s
			}
		}
		for i, s := range starts {
			end := len(line)
			if i+1 < len(starts) {
				end = starts[i+1]
			}
			if s <= col && col < end {
				next = end
			}
		}
		if got := prevBoundary(line, col); got != prev {
			t.Fatalf("prevBoundary(%q, %d) = %d, want %d", line, col, got, prev)
		}
		if got := nextBoundary(line, col); got != next {
			t.Fatalf("nextBoundary(%q, %d) = %d, want %d", line, col, got, next)
		}
		if col < len(line) {
			if got := snapBoundary(line, col); got != snap {
				t.Fatalf("snapBoundary(%q, %d) = %d, want %d", line, col, got, snap)
			}
		}
	}
	for vcol := 0; vcol <= width+2; vcol++ {
		w := refOffsetAtCol(want, vcol, len(line))
		if got := OffsetAtCol(line, vcol, tw); got != w {
			t.Fatalf("OffsetAtCol(%q, %d) = %d, want %d", line, vcol, got, w)
		}
		for right := vcol; right <= width+2; right += 3 {
			lo, hi := rectCols(line, vcol, right, tw)
			if wlo, whi := w, refOffsetAtCol(want, right, len(line)); lo != wlo || hi != whi {
				t.Fatalf("rectCols(%q, %d, %d) = %d, %d, want %d, %d", line, vcol, right, lo, hi, wlo, whi)
			}
		}
	}
}

var layoutSeeds = []string{
	"",
	"plain ascii, nothing special",
	"\ttab\tand  spaces ",
	"á combining after ascii",
	"é̂ two marks",
	"؀123 prepend before digits",
	"x؀y prepend between ascii",
	"🇮🇹🇮🇹🇮 regional indicators, odd count",
	"a🇮🇹b",
	"👩‍💻 zwj sequence",
	"a‍b zwj between ascii",
	"#️⃣ keycap",
	"\x00\x01\x7f controls",
	"\r lone cr",
	"x\xffy invalid byte",
	"\xff\xfe",
	"日本語 wide",
	"é\xff\r",
	`{"k":"value","n":12345},{"k":"vàlue"}`,
}

func TestLayoutMatchesUniseg(t *testing.T) {
	for _, s := range layoutSeeds {
		checkLayout(t, []byte(s))
	}
}

func FuzzLayout(f *testing.F) {
	for _, s := range layoutSeeds {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, line []byte) {
		if bytes.IndexByte(line, '\n') >= 0 || len(line) > 256 {
			t.Skip() // a line never contains '\n'; keep the O(n²) check quick
		}
		checkLayout(t, line)
	})
}

// longLine is a 1 MB minified-JSON-like line, mostly ASCII with a few
// accented letters.
func longLine() []byte {
	return bytes.Repeat([]byte(`{"k":"valué","n":12345},`), 1<<20/25)
}

// The cursor helpers must not allocate, however long the line: the old
// Layout built a slice of every glyph (about 40 bytes per byte of text).
func TestLongLineHelpersDontAllocate(t *testing.T) {
	line := longLine()
	end := len(line)
	allocs := testing.AllocsPerRun(5, func() {
		VisualCol(line, end, 4)
		OffsetAtCol(line, 80, 4)
		prevBoundary(line, end)
		nextBoundary(line, 0)
		snapBoundary(line, end-1)
		IndentCol(line, 4)
		rectCols(line, 10, 20, 4)
	})
	if allocs != 0 {
		t.Fatalf("%v allocations per run, want 0", allocs)
	}
}

func BenchmarkVisualColEndOfLongLine(b *testing.B) {
	line := longLine()
	for b.Loop() {
		VisualCol(line, len(line), 4)
	}
}

func BenchmarkOffsetAtColStartOfLongLine(b *testing.B) {
	line := longLine()
	for b.Loop() {
		OffsetAtCol(line, 80, 4)
	}
}

func BenchmarkArrowAtEndOfLongLine(b *testing.B) {
	line := longLine()
	for b.Loop() {
		prevBoundary(line, len(line))
		nextBoundary(line, len(line)-1)
		snapBoundary(line, len(line)-1)
	}
}

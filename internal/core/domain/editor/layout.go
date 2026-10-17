package editor

import (
	"iter"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

// GlyphKind tells the painter how to draw a glyph.
type GlyphKind uint8

const (
	// GlyphText is a grapheme cluster drawn as its own bytes.
	GlyphText GlyphKind = iota
	// GlyphSpace is an ASCII space. Display returns a blank; the app may
	// draw a whitespace marker instead.
	GlyphSpace
	// GlyphTab is a tab expanded to the next tab stop. Display returns
	// blanks; the app may draw a whitespace marker instead.
	GlyphTab
	// GlyphControl is a control or zero-width character drawn visibly
	// ("^A" for ASCII controls, "¤" otherwise).
	GlyphControl
	// GlyphInvalid is a byte sequence that is not valid UTF-8, drawn as U+FFFD.
	GlyphInvalid
)

// Glyph is one grapheme cluster of a line laid out on screen cells.
type Glyph struct {
	Off, Len   int // byte range within the line
	Col, Width int // first screen cell and number of cells
	Kind       GlyphKind
}

// Display returns the text the painter draws for g; it occupies g.Width cells.
func (g Glyph) Display(line []byte) string {
	switch g.Kind {
	case GlyphSpace, GlyphTab:
		return strings.Repeat(" ", g.Width)
	case GlyphControl:
		if b := line[g.Off]; g.Len == 1 && b < utf8.RuneSelf {
			return "^" + string(rune(b^0x40))
		}
		return "¤"
	case GlyphInvalid:
		return "�"
	default:
		return string(line[g.Off : g.Off+g.Len])
	}
}

// Layout splits a line (without terminator) into grapheme clusters and
// assigns screen cells: tabs expand to multiples of tabWidth, wide clusters
// take two cells, controls are made visible. It builds the whole slice, so
// callers that need only part of a long line should range over Glyphs and
// stop early.
func Layout(line []byte, tabWidth int) []Glyph {
	glyphs := make([]Glyph, 0, len(line))
	for g := range Glyphs(line, tabWidth) {
		glyphs = append(glyphs, g)
	}
	return glyphs
}

// Glyphs yields the glyphs of Layout one at a time, from the start of the
// line. Breaking out of the loop stops the scan, so finding the glyph at a
// column costs time proportional to that column, not to the line length,
// and allocates nothing.
func Glyphs(line []byte, tabWidth int) iter.Seq[Glyph] {
	return func(yield func(Glyph) bool) {
		off, col, state := 0, 0, -1
		for off < len(line) {
			// Printable ASCII next to ASCII: one cell, no uniseg (see eachCluster).
			if b := line[off]; b > ' ' && b < 0x7f && asciiBoundary(line, off+1) {
				if !yield(Glyph{Off: off, Len: 1, Col: col, Width: 1, Kind: GlyphText}) {
					return
				}
				off++
				col++
				state = -1
				continue
			}
			var cluster []byte
			var width int
			if b := line[off]; b < utf8.RuneSelf && asciiBoundary(line, off+1) {
				cluster, width, state = line[off:off+1], 0, -1
			} else {
				cluster, _, width, state = uniseg.FirstGraphemeCluster(line[off:], state)
			}
			g := Glyph{Off: off, Len: len(cluster), Col: col, Width: width, Kind: GlyphText}
			switch {
			case len(cluster) == 1 && cluster[0] == ' ':
				g.Kind = GlyphSpace
				g.Width = 1
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
			if !yield(g) {
				return
			}
			off += len(cluster)
			col += g.Width
		}
	}
}

// VisualCol returns the screen column of byte column col in line.
func VisualCol(line []byte, col, tabWidth int) int {
	vcol := 0
	for g := range Glyphs(line, tabWidth) {
		if g.Off >= col {
			break
		}
		vcol = g.Col + g.Width
	}
	return vcol
}

// OffsetAtCol returns the byte column of the glyph covering screen column
// vcol, or len(line) when vcol is past the end of the line.
func OffsetAtCol(line []byte, vcol, tabWidth int) int {
	for g := range Glyphs(line, tabWidth) {
		if vcol < g.Col+g.Width {
			return g.Off
		}
	}
	return len(line)
}

// asciiBoundary reports whether there is a grapheme cluster boundary before
// byte i of line, judging only from its neighbors: true when i is at either
// end of the line, or when the bytes on both sides are ASCII. Two ASCII
// bytes are always split, because the only ASCII pair that is one cluster,
// "\r\n", never occurs inside a line. false means "unknown", not "no".
func asciiBoundary(line []byte, i int) bool {
	if i <= 0 || i >= len(line) {
		return true
	}
	return line[i-1] < utf8.RuneSelf && line[i] < utf8.RuneSelf
}

// prevBoundary returns the start of the grapheme cluster before byte column col.
func prevBoundary(line []byte, col int) int {
	if col > 0 && col <= len(line) && line[col-1] < utf8.RuneSelf && asciiBoundary(line, col-1) {
		return col - 1
	}
	prev := 0
	forClusters(line, func(start, _ int) bool {
		if start >= col {
			return false
		}
		prev = start
		return true
	})
	return prev
}

// nextBoundary returns the end of the grapheme cluster starting at or
// containing byte column col.
func nextBoundary(line []byte, col int) int {
	if col >= 0 && col < len(line) && line[col] < utf8.RuneSelf && asciiBoundary(line, col) && asciiBoundary(line, col+1) {
		return col + 1
	}
	next := len(line)
	forClusters(line, func(_, end int) bool {
		if end > col {
			next = end
			return false
		}
		return true
	})
	return next
}

// snapBoundary returns the start of the grapheme cluster containing col.
func snapBoundary(line []byte, col int) int {
	if col >= 0 && col < len(line) && line[col] < utf8.RuneSelf && asciiBoundary(line, col) {
		return col
	}
	snap := 0
	forClusters(line, func(start, _ int) bool {
		if start > col {
			return false
		}
		snap = start
		return true
	})
	return min(snap, len(line))
}

func forClusters(line []byte, fn func(start, end int) bool) {
	eachCluster(line, func(start, end, _ int) bool { return fn(start, end) })
}

// eachCluster calls fn with the byte range and uniseg width of every
// grapheme cluster of line, in order, until fn returns false. An ASCII byte
// followed by another ASCII byte (or by the end of the line) is a cluster of
// its own (see asciiBoundary), so it is handled without uniseg; minified
// JSON or long log lines are scanned many times faster this way. The uniseg
// state restarts after such a byte, which is exact because a boundary
// follows it.
func eachCluster(line []byte, fn func(start, end, width int) bool) {
	off, state := 0, -1
	for off < len(line) {
		if b := line[off]; b < utf8.RuneSelf && asciiBoundary(line, off+1) {
			width := 1
			if b < 0x20 || b == 0x7f {
				width = 0
			}
			if !fn(off, off+1, width) {
				return
			}
			off++
			state = -1
			continue
		}
		var cluster []byte
		var width int
		cluster, _, width, state = uniseg.FirstGraphemeCluster(line[off:], state)
		if !fn(off, off+len(cluster), width) {
			return
		}
		off += len(cluster)
	}
}

type charClass uint8

const (
	classSpace charClass = iota
	classWord
	classPunct
)

func classOf(cluster []byte) charClass {
	r, _ := utf8.DecodeRune(cluster)
	switch {
	case unicode.IsSpace(r):
		return classSpace
	case r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r):
		return classWord
	default:
		return classPunct
	}
}

// IndentCol returns the screen column of the first glyph of line that is not
// a space or tab, and false when the line is blank.
func IndentCol(line []byte, tabWidth int) (int, bool) {
	for g := range Glyphs(line, tabWidth) {
		if g.Kind != GlyphSpace && g.Kind != GlyphTab {
			return g.Col, true
		}
	}
	return 0, false
}

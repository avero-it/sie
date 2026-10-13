package editor

import (
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
// take two cells, controls are made visible.
func Layout(line []byte, tabWidth int) []Glyph {
	glyphs := make([]Glyph, 0, len(line))
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

// VisualCol returns the screen column of byte column col in line.
func VisualCol(line []byte, col, tabWidth int) int {
	vcol := 0
	for _, g := range Layout(line, tabWidth) {
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
	for _, g := range Layout(line, tabWidth) {
		if vcol < g.Col+g.Width {
			return g.Off
		}
	}
	return len(line)
}

// prevBoundary returns the start of the grapheme cluster before byte column col.
func prevBoundary(line []byte, col int) int {
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
	off, state := 0, -1
	rest := line
	for len(rest) > 0 {
		var cluster []byte
		cluster, rest, _, state = uniseg.FirstGraphemeCluster(rest, state)
		if !fn(off, off+len(cluster)) {
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

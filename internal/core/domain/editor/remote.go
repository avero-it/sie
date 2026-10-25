package editor

import "github.com/rivo/uniseg"

// Slice returns a copy of the text in [start, end), clamped to the document.
func (d *Document) Slice(start, end int) ([]byte, error) {
	start = max(0, min(start, d.buf.Len()))
	end = max(start, min(end, d.buf.Len()))
	return d.buf.Slice(start, end)
}

// LineOffset returns the offset where line (0-based) starts; a line past
// the end gives the document length.
func (d *Document) LineOffset(line int) int {
	if line < 0 {
		return 0
	}
	return d.lineStart(line)
}

// OffsetAtGrapheme converts a 0-based line and 0-based grapheme column to a
// document offset. A column equal to the number of graphemes is the line end.
// It reports false when the line or the column does not exist.
func (d *Document) OffsetAtGrapheme(line, gcol int) (int, bool) {
	if line < 0 || line >= d.LineCount() || gcol < 0 {
		return 0, false
	}
	start, _ := d.lineRange(line)
	text := d.Line(line)
	if gcol == 0 {
		return start, true
	}
	n, at := 0, 0
	forClusters(text, func(_, end int) bool {
		n++
		at = end
		return n < gcol
	})
	if n < gcol {
		return 0, false
	}
	return start + at, true
}

// GraphemePos converts a document offset to a 0-based line and a 0-based
// grapheme column.
func (d *Document) GraphemePos(off int) (line, gcol int) {
	off = max(0, min(off, d.buf.Len()))
	line = d.lineOf(off)
	start, _ := d.lineRange(line)
	text := d.Line(line)
	col := min(off-start, len(text))
	return line, uniseg.GraphemeClusterCount(string(text[:col]))
}

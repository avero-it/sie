package editor

import (
	"bytes"
	"fmt"
	"slices"
)

// rectState remembers a rectangle made by SelectRect. The corners are kept in
// virtual screen columns, which may lie past the end of a short line, so the
// rectangle can be extended from the same corners. sels is what SelectRect
// produced: the rectangle is only active while the document's selections are
// still exactly those (see activeRect).
type rectState struct {
	aLine, aVCol int // anchor corner
	hLine, hVCol int // head corner, the one that moves
	sels         []Selection
}

// SelectRect selects the rectangle between two corners given as line and
// screen column (e.g. from the mouse). Every line of the rectangle gets one
// selection from the left to the right column, so the result is ordinary
// multiple cursors: typing, Backspace, Delete and undo act on all of them. A
// line shorter than the left edge gets an empty cursor at its end. A glyph is
// inside when its first column is in [left, right) (see OffsetAtCol). The
// head line's selection is the primary one, with the cursor on the side that
// was dragged. Lines are clamped to the document.
func (d *Document) SelectRect(aLine, aVCol, hLine, hVCol int) {
	d.hist.Seal()
	d.goal = -1
	last := max(0, d.LineCount()-1)
	aLine, hLine = max(0, min(aLine, last)), max(0, min(hLine, last))
	aVCol, hVCol = max(0, aVCol), max(0, hVCol)
	left, right := min(aVCol, hVCol), max(aVCol, hVCol)

	// Read the whole block once and walk its lines: asking the buffer for
	// each line's offset would make tall rectangles quadratic.
	lo, hi := min(aLine, hLine), max(aLine, hLine)
	base := d.lineStart(lo)
	_, blockEnd := d.lineRange(hi)
	block, _ := d.buf.Slice(base, blockEnd)

	var primary Selection
	extra := make([]Selection, 0, hi-lo)
	pos := 0
	for line := lo; line <= hi; line++ {
		end := len(block)
		if nl := bytes.IndexByte(block[pos:], '\n'); nl >= 0 {
			end = pos + nl
		}
		text := bytes.TrimSuffix(block[pos:end], []byte("\r"))
		glyphs := Layout(text, d.tabWidth)
		sel := Selection{Anchor: base + pos + offsetIn(glyphs, left, len(text)), Head: base + pos + offsetIn(glyphs, right, len(text))}
		if hVCol < aVCol {
			sel.Anchor, sel.Head = sel.Head, sel.Anchor
		}
		if line == hLine {
			primary = sel
		} else {
			extra = append(extra, sel)
		}
		pos = end + 1
	}
	d.sel, d.extra = primary, extra
	d.rect = &rectState{aLine, aVCol, hLine, hVCol, d.Selections()}
}

// ExtendRect moves the head corner of the rectangle by dLine lines and dVCol
// columns and reselects it. Without an active rectangle it starts one at the
// cursor. This is how column mode grows a selection from the keyboard.
func (d *Document) ExtendRect(dLine, dVCol int) {
	r := d.activeRect()
	if r == nil {
		line, _, vcol := d.Position(d.sel.Head)
		r = &rectState{aLine: line, aVCol: vcol, hLine: line, hVCol: vcol}
	}
	d.SelectRect(r.aLine, r.aVCol, r.hLine+dLine, r.hVCol+dVCol)
}

// activeRect returns the rectangle if the selections are still the ones it
// made; any other change of selection or text makes it inactive.
func (d *Document) activeRect() *rectState {
	if d.rect != nil && slices.Equal(d.rect.sels, d.Selections()) {
		return d.rect
	}
	return nil
}

// SelectedTexts returns the text of every selection, from the top of the
// document to the bottom. Empty selections give empty strings, so a
// rectangle keeps its shape.
func (d *Document) SelectedTexts() []string {
	all := d.Selections()
	slices.SortFunc(all, cmpSel)
	out := make([]string, len(all))
	for i, s := range all {
		start, end := s.Range()
		data, _ := d.buf.Slice(start, end)
		out[i] = string(data)
	}
	return out
}

// InsertEach inserts parts[i] at the i-th cursor in document order, replacing
// its selection, as one undo step. There must be one part per cursor.
func (d *Document) InsertEach(parts []string) error {
	if len(parts) != d.CursorCount() {
		return fmt.Errorf("edit: %d parts for %d cursors", len(parts), d.CursorCount())
	}
	return d.forEachCursor(func() error { return d.insert(parts[d.rank]) })
}

// offsetIn is OffsetAtCol on an already computed layout: the byte column of
// the glyph covering screen column vcol, or n (the line length) past the end.
func offsetIn(glyphs []Glyph, vcol, n int) int {
	for _, g := range glyphs {
		if vcol < g.Col+g.Width {
			return g.Off
		}
	}
	return n
}

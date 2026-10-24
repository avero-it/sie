package editor

// Seg is one screen row of a wrapped line: the glyphs [First, End) and the
// screen columns [StartCol, EndCol) they cover in the unwrapped line.
type Seg struct {
	First, End       int
	StartCol, EndCol int
}

// Segments splits laid-out glyphs into rows of at most cols cells. A row
// breaks after the last space or tab that fits; a word longer than a row is
// broken before the glyph that overflows. A glyph is never split. When the
// text fills its last row exactly, an empty row follows so that the cursor
// at the end of the line has a cell. There is always at least one segment.
// With cols < 2 the line is not wrapped.
func Segments(glyphs []Glyph, cols int) []Seg {
	if cols < 2 || len(glyphs) == 0 {
		end := 0
		if n := len(glyphs); n > 0 {
			end = glyphs[n-1].Col + glyphs[n-1].Width
		}
		return []Seg{{First: 0, End: len(glyphs), EndCol: end}}
	}
	var segs []Seg
	start := 0
	for start < len(glyphs) {
		base := glyphs[start].Col
		end, lastBreak := start, -1
		for end < len(glyphs) && glyphs[end].Col+glyphs[end].Width-base <= cols {
			if k := glyphs[end].Kind; k == GlyphSpace || k == GlyphTab {
				lastBreak = end + 1
			}
			end++
		}
		if end < len(glyphs) { // overflow: choose where to break
			switch {
			case end == start:
				end = start + 1 // a glyph wider than the row
			case lastBreak > start:
				end = lastBreak
			}
		}
		last := glyphs[end-1]
		segs = append(segs, Seg{First: start, End: end, StartCol: base, EndCol: last.Col + last.Width})
		start = end
	}
	if last := segs[len(segs)-1]; last.EndCol-last.StartCol >= cols {
		segs = append(segs, Seg{First: len(glyphs), End: len(glyphs), StartCol: last.EndCol, EndCol: last.EndCol})
	}
	return segs
}

// segIndex returns the segment showing screen column vcol and the cell of it
// within that row. A column past the end belongs to the last segment.
func segIndex(segs []Seg, vcol int) (seg, x int) {
	for i, s := range segs {
		if vcol < s.EndCol {
			return i, vcol - s.StartCol
		}
	}
	last := len(segs) - 1
	return last, vcol - segs[last].StartCol
}

// WrapRows returns how many screen rows line takes when wrapped to cols cells.
func (d *Document) WrapRows(line, cols int) int {
	return len(Segments(Layout(d.Line(line), d.tabWidth), cols))
}

// WrapPos converts a document offset to its line, wrapped row within the
// line, and cell within that row.
func (d *Document) WrapPos(off, cols int) (line, seg, x int) {
	line, _, vcol := d.Position(off)
	seg, x = segIndex(Segments(Layout(d.Line(line), d.tabWidth), cols), vcol)
	return line, seg, x
}

// OffsetAtRow converts a wrapped row of a line and a cell in it (e.g. a mouse
// click) to a document offset. A cell past the end of a row that continues
// lands on the row's last glyph; past the end of the line, on the line end.
func (d *Document) OffsetAtRow(line, seg, x, cols int) int {
	if line < 0 {
		return 0
	}
	if line >= d.LineCount() {
		return d.buf.Len()
	}
	start, _ := d.lineRange(line)
	text := d.Line(line)
	glyphs := Layout(text, d.tabWidth)
	segs := Segments(glyphs, cols)
	seg = max(0, min(seg, len(segs)-1))
	s := segs[seg]
	vcol := s.StartCol + max(0, x)
	for _, g := range glyphs[s.First:s.End] {
		if vcol < g.Col+g.Width {
			return start + g.Off
		}
	}
	if seg == len(segs)-1 {
		return start + len(text)
	}
	return start + glyphs[s.End-1].Off
}

// MoveRows moves every cursor by rows screen rows of text wrapped to cols
// cells (negative is up), keeping the preferred cell within the row.
func (d *Document) MoveRows(rows, cols int, extend bool) {
	d.hist.Seal()
	d.eachCursorMove(func() { d.setHead(d.rowsFrom(d.sel.Head, rows, cols), extend) })
}

func (d *Document) rowsFrom(off, delta, cols int) int {
	line, seg, x := d.WrapPos(off, cols)
	if d.goal < 0 {
		d.goal = x
	}
	segs := Segments(Layout(d.Line(line), d.tabWidth), cols)
	for ; delta < 0; delta++ {
		switch {
		case seg > 0:
			seg--
		case line > 0:
			line--
			segs = Segments(Layout(d.Line(line), d.tabWidth), cols)
			seg = len(segs) - 1
		default:
			return 0
		}
	}
	for ; delta > 0; delta-- {
		switch {
		case seg < len(segs)-1:
			seg++
		case line < d.LineCount()-1:
			line++
			segs = Segments(Layout(d.Line(line), d.tabWidth), cols)
			seg = 0
		default:
			return d.buf.Len()
		}
	}
	return d.OffsetAtRow(line, seg, d.goal, cols)
}

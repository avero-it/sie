package editor

// Viewport is the visible window over a document: Top line and Left screen
// column of the text area, and its size in Rows and Cols.
type Viewport struct {
	Top, Left  int
	Skip       int // wrapped row of Top shown first; only used while wrapping
	Rows, Cols int
	// ScrollOff is the number of lines kept visible above and below the
	// cursor; SideScrollOff the number of columns left and right of it.
	ScrollOff, SideScrollOff int
}

// Resize sets the text area size.
func (v *Viewport) Resize(rows, cols int) {
	v.Rows, v.Cols = max(0, rows), max(0, cols)
}

// Follow scrolls the minimum amount needed to show the cursor at (line, vcol)
// with the configured margins.
func (v *Viewport) Follow(line, vcol int) {
	if v.Rows > 0 {
		m := min(v.ScrollOff, (v.Rows-1)/2)
		if line < v.Top+m {
			v.Top = max(0, line-m)
		}
		if line > v.Top+v.Rows-1-m {
			v.Top = line - v.Rows + 1 + m
		}
	}
	if v.Cols > 0 {
		m := min(v.SideScrollOff, (v.Cols-1)/2)
		if vcol < v.Left+m {
			v.Left = max(0, vcol-m)
		}
		if vcol > v.Left+v.Cols-1-m {
			v.Left = vcol - v.Cols + 1 + m
		}
	}
}

// ScrollBy moves the viewport by delta lines (e.g. mouse wheel) without
// moving the cursor, keeping at least the last line visible.
func (v *Viewport) ScrollBy(delta, lineCount int) {
	v.Top = max(0, min(v.Top+delta, lineCount-1))
}

// The wrapped variants below treat the viewport as a window over screen rows:
// Top is a line and Skip the wrapped row of it shown first. rowsOf reports
// how many rows a line takes.

// rowDist returns how many rows below the first visible row (line, seg) is,
// or -1 above it. Counting stops soon after the window's height.
func (v *Viewport) rowDist(line, seg int, rowsOf func(int) int) int {
	if line < v.Top || (line == v.Top && seg < v.Skip) {
		return -1
	}
	if line == v.Top {
		return seg - v.Skip
	}
	n := rowsOf(v.Top) - v.Skip
	for l := v.Top + 1; l < line && n <= v.Rows; l++ {
		n += rowsOf(l)
	}
	return n + seg
}

// rowsBack returns the row n rows above (line, seg), stopping at the top.
func rowsBack(line, seg, n int, rowsOf func(int) int) (int, int) {
	for n > 0 {
		if seg > 0 {
			take := min(seg, n)
			seg -= take
			n -= take
			continue
		}
		if line == 0 {
			return 0, 0
		}
		line--
		seg = rowsOf(line)
	}
	return line, seg
}

// RowOf returns the window row showing (line, seg); it is negative above the
// window and may be >= Rows below it.
func (v *Viewport) RowOf(line, seg int, rowsOf func(int) int) int {
	return v.rowDist(line, seg, rowsOf)
}

// FollowWrap scrolls the minimum amount needed to show row seg of line, with
// the vertical margin. There is no horizontal scroll while wrapping.
func (v *Viewport) FollowWrap(line, seg, lineCount int, rowsOf func(int) int) {
	v.Left = 0
	if v.Rows <= 0 {
		return
	}
	v.Top = max(0, min(v.Top, lineCount-1))
	v.Skip = max(0, min(v.Skip, rowsOf(v.Top)-1))
	m := min(v.ScrollOff, (v.Rows-1)/2)
	switch d := v.rowDist(line, seg, rowsOf); {
	case d < m:
		v.Top, v.Skip = rowsBack(line, seg, m, rowsOf)
	case d > v.Rows-1-m:
		v.Top, v.Skip = rowsBack(line, seg, v.Rows-1-m, rowsOf)
	}
}

// ScrollRows moves the window by delta rows without moving the cursor.
func (v *Viewport) ScrollRows(delta, lineCount int, rowsOf func(int) int) {
	if delta < 0 {
		v.Top, v.Skip = rowsBack(v.Top, v.Skip, -delta, rowsOf)
		return
	}
	for ; delta > 0; delta-- {
		switch {
		case v.Skip+1 < rowsOf(v.Top):
			v.Skip++
		case v.Top < lineCount-1:
			v.Top, v.Skip = v.Top+1, 0
		default:
			return
		}
	}
}

// LineAt maps a text-area row to a document line (may be past the end).
func (v *Viewport) LineAt(row int) int { return v.Top + row }

// ColAt maps a text-area column to a screen column of the line.
func (v *Viewport) ColAt(col int) int { return v.Left + col }

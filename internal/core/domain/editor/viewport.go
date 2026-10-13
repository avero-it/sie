package editor

// Viewport is the visible window over a document: Top line and Left screen
// column of the text area, and its size in Rows and Cols.
type Viewport struct {
	Top, Left  int
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

// LineAt maps a text-area row to a document line (may be past the end).
func (v *Viewport) LineAt(row int) int { return v.Top + row }

// ColAt maps a text-area column to a screen column of the line.
func (v *Viewport) ColAt(col int) int { return v.Left + col }

package app

import "github.com/avero-it/sie/internal/core/ports"

// rectDrag is a mouse press that may turn into a rectangle: the anchor corner
// as a line and screen column, and the offset it points to.
type rectDrag struct {
	line, vcol int
	off        int
	alt        bool // started with Alt: a click without a drag toggles a cursor
	moved      bool
}

// dragRect reselects the rectangle between the anchor and the cell the mouse
// is over.
func (e *Editor) dragRect(line, vcol int) {
	d := e.rectDrag
	if !d.moved && line == d.line && vcol == d.vcol {
		return // still on the pressed cell: this may be a click
	}
	d.moved = true
	e.doc.SelectRect(d.line, d.vcol, line, vcol)
	e.follow()
}

// endRectDrag finishes a mouse press made for a rectangle. An Alt press that
// never moved is an Alt+click: it adds a cursor, or removes one.
func (e *Editor) endRectDrag() {
	d := e.rectDrag
	e.rectDrag = nil
	if d != nil && d.alt && !d.moved {
		e.doc.AddCursor(d.off)
		e.follow()
	}
}

// extendRect grows the rectangle with a Shift+arrow in column mode. It
// reports whether the key was one of those arrows.
func (e *Editor) extendRect(k ports.Key) bool {
	switch k {
	case ports.KeyLeft:
		e.doc.ExtendRect(0, -1)
	case ports.KeyRight:
		e.doc.ExtendRect(0, 1)
	case ports.KeyUp:
		e.doc.ExtendRect(-1, 0)
	case ports.KeyDown:
		e.doc.ExtendRect(1, 0)
	default:
		return false
	}
	return true
}

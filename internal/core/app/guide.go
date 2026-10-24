package app

import (
	"time"

	"github.com/avero-it/sie/internal/core/domain/editor"
	"github.com/avero-it/sie/internal/core/ports"
)

// guideDuration is how long the indent guide stays on screen.
const guideDuration = 300 * time.Millisecond

// guideTintLevel is the Tint level of the gray guide background.
const guideTintLevel = 4

// indentState is the cursor line's indent as of the last input event.
type indentState struct {
	version uint64
	col     int
	ok      bool // the line has a non-blank glyph
}

// guideState is a visible indent guide: a column and when it started.
type guideState struct {
	col   int
	start time.Time
	on    bool
}

// trackIndent runs after every input event. When an edit moved the cursor
// line's first non-blank glyph to another column, it starts the indent guide.
// Moving the cursor and edits of blank lines never do.
func (e *Editor) trackIndent() {
	if !e.indentGuide || e.wrap {
		return
	}
	line, _, _ := e.doc.CursorPos()
	col, ok := editor.IndentCol(e.doc.Line(line), e.doc.TabWidth())
	cur := indentState{version: e.doc.Version(), col: col, ok: ok}
	if cur.version != e.indent.version && cur.ok && e.indent.ok && cur.col != e.indent.col {
		e.guide = guideState{col: cur.col, start: e.now(), on: true}
	}
	e.indent = cur
}

// pruneGuide drops a finished guide.
func (e *Editor) pruneGuide() {
	if e.guide.on && e.now().Sub(e.guide.start) >= guideDuration {
		e.guide.on = false
	}
}

// drawGuide draws the guide column over the text rows. A line that starts at
// the column shows its first glyph in reverse, other text is only tinted, and
// every other cell (blank, short or missing lines) shows a gray bar.
func (e *Editor) drawGuide(vm *viewBuilder, r regions) {
	e.pruneGuide()
	if !e.guide.on || e.wrap {
		return
	}
	cols := e.width - r.gutter
	x := e.guide.col - e.view.Left
	if x < 0 || x >= cols {
		return
	}
	vm.Animate = true
	tw := e.doc.TabWidth()
	for row := 0; row < r.textRows; row++ {
		line, y := e.view.Top+row, r.textTop+row
		if line >= e.doc.LineCount() {
			vm.add(r.gutter+x, y, "|", ports.StyleGuide)
			continue
		}
		text := e.doc.Line(line)
		var at *editor.Glyph
		glyphs := editor.Layout(text, tw)
		for i, g := range glyphs {
			if g.Col <= e.guide.col && e.guide.col < g.Col+g.Width {
				at = &glyphs[i]
				break
			}
		}
		top, hasTop := editor.IndentCol(text, tw)
		switch {
		case at == nil || at.Kind == editor.GlyphSpace || at.Kind == editor.GlyphTab:
			vm.add(r.gutter+x, y, "|", ports.StyleGuide)
		case hasTop && top == at.Col && at.Col == e.guide.col && x+at.Width <= cols:
			vm.add(r.gutter+x, y, at.Display(text), ports.StyleGuideMatch)
		default:
			vm.Tints = append(vm.Tints, ports.Tint{X: r.gutter + x, Y: y, Level: guideTintLevel})
		}
	}
}

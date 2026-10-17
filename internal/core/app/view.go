package app

import (
	"fmt"
	"strings"

	"github.com/avero-it/sie/internal/core/domain/editor"
	"github.com/avero-it/sie/internal/core/ports"
	"github.com/rivo/uniseg"
)

// Screen layout, top to bottom: menu bar (1 row), text area (rest), prompt
// line (1 row), status line (1 row).
type regions struct {
	menuY, textTop, textRows, promptY, statusY int
	gutter                                     int // line-number column width, incl. trailing space
}

func (e *Editor) regions() regions {
	return regions{
		menuY:    0,
		textTop:  1,
		textRows: max(0, e.height-3),
		promptY:  e.height - 2,
		statusY:  e.height - 1,
		gutter:   len(fmt.Sprint(e.doc.LineCount())) + 2,
	}
}

func (e *Editor) layout() {
	r := e.regions()
	e.view.Resize(r.textRows, e.width-r.gutter)
}

// rect is a screen rectangle; popups list their rows from first.
type rect struct{ x, y, w, h int }

func (r rect) contains(x, y int) bool { return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h }

// menuTitleSpans returns the x range of each menu title in the menu bar.
func (e *Editor) menuTitleSpans() []rect {
	spans := make([]rect, len(e.menus))
	x := 0
	for i, m := range e.menus {
		w := uniseg.StringWidth(m.Title) + 2
		spans[i] = rect{x: x, y: 0, w: w, h: 1}
		x += w
	}
	return spans
}

// dropdownRect returns the open menu's box; row i is item i.
func (e *Editor) dropdownRect() (rect, int) {
	m := e.menus[e.menu.open]
	labelW, hintW := 0, 0
	for _, id := range m.Items {
		labelW = max(labelW, uniseg.StringWidth(e.menuLabel(id)))
		hintW = max(hintW, uniseg.StringWidth(e.shortcut(id)))
	}
	w := labelW + 2
	if hintW > 0 {
		w += hintW + 3
	}
	x := e.menuTitleSpans()[e.menu.open].x
	x = max(0, min(x, e.width-w))
	return rect{x: x, y: 1, w: w, h: len(m.Items)}, labelW
}

// completionRows is how many completion rows fit above the prompt line.
func (e *Editor) completionRows() int {
	r := e.regions()
	limit := maxCompletions
	if e.prompt.browse || e.prompt.recent {
		limit = maxFileRows
	}
	return max(0, min(len(e.prompt.items), limit, r.promptY-r.textTop))
}

func (e *Editor) completionRect() rect {
	rows := e.completionRows()
	w := 0
	for _, it := range e.prompt.items {
		w = max(w, uniseg.StringWidth(it.label)+uniseg.StringWidth(it.hint)+5)
	}
	return rect{x: 0, y: e.regions().promptY - rows, w: min(w, e.width), h: rows}
}

func (e *Editor) showCompletions() bool {
	return e.focus == focusPrompt && (e.prompt.mode == modeCommand || e.prompt.browse || e.prompt.recent) &&
		len(e.prompt.items) > 0
}

// HandleMouse implements clicks on menus, completions, the prompt line and
// the text area (click, shift-click and drag select; Alt+click adds a
// cursor; wheel scrolls).
func (e *Editor) HandleMouse(m ports.MouseEvent) {
	if e.focus == focusEditor && m.Action == ports.MousePress {
		e.clearMessage()
	}
	e.handleMouse(m)
	e.syncHighlight()
	e.trackIndent()
}

func (e *Editor) handleMouse(m ports.MouseEvent) {
	r := e.regions()
	switch m.Button {
	case ports.WheelUp, ports.WheelDown:
		delta := 3
		if m.Button == ports.WheelUp {
			delta = -3
		}
		if e.showCompletions() && e.completionRect().contains(m.X, m.Y) {
			e.selectCompletion(e.prompt.selected + delta/3)
			return
		}
		e.view.ScrollBy(delta, e.doc.LineCount())
		return
	}
	switch m.Action {
	case ports.MouseRelease:
		e.dragging = false
		e.dragUnit = dragChar
		e.endRectDrag()
		return
	case ports.MouseDrag:
		if e.rectDrag != nil {
			line, vcol := e.textCellAt(m.X, m.Y, r)
			e.dragRect(line, vcol)
			return
		}
		if e.dragging && e.dragUnit != dragChar {
			e.dragByUnit(e.textOffsetAt(m.X, m.Y, r))
			e.follow()
		} else if e.dragging {
			e.doc.SetSelection(editor.Selection{Anchor: e.doc.Selection().Anchor, Head: e.textOffsetAt(m.X, m.Y, r)})
			e.follow()
		}
		return
	}
	if m.Button != ports.MouseLeft {
		return
	}

	if e.focus == focusMenu {
		if box, _ := e.dropdownRect(); box.contains(m.X, m.Y) {
			e.menu.item = m.Y - box.y
			e.runMenuItem(e.menu.item)
			return
		}
		if m.Y == r.menuY {
			if i := e.menuTitleAt(m.X); i >= 0 && i != e.menu.open {
				e.openMenu(i)
				return
			}
		}
		e.focus = focusEditor // click elsewhere closes the menu
		return
	}
	if m.Y == r.menuY {
		if i := e.menuTitleAt(m.X); i >= 0 {
			e.openMenu(i)
		}
		return
	}
	if e.showCompletions() {
		if box := e.completionRect(); box.contains(m.X, m.Y) {
			e.prompt.selected = e.prompt.top + m.Y - box.y
			e.submitPrompt()
			return
		}
	}
	if m.Y == r.promptY {
		if e.focus != focusPrompt {
			e.openPrompt()
		}
		return
	}
	if m.Y >= r.textTop && m.Y < r.textTop+r.textRows {
		if e.focus == focusPrompt {
			e.closePrompt()
		}
		off := e.textOffsetAt(m.X, m.Y, r)
		e.startRipple(m.X, m.Y)
		if m.Mod&ports.ModAlt != 0 {
			// Alt+click adds a cursor (or removes one) when the button is
			// released; Alt+drag makes a rectangle instead.
			line, vcol := e.textCellAt(m.X, m.Y, r)
			e.rectDrag = &rectDrag{line: line, vcol: vcol, off: off, alt: true}
			return
		}
		if e.columnMode && m.Mod&ports.ModShift == 0 {
			line, vcol := e.textCellAt(m.X, m.Y, r)
			e.doc.SetSelection(editor.Cursor(off))
			e.rectDrag = &rectDrag{line: line, vcol: vcol, off: off}
			e.follow()
			return
		}
		e.dragUnit = dragChar
		if m.Mod&ports.ModShift != 0 {
			e.doc.SetSelection(editor.Selection{Anchor: e.doc.Selection().Anchor, Head: off})
		} else if m.Clicks == 2 {
			e.dragUnit = dragWord
			e.dragFrom, e.dragTo = e.doc.WordRange(off)
			e.doc.SetSelection(editor.Selection{Anchor: e.dragFrom, Head: e.dragTo})
		} else if m.Clicks >= 3 {
			e.dragUnit = dragLine
			e.dragFrom, e.dragTo = e.doc.LineRange(off)
			e.doc.SetSelection(editor.Selection{Anchor: e.dragFrom, Head: e.dragTo})
		} else {
			e.doc.SetSelection(editor.Cursor(off))
		}
		e.dragging = true
		e.follow()
	}
}

func (e *Editor) menuTitleAt(x int) int {
	for i, s := range e.menuTitleSpans() {
		if s.contains(x, 0) {
			return i
		}
	}
	return -1
}

func (e *Editor) textOffsetAt(x, y int, r regions) int {
	line, vcol := e.textCellAt(x, y, r)
	return e.doc.OffsetAt(line, vcol)
}

// textCellAt maps a screen cell to a document line and screen column. The
// column is not clamped to the line, so it can lie past a short line's end.
func (e *Editor) textCellAt(x, y int, r regions) (line, vcol int) {
	row := max(0, min(y-r.textTop, r.textRows-1))
	return e.view.LineAt(row), e.view.ColAt(max(0, x-r.gutter))
}

// View renders the current state.
func (e *Editor) View() ports.ViewModel {
	e.layout()
	vm := &viewBuilder{ViewModel: ports.ViewModel{Width: e.width, Height: e.height}}
	if e.width <= 0 || e.height <= 0 {
		return vm.ViewModel
	}
	r := e.regions()
	e.drawMenuBar(vm, r)
	e.drawText(vm, r)
	e.drawRipples(vm, r)
	e.drawGuide(vm, r)
	e.drawStatus(vm, r)
	e.drawPrompt(vm, r)
	switch {
	case e.focus == focusMenu:
		e.drawDropdown(vm)
	case e.showCompletions():
		e.drawCompletions(vm)
	}
	return vm.ViewModel
}

type viewBuilder struct{ ports.ViewModel }

func (vm *viewBuilder) add(x, y int, text string, style ports.Style) {
	if y < 0 || y >= vm.Height || x >= vm.Width || text == "" {
		return
	}
	vm.Runs = append(vm.Runs, ports.Run{X: x, Y: y, Text: text, Style: style})
}

func (vm *viewBuilder) fill(y int, style ports.Style) {
	vm.add(0, y, strings.Repeat(" ", vm.Width), style)
}

// row draws left- and right-aligned text on a filled row; the left text is
// truncated so it never runs under the right text.
func (vm *viewBuilder) row(y int, left, right string, style, rightStyle ports.Style) {
	vm.fill(y, style)
	avail := vm.Width
	if right != "" {
		rx := vm.Width - uniseg.StringWidth(right)
		avail = rx - 1
		vm.add(rx, y, right, rightStyle)
	}
	vm.add(0, y, truncate(left, avail), style)
}

func (e *Editor) drawMenuBar(vm *viewBuilder, r regions) {
	vm.fill(r.menuY, ports.StyleMenuBar)
	for i, s := range e.menuTitleSpans() {
		style := ports.StyleMenuBar
		if e.focus == focusMenu && e.menu.open == i {
			style = ports.StyleMenuTitleActive
		}
		vm.add(s.x, r.menuY, " "+e.menus[i].Title+" ", style)
	}
}

func (e *Editor) drawText(vm *viewBuilder, r regions) {
	cols := e.width - r.gutter
	sels := e.doc.Selections()
	selected := func(off int) bool {
		for _, s := range sels {
			if start, end := s.Range(); off >= start && off < end {
				return true
			}
		}
		return false
	}
	secondary := map[int]bool{} // heads of the secondary cursors
	for _, s := range sels[1:] {
		secondary[s.Head] = true
	}
	curLine, _, curVcol := e.doc.CursorPos()
	tw := e.doc.TabWidth()
	for row := 0; row < r.textRows; row++ {
		line, y := e.view.Top+row, r.textTop+row
		if line >= e.doc.LineCount() {
			e.drawEmptyCells(vm, r, y, 0)
			continue
		}
		gutterStyle := ports.StyleGutter
		if line == curLine {
			gutterStyle = ports.StyleGutterCurrent
		}
		vm.add(0, y, fmt.Sprintf("%*d ", r.gutter-1, line+1), gutterStyle)

		text := e.doc.Line(line)
		start := e.doc.OffsetAt(line, 0)
		tokens := e.cursorAt(start)
		width := 0
		for g := range editor.Glyphs(text, tw) {
			width = g.Col + g.Width
			x := g.Col - e.view.Left
			if x+g.Width <= 0 {
				continue
			}
			if x >= cols {
				break
			}
			disp, style := e.glyphLook(g, text, selected(start+g.Off), tokens.class(start+g.Off))
			if secondary[start+g.Off] {
				style = ports.StyleCursorSecondary
			}
			if x < 0 || x+g.Width > cols {
				// Glyph cut by the viewport edge: show the visible cells blank.
				visible := min(x+g.Width, cols) - max(x, 0)
				disp, x = strings.Repeat(" ", visible), max(x, 0)
			}
			vm.add(r.gutter+x, y, disp, style)
		}
		// A selected line break shows as one selected cell after the text.
		end := start + len(text)
		fill := max(0, width-e.view.Left)
		breakCell := line < e.doc.LineCount()-1 && selected(end)
		switch {
		case secondary[end] && width >= e.view.Left && fill < cols:
			vm.add(r.gutter+fill, y, " ", ports.StyleCursorSecondary)
			fill++
		case breakCell:
			if fill < cols {
				vm.add(r.gutter+fill, y, " ", ports.StyleSelection)
			}
			fill++
		}
		e.drawEmptyCells(vm, r, y, fill)
	}
	if e.focus == focusEditor {
		x, y := r.gutter+curVcol-e.view.Left, r.textTop+curLine-e.view.Top
		vm.Cursor = ports.Cursor{X: x, Y: y, Visible: x >= r.gutter && x < e.width && y >= r.textTop && y < r.textTop+r.textRows}
	}
}

// truncate cuts s to at most cells screen cells, ending with "…" if cut.
func truncate(s string, cells int) string {
	if cells <= 0 {
		return ""
	}
	if uniseg.StringWidth(s) <= cells {
		return s
	}
	var b strings.Builder
	w, state := 0, -1
	for len(s) > 0 {
		var cl string
		var cw int
		cl, s, cw, state = uniseg.FirstGraphemeClusterInString(s, state)
		if w+cw > cells-1 {
			break
		}
		b.WriteString(cl)
		w += cw
	}
	return b.String() + "…"
}

// Faint markers (all one cell wide, East Asian width neutral or Latin-1).
const (
	emptyCellMarker = "·" // a cell holding no character
	spaceMarker     = "␣"
	tabMarker       = "»" // first cell of a tab; the rest stay blank
)

// drawEmptyCells marks the text-area cells of row y from column x (relative
// to the text area) to the right edge, if empty cells are shown.
func (e *Editor) drawEmptyCells(vm *viewBuilder, r regions, y, x int) {
	if cols := e.width - r.gutter; e.showEmptyCells && x < cols {
		vm.add(r.gutter+x, y, strings.Repeat(emptyCellMarker, cols-x), ports.StyleEmptyCell)
	}
}

// glyphLook returns what to draw for g and in which style.
func (e *Editor) glyphLook(g editor.Glyph, line []byte, selected bool, class ports.TokenClass) (string, ports.Style) {
	if e.showInvisibles {
		marker := ""
		switch g.Kind {
		case editor.GlyphSpace:
			marker = spaceMarker
		case editor.GlyphTab:
			marker = tabMarker + strings.Repeat(" ", g.Width-1)
		}
		if marker != "" {
			if selected {
				return marker, ports.StyleWhitespaceSelected
			}
			return marker, ports.StyleWhitespace
		}
	}
	style := glyphStyle(g.Kind, selected)
	if style == ports.StyleDefault {
		style = classStyle(class) // selection and special glyphs win over syntax
	}
	return g.Display(line), style
}

func glyphStyle(k editor.GlyphKind, selected bool) ports.Style {
	special := k == editor.GlyphControl || k == editor.GlyphInvalid
	switch {
	case special && selected:
		return ports.StyleSpecialSelected
	case special:
		return ports.StyleSpecial
	case selected:
		return ports.StyleSelection
	default:
		return ports.StyleDefault
	}
}

func (e *Editor) drawStatus(vm *viewBuilder, r regions) {
	name := e.path
	if name == "" {
		name = "[No Name]"
	}
	if e.doc.Modified() {
		name += " ●"
	}
	line, _, vcol := e.doc.CursorPos()
	enc := "UTF-8"
	if e.doc.HasBOM() {
		enc += " BOM"
	}
	cursors := ""
	if n := e.doc.CursorCount(); n > 1 {
		cursors = fmt.Sprintf(" (%d cursors)", n)
	}
	if e.columnMode {
		cursors += "   COLUMN"
	}
	right := fmt.Sprintf("Ln %d, Col %d%s   %s   %s   %s ", line+1, vcol+1, cursors, e.languageLabel(), enc, e.doc.LineEnding())
	vm.row(r.statusY, " sie "+e.version()+" - "+name, right, ports.StyleStatus, ports.StyleStatus)
}

// idleText fills the prompt line when there is nothing else to show.
const idleText = "write a command…"

func (e *Editor) drawPrompt(vm *viewBuilder, r regions) {
	y := r.promptY
	vm.fill(y, ports.StylePromptBar)
	if e.focus != focusPrompt {
		switch e.msg.kind {
		case msgNone:
			avail := e.width
			if hint := e.shortcut("ui.prompt"); hint != "" {
				hx := e.width - uniseg.StringWidth(hint) - 1
				avail = hx - 1
				vm.add(hx, y, hint, ports.StyleEchoHint)
			}
			vm.add(1, y, truncate(idleText, avail-1), ports.StylePromptIdle)
		case msgEcho:
			avail := e.width
			if e.msg.hint != "" {
				hx := e.width - uniseg.StringWidth(e.msg.hint) - 1
				avail = hx - 1
				vm.add(hx, y, e.msg.hint, ports.StyleEchoHint)
			}
			vm.add(0, y, truncate("> "+e.msg.text, avail), ports.StyleEcho)
		case msgInfo:
			vm.add(0, y, e.msg.text, ports.StyleEcho)
		case msgError:
			vm.add(0, y, e.msg.text, ports.StyleError)
		}
		return
	}
	p := &e.prompt
	var label string
	switch p.mode {
	case modeCommand:
		label = "> "
	case modeArgs:
		label = p.cmd.Args[len(p.args)].Prompt + ": "
	case modeConfirm:
		label = p.question + " (y/n) "
	case modeReplace:
		label = p.question + " (y/n/a/q) "
	case modeChoice:
		// The answers must stay visible: shorten the question instead.
		answers := ""
		for _, c := range p.choices {
			answers += "  " + c.label
		}
		answers += "  Esc "
		label = truncate(p.question, e.width-uniseg.StringWidth(answers)) + answers
	}
	lw := uniseg.StringWidth(label)
	vm.add(0, y, label, ports.StylePromptLabel)
	vm.add(lw, y, p.input.String(), ports.StylePromptInput)
	vm.Cursor = ports.Cursor{X: lw + p.input.CursorCells(), Y: y, Visible: true}
}

func (e *Editor) drawDropdown(vm *viewBuilder) {
	box, labelW := e.dropdownRect()
	for i, id := range e.menus[e.menu.open].Items {
		y := box.y + i
		if id == separator {
			vm.add(box.x, y, strings.Repeat("─", box.w), ports.StyleMenuItem)
			continue
		}
		style, hintStyle := ports.StyleMenuItem, ports.StyleMenuHint
		if i == e.menu.item {
			style, hintStyle = ports.StyleMenuItemSelected, ports.StyleMenuItemSelected
		}
		vm.add(box.x, y, strings.Repeat(" ", box.w), style)
		vm.add(box.x+1, y, e.menuLabel(id), style)
		if hint := e.shortcut(id); hint != "" {
			vm.add(box.x+labelW+4, y, hint, hintStyle)
		}
	}
}

func (e *Editor) drawCompletions(vm *viewBuilder) {
	box := e.completionRect()
	p := &e.prompt
	for row := 0; row < box.h; row++ {
		i := p.top + row
		if i >= len(p.items) {
			break
		}
		it, y := p.items[i], box.y+row
		style, hintStyle := ports.StyleMenuItem, ports.StyleMenuHint
		if i == p.selected {
			style, hintStyle = ports.StyleMenuItemSelected, ports.StyleMenuItemSelected
		}
		vm.add(box.x, y, strings.Repeat(" ", box.w), style)
		vm.add(box.x+1, y, it.label, style)
		if it.hint != "" {
			vm.add(box.x+box.w-uniseg.StringWidth(it.hint)-1, y, it.hint, hintStyle)
		}
	}
}

// dragUnit is the granularity of a mouse drag: characters, or whole words or
// lines after a double or triple click.
type dragUnit uint8

const (
	dragChar dragUnit = iota
	dragWord
	dragLine
)

// dragByUnit extends the selection made by a double or triple click to the
// word or line under off, keeping the original unit selected.
func (e *Editor) dragByUnit(off int) {
	lo, hi := e.doc.WordRange(off)
	if e.dragUnit == dragLine {
		lo, hi = e.doc.LineRange(off)
	}
	if off < e.dragFrom {
		e.doc.SetSelection(editor.Selection{Anchor: e.dragTo, Head: lo})
	} else {
		e.doc.SetSelection(editor.Selection{Anchor: e.dragFrom, Head: hi})
	}
}

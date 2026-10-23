package editor

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
)

// lineEdit replaces [off, off+rm) with ins.
type lineEdit struct {
	off, rm int
	ins     string
}

// applyLineEdits applies edits (sorted by offset, non-overlapping) bottom-up
// as one undo step. The selection follows the text: an offset after an edit
// shifts, one inside a removed range moves to its start, and an offset at
// the start of a pure insertion stays before it.
func (d *Document) applyLineEdits(edits []lineEdit) error {
	if len(edits) == 0 {
		return nil
	}
	sel := d.sel
	shift := func(p int, e lineEdit) int {
		switch {
		case p >= e.off+e.rm && !(e.rm == 0 && p == e.off):
			return p + len(e.ins) - e.rm
		case p > e.off:
			return e.off + len(e.ins)
		}
		return p
	}
	d.BeginUndoGroup()
	defer d.EndUndoGroup()
	for i := len(edits) - 1; i >= 0; i-- {
		e := edits[i]
		sel = Selection{shift(sel.Anchor, e), shift(sel.Head, e)}
		if err := d.replace(e.off, e.off+e.rm, e.ins, sel, ""); err != nil {
			return err
		}
	}
	d.hist.Amend(d.state())
	return nil
}

// SelectsLines reports whether the only selection spans more than one line.
func (d *Document) SelectsLines() bool {
	if len(d.extra) > 0 || d.sel.Empty() {
		return false
	}
	first, last := d.selectedLines()
	return last > first
}

// Indent adds a tab at the start of every non-blank line touched by the
// selection, as one undo step.
func (d *Document) Indent() error {
	first, last := d.selectedLines()
	var edits []lineEdit
	for line := first; line <= last; line++ {
		if !d.isBlank(line) {
			edits = append(edits, lineEdit{off: d.lineStart(line), ins: "\t"})
		}
	}
	if err := d.applyLineEdits(edits); err != nil {
		return fmt.Errorf("indent: %w", err)
	}
	return nil
}

// outdentLen is the number of bytes one outdent removes from text's leading
// whitespace: a tab, else up to a tab stop's worth of spaces.
func (d *Document) outdentLen(text []byte) int {
	if len(text) > 0 && text[0] == '\t' {
		return 1
	}
	n := 0
	for n < len(text) && n < d.tabWidth && text[n] == ' ' {
		n++
	}
	return n
}

func (d *Document) outdentEdits(first, last int) []lineEdit {
	var edits []lineEdit
	for line := first; line <= last; line++ {
		if rm := d.outdentLen(d.Line(line)); rm > 0 && !d.isBlank(line) {
			edits = append(edits, lineEdit{off: d.lineStart(line), rm: rm})
		}
	}
	return edits
}

// Outdent is Shift+Tab. With a selection spanning lines it removes one indent
// level from each of them. Otherwise, at each cursor, it deletes the tab or
// the spaces (up to the previous tab stop) directly left of the cursor, and
// when there are none it removes one indent level from the cursor's line.
func (d *Document) Outdent() error {
	if d.SelectsLines() {
		first, last := d.selectedLines()
		if err := d.applyLineEdits(d.outdentEdits(first, last)); err != nil {
			return fmt.Errorf("outdent: %w", err)
		}
		return nil
	}
	return d.forEachCursor(d.outdentAtCursor)
}

func (d *Document) outdentAtCursor() error {
	if d.sel.Empty() {
		off := d.sel.Head
		ls, _ := d.lineRange(d.lineOf(off))
		left := d.Line(d.lineOf(off))[:off-ls]
		rm := 0
		switch {
		case len(left) > 0 && left[len(left)-1] == '\t':
			rm = 1
		case len(left) > 0 && left[len(left)-1] == ' ':
			n := len(left) - len(bytes.TrimRight(left, " "))
			_, _, vcol := d.Position(off)
			want := vcol % d.tabWidth
			if want == 0 {
				want = d.tabWidth
			}
			rm = min(n, want)
		}
		if rm > 0 {
			return d.replace(off-rm, off, "", Cursor(off-rm), "")
		}
	}
	start, _ := d.sel.Range()
	line := d.lineOf(start)
	ls, _ := d.lineRange(line)
	rm := d.outdentLen(d.Line(line))
	if rm == 0 {
		return nil
	}
	return d.replace(ls, ls+rm, "", d.sel.mapReplace(ls, ls+rm, 0), "")
}

// DeleteWordBack deletes each selection, or the word before each cursor.
func (d *Document) DeleteWordBack() error {
	return d.forEachCursor(func() error {
		if !d.sel.Empty() {
			return d.deleteSelection()
		}
		off := d.sel.Head
		prev := d.wordLeft(off)
		if prev == off {
			return nil
		}
		return d.replace(prev, off, "", Cursor(prev), "")
	})
}

// DeleteWordForward deletes each selection, or the word after each cursor.
func (d *Document) DeleteWordForward() error {
	return d.forEachCursor(func() error {
		if !d.sel.Empty() {
			return d.deleteSelection()
		}
		off := d.sel.Head
		next := d.wordRight(off)
		if next == off {
			return nil
		}
		return d.replace(off, next, "", Cursor(off), "")
	})
}

// MoveLines moves the lines touched by the selection one line up or down,
// keeping the selection on them. At the document edge it does nothing.
func (d *Document) MoveLines(up bool) error {
	first, last := d.selectedLines()
	if (up && first == 0) || (!up && last >= d.LineCount()-1) {
		return nil
	}
	// a and b are the two blocks to swap, sep is what lies between them.
	a0, a1 := d.lineStart(first), d.lineEnd(last)
	b0, b1 := d.lineStart(first-1), d.lineEnd(first-1)
	if !up {
		b0, b1 = d.lineStart(last+1), d.lineEnd(last+1)
	}
	var lo0, lo1, hi0, hi1 int // the upper block then the lower block
	if up {
		lo0, lo1, hi0, hi1 = b0, b1, a0, a1
	} else {
		lo0, lo1, hi0, hi1 = a0, a1, b0, b1
	}
	upper, err1 := d.buf.Slice(lo0, lo1)
	sep, err2 := d.buf.Slice(lo1, hi0)
	lower, err3 := d.buf.Slice(hi0, hi1)
	if err := errors.Join(err1, err2, err3); err != nil {
		return fmt.Errorf("move lines: %w", err)
	}
	text := string(lower) + string(sep) + string(upper)
	// The selected block travels past the other block and the separator.
	delta := len(lower) + len(sep)
	if up {
		delta = -(len(upper) + len(sep))
	}
	after := Selection{d.sel.Anchor + delta, d.sel.Head + delta}
	if err := d.replace(lo0, hi1, text, after, ""); err != nil {
		return fmt.Errorf("move lines: %w", err)
	}
	return nil
}

// lineEnd is the end of line's content (before its line break).
func (d *Document) lineEnd(line int) int {
	_, end := d.lineRange(line)
	return end
}

// SelectOccurrence adds a cursor selecting the next (or previous) occurrence
// of the primary selection's text, which becomes the primary selection. With
// an empty selection it first selects the word under the cursor. Matches
// wrap around the document and skip text that is already selected. It
// reports whether anything was selected.
func (d *Document) SelectOccurrence(forward bool) bool {
	if d.sel.Empty() {
		s, e := d.WordRange(d.sel.Head)
		if e == s {
			return false
		}
		d.SetSelection(Selection{s, e})
		return true
	}
	q := []byte(d.SelectedText())
	if len(bytes.TrimSpace(q)) == 0 {
		return false
	}
	opts := SearchOptions{CaseSensitive: true}
	start, end := d.sel.Range()
	n := d.buf.Len()
	from, to := end, start
	for range d.CursorCount() + 1 {
		var at int
		if forward {
			at = d.scan(q, from, n, opts)
			if at < 0 {
				at = d.scan(q, 0, from, opts)
			}
		} else {
			at = d.scanBack(q, 0, to, opts)
			if at < 0 {
				at = d.scanBack(q, to, n, opts)
			}
		}
		if at < 0 {
			return false
		}
		cand := Selection{at, at + len(q)}
		if d.selected(cand) {
			// Already taken: look past it.
			from, to = cand.Head, cand.Anchor
			continue
		}
		d.hist.Seal()
		d.goal = -1
		d.extra = append(d.extra, d.sel)
		d.sel = cand
		d.mergeCursors()
		return true
	}
	return false
}

// selected reports whether c overlaps one of the current selections.
func (d *Document) selected(c Selection) bool {
	for _, s := range d.Selections() {
		if overlaps(s, c) {
			return true
		}
	}
	return false
}

// ToggleComment comments the non-blank lines touched by the selection with
// prefix, placed at their common indent, or uncomments them when they all
// carry it. It reports whether the lines were commented. One undo step.
func (d *Document) ToggleComment(prefix string) (commented bool, err error) {
	first, last := d.selectedLines()
	var lines []int
	indent := -1
	allCommented := true
	for line := first; line <= last; line++ {
		if d.isBlank(line) {
			continue
		}
		lines = append(lines, line)
		text := d.Line(line)
		lead := leadingSpace(text)
		if indent < 0 || lead < indent {
			indent = lead
		}
		if !strings.HasPrefix(string(text[lead:]), prefix) {
			allCommented = false
		}
	}
	if len(lines) == 0 {
		return false, nil
	}
	var edits []lineEdit
	for _, line := range lines {
		start := d.lineStart(line)
		text := d.Line(line)
		if allCommented {
			at := leadingSpace(text)
			rm := len(prefix)
			if bytes.HasPrefix(text[at+rm:], []byte(" ")) {
				rm++
			}
			edits = append(edits, lineEdit{off: start + at, rm: rm})
		} else {
			edits = append(edits, lineEdit{off: start + indent, ins: prefix + " "})
		}
	}
	if err := d.applyLineEdits(edits); err != nil {
		return false, fmt.Errorf("toggle comment: %w", err)
	}
	return !allCommented, nil
}

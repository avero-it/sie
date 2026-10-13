package editor

import (
	"fmt"
	"strings"

	"github.com/avero-it/sie/internal/core/domain/history"
	"github.com/rivo/uniseg"
)

// Undo-coalescing keys: consecutive edits with the same key and no cursor
// movement in between form one undo step.
const (
	keyType      = "type"
	keyBackspace = "backspace"
	keyDelete    = "delete"
	keyReplace   = "replace-all"
)

// Insert replaces the selection with text. Line breaks in text are
// normalized to the document's line ending. Typing single graphemes
// coalesces into one undo step; pastes and newlines are their own step.
func (d *Document) Insert(text string) error {
	norm := d.normalizeEOL(text)
	start, end := d.sel.Range()
	key := ""
	if d.sel.Empty() && !strings.ContainsAny(norm, "\r\n") && uniseg.GraphemeClusterCount(norm) == 1 {
		key = keyType
	}
	return d.replace(start, end, norm, Cursor(start+len(norm)), key)
}

// InsertNewline inserts a line break and copies the current line's leading
// whitespace (auto-indent).
func (d *Document) InsertNewline() error {
	start, _ := d.sel.Range()
	line := d.lineOf(start)
	text := d.Line(line)
	indent := len(text) - len(strings.TrimLeft(string(text), " \t"))
	ls, _ := d.lineRange(line)
	indent = min(indent, start-ls)
	return d.Insert(string(d.eol.Bytes()) + string(text[:indent]))
}

// Backspace deletes the selection, or the grapheme (or line break) before the cursor.
func (d *Document) Backspace() error {
	if !d.sel.Empty() {
		return d.DeleteSelection()
	}
	off := d.sel.Head
	prev := d.prevPos(off)
	if prev == off {
		return nil
	}
	return d.replace(prev, off, "", Cursor(prev), keyBackspace)
}

// DeleteForward deletes the selection, or the grapheme (or line break) after the cursor.
func (d *Document) DeleteForward() error {
	if !d.sel.Empty() {
		return d.DeleteSelection()
	}
	off := d.sel.Head
	next := d.nextPos(off)
	if next == off {
		return nil
	}
	return d.replace(off, next, "", Cursor(off), keyDelete)
}

// DeleteSelection removes the selected text as one undo step.
func (d *Document) DeleteSelection() error {
	start, end := d.sel.Range()
	if start == end {
		return nil
	}
	return d.replace(start, end, "", Cursor(start), "")
}

// DeleteLine deletes the lines touched by the selection, including their
// line break, and puts the cursor at the start of the following line.
func (d *Document) DeleteLine() error {
	first, last := d.selectedLines()
	start, end := d.lineStart(first), d.lineStart(last+1)
	cursor := start
	if last == d.LineCount()-1 && first > 0 {
		// No line after the block: remove the line break before it instead.
		_, start = d.lineRange(first - 1)
		cursor = d.lineStart(first - 1)
	}
	if start == end {
		return nil
	}
	return d.replace(start, end, "", Cursor(cursor), "")
}

// DuplicateLine copies the lines touched by the selection below themselves
// and moves the selection onto the copy.
func (d *Document) DuplicateLine() error {
	first, last := d.selectedLines()
	start := d.lineStart(first)
	_, end := d.lineRange(last)
	block, err := d.buf.Slice(start, end)
	if err != nil {
		return fmt.Errorf("duplicate line: %w", err)
	}
	ins := string(d.eol.Bytes()) + string(block)
	shift := len(ins)
	after := Selection{d.sel.Anchor + shift, d.sel.Head + shift}
	return d.replace(end, end, ins, after, "")
}

// SelectLine selects the cursor's line including its line break; repeated
// calls extend the selection by one line.
func (d *Document) SelectLine() {
	// A whole-line selection ends at the next line's start, so taking the
	// line after the end's line extends it by one.
	start, end := d.sel.Range()
	d.SetSelection(Selection{d.lineStart(d.lineOf(start)), d.lineStart(d.lineOf(end) + 1)})
}

// SelectAll selects the whole document.
func (d *Document) SelectAll() {
	d.SetSelection(Selection{0, d.buf.Len()})
}

// Undo reverts the last undo step. It reports false when there was nothing to undo.
func (d *Document) Undo() (bool, error) {
	sel, ok, err := d.hist.Undo(d.buf)
	if err != nil || !ok {
		return false, err
	}
	d.sel = Selection{d.snap(sel.Anchor), d.snap(sel.Head)}
	d.goal = -1
	return true, nil
}

// Redo re-applies the last undone step. It reports false when there was nothing to redo.
func (d *Document) Redo() (bool, error) {
	sel, ok, err := d.hist.Redo(d.buf)
	if err != nil || !ok {
		return false, err
	}
	d.sel = Selection{d.snap(sel.Anchor), d.snap(sel.Head)}
	d.goal = -1
	return true, nil
}

// replace swaps [start, end) for text, sets the selection to after and
// records one history edit with the given coalescing key.
func (d *Document) replace(start, end int, text string, after Selection, key string) error {
	before := d.sel
	removed, err := d.buf.Delete(start, end-start)
	if err != nil {
		return fmt.Errorf("edit: %w", err)
	}
	if err := d.buf.Insert(start, []byte(text)); err != nil {
		return fmt.Errorf("edit: %w", err)
	}
	// An edit can fuse clusters (e.g. "x" typed before a combining mark),
	// so move the selection past any cluster it now falls inside.
	after = Selection{d.snapForward(after.Anchor), d.snapForward(after.Head)}
	d.sel = after
	d.goal = -1
	d.hist.Record(history.Edit{Off: start, Removed: removed, Inserted: []byte(text)}, before, after, key)
	return nil
}

// selectedLines returns the first and last line touched by the selection. A
// selection ending at column 0 does not include that last line.
func (d *Document) selectedLines() (first, last int) {
	start, end := d.sel.Range()
	first, last = d.lineOf(start), d.lineOf(end)
	if end > start && last > first && end == d.lineStart(last) {
		last--
	}
	return first, last
}

func (d *Document) normalizeEOL(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if eol := string(d.eol.Bytes()); eol != "\n" {
		text = strings.ReplaceAll(text, "\n", eol)
	}
	return text
}

// Package editor holds the editing model of sie: a Document (buffer, undo
// history, selection, line-ending/BOM metadata), grapheme-aware cursor
// motions, edits, search, line layout and viewport math. It is pure: no I/O,
// no terminal.
//
// Offsets are byte offsets into the document (BOM excluded). The cursor never
// sits inside a grapheme cluster or between the '\r' and '\n' of a CRLF.
package editor

import (
	"github.com/avero-it/sie/internal/core/domain/buffer"
	"github.com/avero-it/sie/internal/core/domain/history"
)

// Options configures a new Document.
type Options struct {
	TabWidth   int               // cells per tab stop; <= 0 means 4
	DefaultEOL buffer.LineEnding // used when the content has no line breaks
	UndoLimit  int               // max undo steps; <= 0 means unlimited
}

// Selection is the range between Anchor and Head; Head is the cursor.
type Selection struct {
	Anchor, Head int
}

// Empty reports whether nothing is selected.
func (s Selection) Empty() bool { return s.Anchor == s.Head }

// Range returns the selection as start <= end.
func (s Selection) Range() (start, end int) {
	if s.Anchor <= s.Head {
		return s.Anchor, s.Head
	}
	return s.Head, s.Anchor
}

// Cursor returns a collapsed selection at off.
func Cursor(off int) Selection { return Selection{off, off} }

// Document is an editable text document with undo and a single selection.
type Document struct {
	buf      *buffer.Buffer
	hist     *history.History[Selection]
	sel      Selection
	goal     int // preferred screen column for vertical moves; -1 if unset
	eol      buffer.LineEnding
	bom      bool
	tabWidth int
}

// NewDocument creates a document from file content. A UTF-8 BOM is removed
// from the text and restored by Bytes; the line ending is detected.
func NewDocument(content []byte, opts Options) *Document {
	bom, text := buffer.SplitBOM(content)
	tw := opts.TabWidth
	if tw <= 0 {
		tw = 4
	}
	return &Document{
		buf:      buffer.New(text),
		hist:     history.New[Selection](opts.UndoLimit),
		goal:     -1,
		eol:      buffer.DetectLineEnding(text, opts.DefaultEOL),
		bom:      bom,
		tabWidth: tw,
	}
}

// Bytes returns the content to save, including the BOM if the file had one.
func (d *Document) Bytes() []byte {
	text := d.buf.Bytes()
	if d.bom {
		return append(buffer.UTF8BOM(), text...)
	}
	return text
}

// Text returns the document text (without BOM).
func (d *Document) Text() string { return string(d.buf.Bytes()) }

// Len returns the text length in bytes.
func (d *Document) Len() int { return d.buf.Len() }

// LineEnding returns the line ending used for new lines.
func (d *Document) LineEnding() buffer.LineEnding { return d.eol }

// HasBOM reports whether the file had a UTF-8 BOM.
func (d *Document) HasBOM() bool { return d.bom }

// TabWidth returns the tab stop width in cells.
func (d *Document) TabWidth() int { return d.tabWidth }

// LineCount returns the number of lines.
func (d *Document) LineCount() int { return d.buf.LineCount() }

// Line returns line's content without its terminator; out of range lines are empty.
func (d *Document) Line(line int) []byte {
	if line < 0 || line >= d.LineCount() {
		return nil
	}
	start, end := d.lineRange(line)
	data, _ := d.buf.Slice(start, end)
	return data
}

// Modified reports whether the document differs from the last saved state.
func (d *Document) Modified() bool { return d.hist.Modified() }

// MarkSaved records the current state as saved.
func (d *Document) MarkSaved() { d.hist.MarkSaved() }

// CanUndo reports whether Undo would do something.
func (d *Document) CanUndo() bool { return d.hist.CanUndo() }

// CanRedo reports whether Redo would do something.
func (d *Document) CanRedo() bool { return d.hist.CanRedo() }

// Selection returns the current selection.
func (d *Document) Selection() Selection { return d.sel }

// SetSelection sets the selection, clamping and snapping both ends to valid
// cursor positions. It ends the current undo group.
func (d *Document) SetSelection(s Selection) {
	d.hist.Seal()
	d.goal = -1
	d.sel = Selection{d.snap(s.Anchor), d.snap(s.Head)}
}

// CursorPos returns the cursor's line, byte column and screen column.
func (d *Document) CursorPos() (line, col, vcol int) {
	return d.Position(d.sel.Head)
}

// Position converts a document offset to line, byte column and screen column.
func (d *Document) Position(off int) (line, col, vcol int) {
	off = max(0, min(off, d.buf.Len()))
	line = d.lineOf(off)
	start, _ := d.lineRange(line)
	col = off - start
	return line, col, VisualCol(d.Line(line), col, d.tabWidth)
}

// OffsetAt converts a line and screen column (e.g. a mouse click) to a
// document offset, clamped to the document.
func (d *Document) OffsetAt(line, vcol int) int {
	if line < 0 {
		return 0
	}
	if line >= d.LineCount() {
		return d.buf.Len()
	}
	start, _ := d.lineRange(line)
	return start + OffsetAtCol(d.Line(line), max(0, vcol), d.tabWidth)
}

// SelectedText returns the selected text.
func (d *Document) SelectedText() string {
	start, end := d.sel.Range()
	data, _ := d.buf.Slice(start, end)
	return string(data)
}

// lineOf returns the line of a valid offset.
func (d *Document) lineOf(off int) int {
	line, _ := d.buf.LineOf(off)
	return line
}

func (d *Document) lineStart(line int) int {
	if line >= d.LineCount() {
		return d.buf.Len()
	}
	start, _ := d.buf.LineStart(line)
	return start
}

// lineRange returns the content range of a valid line, excluding "\n" or "\r\n".
func (d *Document) lineRange(line int) (start, end int) {
	start, end, _ = d.buf.LineBounds(line)
	if line < d.LineCount()-1 && end > start {
		if last, _ := d.buf.Slice(end-1, end); len(last) == 1 && last[0] == '\r' {
			end--
		}
	}
	return start, end
}

// snap clamps off to the document and moves it to a valid cursor position.
func (d *Document) snap(off int) int {
	off = max(0, min(off, d.buf.Len()))
	line := d.lineOf(off)
	start, end := d.lineRange(line)
	if off >= end {
		return end
	}
	return start + snapBoundary(d.Line(line), off-start)
}

// snapForward is like snap but moves an offset inside a cluster to its end.
func (d *Document) snapForward(off int) int {
	off = max(0, min(off, d.buf.Len()))
	line := d.lineOf(off)
	start, end := d.lineRange(line)
	if off >= end {
		return end
	}
	text, col := d.Line(line), off-start
	if snapBoundary(text, col) == col {
		return off
	}
	return start + nextBoundary(text, col)
}

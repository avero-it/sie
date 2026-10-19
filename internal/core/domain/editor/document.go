// Package editor holds the editing model of sie: a Document (buffer, undo
// history, selection, line-ending/BOM metadata), grapheme-aware cursor
// motions, edits, search, line layout and viewport math. It is pure: no I/O,
// no terminal.
//
// Offsets are byte offsets into the document (BOM excluded). The cursor never
// sits inside a grapheme cluster or between the '\r' and '\n' of a CRLF.
package editor

import (
	"fmt"
	"io"

	"github.com/avero-it/sie/internal/core/domain/buffer"
	"github.com/avero-it/sie/internal/core/domain/history"
)

// Options configures a new Document.
type Options struct {
	TabWidth   int               // cells per tab stop; <= 0 means 4
	DefaultEOL buffer.LineEnding // used when the content has no line breaks
	UndoLimit  int               // max undo steps; <= 0 means unlimited
	UndoBudget int               // max bytes of text the undo history keeps; <= 0 means 256 MB
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

// Document is an editable text document with undo, a primary selection
// and optional secondary cursors (see AddCursor).
type Document struct {
	buf        *buffer.Buffer
	hist       *history.History[cursors]
	sel        Selection
	extra      []Selection // secondary selections, sorted, never overlapping
	op         *cursors    // state before the running forEachCursor, if any
	opSeq      int         // numbers forEachCursor runs, for undo keys
	rank       int         // document-order index of the cursor forEachCursor is running on
	rect       *rectState  // the last rectangle made by SelectRect; see activeRect
	goal       int         // preferred screen column for vertical moves; -1 if unset
	eol        buffer.LineEnding
	bom        bool
	tabWidth   int
	undoBudget int      // see Options.UndoBudget
	undoLost   bool     // the last change was too large to undo; see TakeUndoLost
	version    uint64   // see Version
	changes    []Change // recent mutations, oldest first; see ChangesSince
}

// NewDocument creates a document from file content. A UTF-8 BOM is removed
// from the text and restored by Bytes; the line ending is detected.
func NewDocument(content []byte, opts Options) *Document {
	bom, text := buffer.SplitBOM(content)
	return newDocument(buffer.New(text), bom, buffer.DetectLineEnding(text, opts.DefaultEOL), opts)
}

// eolSample is how much of a large file is looked at to detect its line ending.
const eolSample = 1 << 20

// NewDocumentFromSource creates a document over src without loading it:
// the file stays where it is and is read in ranges as the text is shown or
// edited. The BOM is detected and skipped as in NewDocument; the line ending
// is taken from the first megabyte. Check Err after editing: it reports
// that src can no longer be read.
func NewDocumentFromSource(src buffer.Source, opts Options) (*Document, error) {
	head := make([]byte, min(src.Len(), len(buffer.UTF8BOM())))
	if _, err := src.ReadAt(head, 0); err != nil && len(head) > 0 && err != io.EOF {
		return nil, fmt.Errorf("read start of file: %w", err)
	}
	bom, _ := buffer.SplitBOM(head)
	text := buffer.Skip(src, bomLen(bom))
	sample := make([]byte, min(text.Len(), eolSample))
	if n, err := text.ReadAt(sample, 0); n < len(sample) {
		return nil, fmt.Errorf("read start of file: %w", err)
	}
	buf, err := buffer.NewFromSource(text)
	if err != nil {
		return nil, fmt.Errorf("index file: %w", err)
	}
	return newDocument(buf, bom, buffer.DetectLineEnding(sample, opts.DefaultEOL), opts), nil
}

func bomLen(has bool) int {
	if has {
		return len(buffer.UTF8BOM())
	}
	return 0
}

func newDocument(buf *buffer.Buffer, bom bool, eol buffer.LineEnding, opts Options) *Document {
	tw := opts.TabWidth
	if tw <= 0 {
		tw = 4
	}
	hist := history.New[cursors](opts.UndoLimit)
	budget := opts.UndoBudget
	if budget <= 0 {
		budget = defaultUndoBudget
	}
	hist.SetBudget(budget)
	return &Document{
		undoBudget: budget,
		buf:        buf,
		hist:       hist,
		goal:       -1,
		eol:        eol,
		bom:        bom,
		tabWidth:   tw,
	}
}

// Bytes returns the content to save, including the BOM if the file had one.
// It holds the whole document in memory: use WriteTo for large ones.
func (d *Document) Bytes() []byte {
	text := d.buf.Bytes()
	if d.bom {
		return append(buffer.UTF8BOM(), text...)
	}
	return text
}

// WriteTo streams the content to save (BOM included) to w, a chunk at a time.
func (d *Document) WriteTo(w io.Writer) (int64, error) {
	var total int64
	if d.bom {
		n, err := w.Write(buffer.UTF8BOM())
		total = int64(n)
		if err != nil {
			return total, err
		}
	}
	n, err := d.buf.WriteTo(w)
	return total + n, err
}

// Rebase points the document at src, the whole file as just saved, whose
// content must equal Bytes. The undo history and the selection are kept.
func (d *Document) Rebase(src buffer.Source) error {
	return d.buf.Rebase(buffer.Skip(src, bomLen(d.bom)))
}

// defaultUndoBudget is how much text the undo history keeps (removed and
// inserted). A single change over it cannot be undone, and clears the history.
const defaultUndoBudget = 256 << 20

// TakeUndoLost reports, once, that a change was too large to keep for undo
// and the undo history was cleared.
func (d *Document) TakeUndoLost() bool {
	lost := d.undoLost
	d.undoLost = false
	return lost
}

// Err reports a failure to read the file behind the document (it changed or
// vanished). Once set, the text shown and saved can no longer be trusted.
func (d *Document) Err() error { return d.buf.Err() }

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

// BeginUndoGroup makes all edits until EndUndoGroup one undo step, even
// across selection changes (e.g. an interactive replace session).
func (d *Document) BeginUndoGroup() { d.hist.BeginGroup() }

// EndUndoGroup closes the group opened by BeginUndoGroup.
func (d *Document) EndUndoGroup() { d.hist.EndGroup() }

// CanUndo reports whether Undo would do something.
func (d *Document) CanUndo() bool { return d.hist.CanUndo() }

// CanRedo reports whether Redo would do something.
func (d *Document) CanRedo() bool { return d.hist.CanRedo() }

// Selection returns the primary selection.
func (d *Document) Selection() Selection { return d.sel }

// SetSelection sets the selection, clamping and snapping both ends to valid
// cursor positions, and removes the secondary cursors. It ends the current
// undo group.
func (d *Document) SetSelection(s Selection) {
	d.hist.Seal()
	d.extra = nil
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

// Package buffer implements the text storage of sie as a piece table.
//
// The document is described by an ordered list of pieces, each referring to a
// span of either the original content (read-only, never mutated) or the add
// buffer (append-only). Offsets are byte offsets into the logical document.
// Only '\n' terminates a line; a preceding '\r' (CRLF) is part of the line's
// bytes and is stripped by Line.
package buffer

import (
	"bytes"
	"errors"
	"fmt"
	"io"
)

// ErrOutOfRange is returned when an offset, length or line is outside the document.
var ErrOutOfRange = errors.New("buffer: out of range")

type source uint8

const (
	srcOriginal source = iota
	srcAdd
)

type piece struct {
	src      source
	start    int // offset into the source buffer
	length   int
	newlines int // number of '\n' in the span
	origNL   int // original pieces: '\n' in the original before start (locates lines without scanning)
}

// Buffer is a piece-table text buffer. The zero value is an empty buffer.
type Buffer struct {
	original Source
	nlIndex  []int // newlines in original before each indexStride (see source.go)
	origNL   int   // newlines in the whole original
	add      []byte
	pieces   []piece
	length   int
	newlines int
	err      error // first failed read of the original; sticky
}

// New returns a buffer holding content. The buffer keeps a reference to
// content and never modifies it; callers must not modify it either.
func New(content []byte) *Buffer {
	b, _ := NewFromSource(memSource(content)) // a memory source cannot fail
	return b
}

// NewFromSource returns a buffer over src, scanning it once to index its
// lines. The buffer never loads src whole; src must not change while in use.
func NewFromSource(src Source) (*Buffer, error) {
	b := &Buffer{original: src}
	if err := b.reset(); err != nil {
		return nil, err
	}
	return b, nil
}

// reset makes the original the whole document.
func (b *Buffer) reset() error {
	b.add, b.pieces, b.length, b.newlines = nil, nil, 0, 0
	if err := b.buildIndex(); err != nil {
		return err
	}
	if n := b.original.Len(); n > 0 {
		b.pieces = []piece{{src: srcOriginal, start: 0, length: n, newlines: b.origNL}}
		b.length = n
		b.newlines = b.origNL
	}
	return nil
}

// Rebase replaces the original by src, whose content must equal the current
// document (e.g. the file just saved), and drops the piece list.
func (b *Buffer) Rebase(src Source) error {
	if src.Len() != b.length {
		return fmt.Errorf("rebase onto %d bytes, document has %d: %w", src.Len(), b.length, ErrOutOfRange)
	}
	old := b.original
	b.original = src
	if err := b.reset(); err != nil {
		b.original = old
		return err
	}
	b.err = nil
	return nil
}

// Err returns the first error from reading the original (e.g. the file
// changed on disk). Reads that fail return zero bytes, so callers must check
// Err and stop editing instead of acting on the result.
func (b *Buffer) Err() error { return b.err }

func (b *Buffer) fail(err error) {
	if b.err == nil {
		b.err = err
	}
}

// view returns bytes [lo, hi) of piece p. The result may alias storage and
// must not be modified. A failed read of the original sets Err and returns
// zeros of the requested length.
func (b *Buffer) view(p piece, lo, hi int) []byte {
	if p.src == srcAdd {
		return b.add[p.start+lo : p.start+hi]
	}
	data, err := b.readOrig(p.start+lo, hi-lo)
	if err != nil {
		b.fail(err)
		return make([]byte, hi-lo)
	}
	return data
}

// countNL counts the '\n' in the first n bytes of piece p without loading
// it whole.
func (b *Buffer) countNL(p piece, n int) int {
	if p.src == srcAdd {
		return bytes.Count(b.add[p.start:p.start+n], []byte{'\n'})
	}
	return b.origNLBefore(p.start+n) - p.origNL
}

// writeChunk is how much WriteTo reads and writes at a time.
const writeChunk = 1 << 20

// WriteTo streams the document to w without building it in memory.
func (b *Buffer) WriteTo(w io.Writer) (int64, error) {
	var total int64
	for _, p := range b.pieces {
		for lo := 0; lo < p.length; lo += writeChunk {
			data := b.view(p, lo, min(lo+writeChunk, p.length))
			if b.err != nil {
				return total, b.err
			}
			n, err := w.Write(data)
			total += int64(n)
			if err != nil {
				return total, err
			}
		}
	}
	return total, nil
}

// Len returns the document length in bytes.
func (b *Buffer) Len() int { return b.length }

// LineCount returns the number of lines. An empty document has one line, and
// a trailing '\n' starts a new (empty) last line.
func (b *Buffer) LineCount() int { return b.newlines + 1 }

// Bytes returns a copy of the whole document.
func (b *Buffer) Bytes() []byte {
	out := make([]byte, 0, b.length)
	for _, p := range b.pieces {
		out = append(out, b.view(p, 0, p.length)...)
	}
	return out
}

// Slice returns a copy of the bytes in [start, end).
func (b *Buffer) Slice(start, end int) ([]byte, error) {
	if start < 0 || end < start || end > b.length {
		return nil, fmt.Errorf("slice [%d,%d) of %d: %w", start, end, b.length, ErrOutOfRange)
	}
	out := make([]byte, 0, end-start)
	pos := 0
	for _, p := range b.pieces {
		pend := pos + p.length
		if pend > start && pos < end {
			lo := max(start, pos) - pos
			hi := min(end, pend) - pos
			out = append(out, b.view(p, lo, hi)...)
		}
		if pend >= end {
			break
		}
		pos = pend
	}
	return out, nil
}

// ReadAt implements io.ReaderAt over the document.
func (b *Buffer) ReadAt(dst []byte, off int64) (int, error) {
	if off < 0 {
		return 0, fmt.Errorf("read at %d: %w", off, ErrOutOfRange)
	}
	if off >= int64(b.length) {
		return 0, io.EOF
	}
	end := min(int(off)+len(dst), b.length)
	data, err := b.Slice(int(off), end)
	if err != nil {
		return 0, err
	}
	n := copy(dst, data)
	if n < len(dst) {
		return n, io.EOF
	}
	return n, nil
}

// Insert inserts text at byte offset off.
func (b *Buffer) Insert(off int, text []byte) error {
	if off < 0 || off > b.length {
		return fmt.Errorf("insert at %d of %d: %w", off, b.length, ErrOutOfRange)
	}
	if len(text) == 0 {
		return nil
	}
	addStart := len(b.add)
	b.add = append(b.add, text...)
	nl := bytes.Count(text, []byte{'\n'})
	np := piece{src: srcAdd, start: addStart, length: len(text), newlines: nl}
	b.length += len(text)
	b.newlines += nl

	i, inner := b.locate(off)
	if inner == 0 {
		// Typing at the end of the previous add piece just extends it.
		if i > 0 {
			prev := &b.pieces[i-1]
			if prev.src == srcAdd && prev.start+prev.length == addStart {
				prev.length += np.length
				prev.newlines += np.newlines
				return nil
			}
		}
		b.pieces = insertPieces(b.pieces, i, np)
		return nil
	}

	p := b.pieces[i]
	left, right := b.split(p, inner)
	b.pieces[i] = left
	b.pieces = insertPieces(b.pieces, i+1, np, right)
	return nil
}

// Delete removes n bytes starting at off and returns the removed bytes.
func (b *Buffer) Delete(off, n int) ([]byte, error) {
	if off < 0 || n < 0 || off+n > b.length {
		return nil, fmt.Errorf("delete [%d,%d) of %d: %w", off, off+n, b.length, ErrOutOfRange)
	}
	if n == 0 {
		return nil, nil
	}
	removed, err := b.Slice(off, off+n)
	if err != nil {
		return nil, err
	}
	b.cut(off, n)
	return removed, nil
}

// Discard removes n bytes starting at off without reading them, for
// deletions too large to keep a copy of.
func (b *Buffer) Discard(off, n int) error {
	if off < 0 || n < 0 || off+n > b.length {
		return fmt.Errorf("discard [%d,%d) of %d: %w", off, off+n, b.length, ErrOutOfRange)
	}
	if n > 0 {
		b.cut(off, n)
	}
	return nil
}

// cut removes [off, off+n), which must lie inside the document.
func (b *Buffer) cut(off, n int) {
	end := off + n
	out := make([]piece, 0, len(b.pieces)+1)
	pos, removedNL := 0, 0
	for _, p := range b.pieces {
		pend := pos + p.length
		switch {
		case pend <= off || pos >= end:
			out = append(out, p)
		default:
			kept := 0
			if pos < off {
				left, _ := b.split(p, off-pos)
				out = append(out, left)
				kept += left.newlines
			}
			if pend > end {
				_, right := b.split(p, end-pos)
				out = append(out, right)
				kept += right.newlines
			}
			removedNL += p.newlines - kept
		}
		pos = pend
	}
	b.pieces = out
	b.length -= n
	b.newlines -= removedNL
}

// LineStart returns the byte offset where line (0-based) begins.
func (b *Buffer) LineStart(line int) (int, error) {
	if line < 0 || line > b.newlines {
		return 0, fmt.Errorf("line %d of %d: %w", line, b.LineCount(), ErrOutOfRange)
	}
	if line == 0 {
		return 0, nil
	}
	seen, pos := 0, 0
	for _, p := range b.pieces {
		if seen+p.newlines >= line {
			k := line - seen // the k-th newline of this piece ends line-1
			if p.src == srcOriginal {
				off := b.origLineStart(p.origNL + k)
				return pos + off - p.start, b.err
			}
			for j, c := range b.add[p.start : p.start+p.length] {
				if c == '\n' {
					if k--; k == 0 {
						return pos + j + 1, nil
					}
				}
			}
		}
		seen += p.newlines
		pos += p.length
	}
	// Unreachable while the newline counts are consistent.
	return 0, fmt.Errorf("line %d: inconsistent line index: %w", line, ErrOutOfRange)
}

// LineOf returns the 0-based line containing byte offset off. Offset Len()
// is valid and belongs to the last line.
func (b *Buffer) LineOf(off int) (int, error) {
	if off < 0 || off > b.length {
		return 0, fmt.Errorf("offset %d of %d: %w", off, b.length, ErrOutOfRange)
	}
	line, pos := 0, 0
	for _, p := range b.pieces {
		if pos+p.length <= off {
			line += p.newlines
			pos += p.length
			continue
		}
		line += b.countNL(p, off-pos)
		break
	}
	return line, b.err
}

// Line returns a copy of line's content without its terminator ("\n" or
// "\r\n"). A '\r' on the last line has no '\n' after it and is content.
func (b *Buffer) Line(line int) ([]byte, error) {
	start, end, err := b.LineBounds(line)
	if err != nil {
		return nil, err
	}
	data, err := b.Slice(start, end)
	if err != nil {
		return nil, err
	}
	if line < b.newlines {
		data = bytes.TrimSuffix(data, []byte{'\r'})
	}
	return data, nil
}

// LineBounds returns [start, end) of line, where end excludes the '\n'
// terminator (a CRLF line's '\r' is included).
func (b *Buffer) LineBounds(line int) (start, end int, err error) {
	start, err = b.LineStart(line)
	if err != nil {
		return 0, 0, err
	}
	if line == b.newlines {
		return start, b.length, nil
	}
	next, err := b.LineStart(line + 1)
	if err != nil {
		return 0, 0, err
	}
	return start, next - 1, nil
}

// locate returns the index of the piece containing off and the offset inside
// it. For off == Len() it returns (len(pieces), 0).
func (b *Buffer) locate(off int) (index, inner int) {
	pos := 0
	for i, p := range b.pieces {
		if off < pos+p.length {
			return i, off - pos
		}
		pos += p.length
	}
	return len(b.pieces), 0
}

// split cuts p at inner (0 < inner < p.length) into two pieces.
func (b *Buffer) split(p piece, inner int) (piece, piece) {
	leftNL := b.countNL(p, inner)
	left := piece{src: p.src, start: p.start, length: inner, newlines: leftNL, origNL: p.origNL}
	right := piece{src: p.src, start: p.start + inner, length: p.length - inner, newlines: p.newlines - leftNL, origNL: p.origNL + leftNL}
	return left, right
}

func insertPieces(ps []piece, i int, add ...piece) []piece {
	ps = append(ps, add...)
	copy(ps[i+len(add):], ps[i:])
	copy(ps[i:], add)
	return ps
}

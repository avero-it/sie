package buffer

import (
	"bytes"
	"fmt"
	"io"
	"sort"
)

// Source is the read-only original content of a Buffer. It may live on disk:
// the buffer reads it in small ranges and never holds it whole. Len must not
// change, and ReadAt must follow io.ReaderAt (a short read needs an error).
type Source interface {
	io.ReaderAt
	Len() int
}

// memSource is a Source held in memory; reads alias the slice, no copy.
type memSource []byte

func (m memSource) Len() int { return len(m) }

func (m memSource) ReadAt(dst []byte, off int64) (int, error) {
	if off < 0 || off > int64(len(m)) {
		return 0, fmt.Errorf("read at %d of %d: %w", off, len(m), ErrOutOfRange)
	}
	n := copy(dst, m[off:])
	if n < len(dst) {
		return n, io.EOF
	}
	return n, nil
}

// Skip returns src without its first n bytes (e.g. a BOM the document keeps
// out of the text).
func Skip(src Source, n int) Source {
	if n <= 0 {
		return src
	}
	return skipSource{src: src, skip: n}
}

type skipSource struct {
	src  Source
	skip int
}

func (s skipSource) Len() int { return s.src.Len() - s.skip }

func (s skipSource) ReadAt(dst []byte, off int64) (int, error) {
	return s.src.ReadAt(dst, off+int64(s.skip))
}

// indexStride is the span of original text covered by one line-index entry.
// Finding a line costs a binary search plus counting at most one stride.
const indexStride = 64 << 10

// readOrig returns original[off:off+n]. Memory sources alias their storage;
// others are read into a new slice.
func (b *Buffer) readOrig(off, n int) ([]byte, error) {
	if m, ok := b.original.(memSource); ok {
		return m[off : off+n], nil
	}
	out := make([]byte, n)
	got, err := b.original.ReadAt(out, int64(off))
	if got < n {
		if err == nil {
			err = io.ErrUnexpectedEOF
		}
		return nil, fmt.Errorf("read original [%d,%d): %w", off, off+n, err)
	}
	return out, nil
}

// buildIndex scans the original once and records the newline count before
// every stride. Memory is 8 bytes per 64 KB of text.
func (b *Buffer) buildIndex() error {
	n := b.original.Len()
	b.nlIndex = make([]int, 0, n/indexStride+1)
	total := 0
	for off := 0; off < n; off += indexStride {
		b.nlIndex = append(b.nlIndex, total)
		chunk, err := b.readOrig(off, min(indexStride, n-off))
		if err != nil {
			return err
		}
		total += bytes.Count(chunk, []byte{'\n'})
	}
	b.origNL = total
	return nil
}

// origNLBefore returns the number of '\n' in original[:off].
func (b *Buffer) origNLBefore(off int) int {
	if len(b.nlIndex) == 0 || off <= 0 {
		return 0
	}
	i := min(off/indexStride, len(b.nlIndex)-1)
	start := i * indexStride
	chunk, err := b.readOrig(start, off-start)
	if err != nil {
		b.fail(err)
		return b.nlIndex[i]
	}
	return b.nlIndex[i] + bytes.Count(chunk, []byte{'\n'})
}

// origLineStart returns the offset just after the ord-th (1-based) '\n' of
// the original.
func (b *Buffer) origLineStart(ord int) int {
	n := b.original.Len()
	i := sort.Search(len(b.nlIndex), func(i int) bool { return b.nlIndex[i] >= ord }) - 1
	if i < 0 {
		return n
	}
	start := i * indexStride
	chunk, err := b.readOrig(start, min(indexStride, n-start))
	if err != nil {
		b.fail(err)
		return n
	}
	pos := 0
	for k := ord - b.nlIndex[i]; k > 0; k-- {
		j := bytes.IndexByte(chunk[pos:], '\n')
		if j < 0 {
			return n // inconsistent index; unreachable for a stable source
		}
		pos += j + 1
	}
	return start + pos
}

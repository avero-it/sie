package buffer

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

// bigText returns about n bytes of numbered lines, long enough to span
// several index strides.
func bigText(n int) []byte {
	var sb strings.Builder
	for i := 0; sb.Len() < n; i++ {
		fmt.Fprintf(&sb, "line %d %s\n", i, strings.Repeat("x", i%40))
	}
	return []byte(sb.String())
}

func TestLargeSourceMatchesMemory(t *testing.T) {
	data := bigText(5*indexStride + 123)
	mem, file := New(bytes.Clone(data)), newReaderBuffer(t, data)
	// Edit both the same way, in the middle of the original.
	for _, b := range []*Buffer{mem, file} {
		if err := b.Insert(len(data)/2, []byte("A\nB\n")); err != nil {
			t.Fatal(err)
		}
		if _, err := b.Delete(len(data)/3, 1000); err != nil {
			t.Fatal(err)
		}
	}
	if mem.LineCount() != file.LineCount() {
		t.Fatalf("LineCount %d vs %d", mem.LineCount(), file.LineCount())
	}
	for line := 0; line < mem.LineCount(); line += 97 {
		m, _ := mem.LineStart(line)
		f, err := file.LineStart(line)
		if err != nil || m != f {
			t.Fatalf("LineStart(%d) = %d, %v; want %d", line, f, err, m)
		}
		if l, _ := file.LineOf(f); l != line {
			t.Fatalf("LineOf(%d) = %d, want %d", f, l, line)
		}
	}
	if !bytes.Equal(mem.Bytes(), file.Bytes()) {
		t.Fatal("content differs")
	}
	if file.Err() != nil {
		t.Fatal(file.Err())
	}
}

func TestLargeSourceIsLazy(t *testing.T) {
	data := bigText(20 * indexStride)
	src := &readerSource{data: data}
	b, err := NewFromSource(src)
	if err != nil {
		t.Fatal(err)
	}
	src.read = 0 // indexing reads everything once
	last := b.LineCount() - 1
	start, err := b.LineStart(last / 2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Line(last / 2); err != nil {
		t.Fatal(err)
	}
	if err := b.Insert(start, []byte("typed")); err != nil {
		t.Fatal(err)
	}
	if _, err := b.LineOf(b.Len()); err != nil {
		t.Fatal(err)
	}
	if limit := 4 * indexStride; src.read > limit {
		t.Errorf("read %d bytes of a %d byte file for line lookups and an edit, want <= %d", src.read, len(data), limit)
	}
}

func TestWriteTo(t *testing.T) {
	data := bigText(3*writeChunk + 17)
	b := newReaderBuffer(t, data)
	_ = b.Insert(5, []byte("<<>>"))
	_, _ = b.Delete(len(data)/2, 4096)
	var out bytes.Buffer
	n, err := b.WriteTo(&out)
	if err != nil || int(n) != b.Len() || !bytes.Equal(out.Bytes(), b.Bytes()) {
		t.Fatalf("WriteTo = %d, %v (len %d)", n, err, b.Len())
	}
}

func TestRebase(t *testing.T) {
	b := newReaderBuffer(t, bigText(2*indexStride))
	_ = b.Insert(10, []byte("x\ny\n"))
	_, _ = b.Delete(500, 300)
	want := b.Bytes()
	lines := b.LineCount()
	if err := b.Rebase(&readerSource{data: bytes.Clone(want)}); err != nil {
		t.Fatal(err)
	}
	if len(b.pieces) != 1 || !bytes.Equal(b.Bytes(), want) || b.LineCount() != lines {
		t.Fatalf("after rebase: %d pieces, %d lines (want %d)", len(b.pieces), b.LineCount(), lines)
	}
	if err := b.Rebase(&readerSource{data: want[1:]}); err == nil {
		t.Error("rebase onto a different length succeeded")
	}
}

type failingSource struct {
	*readerSource
	fail bool
}

func (f *failingSource) ReadAt(dst []byte, off int64) (int, error) {
	if f.fail {
		return 0, errors.New("file changed")
	}
	return f.readerSource.ReadAt(dst, off)
}

func TestReadFailureIsSticky(t *testing.T) {
	src := &failingSource{readerSource: &readerSource{data: bigText(3 * indexStride)}}
	b, err := NewFromSource(src)
	if err != nil {
		t.Fatal(err)
	}
	src.fail = true
	got, _ := b.Slice(100, 200)
	if len(got) != 100 || !bytes.Equal(got, make([]byte, 100)) {
		t.Errorf("failed read returned %q, want 100 zero bytes", got)
	}
	if b.Err() == nil {
		t.Fatal("Err is nil after a failed read")
	}
	if _, err := b.WriteTo(io.Discard); err == nil {
		t.Error("WriteTo succeeded after a failed read")
	}
	src.fail = true
	if _, err := NewFromSource(src); err == nil {
		t.Error("NewFromSource succeeded on an unreadable source")
	}
}

func TestSkip(t *testing.T) {
	b, err := NewFromSource(Skip(&readerSource{data: []byte("\xef\xbb\xbfhi\nyo")}, 3))
	if err != nil || string(b.Bytes()) != "hi\nyo" || b.LineCount() != 2 {
		t.Fatalf("Skip: %q, %v", b.Bytes(), err)
	}
}

func TestDiscardMatchesDelete(t *testing.T) {
	data := bigText(3*indexStride + 5)
	for _, tc := range []struct{ off, n int }{{0, 10}, {100, len(data) - 200}, {indexStride - 3, 7}, {0, len(data)}, {len(data) - 1, 1}} {
		a, b := newReaderBuffer(t, data), newReaderBuffer(t, data)
		_ = a.Insert(50, []byte("x\ny\n")) // pieces of both kinds
		_ = b.Insert(50, []byte("x\ny\n"))
		if _, err := a.Delete(tc.off, tc.n); err != nil {
			t.Fatal(err)
		}
		if err := b.Discard(tc.off, tc.n); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(a.Bytes(), b.Bytes()) || a.LineCount() != b.LineCount() || a.Len() != b.Len() {
			t.Errorf("Discard(%d,%d) differs from Delete", tc.off, tc.n)
		}
		checkInvariants(t, b, b.Bytes())
	}
	if err := newReaderBuffer(t, data).Discard(0, len(data)+1); err == nil {
		t.Error("Discard out of range succeeded")
	}
}

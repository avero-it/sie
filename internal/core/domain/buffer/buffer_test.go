package buffer

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestInsertDelete(t *testing.T) {
	tests := []struct {
		name    string
		initial string
		ops     func(b *Buffer) error
		want    string
	}{
		{"empty", "", func(*Buffer) error { return nil }, ""},
		{"insert into empty", "", func(b *Buffer) error { return b.Insert(0, []byte("hi")) }, "hi"},
		{"insert at start", "world", func(b *Buffer) error { return b.Insert(0, []byte("hello ")) }, "hello world"},
		{"insert at end", "hello", func(b *Buffer) error { return b.Insert(5, []byte("!")) }, "hello!"},
		{"insert in middle", "held", func(b *Buffer) error { return b.Insert(2, []byte("l")) }, "helld"},
		{"typing appends", "", func(b *Buffer) error {
			for i, c := range "abc" {
				if err := b.Insert(i, []byte(string(c))); err != nil {
					return err
				}
			}
			return nil
		}, "abc"},
		{"delete middle", "hello world", func(b *Buffer) error { _, err := b.Delete(5, 6); return err }, "hello"},
		{"delete across pieces", "ad", func(b *Buffer) error {
			if err := b.Insert(1, []byte("bc")); err != nil {
				return err
			}
			_, err := b.Delete(1, 2)
			return err
		}, "ad"},
		{"delete everything", "abc", func(b *Buffer) error { _, err := b.Delete(0, 3); return err }, ""},
		{"delete zero", "abc", func(b *Buffer) error { _, err := b.Delete(1, 0); return err }, "abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := New([]byte(tt.initial))
			if err := tt.ops(b); err != nil {
				t.Fatalf("ops: %v", err)
			}
			if got := string(b.Bytes()); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
			checkInvariants(t, b, []byte(tt.want))
		})
	}
}

func TestDeleteReturnsRemoved(t *testing.T) {
	b := New([]byte("hello world"))
	got, err := b.Delete(2, 5)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "llo w" {
		t.Errorf("removed %q, want %q", got, "llo w")
	}
}

func TestOriginalNeverMutated(t *testing.T) {
	orig := []byte("abc\ndef")
	keep := bytes.Clone(orig)
	b := New(orig)
	_ = b.Insert(2, []byte("XYZ"))
	_, _ = b.Delete(0, 4)
	if !bytes.Equal(orig, keep) {
		t.Errorf("original mutated: %q", orig)
	}
}

func TestOutOfRange(t *testing.T) {
	b := New([]byte("abc"))
	checks := map[string]error{
		"insert -1":    b.Insert(-1, []byte("x")),
		"insert 4":     b.Insert(4, []byte("x")),
		"delete past":  second(b.Delete(2, 2)),
		"delete neg":   second(b.Delete(-1, 1)),
		"slice bad":    second(b.Slice(2, 1)),
		"line -1":      second(b.LineStart(-1)),
		"line 1":       second(b.LineStart(1)),
		"lineof past":  second(b.LineOf(4)),
		"line content": second(b.Line(2)),
	}
	for name, err := range checks {
		if !errors.Is(err, ErrOutOfRange) {
			t.Errorf("%s: got %v, want ErrOutOfRange", name, err)
		}
	}
}

func TestLines(t *testing.T) {
	tests := []struct {
		content string
		lines   []string
	}{
		{"", []string{""}},
		{"a", []string{"a"}},
		{"a\n", []string{"a", ""}},
		{"a\nbb\n\nccc", []string{"a", "bb", "", "ccc"}},
		{"a\r\nb\r\n", []string{"a", "b", ""}},
		{"lone\rcr", []string{"lone\rcr"}},
		{"a\r\nb\r", []string{"a", "b\r"}},
	}
	for _, tt := range tests {
		b := New([]byte(tt.content))
		if got := b.LineCount(); got != len(tt.lines) {
			t.Errorf("%q: LineCount = %d, want %d", tt.content, got, len(tt.lines))
			continue
		}
		for i, want := range tt.lines {
			got, err := b.Line(i)
			if err != nil {
				t.Fatalf("%q line %d: %v", tt.content, i, err)
			}
			if string(got) != want {
				t.Errorf("%q line %d = %q, want %q", tt.content, i, got, want)
			}
		}
	}
}

func TestReadAt(t *testing.T) {
	b := New([]byte("hello"))
	_ = b.Insert(5, []byte(" world"))
	p := make([]byte, 4)
	n, err := b.ReadAt(p, 3)
	if n != 4 || err != nil || string(p) != "lo w" {
		t.Errorf("ReadAt = %d %v %q", n, err, p)
	}
	n, err = b.ReadAt(p, 9)
	if n != 2 || err != io.EOF {
		t.Errorf("short ReadAt = %d %v", n, err)
	}
	if _, err := b.ReadAt(p, 11); err != io.EOF {
		t.Errorf("ReadAt at end = %v, want EOF", err)
	}
}

func FuzzPieceTable(f *testing.F) {
	f.Add([]byte("hello\nworld\r\n"), []byte{0, 3, 2, 1, 5, 9, 0, 0, 4})
	f.Add([]byte(""), []byte{0, 0, 1, 0, 0, 2})
	f.Fuzz(func(t *testing.T, initial, ops []byte) {
		b := New(bytes.Clone(initial))
		model := bytes.Clone(initial)
		inserts := [][]byte{[]byte("x"), []byte("\n"), []byte("ab\r\ncd"), []byte("é"), []byte("\n\n")}
		for i := 0; i+2 < len(ops); i += 3 {
			kind, a, c := ops[i], int(ops[i+1]), int(ops[i+2])
			if kind%2 == 0 {
				off := a % (len(model) + 1)
				text := inserts[c%len(inserts)]
				if err := b.Insert(off, text); err != nil {
					t.Fatalf("insert(%d): %v", off, err)
				}
				model = append(model[:off:off], append(bytes.Clone(text), model[off:]...)...)
			} else {
				if len(model) == 0 {
					continue
				}
				off := a % len(model)
				n := c % (len(model) - off + 1)
				removed, err := b.Delete(off, n)
				if err != nil {
					t.Fatalf("delete(%d,%d): %v", off, n, err)
				}
				if !bytes.Equal(removed, model[off:off+n]) {
					t.Fatalf("delete(%d,%d) removed %q, want %q", off, n, removed, model[off:off+n])
				}
				model = append(model[:off:off], model[off+n:]...)
			}
		}
		checkInvariants(t, b, model)
	})
}

// checkInvariants verifies b against a naive model of its content.
func checkInvariants(t *testing.T, b *Buffer, model []byte) {
	t.Helper()
	if !bytes.Equal(b.Bytes(), model) {
		t.Fatalf("content %q, want %q", b.Bytes(), model)
	}
	if b.Len() != len(model) {
		t.Fatalf("Len = %d, want %d", b.Len(), len(model))
	}
	sum, nl := 0, 0
	for _, p := range b.pieces {
		if p.length <= 0 {
			t.Fatalf("empty piece %+v", p)
		}
		if got := bytes.Count(b.span(p), []byte{'\n'}); got != p.newlines {
			t.Fatalf("piece %+v newlines = %d, want %d", p, p.newlines, got)
		}
		sum += p.length
		nl += p.newlines
	}
	if sum != b.length || nl != b.newlines {
		t.Fatalf("totals: len %d/%d newlines %d/%d", sum, b.length, nl, b.newlines)
	}
	lines := bytes.Split(model, []byte{'\n'})
	if b.LineCount() != len(lines) {
		t.Fatalf("LineCount = %d, want %d", b.LineCount(), len(lines))
	}
	start := 0
	for i, want := range lines {
		got, err := b.LineStart(i)
		if err != nil || got != start {
			t.Fatalf("LineStart(%d) = %d, %v; want %d", i, got, err, start)
		}
		for off := start; off <= start+len(want); off++ {
			if l, err := b.LineOf(off); err != nil || l != i {
				t.Fatalf("LineOf(%d) = %d, %v; want %d", off, l, err, i)
			}
		}
		start += len(want) + 1
	}
	if len(model) > 0 {
		mid := len(model) / 2
		s, err := b.Slice(mid/2, mid+1)
		if err != nil || !bytes.Equal(s, model[mid/2:mid+1]) {
			t.Fatalf("Slice(%d,%d) = %q, %v", mid/2, mid+1, s, err)
		}
	}
}

func second[T any](_ T, err error) error { return err }

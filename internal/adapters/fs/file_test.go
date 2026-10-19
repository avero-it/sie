package fs

import (
	"bytes"
	"errors"
	"io"
	iofs "io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func pattern(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte('a' + i%26)
	}
	return b
}

func openTemp(t *testing.T, data []byte) (path string, f interface {
	io.ReaderAt
	Len() int
	Close() error
}) {
	t.Helper()
	path = filepath.Join(t.TempDir(), "big.txt")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Store{}.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return path, f
}

func TestOpenReadAt(t *testing.T) {
	data := pattern(3*chunkSize + 1234)
	_, f := openTemp(t, data)
	if f.Len() != len(data) {
		t.Fatalf("Len = %d, want %d", f.Len(), len(data))
	}
	for _, tc := range []struct{ off, n int }{
		{0, 10}, {chunkSize - 5, 10}, {chunkSize, 1}, {2*chunkSize - 1, chunkSize + 2}, // across chunks
		{10, 2 * chunkSize}, {0, len(data)}, // direct reads
		{len(data) - 7, 7},
	} {
		got := make([]byte, tc.n)
		n, err := f.ReadAt(got, int64(tc.off))
		if err != nil || n != tc.n || !bytes.Equal(got, data[tc.off:tc.off+tc.n]) {
			t.Errorf("ReadAt(%d bytes at %d) = %d, %v", tc.n, tc.off, n, err)
		}
	}
	// Past the end: a short read with EOF, like os.File.
	got := make([]byte, 10)
	if n, err := f.ReadAt(got, int64(len(data)-4)); n != 4 || err != io.EOF || !bytes.Equal(got[:4], data[len(data)-4:]) {
		t.Errorf("short ReadAt = %d, %v", n, err)
	}
	if n, err := f.ReadAt(got, int64(len(data))); n != 0 || err != io.EOF {
		t.Errorf("ReadAt at end = %d, %v", n, err)
	}
}

func TestOpenCacheIsBounded(t *testing.T) {
	data := pattern((cacheChunks + 10) * chunkSize)
	_, f := openTemp(t, data)
	one := make([]byte, 1)
	for i := 0; i < cacheChunks+10; i++ {
		if _, err := f.ReadAt(one, int64(i*chunkSize)); err != nil {
			t.Fatal(err)
		}
	}
	if got := f.(*file).lru.Len(); got != cacheChunks {
		t.Errorf("cached %d chunks, want %d", got, cacheChunks)
	}
}

func TestOpenMissingAndDirectory(t *testing.T) {
	dir := t.TempDir()
	if _, err := (Store{}).Open(filepath.Join(dir, "nope")); !errors.Is(err, iofs.ErrNotExist) {
		t.Errorf("missing: %v, want ErrNotExist", err)
	}
	if _, err := (Store{}).Open(dir); err == nil {
		t.Error("opening a directory succeeded")
	}
}

func TestOpenDetectsChange(t *testing.T) {
	for name, change := range map[string]func(path string) error{
		"append": func(p string) error {
			f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
			if err != nil {
				return err
			}
			defer f.Close()
			_, err = f.WriteString("more")
			return err
		},
		"same size rewrite": func(p string) error {
			if err := os.WriteFile(p, pattern(2*chunkSize), 0o644); err != nil {
				return err
			}
			later := time.Now().Add(time.Hour)
			return os.Chtimes(p, later, later)
		},
		"replaced": func(p string) error {
			other := p + ".new"
			if err := os.WriteFile(other, pattern(2*chunkSize), 0o644); err != nil {
				return err
			}
			return os.Rename(other, p)
		},
		"removed": os.Remove,
	} {
		t.Run(name, func(t *testing.T) {
			path, f := openTemp(t, pattern(2*chunkSize))
			one := make([]byte, 1)
			if _, err := f.ReadAt(one, 0); err != nil { // cache chunk 0
				t.Fatal(err)
			}
			if err := change(path); err != nil {
				t.Fatal(err)
			}
			if _, err := f.ReadAt(one, chunkSize); !errors.Is(err, ErrChangedOnDisk) { // uncached chunk
				t.Errorf("cache miss after change: %v, want ErrChangedOnDisk", err)
			}
			if _, err := f.ReadAt(make([]byte, chunkSize), 0); !errors.Is(err, ErrChangedOnDisk) { // direct read
				t.Errorf("direct read after change: %v, want ErrChangedOnDisk", err)
			}
		})
	}
}

func TestWriteFromSameFile(t *testing.T) {
	// Saving a document that reads from the file it replaces.
	data := pattern(2 * chunkSize)
	path, f := openTemp(t, data)
	src := readerTo{f}
	if err := (Store{}).WriteFrom(path, src); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("after save: %d bytes, %v", len(got), err)
	}
	// The open view still reads the old file, but it is no longer the one at path.
	if _, err := f.ReadAt(make([]byte, chunkSize), 0); !errors.Is(err, ErrChangedOnDisk) {
		t.Errorf("read after replace: %v, want ErrChangedOnDisk", err)
	}
}

type readerTo struct {
	f interface {
		io.ReaderAt
		Len() int
	}
}

func (r readerTo) WriteTo(w io.Writer) (int64, error) {
	return io.Copy(w, io.NewSectionReader(r.f, 0, int64(r.f.Len())))
}

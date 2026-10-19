package fs

import (
	"container/list"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/avero-it/sie/internal/core/ports"
)

// ErrChangedOnDisk is returned by reads of an open file that was modified,
// truncated, replaced or removed by someone else.
var ErrChangedOnDisk = errors.New("file changed on disk")

const (
	chunkSize   = 256 << 10 // unit of caching and of small reads
	cacheChunks = 32        // 8 MB of cached file text at most
)

// Open returns a read-only view of the file that reads it in ranges: a
// chunk cache serves the small, scattered reads of editing, and large
// sequential reads (saving, searching) go straight to the disk. Whenever it
// has to go to the disk it first checks that the file is still the one that
// was opened, and fails with ErrChangedOnDisk if not.
func (Store) Open(path string) (ports.File, error) {
	f, err := openShared(path)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	info, err := f.Stat()
	if err == nil && info.IsDir() {
		err = fmt.Errorf("%s is a directory", path)
	}
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("open: %w", err)
	}
	return &file{
		f:     f,
		path:  path,
		info:  info,
		size:  info.Size(),
		mod:   info.ModTime(),
		cache: map[int64]*list.Element{},
		lru:   list.New(),
	}, nil
}

type file struct {
	f    *os.File
	path string
	info os.FileInfo
	size int64
	mod  time.Time

	mu    sync.Mutex
	cache map[int64]*list.Element // chunk index -> element holding *chunk
	lru   *list.List              // front = most recently used
}

type chunk struct {
	idx  int64
	data []byte
}

func (f *file) Len() int { return int(f.size) }

func (f *file) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cache, f.lru = map[int64]*list.Element{}, list.New()
	return f.f.Close()
}

// ReadAt implements io.ReaderAt.
func (f *file) ReadAt(dst []byte, off int64) (int, error) {
	if off < 0 {
		return 0, fmt.Errorf("read at %d: negative offset", off)
	}
	if off >= f.size {
		return 0, io.EOF
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	n := int(min(int64(len(dst)), f.size-off))
	var err error
	if n >= chunkSize {
		err = f.readDirect(dst[:n], off)
	} else {
		err = f.readCached(dst[:n], off)
	}
	if err != nil {
		return 0, err
	}
	if n < len(dst) {
		return n, io.EOF
	}
	return n, nil
}

func (f *file) readDirect(dst []byte, off int64) error {
	if err := f.verify(); err != nil {
		return err
	}
	if _, err := f.f.ReadAt(dst, off); err != nil {
		return fmt.Errorf("read %s: %w", f.path, err)
	}
	return nil
}

func (f *file) readCached(dst []byte, off int64) error {
	for len(dst) > 0 {
		idx := off / chunkSize
		c, err := f.chunk(idx)
		if err != nil {
			return err
		}
		n := copy(dst, c.data[off-idx*chunkSize:])
		dst, off = dst[n:], off+int64(n)
	}
	return nil
}

func (f *file) chunk(idx int64) (*chunk, error) {
	if el, ok := f.cache[idx]; ok {
		f.lru.MoveToFront(el)
		return el.Value.(*chunk), nil
	}
	if err := f.verify(); err != nil {
		return nil, err
	}
	start := idx * chunkSize
	c := &chunk{idx: idx, data: make([]byte, min(chunkSize, f.size-start))}
	if _, err := f.f.ReadAt(c.data, start); err != nil {
		return nil, fmt.Errorf("read %s: %w", f.path, err)
	}
	f.cache[idx] = f.lru.PushFront(c)
	if f.lru.Len() > cacheChunks {
		oldest := f.lru.Back()
		f.lru.Remove(oldest)
		delete(f.cache, oldest.Value.(*chunk).idx)
	}
	return c, nil
}

// verify checks that path still names the file that was opened, with the
// same size and modification time.
func (f *file) verify() error {
	now, err := os.Stat(f.path)
	switch {
	case err != nil:
		return fmt.Errorf("%s: %w: %w", f.path, ErrChangedOnDisk, err)
	case !os.SameFile(f.info, now), now.Size() != f.size, !now.ModTime().Equal(f.mod):
		return fmt.Errorf("%s: %w", f.path, ErrChangedOnDisk)
	}
	return nil
}

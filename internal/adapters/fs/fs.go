// Package fs implements ports.FileStore on the operating system's
// filesystem with atomic saves.
package fs

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	iofs "io/fs"
	"os"
	"path/filepath"

	"github.com/avero-it/sie/internal/core/ports"
)

// ErrReadOnly is returned when saving over a file without write permission.
var ErrReadOnly = errors.New("file is read-only")

const newFileMode = 0o644

// Store is the OS-backed FileStore.
type Store struct{}

var _ ports.FileStore = Store{}

// Read returns the file's bytes; a missing file wraps io/fs.ErrNotExist.
func (Store) Read(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	return data, nil
}

// Exists reports whether path exists.
func (Store) Exists(path string) (bool, error) {
	_, err := os.Stat(path)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, iofs.ErrNotExist):
		return false, nil
	default:
		return false, err
	}
}

// List returns the entries of dir. Symbolic links to directories count as
// directories; broken links are listed as files.
func (Store) List(dir string) ([]ports.DirEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}
	out := make([]ports.DirEntry, 0, len(entries))
	for _, e := range entries {
		isDir := e.IsDir()
		if e.Type()&iofs.ModeSymlink != 0 {
			if info, err := os.Stat(filepath.Join(dir, e.Name())); err == nil {
				isDir = info.IsDir()
			}
		}
		out = append(out, ports.DirEntry{Name: e.Name(), Dir: isDir})
	}
	return out, nil
}

// Write saves data atomically; see WriteFrom.
func (s Store) Write(path string, data []byte) error {
	return s.WriteFrom(path, bytes.NewReader(data))
}

// WriteFrom saves what src writes atomically: it writes a temporary file
// next to the target, syncs it and renames it over the target, so a crash
// never leaves a half-written file. Symlinks are followed (the link is kept)
// and the target's permissions are preserved. src may read from a file
// opened with Open on the same path: the old contents stay readable until
// that file is closed.
func (Store) WriteFrom(path string, src io.WriterTo) (err error) {
	target := path
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		target = resolved
	}
	mode := iofs.FileMode(newFileMode)
	if info, err := os.Stat(target); err == nil {
		if info.IsDir() {
			return fmt.Errorf("write %s: is a directory", path)
		}
		mode = info.Mode().Perm()
		if mode&0o200 == 0 {
			return fmt.Errorf("write %s: %w", path, ErrReadOnly)
		}
	}

	tmp, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+".sie-*")
	if err != nil {
		return fmt.Errorf("write: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()
	if _, err = src.WriteTo(tmp); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if err = tmp.Sync(); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if err = os.Chmod(tmp.Name(), mode); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if err = os.Rename(tmp.Name(), target); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return nil
}

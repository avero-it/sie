// Package fs implements ports.FileStore on the operating system's
// filesystem with atomic saves.
package fs

import (
	"errors"
	"fmt"
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

// Write saves data atomically: it writes a temporary file next to the
// target, syncs it and renames it over the target, so a crash never leaves
// a half-written file. Symlinks are followed (the link is kept) and the
// target's permissions are preserved.
func (Store) Write(path string, data []byte) (err error) {
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
	if _, err = tmp.Write(data); err != nil {
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

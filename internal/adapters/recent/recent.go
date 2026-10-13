// Package recent implements ports.RecentFiles as a small JSON file,
// ~/.sie/recent.json by default. Every Add re-reads the file and merges, so
// several running editors keep each other's entries; writes are atomic.
package recent

import (
	"encoding/json"
	"errors"
	"fmt"
	iofs "io/fs"
	"os"
	"path/filepath"

	"github.com/avero-it/sie/internal/core/ports"
)

// Limit is the number of files kept.
const Limit = 30

// Store is the JSON-file backed recent files list.
type Store struct {
	path string
}

var _ ports.RecentFiles = (*Store)(nil)

type fileFormat struct {
	Version int      `json:"version"`
	Files   []string `json:"files"`
}

// New returns a store kept at path (created on first Add).
func New(path string) *Store { return &Store{path: path} }

// DefaultPath is ~/.sie/recent.json, next to the user config.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".sie", "recent.json"), nil
}

// List returns the stored paths, most recent first. A missing file is an
// empty list.
func (s *Store) List() ([]string, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, iofs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("recent files: %w", err)
	}
	var f fileFormat
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("recent files %s: %w", s.path, err)
	}
	return f.Files, nil
}

// Add puts path first, removes duplicates and files that no longer exist,
// keeps at most Limit entries and writes the file atomically.
func (s *Store) Add(path string) error {
	files, err := s.List()
	if err != nil {
		files = nil // a corrupt file is replaced rather than blocking the editor
	}
	out := []string{path}
	for _, f := range files {
		if f == path || len(out) == Limit {
			continue
		}
		if _, err := os.Stat(f); err != nil {
			continue // gone: drop it
		}
		out = append(out, f)
	}
	data, err := json.MarshalIndent(fileFormat{Version: 1, Files: out}, "", "  ")
	if err != nil {
		return fmt.Errorf("recent files: %w", err)
	}
	return writeAtomic(s.path, append(data, '\n'))
}

func writeAtomic(path string, data []byte) (err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("recent files: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".recent-*")
	if err != nil {
		return fmt.Errorf("recent files: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		return fmt.Errorf("recent files: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("recent files: %w", err)
	}
	if err = os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("recent files: %w", err)
	}
	return nil
}

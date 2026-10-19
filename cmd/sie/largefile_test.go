package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/avero-it/sie/internal/adapters/fs"
	"github.com/avero-it/sie/internal/adapters/keymap"
	"github.com/avero-it/sie/internal/core/app"
	"github.com/avero-it/sie/internal/core/ports"
)

// bigFileMB is the size of the generated file; set SIE_BIG_MB=1024 to try a
// gigabyte by hand.
func bigFileMB(t *testing.T) int {
	if s := os.Getenv("SIE_BIG_MB"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	if testing.Short() {
		return 4
	}
	return 48
}

func writeBigFile(t *testing.T, mb int) (path string, lines int) {
	t.Helper()
	path = filepath.Join(t.TempDir(), "big.log")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := bufio.NewWriterSize(f, 1<<20)
	for size := 0; size < mb<<20; lines++ {
		n, _ := fmt.Fprintf(w, "2026-10-19T10:%02d:%02d INFO request %d served in %dms path=/api/v1/items/%d\n", lines/60%60, lines%60, lines, lines%97, lines)
		size += n
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path, lines
}

func TestEditBigFile(t *testing.T) {
	mb := bigFileMB(t)
	path, lines := writeBigFile(t, mb)
	e, err := app.New(app.Config{Files: fs.Store{}, Keymap: keymap.Default(), TabWidth: 4, LargeFileSize: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	e.Resize(120, 40)

	timed := func(what string, limit time.Duration, f func()) {
		t.Helper()
		start := time.Now()
		f()
		d := time.Since(start)
		t.Logf("%-28s %v", what, d)
		if d > limit {
			t.Errorf("%s took %v, want <= %v", what, d, limit)
		}
	}
	timed("open", 10*time.Second, func() {
		if err := e.Dispatch("file.open", path); err != nil {
			t.Fatal(err)
		}
	})
	for name, line := range map[string]int{"go to last line": lines, "go to middle line": lines / 2} {
		timed(name, time.Second, func() {
			if err := e.Dispatch("search.gotoLine", strconv.Itoa(line)); err != nil {
				t.Fatal(err)
			}
		})
	}
	timed("type 50 chars + render", time.Second, func() {
		for _, r := range "hello, large file; typing in the middle of it all" {
			e.HandleKey(ports.KeyEvent{Key: ports.KeyRune, Rune: r})
			_ = e.View()
		}
	})
	timed("scroll 40 pages", 2*time.Second, func() {
		for range 40 {
			e.HandleKey(ports.KeyEvent{Key: ports.KeyPgDn})
			_ = e.View()
		}
	})
	timed("find near the end", 20*time.Second, func() {
		if err := e.Dispatch("search.find", fmt.Sprintf("request %d served", lines-3)); err != nil {
			t.Fatal(err)
		}
	})
	var ms runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&ms)
	t.Logf("heap in use after editing: %.1f MB (file %d MB)", float64(ms.HeapAlloc)/(1<<20), mb)
	if mb >= 32 && ms.HeapAlloc > uint64(mb<<20)/2 {
		t.Errorf("heap %d MB for a %d MB file", ms.HeapAlloc>>20, mb)
	}

	timed("save", 20*time.Second, func() {
		if err := e.Dispatch("file.save"); err != nil {
			t.Fatal(err)
		}
	})
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte("hello, large file; typing in the middle of it all")) {
		t.Error("typed text missing from the saved file")
	}
	if want := lines + 0; bytes.Count(got, []byte{'\n'}) != want {
		t.Errorf("saved file has %d lines, want %d", bytes.Count(got, []byte{'\n'}), want)
	}
	// The view must still work on the re-opened file.
	e.HandleKey(ports.KeyEvent{Key: ports.KeyRune, Rune: '!'})
	_ = e.View()
	if err := e.Dispatch("file.save"); err != nil {
		t.Fatalf("second save: %v", err)
	}

	// Deleting everything must not copy the file (it is over the undo
	// budget when the file is big), and the editor must stay usable.
	timed("select all + delete", 20*time.Second, func() {
		e.HandleKey(ports.Ctrl('a'))
		e.HandleKey(ports.KeyEvent{Key: ports.KeyDelete})
		_ = e.View()
	})
	runtime.GC()
	runtime.ReadMemStats(&ms)
	t.Logf("heap after deleting everything: %.1f MB", float64(ms.HeapAlloc)/(1<<20))
	if mb >= 600 && ms.HeapAlloc > 100<<20 {
		t.Errorf("heap %d MB after deleting a %d MB file", ms.HeapAlloc>>20, mb)
	}
	e.HandleKey(ports.KeyEvent{Key: ports.KeyRune, Rune: 'z'})
	if got := e.View(); got.Cursor.X < 0 {
		t.Error("no cursor after editing the emptied document")
	}
}

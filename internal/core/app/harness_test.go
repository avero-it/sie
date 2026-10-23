package app

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/avero-it/sie/internal/core/ports"
	"github.com/rivo/uniseg"
)

type memFS struct {
	files    map[string][]byte
	writeErr error
	work     string           // relative keys live here (for List)
	listErr  map[string]error // List errors by directory
	opened   []*memFile       // every file handed out by Open
}

// key finds the stored key for path; like the real filesystem, relative
// paths also resolve against the working directory.
func (m *memFS) key(path string) (string, bool) {
	if _, ok := m.files[path]; ok {
		return path, true
	}
	// Keys are written with "/"; the editor builds native paths
	// (cmd\sie\main.go on Windows), like the real filesystem accepts.
	if k := filepath.ToSlash(path); m.files[k] != nil {
		return k, true
	}
	if m.work != "" && !filepath.IsAbs(path) {
		if k := filepath.Join(m.work, path); m.files[k] != nil {
			return k, true
		}
	}
	return path, false
}

// memFile is an open file of memFS. A failure set in err is returned by
// every read, like a file that changed on disk.
type memFile struct {
	data   []byte
	err    error
	closed bool
	reads  int // bytes read through ReadAt
}

func (f *memFile) Len() int { return len(f.data) }

func (f *memFile) Close() error {
	f.closed = true
	return nil
}

func (f *memFile) ReadAt(dst []byte, off int64) (int, error) {
	if f.err != nil {
		return 0, f.err
	}
	if off >= int64(len(f.data)) {
		return 0, io.EOF
	}
	n := copy(dst, f.data[off:])
	f.reads += n
	if n < len(dst) {
		return n, io.EOF
	}
	return n, nil
}

func (m *memFS) Open(path string) (ports.File, error) {
	k, ok := m.key(path)
	if !ok {
		return nil, fmt.Errorf("open %s: %w", path, fs.ErrNotExist)
	}
	f := &memFile{data: m.files[k]}
	m.opened = append(m.opened, f)
	return f, nil
}

func (m *memFS) WriteFrom(path string, src io.WriterTo) error {
	if m.writeErr != nil {
		return m.writeErr
	}
	var buf bytes.Buffer
	if _, err := src.WriteTo(&buf); err != nil {
		return err
	}
	m.files[path] = buf.Bytes()
	return nil
}

func (m *memFS) Exists(path string) (bool, error) {
	_, ok := m.key(path)
	return ok, nil
}

// List derives directory entries from the file keys; a key ending in "/"
// is an empty directory.
func (m *memFS) List(dir string) ([]ports.DirEntry, error) {
	if err := m.listErr[dir]; err != nil {
		return nil, err
	}
	work := m.work
	if work == "" {
		work = "."
	}
	seen := map[string]bool{}
	var out []ports.DirEntry
	for key := range m.files {
		isDirKey := strings.HasSuffix(key, "/")
		abs := filepath.FromSlash(strings.TrimSuffix(key, "/"))
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(work, abs)
		}
		rel, err := filepath.Rel(dir, abs)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			continue
		}
		first, rest, nested := strings.Cut(filepath.ToSlash(rel), "/")
		isDir := nested && rest != "" || isDirKey && !nested
		if !seen[first] {
			seen[first] = true
			out = append(out, ports.DirEntry{Name: first, Dir: isDir || nested})
		}
	}
	return out, nil
}

type memClip struct {
	text string
	err  error
}

func (c *memClip) Copy(s string) error    { c.text = s; return c.err }
func (c *memClip) Paste() (string, error) { return c.text, c.err }

// testKeymap is a subset of the default keymap (the app may not import adapters).
type testKeymap map[ports.KeyEvent]string

func (k testKeymap) Command(e ports.KeyEvent) (string, bool) { id, ok := k[e]; return id, ok }

func (k testKeymap) IsPrefix(e ports.KeyEvent) bool { return e == ports.Ctrl('p') }

// testChords is a subset of the default Ctrl+P chords.
var testChords = map[rune]string{
	'l': "selection.trimLeadingSpaces",
	'p': "selection.alignPrevious",
	'n': "selection.alignNext",
	'u': "edit.moveLineUp",
	'j': "edit.moveLineDown",
	'm': "edit.toggleComment",
	'v': "selection.selectNext",
	'q': "selection.selectPrevious",
}

func (k testKeymap) Chord(prefix, e ports.KeyEvent) (string, bool) {
	id, ok := testChords[e.Rune]
	return id, ok && prefix == ports.Ctrl('p') && e.Key == ports.KeyRune && e.Mod == 0
}

func (k testKeymap) Shortcut(id string) string {
	for e, c := range k {
		if c == id {
			return e.String()
		}
	}
	for r, c := range testChords {
		if c == id {
			return "Ctrl+P " + strings.ToUpper(string(r))
		}
	}
	return ""
}

var keys = testKeymap{
	ports.Ctrl('s'):    "file.save",
	ports.Ctrl('q'):    "file.quit",
	ports.Ctrl('z'):    "edit.undo",
	ports.Ctrl('x'):    "edit.cut",
	ports.Ctrl('c'):    "edit.copy",
	ports.Ctrl('v'):    "edit.paste",
	ports.Ctrl('f'):    "search.find",
	ports.Ctrl('g'):    "search.gotoLine",
	ports.CtrlAlt('p'): "ui.prompt",
	ports.CtrlAlt('m'): "ui.menu",
	ports.Ctrl('a'):    "selection.all",
	ports.Ctrl('d'):    "edit.duplicateLine",
	ports.Ctrl('k'):    "edit.deleteLine",
	ports.Ctrl('l'):    "selection.line",
	ports.Ctrl('o'):    "file.open",
	ports.Ctrl('e'):    "file.openRecent",
	ports.Ctrl('w'):    "file.close",
	ports.Ctrl('t'):    "edit.validate",
	ports.Ctrl('u'):    "selection.toggleColumnMode",
	ports.Ctrl('n'):    "file.new",
	ports.Ctrl('y'):    "edit.redo",
	ports.Ctrl('r'):    "search.replace",
}

type harness struct {
	*Editor
	t    *testing.T
	fs   *memFS
	clip *memClip
}

func newHarness(t *testing.T, files map[string]string) *harness {
	t.Helper()
	m := &memFS{files: map[string][]byte{}}
	for k, v := range files {
		m.files[k] = []byte(v)
	}
	clip := &memClip{}
	e, err := New(Config{Files: m, Clipboard: clip, Keymap: keys, Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	e.Resize(40, 10)
	return &harness{Editor: e, t: t, fs: m, clip: clip}
}

func (h *harness) keys(ks ...ports.KeyEvent) {
	for _, k := range ks {
		h.HandleKey(k)
	}
}

func (h *harness) typeText(s string) {
	for _, r := range s {
		switch r {
		case '\n':
			h.HandleKey(ports.KeyEvent{Key: ports.KeyEnter})
		default:
			h.HandleKey(ports.KeyEvent{Key: ports.KeyRune, Rune: r})
		}
	}
}

func key(k ports.Key) ports.KeyEvent { return ports.KeyEvent{Key: k} }

func (h *harness) click(x, y int, mod ports.Mod) {
	h.HandleMouse(ports.MouseEvent{X: x, Y: y, Button: ports.MouseLeft, Action: ports.MousePress, Mod: mod})
	h.HandleMouse(ports.MouseEvent{X: x, Y: y, Action: ports.MouseRelease})
}

// screen renders the ViewModel to text rows (right-trimmed).
func (h *harness) screen() []string {
	vm := h.View()
	cells := make([][]string, vm.Height)
	for y := range cells {
		cells[y] = strings.Split(strings.Repeat(" ", vm.Width), "")
	}
	for _, r := range vm.Runs {
		x, rest, state := r.X, r.Text, -1
		for len(rest) > 0 {
			var cl string
			var w int
			cl, rest, w, state = uniseg.FirstGraphemeClusterInString(rest, state)
			if x >= 0 && x < vm.Width {
				cells[r.Y][x] = cl
				if w == 2 && x+1 < vm.Width {
					cells[r.Y][x+1] = ""
				}
			}
			x += max(w, 1)
		}
	}
	rows := make([]string, vm.Height)
	for y, c := range cells {
		rows[y] = strings.TrimRight(strings.Join(c, ""), " ")
	}
	return rows
}

func (h *harness) row(y int) string {
	h.t.Helper()
	rows := h.screen()
	if y < 0 {
		y += len(rows)
	}
	return rows[y]
}

// styleAt returns the style of the last run covering cell (x, y).
func (h *harness) styleAt(x, y int) ports.Style {
	vm := h.View()
	style := ports.StyleDefault
	for _, r := range vm.Runs {
		if r.Y == y && x >= r.X && x < r.X+uniseg.StringWidth(r.Text) {
			style = r.Style
		}
	}
	return style
}

func (h *harness) wantRow(y int, want string) {
	h.t.Helper()
	if got := h.row(y); got != want {
		h.t.Errorf("row %d:\n got %q\nwant %q", y, got, want)
	}
}

func (h *harness) wantRowPrefix(y int, want string) {
	h.t.Helper()
	if got := h.row(y); !strings.HasPrefix(got, want) {
		h.t.Errorf("row %d:\n got %q\nwant prefix %q", y, got, want)
	}
}

var errDisk = errors.New("disk full")

func (h *harness) wantRowContains(y int, want string) {
	h.t.Helper()
	if got := h.row(y); !strings.Contains(got, want) {
		h.t.Errorf("row %d:\n got %q\nwant it to contain %q", y, got, want)
	}
}

// memRecent is an in-memory ports.RecentFiles.
type memRecent struct{ files []string }

func (r *memRecent) List() ([]string, error) { return append([]string(nil), r.files...), nil }

func (r *memRecent) Add(path string) error {
	out := []string{path}
	for _, f := range r.files {
		if f != path {
			out = append(out, f)
		}
	}
	r.files = out
	return nil
}

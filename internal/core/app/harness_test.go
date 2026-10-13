package app

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"testing"

	"github.com/avero-it/sie/internal/core/ports"
	"github.com/rivo/uniseg"
)

type memFS struct {
	files    map[string][]byte
	writeErr error
}

func (m *memFS) Read(path string) ([]byte, error) {
	data, ok := m.files[path]
	if !ok {
		return nil, fmt.Errorf("read %s: %w", path, fs.ErrNotExist)
	}
	return data, nil
}

func (m *memFS) Write(path string, data []byte) error {
	if m.writeErr != nil {
		return m.writeErr
	}
	m.files[path] = append([]byte(nil), data...)
	return nil
}

func (m *memFS) Exists(path string) (bool, error) {
	_, ok := m.files[path]
	return ok, nil
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

func (k testKeymap) Binding(id string) (ports.KeyEvent, bool) {
	for e, c := range k {
		if c == id {
			return e, true
		}
	}
	return ports.KeyEvent{}, false
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

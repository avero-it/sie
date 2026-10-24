package app

import (
	"strings"
	"testing"

	"github.com/avero-it/sie/internal/core/ports"
)

// wrapSample has a line of 43 cells, which wraps after "cccccccccc " at 37.
const wrapSample = "aaaaaaaaaa bbbbbbbbbb cccccccccc dddddddddd\nshort"

func wrapHarness(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t, nil)
	if err := h.doc.Insert(wrapSample); err != nil {
		t.Fatal(err)
	}
	_ = h.Dispatch("view.toggleWrap")
	return h
}

func TestWrapRender(t *testing.T) {
	h := wrapHarness(t)
	h.wantRow(rowText, " 1 aaaaaaaaaa bbbbbbbbbb cccccccccc")
	h.wantRow(rowText+1, "   dddddddddd")
	h.wantRow(rowText+2, " 2 short")
	h.wantRowContains(rowStatus, "WRAP")
	_ = h.Dispatch("view.toggleWrap")
	h.wantRow(rowText, " 1 aaaaaaaaaa bbbbbbbbbb cccccccccc dddd")
	if strings.Contains(h.screen()[rowStatus], "WRAP") {
		t.Error("WRAP still shown after turning wrap off")
	}
}

func TestWrapMenuTickAndEcho(t *testing.T) {
	h := newHarness(t, nil)
	_ = h.Dispatch("view.toggleWrap")
	h.wantRowPrefix(rowPrompt, "> ✓ Wrap lines — on")
	_ = h.Dispatch("view.toggleWrap")
	h.wantRowPrefix(rowPrompt, ">   Wrap lines — off")
}

func TestWrapCursorAndClick(t *testing.T) {
	h := wrapHarness(t)
	// A click on the continuation row lands inside the wrapped text.
	h.click(3+4, rowText+1, 0)
	if got := h.doc.Selection().Head; got != len("aaaaaaaaaa bbbbbbbbbb cccccccccc ")+4 {
		t.Errorf("click on row 2: head = %d", got)
	}
	if c := h.View().Cursor; c.X != 3+4 || c.Y != rowText+1 {
		t.Errorf("cursor = %+v, want (7, %d)", c, rowText+1)
	}
	// Down moves to the next row (here the next line), Up comes back.
	h.HandleKey(key(ports.KeyDown))
	if line, _, _ := h.doc.CursorPos(); line != 1 {
		t.Errorf("Down: line = %d, want 1", line)
	}
	h.HandleKey(key(ports.KeyUp))
	if c := h.View().Cursor; c.Y != rowText+1 {
		t.Errorf("Up: cursor row = %d, want %d", c.Y, rowText+1)
	}
	// Up again goes to the first row of the same line, not the previous line.
	h.HandleKey(key(ports.KeyUp))
	if line, _, _ := h.doc.CursorPos(); line != 0 {
		t.Errorf("Up: line = %d, want 0", line)
	}
	if c := h.View().Cursor; c.Y != rowText {
		t.Errorf("Up: cursor row = %d, want %d", c.Y, rowText)
	}
}

func TestWrapScrollsLongLine(t *testing.T) {
	h := newHarness(t, nil)
	_ = h.Dispatch("view.toggleWrap")
	h.typeText(strings.Repeat("word ", 40)) // 200 cells: 6 rows
	h.typeText("\nnext")
	h.HandleKey(key(ports.KeyUp))
	for i := 0; i < 8; i++ {
		h.HandleKey(key(ports.KeyUp))
	}
	if c := h.View().Cursor; !c.Visible {
		t.Fatalf("cursor not visible after moving up: %+v (Top=%d Skip=%d)", c, h.view.Top, h.view.Skip)
	}
	h.keys(ports.KeyEvent{Key: ports.KeyHome, Mod: ports.ModCtrl})
	if h.view.Top != 0 || h.view.Skip != 0 {
		t.Errorf("Ctrl+Home: Top,Skip = %d,%d", h.view.Top, h.view.Skip)
	}
}

func TestColumnModeTurnsWrapOff(t *testing.T) {
	h := wrapHarness(t)
	h.keys(ports.Ctrl('u'))
	if h.wrap || !h.columnMode {
		t.Errorf("wrap = %v, columnMode = %v; want wrap off, column mode on", h.wrap, h.columnMode)
	}
	if err := h.Dispatch("view.toggleWrap"); err == nil {
		t.Error("wrap turned on during column mode")
	}
}

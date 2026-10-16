package app

import (
	"slices"
	"strings"
	"testing"

	"github.com/avero-it/sie/internal/core/ports"
)

const rectText = "abcdef\nghijkl\nmnopqr"

// rectHarness types rectText; the cursor ends up at its end.
func rectHarness(t *testing.T, text string) *harness {
	t.Helper()
	h := newHarness(t, nil)
	h.typeText(text)
	return h
}

// drag presses at (x1,y1), moves to (x2,y2) and releases there. The gutter
// is 3 cells wide for documents under 10 lines.
func (h *harness) drag(x1, y1, x2, y2 int, mod ports.Mod) {
	h.HandleMouse(ports.MouseEvent{X: x1, Y: y1, Button: ports.MouseLeft, Action: ports.MousePress, Mod: mod})
	h.HandleMouse(ports.MouseEvent{X: x2, Y: y2, Button: ports.MouseLeft, Action: ports.MouseDrag, Mod: mod})
	h.HandleMouse(ports.MouseEvent{X: x2, Y: y2, Action: ports.MouseRelease})
}

func (h *harness) texts() []string { return h.doc.SelectedTexts() }

func TestAltDragSelectsARectangle(t *testing.T) {
	h := rectHarness(t, rectText)
	h.drag(3+1, rowText, 3+3, rowText+2, ports.ModAlt)
	if got, want := h.texts(), []string{"bc", "hi", "no"}; !slices.Equal(got, want) {
		t.Fatalf("selected %q, want %q", got, want)
	}
	h.wantRowContains(rowStatus, "(3 cursors)")
	for y := rowText; y < rowText+3; y++ {
		if got := h.styleAt(3+1, y); got != ports.StyleSelection {
			t.Errorf("row %d col 1 style %v, want selection", y, got)
		}
		if got := h.styleAt(3+4, y); got == ports.StyleSelection {
			t.Errorf("row %d col 4 is selected", y)
		}
	}
	h.typeText("X")
	if got, want := h.doc.Text(), "aXdef\ngXjkl\nmXpqr"; got != want {
		t.Errorf("typing: %q, want %q", got, want)
	}
	h.keys(ports.Ctrl('z'))
	if got := h.doc.Text(); got != rectText {
		t.Errorf("undo: %q", got)
	}
	if got, want := h.texts(), []string{"bc", "hi", "no"}; !slices.Equal(got, want) {
		t.Errorf("undo restores the rectangle: %q", got)
	}
}

func TestAltDragBackwardsAndPastLineEnds(t *testing.T) {
	h := rectHarness(t, "abcdef\nx\nghijkl")
	// From the bottom right to the top left; the click is far past "x".
	h.drag(3+4, rowText+2, 3+2, rowText, ports.ModAlt)
	if got, want := h.texts(), []string{"cd", "", "ij"}; !slices.Equal(got, want) {
		t.Errorf("selected %q, want %q", got, want)
	}
	// The short line's cursor is drawn at its end.
	if got := h.styleAt(3+1, rowText+1); got != ports.StyleCursorSecondary {
		t.Errorf("short line cursor style %v", got)
	}
}

func TestAltClickWithoutDragStillTogglesACursor(t *testing.T) {
	h := rectHarness(t, rectText)
	h.click(3, rowText, 0)
	h.click(3+2, rowText+1, ports.ModAlt)
	if n := h.doc.CursorCount(); n != 2 {
		t.Fatalf("cursors = %d, want 2", n)
	}
	h.click(3+2, rowText+1, ports.ModAlt)
	if n := h.doc.CursorCount(); n != 1 {
		t.Errorf("cursors = %d, want 1", n)
	}
}

func TestPlainDragStaysAnOrdinarySelection(t *testing.T) {
	h := rectHarness(t, rectText)
	h.drag(3+1, rowText, 3+3, rowText+2, 0)
	if n := h.doc.CursorCount(); n != 1 {
		t.Errorf("cursors = %d, want 1", n)
	}
}

func TestColumnModeToggle(t *testing.T) {
	h := rectHarness(t, rectText)
	h.keys(ports.Ctrl('u'))
	h.wantRowContains(rowPrompt, "✓ Column selection — on")
	h.wantRowContains(rowStatus, "COLUMN")
	h.keys(ports.Ctrl('u'))
	h.wantRowContains(rowPrompt, "  Column selection — off")
	if got := h.row(rowStatus); strings.Contains(got, "COLUMN") {
		t.Errorf("status still shows COLUMN: %q", got)
	}
}

func TestColumnModeShiftArrows(t *testing.T) {
	h := rectHarness(t, rectText)
	h.click(3+1, rowText, 0)
	h.keys(ports.Ctrl('u'))
	shift := func(k ports.Key) { h.HandleKey(ports.KeyEvent{Key: k, Mod: ports.ModShift}) }
	shift(ports.KeyRight)
	shift(ports.KeyRight)
	shift(ports.KeyDown)
	shift(ports.KeyDown)
	if got, want := h.texts(), []string{"bc", "hi", "no"}; !slices.Equal(got, want) {
		t.Fatalf("selected %q, want %q", got, want)
	}
	shift(ports.KeyLeft)
	shift(ports.KeyUp)
	if got, want := h.texts(), []string{"b", "h"}; !slices.Equal(got, want) {
		t.Errorf("shrunk to %q, want %q", got, want)
	}
	h.keys(key(ports.KeyBackspace))
	if got, want := h.doc.Text(), "acdef\ngijkl\nmnopqr"; got != want {
		t.Errorf("backspace: %q, want %q", got, want)
	}
	// A plain arrow moves every cursor (ordinary multiple cursors); Esc
	// leaves them, and the mode stays on.
	h.keys(key(ports.KeyRight))
	if n := h.doc.CursorCount(); n != 2 {
		t.Errorf("cursors = %d, want 2", n)
	}
	h.keys(key(ports.KeyEsc))
	if n := h.doc.CursorCount(); n != 1 {
		t.Errorf("cursors = %d, want 1", n)
	}
	h.wantRowContains(rowStatus, "COLUMN")
}

func TestColumnModeShiftArrowsOffMeansNormalSelection(t *testing.T) {
	h := rectHarness(t, rectText)
	h.click(3+1, rowText, 0)
	h.HandleKey(ports.KeyEvent{Key: ports.KeyDown, Mod: ports.ModShift})
	if n := h.doc.CursorCount(); n != 1 {
		t.Errorf("cursors = %d, want 1", n)
	}
}

func TestColumnModeDragMakesARectangle(t *testing.T) {
	h := rectHarness(t, rectText)
	h.keys(ports.Ctrl('u'))
	h.drag(3+1, rowText, 3+3, rowText+2, 0)
	if got, want := h.texts(), []string{"bc", "hi", "no"}; !slices.Equal(got, want) {
		t.Errorf("selected %q, want %q", got, want)
	}
	// A plain click collapses it.
	h.click(3, rowText, 0)
	if n := h.doc.CursorCount(); n != 1 {
		t.Errorf("cursors = %d, want 1", n)
	}
}

func TestEscClearsTheRectangle(t *testing.T) {
	h := rectHarness(t, rectText)
	h.drag(3+1, rowText, 3+3, rowText+2, ports.ModAlt)
	h.keys(key(ports.KeyEsc))
	if n := h.doc.CursorCount(); n != 1 {
		t.Errorf("cursors = %d, want 1", n)
	}
}

func TestBlockCopyPaste(t *testing.T) {
	h := rectHarness(t, "abcdef\nx\nghijkl")
	h.drag(3+2, rowText, 3+4, rowText+2, ports.ModAlt)
	h.keys(ports.Ctrl('c'))
	if got, want := h.clip.text, "cd\n\nij"; got != want {
		t.Fatalf("clipboard %q, want %q", got, want)
	}
	if n := h.doc.CursorCount(); n != 3 {
		t.Errorf("copy kept %d cursors, want 3", n)
	}

	// Three cursors at column 0 get one line each.
	h.drag(3, rowText, 3, rowText+2, ports.ModAlt)
	h.keys(ports.Ctrl('v'))
	if got, want := h.doc.Text(), "cdabcdef\nx\nijghijkl"; got != want {
		t.Errorf("block paste: %q, want %q", got, want)
	}
	h.keys(ports.Ctrl('z'))
	if got, want := h.doc.Text(), "abcdef\nx\nghijkl"; got != want {
		t.Errorf("undo of the block paste: %q, want %q", got, want)
	}

	// With another number of cursors the whole text goes to each of them.
	h.click(3, rowText, 0)
	h.click(3, rowText+2, ports.ModAlt)
	h.keys(ports.Ctrl('v'))
	if got, want := h.doc.Text(), "cd\n\nijabcdef\nx\ncd\n\nijghijkl"; got != want {
		t.Errorf("whole paste: %q, want %q", got, want)
	}

	// A single cursor pastes the whole block.
	h2 := rectHarness(t, "k")
	h2.clip.text = "cd\n\nij"
	h2.keys(ports.Ctrl('v'))
	if got, want := h2.doc.Text(), "kcd\n\nij"; got != want {
		t.Errorf("single cursor: %q, want %q", got, want)
	}
}

func TestPasteWholeLinesOntoEachCursor(t *testing.T) {
	// Lines copied with one cursor end in a line break: that is a
	// terminator, so two lines still fit two cursors.
	h := multiCursorHarness(t)
	h.clip.text = "1\n2\n"
	h.keys(ports.Ctrl('v'))
	if got, want := h.doc.Text(), "1ab\n2cd"; got != want {
		t.Errorf("text %q, want %q", got, want)
	}
}

func TestBlockCut(t *testing.T) {
	h := rectHarness(t, rectText)
	h.drag(3+1, rowText, 3+3, rowText+2, ports.ModAlt)
	h.keys(ports.Ctrl('x'))
	if got, want := h.clip.text, "bc\nhi\nno"; got != want {
		t.Errorf("clipboard %q, want %q", got, want)
	}
	if got, want := h.doc.Text(), "adef\ngjkl\nmpqr"; got != want {
		t.Errorf("text %q, want %q", got, want)
	}
}

func TestCopyWithOnlyEmptyCursorsDoesNothing(t *testing.T) {
	h := multiCursorHarness(t)
	h.clip.text = "keep"
	h.keys(ports.Ctrl('x'))
	if h.clip.text != "keep" || h.doc.Text() != "ab\ncd" {
		t.Errorf("clipboard %q, text %q", h.clip.text, h.doc.Text())
	}
	h.wantRowContains(rowPrompt, "nothing selected")
}

func TestColumnSelectionIsInTheSelectionMenu(t *testing.T) {
	h := rectHarness(t, rectText)
	h.keys(ports.CtrlAlt('m'))
	for range 5 {
		h.keys(key(ports.KeyRight)) // File Edit Selection
		if h.menu.open == 2 {
			break
		}
	}
	found := false
	for _, row := range h.screen() {
		if strings.Contains(row, "Column selection") && strings.Contains(row, "Ctrl+U") {
			found = true
		}
	}
	if !found {
		t.Errorf("menu does not list Column selection with its shortcut:\n%q", h.screen())
	}
}

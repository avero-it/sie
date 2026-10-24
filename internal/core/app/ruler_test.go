package app

import (
	"strconv"
	"strings"
	"testing"

	"github.com/avero-it/sie/internal/core/ports"
)

func rulerHarness(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t, nil)
	h.columnRuler = true
	h.Resize(40, 10)
	return h
}

func TestRulerShowsColumns(t *testing.T) {
	h := rulerHarness(t)
	h.typeText("hello")
	// A one-digit line count makes the gutter 3 cells wide.
	h.wantRowPrefix(rowText, "   .........10........20........30")
	h.wantRowContains(rowText+1, "hello")
}

func TestRulerFollowsHorizontalScroll(t *testing.T) {
	h := rulerHarness(t)
	h.typeText(strings.Repeat("x", 100))
	left := h.view.Left
	if left == 0 {
		t.Fatal("did not scroll")
	}
	row := h.row(rowText)[3:]
	next := strconv.Itoa((left/10 + 1) * 10)
	if got, want := strings.Index(row, next), (left/10+1)*10-1-left; got != want {
		t.Errorf("column %s at x=%d, want %d (row %q, left %d)", next, got, want, row, left)
	}
}

func TestRulerHiddenWhenOffWrappingOrTiny(t *testing.T) {
	h := rulerHarness(t)
	h.typeText("a")
	_ = h.Dispatch("view.toggleColumnRuler")
	h.wantRowContains(rowText, "a")
	_ = h.Dispatch("view.toggleColumnRuler")
	h.wantRowContains(rowText, "..")
	_ = h.Dispatch("view.toggleWrap")
	h.wantRowContains(rowText, "a")
	_ = h.Dispatch("view.toggleWrap")
	h.wantRowContains(rowText, "..")
	h.Resize(40, 6) // 3 text rows
	if h.regions().rulerY != 0 {
		t.Error("ruler on a tiny terminal")
	}
}

func TestRulerOffsetsClicks(t *testing.T) {
	h := rulerHarness(t)
	h.typeText("one\ntwo")
	h.click(5, rowText+1, 0) // first text row
	if line, _, _ := h.doc.CursorPos(); line != 0 {
		t.Errorf("clicked line %d, want 0", line)
	}
}

func TestWrapMarksContinuationRows(t *testing.T) {
	h := wrapHarness(t)
	if strings.Contains(h.row(rowText), wrapMark) {
		t.Errorf("first row %q has a marker", h.row(rowText))
	}
	h.wantRow(rowText+1, " "+wrapMark+" dddddddddd")
	if got := h.styleAt(1, rowText+1); got != ports.StyleWrapMark {
		t.Errorf("marker style %v", got)
	}
}

package app

import (
	"fmt"
	"strings"
	"testing"

	"github.com/avero-it/sie/internal/core/ports"
)

// splitHarness is a 40x14 screen (11 text rows) with 30 numbered lines, the
// ruler off, so the panes are rows 1-5 and 7-11 around a divider on row 6.
func splitHarness(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t, nil)
	h.Resize(40, 14)
	var b strings.Builder
	for i := 1; i <= 30; i++ {
		fmt.Fprintf(&b, "line%d\n", i)
	}
	h.typeText(b.String())
	h.Dispatch("search.gotoLine", "1")
	h.Dispatch("view.toggleSplit")
	return h
}

func TestSplitDrawsTwoPanesAndDivider(t *testing.T) {
	h := splitHarness(t)
	r := h.regions()
	if !r.split || r.dividerY != 6 || r.textTop != 1 || r.textRows != 5 || r.other.textTop != 7 || r.other.textRows != 5 {
		t.Fatalf("regions = %+v", r)
	}
	if got := h.styleAt(0, r.dividerY); got != ports.StyleDivider {
		t.Errorf("divider style = %v", got)
	}
	h.wantRowContains(1, "line1")
	h.wantRowContains(7, "line1") // the new pane starts where the focused one is
}

func TestSplitPanesScrollIndependently(t *testing.T) {
	h := splitHarness(t)
	h.HandleMouse(ports.MouseEvent{X: 5, Y: 8, Button: ports.WheelDown})
	h.wantRowContains(1, "line1")
	h.wantRowContains(7, "line4")
}

func TestSplitClickSwitchesPane(t *testing.T) {
	h := splitHarness(t)
	h.HandleMouse(ports.MouseEvent{X: 5, Y: 8, Button: ports.WheelDown}) // lower pane shows line4 first
	h.click(4, 8, 0)                                                     // second row of the lower pane: line 5
	if line, _, _ := h.doc.CursorPos(); line != 4 {
		t.Errorf("cursor line = %d, want 4", line+1)
	}
	if !h.focusBottom {
		t.Error("lower pane not focused")
	}
	if c := h.View().Cursor; !c.Visible || c.Y < 7 || c.Y > 11 { // scroll margin may move it within the lower pane
		t.Errorf("cursor = %+v", c)
	}
}

func TestSplitOffAndTooSmall(t *testing.T) {
	h := splitHarness(t)
	h.Dispatch("view.toggleSplit")
	if r := h.regions(); r.split || r.textRows != 11 {
		t.Errorf("after off: %+v", r)
	}
	h.Dispatch("view.toggleSplit")
	h.Resize(40, 10) // 7 text rows: below minSplitRows
	if r := h.regions(); r.split {
		t.Errorf("tiny screen still split: %+v", r)
	}
}

func TestSplitSwitchPane(t *testing.T) {
	h := splitHarness(t)
	h.Dispatch("view.focusOtherPane")
	if !h.focusBottom {
		t.Error("focus did not move to the lower pane")
	}
	h.Dispatch("view.toggleSplit")
	if err := h.cmdFocusOtherPane(nil); err == nil {
		t.Error("switching panes without a split should fail")
	}
}

func TestSplitRulerPerPane(t *testing.T) {
	h := splitHarness(t)
	h.columnRuler = true
	r := h.regions()
	if r.rulerY != 1 || r.textTop != 2 || r.textRows != 4 || r.other.rulerY != 7 || r.other.textTop != 8 {
		t.Errorf("regions = %+v", r)
	}
}

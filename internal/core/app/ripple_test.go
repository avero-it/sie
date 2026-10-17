package app

import (
	"testing"
	"time"

	"github.com/avero-it/sie/internal/core/ports"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func rippleHarness(t *testing.T) (*harness, *fakeClock) {
	t.Helper()
	h := newHarness(t, nil)
	clk := &fakeClock{t: time.Unix(1000, 0)}
	h.cfg.Now = clk.now
	h.clickRipple = true
	return h, clk
}

// tintedX returns the columns tinted on a screen row.
func tintedX(vm ports.ViewModel, y int) []int {
	var xs []int
	for _, tn := range vm.Tints {
		if tn.Y == y {
			xs = append(xs, tn.X)
		}
	}
	return xs
}

func TestRippleSpreadsAndFades(t *testing.T) {
	h, clk := rippleHarness(t)
	h.typeText("hello")
	if h.View().Animate {
		t.Fatal("animating before any click")
	}
	h.click(20, rowText+3, 0)
	vm := h.View()
	if !vm.Animate || len(vm.Tints) == 0 {
		t.Fatalf("no ripple after a click: animate=%v tints=%d", vm.Animate, len(vm.Tints))
	}
	reach := func() int { // widest distance from the click on its row
		far := 0
		for _, x := range tintedX(h.View(), rowText+3) {
			far = max(far, max(x-20, 20-x))
		}
		return far
	}
	clk.t = clk.t.Add(100 * time.Millisecond)
	early := reach()
	clk.t = clk.t.Add(200 * time.Millisecond)
	if late := reach(); late <= early {
		t.Errorf("ring did not move outward: %d then %d", early, late)
	}
	clk.t = clk.t.Add(300 * time.Millisecond)
	h.Poll()
	if vm := h.View(); vm.Animate || len(vm.Tints) != 0 {
		t.Errorf("ripple outlived its duration: animate=%v tints=%d", vm.Animate, len(vm.Tints))
	}
}

func TestRippleStaysInTextArea(t *testing.T) {
	h, clk := rippleHarness(t)
	h.click(20, rowText, 0)
	clk.t = clk.t.Add(150 * time.Millisecond)
	for _, tn := range h.View().Tints {
		if tn.Y < rowText || tn.Y >= h.regions().textTop+h.regions().textRows {
			t.Fatalf("tint outside the text area: %+v", tn)
		}
	}
}

func TestRippleOnlyOnTextPresses(t *testing.T) {
	h, _ := rippleHarness(t)
	h.click(1, rowMenu, 0) // menu bar
	h.HandleKey(key(ports.KeyEsc))
	h.click(5, h.regions().promptY, 0) // prompt line
	h.HandleKey(key(ports.KeyEsc))
	if len(h.ripples) != 0 {
		t.Errorf("ripple from menu or prompt click: %d", len(h.ripples))
	}
	h.HandleMouse(ports.MouseEvent{X: 10, Y: rowText, Button: ports.MouseLeft, Action: ports.MousePress, Clicks: 1})
	h.HandleMouse(ports.MouseEvent{X: 14, Y: rowText, Action: ports.MouseDrag})
	h.HandleMouse(ports.MouseEvent{X: 14, Y: rowText, Action: ports.MouseRelease})
	if len(h.ripples) != 1 {
		t.Errorf("want one ripple for press+drag, got %d", len(h.ripples))
	}
}

func TestRippleCapAndToggle(t *testing.T) {
	h, _ := rippleHarness(t)
	for i := range 5 {
		h.click(10+i, rowText+1, 0)
	}
	if len(h.ripples) != rippleMax {
		t.Errorf("ripples alive: %d, want %d", len(h.ripples), rippleMax)
	}
	if err := h.Dispatch("view.toggleClickRipple"); err != nil {
		t.Fatal(err)
	}
	if h.clickRipple {
		t.Fatal("toggle did not turn the ripple off")
	}
	h.ripples = nil
	h.click(10, rowText+1, 0)
	if len(h.ripples) != 0 {
		t.Error("ripple started while switched off")
	}
}

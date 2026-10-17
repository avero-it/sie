package app

import (
	"testing"
	"time"

	"github.com/avero-it/sie/internal/core/ports"
)

const guideSample = "def f():\n    x = 1\n    if x:\n        y = 2\n\n    return x"

func guideHarness(t *testing.T) (*harness, *fakeClock) {
	t.Helper()
	h := newHarness(t, nil)
	clk := &fakeClock{t: time.Unix(2000, 0)}
	h.cfg.Now = clk.now
	if err := h.doc.Insert(guideSample); err != nil { // typing would auto-indent
		t.Fatal(err)
	}
	h.indentGuide = true
	h.trackIndent() // baseline: the cursor sits on the last line
	return h, clk
}

func TestIndentGuideTriggers(t *testing.T) {
	h, _ := guideHarness(t)
	// Typing at the end of an indented line does not move its top.
	h.typeText("z")
	if h.guide.on {
		t.Error("typing mid-line fired the guide")
	}
	// Neither does moving the cursor to a line with another indent.
	h.HandleKey(key(ports.KeyUp))
	h.HandleKey(key(ports.KeyUp))
	if h.guide.on {
		t.Error("cursor movement fired the guide")
	}
	// The cursor is on line 3, "        y = 2".
	h.HandleKey(key(ports.KeyHome)) // smart home: before "y"
	h.HandleKey(key(ports.KeyBackspace))
	if !h.guide.on || h.guide.col != 7 {
		t.Fatalf("backspace in the indent: on=%v col=%d, want col 7", h.guide.on, h.guide.col)
	}
	h.guide.on = false
	h.typeText(" ")
	if !h.guide.on || h.guide.col != 8 {
		t.Errorf("space in the indent: on=%v col=%d, want col 8", h.guide.on, h.guide.col)
	}
	// Undo moves the top back.
	h.guide.on = false
	h.Dispatch("edit.undo")
	if !h.guide.on {
		t.Error("undo of an indent change did not fire the guide")
	}
}

func TestIndentGuideIgnoresBlankLines(t *testing.T) {
	h, _ := guideHarness(t)
	h.HandleKey(key(ports.KeyUp))
	h.HandleKey(key(ports.KeyUp)) // the blank line
	h.typeText("   ")
	if h.guide.on {
		t.Error("indenting a blank line fired the guide")
	}
}

func TestIndentGuideDrawsAndExpires(t *testing.T) {
	h, clk := guideHarness(t)
	h.guide = guideState{col: 4, start: clk.t, on: true}
	vm := h.View()
	if !vm.Animate {
		t.Error("guide should ask for frames")
	}
	cell := func(row int) (string, ports.Style) {
		y := rowText + row
		var text string
		var style ports.Style
		for _, r := range vm.Runs {
			if r.Y == y && r.X == 3+4 {
				text, style = r.Text, r.Style
			}
		}
		return text, style
	}
	// Line 1 "    x = 1" starts at column 4: its glyph, reversed.
	if txt, st := cell(1); txt != "x" || st != ports.StyleGuideMatch {
		t.Errorf("row 1: %q style %d", txt, st)
	}
	// Line 0 "def f():" has text at column 4 that is not a line top: tinted.
	tinted := false
	for _, tn := range vm.Tints {
		if tn.Y == rowText && tn.X == 3+4 && tn.Level == guideTintLevel {
			tinted = true
		}
	}
	if !tinted {
		t.Error("row 0 not tinted")
	}
	// Line 3 is deeper ("        y = 2"): column 4 is leading whitespace.
	if txt, st := cell(3); txt != "|" || st != ports.StyleGuide {
		t.Errorf("row 3: %q style %d", txt, st)
	}
	// Blank line 4 and rows past the end get a bar.
	if txt, _ := cell(4); txt != "|" {
		t.Errorf("blank row: %q", txt)
	}
	if txt, _ := cell(6); txt != "|" {
		t.Errorf("row past the end: %q", txt)
	}
	clk.t = clk.t.Add(guideDuration)
	h.Poll()
	if vm := h.View(); vm.Animate {
		t.Error("guide outlived its duration")
	}
}

func TestIndentGuideOffAndScrolledOut(t *testing.T) {
	h, clk := guideHarness(t)
	h.indentGuide = false
	h.HandleKey(key(ports.KeyHome))
	h.typeText(" ")
	if h.guide.on {
		t.Error("guide started while switched off")
	}
	h.indentGuide = true
	h.guide = guideState{col: 4, start: clk.t, on: true}
	h.view.Left = 10
	if h.View().Animate {
		t.Error("guide drawn although its column is scrolled out")
	}
}

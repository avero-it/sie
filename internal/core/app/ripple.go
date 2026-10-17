package app

import (
	"math"
	"time"

	"github.com/avero-it/sie/internal/core/ports"
)

const (
	rippleDuration = 500 * time.Millisecond
	rippleRadius   = 4.0 // rows; columns are twice as many
	rippleRings    = 3
	rippleWidth    = 0.7 // ring half-width, in rows
	rippleMax      = 3   // ripples alive at once
)

// ripple is a click wave: rings spread from a screen cell and fade out.
type ripple struct {
	x, y  int
	start time.Time
}

func (e *Editor) now() time.Time {
	if e.cfg.Now != nil {
		return e.cfg.Now()
	}
	return time.Now()
}

// startRipple adds a wave at a screen cell, dropping the oldest beyond the cap.
func (e *Editor) startRipple(x, y int) {
	if !e.clickRipple {
		return
	}
	e.ripples = append(e.ripples, ripple{x: x, y: y, start: e.now()})
	if len(e.ripples) > rippleMax {
		e.ripples = e.ripples[len(e.ripples)-rippleMax:]
	}
}

// pruneRipples drops finished waves.
func (e *Editor) pruneRipples() {
	now := e.now()
	live := e.ripples[:0]
	for _, rp := range e.ripples {
		if now.Sub(rp.start) < rippleDuration {
			live = append(live, rp)
		}
	}
	e.ripples = live
}

// drawRipples adds the tinted cells of all live waves, clipped to the text area.
func (e *Editor) drawRipples(vm *viewBuilder, r regions) {
	e.pruneRipples()
	if len(e.ripples) == 0 {
		return
	}
	vm.Animate = true
	now := e.now()
	for y := r.textTop; y < r.textTop+r.textRows; y++ {
		for x := 0; x < e.width; x++ {
			var best uint8
			for _, rp := range e.ripples {
				best = max(best, rippleLevel(x-rp.x, y-rp.y, now.Sub(rp.start)))
			}
			if best > 0 {
				vm.Tints = append(vm.Tints, ports.Tint{X: x, Y: y, Level: best})
			}
		}
	}
}

// rippleLevel is the tint (0 none, 1 faint … 3 strong) of the cell at offset
// (dx, dy) from the origin, age after the click. Cells are about twice as
// tall as wide, so dx counts half.
func rippleLevel(dx, dy int, age time.Duration) uint8 {
	p := float64(age) / float64(rippleDuration)
	if p < 0 || p >= 1 {
		return 0
	}
	d := math.Hypot(float64(dx)/2, float64(dy))
	lead := rippleRadius * (1 - (1-p)*(1-p)) // ease out
	var best float64
	for k := range rippleRings {
		off := math.Abs(d - (lead - float64(k)))
		if off >= rippleWidth {
			continue
		}
		s := (1 - p) * (1 - 0.3*float64(k)) * (1 - off/rippleWidth)
		best = max(best, s)
	}
	switch {
	case best > 0.5:
		return 3
	case best > 0.25:
		return 2
	case best > 0.08:
		return 1
	}
	return 0
}

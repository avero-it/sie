package tui

import (
	"time"

	"github.com/avero-it/sie/internal/core/ports"
	"github.com/gdamore/tcell/v2"
)

var specialKeys = map[tcell.Key]ports.Key{
	tcell.KeyEnter:     ports.KeyEnter,
	tcell.KeyLF:        ports.KeyEnter, // Ctrl+J; some terminals send LF for Enter
	tcell.KeyTab:       ports.KeyTab,
	tcell.KeyBackspace: ports.KeyBackspace, // also Ctrl+H on many terminals
	tcell.KeyDelete:    ports.KeyDelete,
	tcell.KeyInsert:    ports.KeyInsert,
	tcell.KeyEsc:       ports.KeyEsc,
	tcell.KeyUp:        ports.KeyUp,
	tcell.KeyDown:      ports.KeyDown,
	tcell.KeyLeft:      ports.KeyLeft,
	tcell.KeyRight:     ports.KeyRight,
	tcell.KeyHome:      ports.KeyHome,
	tcell.KeyEnd:       ports.KeyEnd,
	tcell.KeyPgUp:      ports.KeyPgUp,
	tcell.KeyPgDn:      ports.KeyPgDn,
}

// normalizeKey converts a tcell key event to a ports.KeyEvent, resolving
// terminal aliases. It reports false for keys sie does not use.
func normalizeKey(ev *tcell.EventKey) (ports.KeyEvent, bool) {
	mod := normalizeMods(ev.Modifiers())
	k := ev.Key()
	switch {
	case k == tcell.KeyRune:
		r := ev.Rune()
		if mod&ports.ModCtrl != 0 && r >= 'A' && r <= 'Z' {
			r += 'a' - 'A' // keymaps use lower-case letters for Ctrl combinations
		}
		return ports.KeyEvent{Key: ports.KeyRune, Rune: r, Mod: mod &^ ports.ModShift}, true

	case k == tcell.KeyEnter && mod&ports.ModAlt != 0:
		// Ctrl+Alt+M and Alt+Enter send the same bytes (ESC CR).
		return ports.CtrlAlt('m'), true

	case k == tcell.KeyBacktab:
		return ports.KeyEvent{Key: ports.KeyTab, Mod: mod | ports.ModShift}, true

	case k == tcell.KeyCtrlSpace:
		return ports.KeyEvent{Key: ports.KeyRune, Rune: ' ', Mod: mod | ports.ModCtrl}, true
	}
	if pk, ok := specialKeys[k]; ok {
		if pk == ports.KeyEnter || pk == ports.KeyTab || pk == ports.KeyBackspace || pk == ports.KeyEsc {
			// These are Ctrl+M/J, Ctrl+I, Ctrl+H, Ctrl+[ on the wire: Ctrl is noise.
			mod &^= ports.ModCtrl
		}
		return ports.KeyEvent{Key: pk, Mod: mod}, true
	}
	if r, ok := ctrlLetter(k); ok {
		// With an ESC prefix (Alt) tcell reports Ctrl+Alt+X with only
		// ModAlt set, so Ctrl is implied by the key itself.
		mod = (mod | ports.ModCtrl) &^ ports.ModShift
		if alias, ok := ctrlAliases[r]; ok && mod&ports.ModAlt == 0 {
			// Terminals with extended key reporting can send a distinct
			// Ctrl+M/J/I/H; legacy ones cannot. Behave the same on both.
			return ports.KeyEvent{Key: alias}, true
		}
		return ports.KeyEvent{Key: ports.KeyRune, Rune: r, Mod: mod}, true
	}
	return ports.KeyEvent{}, false
}

// ctrlLetter returns the letter of a Ctrl+letter key. tcell reports these
// either as control bytes (KeySOH..KeySUB, from legacy terminal input) or
// as KeyCtrlA..KeyCtrlZ (from extended key reporting).
func ctrlLetter(k tcell.Key) (rune, bool) {
	switch {
	case k >= tcell.KeyCtrlA && k <= tcell.KeyCtrlZ:
		return rune('a' + (k - tcell.KeyCtrlA)), true
	case k >= tcell.KeySOH && k <= tcell.KeySUB:
		return rune('a' + (k - tcell.KeySOH)), true
	}
	return 0, false
}

// ctrlAliases are Ctrl+letters that legacy terminals send as other keys.
var ctrlAliases = map[rune]ports.Key{
	'm': ports.KeyEnter,
	'j': ports.KeyEnter,
	'i': ports.KeyTab,
	'h': ports.KeyBackspace,
}

func normalizeMods(m tcell.ModMask) ports.Mod {
	var mod ports.Mod
	if m&tcell.ModShift != 0 {
		mod |= ports.ModShift
	}
	if m&tcell.ModCtrl != 0 {
		mod |= ports.ModCtrl
	}
	if m&tcell.ModAlt != 0 {
		mod |= ports.ModAlt
	}
	if m&tcell.ModMeta != 0 {
		// tcell reports the kitty protocol's Super (macOS Cmd) as Meta.
		mod |= ports.ModSuper
	}
	return mod
}

// mouseTracker turns tcell's button-state events into press, drag and
// release transitions.
type mouseTracker struct {
	held tcell.ButtonMask

	lastPress time.Time
	lastX     int
	lastY     int
	clicks    int
}

// multiClickGap is the longest pause between presses of one multi-click.
const multiClickGap = 400 * time.Millisecond

func (t *mouseTracker) normalize(ev *tcell.EventMouse) (ports.MouseEvent, bool) {
	x, y := ev.Position()
	me := ports.MouseEvent{X: x, Y: y, Mod: normalizeMods(ev.Modifiers())}
	buttons := ev.Buttons()
	switch {
	case buttons&tcell.WheelUp != 0:
		me.Button, me.Action = ports.WheelUp, ports.MousePress
		return me, true
	case buttons&tcell.WheelDown != 0:
		me.Button, me.Action = ports.WheelDown, ports.MousePress
		return me, true
	}
	pressed := buttons & (tcell.Button1 | tcell.Button2 | tcell.Button3)
	prev := t.held
	t.held = pressed
	switch {
	case pressed != 0 && prev == 0:
		me.Button, me.Action = mouseButton(pressed), ports.MousePress
		if me.Button == ports.MouseLeft {
			me.Clicks = t.countClick(x, y, ev.When())
		}
	case pressed != 0:
		me.Button, me.Action = mouseButton(pressed), ports.MouseDrag
	case prev != 0:
		me.Button, me.Action = mouseButton(prev), ports.MouseRelease
	default:
		return me, false // plain motion
	}
	return me, true
}

// countClick returns 1, 2 or 3: presses on the same cell in quick succession
// count up, and a fourth starts over.
func (t *mouseTracker) countClick(x, y int, when time.Time) int {
	if t.clicks > 0 && t.clicks < 3 && x == t.lastX && y == t.lastY && when.Sub(t.lastPress) <= multiClickGap {
		t.clicks++
	} else {
		t.clicks = 1
	}
	t.lastPress, t.lastX, t.lastY = when, x, y
	return t.clicks
}

func mouseButton(b tcell.ButtonMask) ports.MouseButton {
	switch {
	case b&tcell.Button1 != 0:
		return ports.MouseLeft
	case b&tcell.Button2 != 0:
		return ports.MouseRight
	default:
		return ports.MouseMiddle
	}
}

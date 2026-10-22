// Package keymap provides the default modeless key bindings (see the keymap
// table in CLAUDE.md). It maps keys to command IDs only.
package keymap

import (
	"fmt"

	"github.com/avero-it/sie/internal/core/ports"
)

// Binding binds one key to a command ID.
type Binding struct {
	Key     ports.KeyEvent
	Command string
}

// Chord binds a two-key sequence (Prefix, then Key) to a command ID.
type Chord struct {
	Prefix  ports.KeyEvent
	Key     ports.KeyEvent
	Command string
}

// Keymap implements ports.Keymap. The first binding of a command is the
// shortcut shown in menus, completions and the echo.
type Keymap struct {
	byKey map[ports.KeyEvent]string
	byCmd map[string]ports.KeyEvent

	prefixes map[ports.KeyEvent]bool
	chords   map[[2]ports.KeyEvent]string
	chordOf  map[string]string // command -> displayed chord
}

var _ ports.Keymap = (*Keymap)(nil)

// New builds a keymap; a key may be bound only once, and a chord prefix
// cannot also be a single-key binding.
func New(bindings []Binding, chords []Chord) (*Keymap, error) {
	km := &Keymap{
		byKey: map[ports.KeyEvent]string{}, byCmd: map[string]ports.KeyEvent{},
		prefixes: map[ports.KeyEvent]bool{}, chords: map[[2]ports.KeyEvent]string{}, chordOf: map[string]string{},
	}
	for _, b := range bindings {
		if prev, dup := km.byKey[b.Key]; dup {
			return nil, fmt.Errorf("keymap: %s bound to both %q and %q", b.Key, prev, b.Command)
		}
		km.byKey[b.Key] = b.Command
		if _, ok := km.byCmd[b.Command]; !ok {
			km.byCmd[b.Command] = b.Key
		}
	}
	for _, c := range chords {
		if prev, ok := km.byKey[c.Prefix]; ok {
			return nil, fmt.Errorf("keymap: chord prefix %s is also bound to %q", c.Prefix, prev)
		}
		seq := [2]ports.KeyEvent{c.Prefix, c.Key}
		if prev, dup := km.chords[seq]; dup {
			return nil, fmt.Errorf("keymap: %s %s bound to both %q and %q", c.Prefix, c.Key, prev, c.Command)
		}
		km.prefixes[c.Prefix] = true
		km.chords[seq] = c.Command
		if _, ok := km.chordOf[c.Command]; !ok {
			km.chordOf[c.Command] = c.Prefix.String() + " " + c.Key.String()
		}
	}
	return km, nil
}

// Default returns the v0 modeless keymap. Ctrl+Alt+letter is reserved for
// editor-level functions. Never bind Ctrl+M/I/[ (Enter/Tab/Esc), Ctrl+H/J,
// Ctrl+Shift combinations or Alt+Enter (it is Ctrl+Alt+M).
func Default() *Keymap {
	km, err := New(DefaultBindings(), DefaultChords())
	if err != nil {
		panic(err) // static table; covered by tests
	}
	return km
}

// DefaultBindings returns the default binding table.
func DefaultBindings() []Binding {
	c := ports.Ctrl
	return []Binding{
		{c('n'), "file.new"},
		{c('o'), "file.open"},
		{c('e'), "file.openRecent"},
		{c('s'), "file.save"},
		{c('w'), "file.close"},
		{c('q'), "file.quit"},
		{c('z'), "edit.undo"},
		{c('y'), "edit.redo"},
		{c('x'), "edit.cut"},
		{c('c'), "edit.copy"},
		{c('v'), "edit.paste"},
		{c('d'), "edit.duplicateLine"},
		{c('k'), "edit.deleteLine"},
		{c('a'), "selection.all"},
		{c('l'), "selection.line"},
		{c('f'), "search.find"},
		{c('r'), "search.replace"},
		{c('g'), "search.gotoLine"},
		{c('t'), "edit.validate"},
		{c('u'), "selection.toggleColumnMode"},
		{ports.CtrlAlt('p'), "ui.prompt"},
		{ports.CtrlAlt('m'), "ui.menu"},
	}
}

// DefaultChords returns the Ctrl+P chords. Every command that has no single
// key is reachable here.
func DefaultChords() []Chord {
	p := ports.Ctrl('p')
	r := func(l rune) ports.KeyEvent { return ports.KeyEvent{Key: ports.KeyRune, Rune: l} }
	var out []Chord
	for _, c := range []struct {
		key rune
		cmd string
	}{
		{'a', "file.saveAs"},
		{'k', "edit.deleteToLineEnd"},
		{'l', "selection.trimLeadingSpaces"},
		{'t', "selection.trimTrailingSpaces"},
		{'p', "selection.alignPrevious"},
		{'n', "selection.alignNext"},
		{'f', "search.next"},
		{'b', "search.prev"},
		{'r', "search.replaceAll"},
		{'i', "view.toggleInvisibles"},
		{'e', "view.toggleEmptyCells"},
		{'c', "view.toggleClickRipple"},
		{'g', "view.toggleIndentGuide"},
		{'s', "view.toggleSyntax"},
		{'y', "view.setLanguage"},
		{'w', "view.toggleValidateOnSave"},
		{'h', "help.about"},
	} {
		out = append(out, Chord{p, r(c.key), c.cmd})
	}
	return out
}

// Command returns the command bound to k.
func (km *Keymap) Command(k ports.KeyEvent) (string, bool) {
	id, ok := km.byKey[k]
	return id, ok
}

// IsPrefix reports whether k starts a chord.
func (km *Keymap) IsPrefix(k ports.KeyEvent) bool { return km.prefixes[k] }

// Chord returns the command bound to prefix followed by k. Holding Ctrl
// through the second key (Ctrl+P Ctrl+A) works like releasing it.
func (km *Keymap) Chord(prefix, k ports.KeyEvent) (string, bool) {
	if id, ok := km.chords[[2]ports.KeyEvent{prefix, k}]; ok {
		return id, true
	}
	if k.Key == ports.KeyRune && k.Mod == ports.ModCtrl {
		k.Mod = 0
		id, ok := km.chords[[2]ports.KeyEvent{prefix, k}]
		return id, ok
	}
	return "", false
}

// Shortcut returns the single key of a command, else its chord.
func (km *Keymap) Shortcut(id string) string {
	if k, ok := km.byCmd[id]; ok {
		return k.String()
	}
	return km.chordOf[id]
}

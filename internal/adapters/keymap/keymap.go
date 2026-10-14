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

// Keymap implements ports.Keymap. The first binding of a command is the
// shortcut shown in menus, completions and the echo.
type Keymap struct {
	byKey map[ports.KeyEvent]string
	byCmd map[string]ports.KeyEvent
}

var _ ports.Keymap = (*Keymap)(nil)

// New builds a keymap; a key may be bound only once.
func New(bindings []Binding) (*Keymap, error) {
	km := &Keymap{byKey: map[ports.KeyEvent]string{}, byCmd: map[string]ports.KeyEvent{}}
	for _, b := range bindings {
		if prev, dup := km.byKey[b.Key]; dup {
			return nil, fmt.Errorf("keymap: %s bound to both %q and %q", b.Key, prev, b.Command)
		}
		km.byKey[b.Key] = b.Command
		if _, ok := km.byCmd[b.Command]; !ok {
			km.byCmd[b.Command] = b.Key
		}
	}
	return km, nil
}

// Default returns the v0 modeless keymap. Ctrl+Alt+letter is reserved for
// editor-level functions. Never bind Ctrl+M/I/[ (Enter/Tab/Esc), Ctrl+H/J,
// Ctrl+Shift combinations or Alt+Enter (it is Ctrl+Alt+M).
func Default() *Keymap {
	km, err := New(DefaultBindings())
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
		{ports.CtrlAlt('p'), "ui.prompt"},
		{ports.CtrlAlt('m'), "ui.menu"},
	}
}

// Command returns the command bound to k.
func (km *Keymap) Command(k ports.KeyEvent) (string, bool) {
	id, ok := km.byKey[k]
	return id, ok
}

// Binding returns the shortcut of a command.
func (km *Keymap) Binding(id string) (ports.KeyEvent, bool) {
	k, ok := km.byCmd[id]
	return k, ok
}

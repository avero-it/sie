// Package ports defines the boundary of the core: the driving interface UIs
// call (EditorService), the driven interfaces the core needs (FileStore,
// Clipboard, Keymap) and the plain value types crossing the boundary. It
// contains no logic beyond trivial formatting.
package ports

import (
	"strings"
	"unicode"
)

// Key identifies a non-text key. Text keys use KeyRune with KeyEvent.Rune.
type Key uint8

const (
	KeyRune Key = iota
	KeyEnter
	KeyTab
	KeyBackspace
	KeyDelete
	KeyInsert
	KeyEsc
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyHome
	KeyEnd
	KeyPgUp
	KeyPgDn
)

var keyNames = map[Key]string{
	KeyEnter: "Enter", KeyTab: "Tab", KeyBackspace: "Backspace", KeyDelete: "Del",
	KeyInsert: "Ins", KeyEsc: "Esc", KeyUp: "Up", KeyDown: "Down", KeyLeft: "Left",
	KeyRight: "Right", KeyHome: "Home", KeyEnd: "End", KeyPgUp: "PgUp", KeyPgDn: "PgDn",
}

// Mod is a set of modifier keys.
type Mod uint8

const (
	ModShift Mod = 1 << iota
	ModCtrl
	ModAlt
	// ModSuper is Cmd on macOS (Super/Win elsewhere). Only terminals with
	// extended key reporting (kitty protocol) deliver it.
	ModSuper
)

// KeyEvent is a normalized key press. Adapters must deliver Ctrl/Alt
// combinations of letters as KeyRune with a lower-case Rune and the
// modifiers set (Ctrl+S is {KeyRune, 's', ModCtrl}), and resolve terminal
// aliases (Alt+Enter arrives as Ctrl+Alt+M). KeyEvent is comparable and can
// be used as a map key.
type KeyEvent struct {
	Key  Key
	Rune rune
	Mod  Mod
}

// Ctrl returns the Ctrl+r key event.
func Ctrl(r rune) KeyEvent { return KeyEvent{Key: KeyRune, Rune: unicode.ToLower(r), Mod: ModCtrl} }

// CtrlAlt returns the Ctrl+Alt+r key event.
func CtrlAlt(r rune) KeyEvent {
	return KeyEvent{Key: KeyRune, Rune: unicode.ToLower(r), Mod: ModCtrl | ModAlt}
}

// String formats the key for display, e.g. "Ctrl+S", "Ctrl+Alt+P", "Shift+Left".
func (k KeyEvent) String() string {
	var b strings.Builder
	if k.Mod&ModSuper != 0 {
		b.WriteString("Cmd+")
	}
	if k.Mod&ModCtrl != 0 {
		b.WriteString("Ctrl+")
	}
	if k.Mod&ModAlt != 0 {
		b.WriteString("Alt+")
	}
	if k.Mod&ModShift != 0 {
		b.WriteString("Shift+")
	}
	switch {
	case k.Key != KeyRune:
		b.WriteString(keyNames[k.Key])
	case k.Rune == ' ':
		b.WriteString("Space")
	default:
		b.WriteRune(unicode.ToUpper(k.Rune))
	}
	return b.String()
}

// MouseButton is the button involved in a mouse event.
type MouseButton uint8

const (
	MouseNone MouseButton = iota
	MouseLeft
	MouseMiddle
	MouseRight
	WheelUp
	WheelDown
)

// MouseAction is what happened to the button.
type MouseAction uint8

const (
	MousePress MouseAction = iota
	MouseRelease
	MouseDrag // motion with the button held
)

// MouseEvent is a normalized mouse event in screen cells (0-based).
type MouseEvent struct {
	X, Y   int
	Button MouseButton
	Action MouseAction
	Mod    Mod
}

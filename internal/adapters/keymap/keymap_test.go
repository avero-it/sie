package keymap

import (
	"testing"

	"github.com/avero-it/sie/internal/core/ports"
)

func TestDefaultIsValid(t *testing.T) {
	km := Default()
	if id, ok := km.Command(ports.Ctrl('s')); !ok || id != "file.save" {
		t.Errorf("Ctrl+S = %q, %v", id, ok)
	}
	if k, ok := km.Binding("ui.prompt"); !ok || k.String() != "Ctrl+Alt+P" {
		t.Errorf("ui.prompt binding = %v, %v", k, ok)
	}
}

// TestDefaultAvoidsUnsafeKeys enforces the terminal key rules from CLAUDE.md.
func TestDefaultAvoidsUnsafeKeys(t *testing.T) {
	for _, b := range DefaultBindings() {
		k := b.Key
		ctrl := k.Mod&ports.ModCtrl != 0
		alt := k.Mod&ports.ModAlt != 0
		switch {
		case k.Key == ports.KeyRune && ctrl && !alt && (k.Rune == 'm' || k.Rune == 'i' || k.Rune == '[' || k.Rune == 'h' || k.Rune == 'j'):
			t.Errorf("%s (%s) aliases Enter/Tab/Esc/Backspace/LF in terminals", k, b.Command)
		case ctrl && k.Mod&ports.ModShift != 0:
			t.Errorf("%s (%s): Ctrl+Shift is unreliable in terminals", k, b.Command)
		case k.Key == ports.KeyEnter && alt:
			t.Errorf("%s (%s): Alt+Enter is Ctrl+Alt+M", k, b.Command)
		case ctrl && alt && b.Command[:3] != "ui.":
			t.Errorf("%s (%s): Ctrl+Alt is reserved for editor-level (ui.*) commands", k, b.Command)
		}
	}
}

func TestDuplicateKeyRejected(t *testing.T) {
	_, err := New([]Binding{{ports.Ctrl('s'), "a"}, {ports.Ctrl('s'), "b"}})
	if err == nil {
		t.Error("duplicate key accepted")
	}
}

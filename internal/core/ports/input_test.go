package ports

import "testing"

func TestKeyEventString(t *testing.T) {
	tests := []struct {
		k    KeyEvent
		want string
	}{
		{Ctrl('s'), "Ctrl+S"},
		{Ctrl('S'), "Ctrl+S"},
		{CtrlAlt('p'), "Ctrl+Alt+P"},
		{KeyEvent{Key: KeyLeft, Mod: ModShift}, "Shift+Left"},
		{KeyEvent{Key: KeyRune, Rune: ' ', Mod: ModCtrl}, "Ctrl+Space"},
		{KeyEvent{Key: KeyEsc}, "Esc"},
	}
	for _, tt := range tests {
		if got := tt.k.String(); got != tt.want {
			t.Errorf("%+v: got %q, want %q", tt.k, got, tt.want)
		}
	}
}

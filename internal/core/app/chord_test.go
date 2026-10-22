package app

import (
	"testing"

	"github.com/avero-it/sie/internal/core/ports"
)

func chord(r rune) ports.KeyEvent { return ports.KeyEvent{Key: ports.KeyRune, Rune: r} }

func TestChordRunsCommandAndEchoesShortcut(t *testing.T) {
	h := newHarness(t, nil)
	h.typeText("  x")
	h.keys(ports.Ctrl('p'))
	h.wantRowPrefix(rowPrompt, "Ctrl+P …")
	h.keys(chord('l'))
	if got := h.doc.Text(); got != "x" {
		t.Errorf("text %q", got)
	}
	h.wantRowContains(rowPrompt, "Trim leading spaces")
	h.wantRowContains(rowPrompt, "Ctrl+P L")
}

func TestChordCancelAndUnbound(t *testing.T) {
	h := newHarness(t, nil)
	h.typeText("  x")
	h.keys(ports.Ctrl('p'), key(ports.KeyEsc))
	h.keys(chord('l')) // typed as text again
	if got := h.doc.Text(); got != "  xl" {
		t.Errorf("text %q", got)
	}
	h.keys(ports.Ctrl('p'), chord('z'))
	h.wantRowContains(rowPrompt, "is not bound")
}

func TestAlignLines(t *testing.T) {
	tests := []struct{ name, text, chord, want string }{
		{"prev", "\tab\n    cd", "p", "\tab\n\tcd"},
		{"next", "ab\n  cd\n\n\tef", "n", "ab\n\tcd\n\n\tef"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, nil)
			if err := h.doc.Insert(tt.text); err != nil {
				t.Fatal(err)
			}
			h.keys(ports.KeyEvent{Key: ports.KeyHome, Mod: ports.ModCtrl})
			h.keys(key(ports.KeyDown))
			h.keys(ports.Ctrl('p'), chord(rune(tt.chord[0])))
			if got := h.doc.Text(); got != tt.want {
				t.Errorf("text %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewSelectionCommandsAreInTheMenu(t *testing.T) {
	want := map[string]bool{"selection.trimLeadingSpaces": true, "selection.alignPrevious": true, "selection.alignNext": true}
	for _, m := range DefaultMenus() {
		for _, id := range m.Items {
			delete(want, id)
		}
	}
	if len(want) != 0 {
		t.Errorf("not in any menu: %v", want)
	}
}

package app

import (
	"bytes"
	"testing"

	"github.com/avero-it/sie/internal/core/ports"
)

// benchLongLine opens a 5 MB single-line JSON-like file (minified JSON is
// the usual way to meet one) with the cursor at its start or end.
func benchLongLine(b *testing.B, end bool) *harness {
	line := bytes.Repeat([]byte(`{"k":"valué","n":12345},`), 5_000_000/25)
	h := newHarness(b, map[string]string{"big.json": string(line) + "\n"})
	h.Resize(120, 40)
	if err := h.Dispatch("file.open", "big.json"); err != nil {
		b.Fatal(err)
	}
	if end {
		h.HandleKey(key(ports.KeyEnd))
	}
	h.View()
	return h
}

// Each iteration is one key press and one frame, as in the event loop.
func BenchmarkLongLineArrowAtStart(b *testing.B) {
	h := benchLongLine(b, false)
	for b.Loop() {
		h.HandleKey(key(ports.KeyRight))
		h.View()
		h.HandleKey(key(ports.KeyLeft))
		h.View()
	}
}

func BenchmarkLongLineArrowAtEnd(b *testing.B) {
	h := benchLongLine(b, true)
	for b.Loop() {
		h.HandleKey(key(ports.KeyLeft))
		h.View()
		h.HandleKey(key(ports.KeyRight))
		h.View()
	}
}

func BenchmarkLongLineTypeAtEnd(b *testing.B) {
	h := benchLongLine(b, true)
	for b.Loop() {
		h.HandleKey(ports.KeyEvent{Key: ports.KeyRune, Rune: 'x'})
		h.View()
	}
}

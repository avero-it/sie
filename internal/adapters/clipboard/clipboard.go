// Package clipboard implements ports.Clipboard. Copy always goes out via
// OSC52 (works over SSH in most modern terminals) when a writer is given,
// and also to the native clipboard when one is available. Paste needs the
// native clipboard; without it Paste fails and the app falls back to its
// internal clipboard (terminal paste still arrives as bracketed paste).
package clipboard

import (
	"errors"

	"github.com/avero-it/sie/internal/core/ports"
)

// ErrUnavailable means no clipboard mechanism is available.
var ErrUnavailable = errors.New("clipboard unavailable")

// native is a platform clipboard.
type native interface {
	copy(text string) error
	paste() (string, error)
}

// Clipboard is the system clipboard adapter.
type Clipboard struct {
	osc52  func(data []byte)
	native native
}

var _ ports.Clipboard = (*Clipboard)(nil)

// New returns a clipboard that copies through osc52 (may be nil) and the
// native platform clipboard, if one is found.
func New(osc52 func(data []byte)) *Clipboard {
	return &Clipboard{osc52: osc52, native: detectNative()}
}

// Copy puts text on the clipboard.
func (c *Clipboard) Copy(text string) error {
	if c.osc52 != nil {
		c.osc52([]byte(text))
	}
	if c.native != nil {
		if err := c.native.copy(text); err != nil && c.osc52 == nil {
			return err
		}
		return nil
	}
	if c.osc52 == nil {
		return ErrUnavailable
	}
	return nil
}

// Paste returns the clipboard text.
func (c *Clipboard) Paste() (string, error) {
	if c.native == nil {
		return "", ErrUnavailable
	}
	return c.native.paste()
}

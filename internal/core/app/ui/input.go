// Package ui holds small presentation models used by the app layer: the
// single-line prompt input and the fuzzy matcher.
package ui

import (
	"github.com/rivo/uniseg"
)

// Input is a single-line text field with a cursor, edited by grapheme cluster.
type Input struct {
	text   string
	cursor int // byte offset, always on a grapheme boundary
}

// Set replaces the text and puts the cursor at the end.
func (in *Input) Set(s string) {
	in.text = s
	in.cursor = len(s)
}

// String returns the text.
func (in *Input) String() string { return in.text }

// Insert inserts s at the cursor.
func (in *Input) Insert(s string) {
	in.text = in.text[:in.cursor] + s + in.text[in.cursor:]
	in.cursor += len(s)
}

// Backspace deletes the grapheme before the cursor.
func (in *Input) Backspace() {
	prev := in.prev()
	in.text = in.text[:prev] + in.text[in.cursor:]
	in.cursor = prev
}

// Delete deletes the grapheme after the cursor.
func (in *Input) Delete() {
	in.text = in.text[:in.cursor] + in.text[in.next():]
}

// Left moves the cursor one grapheme left.
func (in *Input) Left() { in.cursor = in.prev() }

// Right moves the cursor one grapheme right.
func (in *Input) Right() { in.cursor = in.next() }

// Home moves the cursor to the start.
func (in *Input) Home() { in.cursor = 0 }

// End moves the cursor to the end.
func (in *Input) End() { in.cursor = len(in.text) }

// CursorCells returns the screen width of the text before the cursor.
func (in *Input) CursorCells() int { return uniseg.StringWidth(in.text[:in.cursor]) }

// prev returns the start of the grapheme cluster ending at the cursor.
func (in *Input) prev() int {
	prev, pos, state := 0, 0, -1
	rest := in.text
	for len(rest) > 0 && pos < in.cursor {
		prev = pos
		var cluster string
		cluster, rest, _, state = uniseg.FirstGraphemeClusterInString(rest, state)
		pos += len(cluster)
	}
	return prev
}

func (in *Input) next() int {
	if in.cursor >= len(in.text) {
		return len(in.text)
	}
	cluster, _, _, _ := uniseg.FirstGraphemeClusterInString(in.text[in.cursor:], -1)
	return in.cursor + len(cluster)
}

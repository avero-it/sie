// Package history implements undo/redo as reversible edits on a text target.
//
// Each Edit is "at Off, Removed was replaced by Inserted", so it can be applied
// in both directions. Edits are grouped: one undo step reverts one Group. The
// caller decides grouping through a coalescing key (e.g. consecutive typing)
// and Seal (e.g. on cursor movement). S is caller state restored on undo/redo,
// typically the cursor and selection.
package history

import "fmt"

// Target is the text a history applies edits to.
type Target interface {
	Insert(off int, text []byte) error
	Delete(off, n int) ([]byte, error)
}

// Edit replaces Removed with Inserted at byte offset Off.
type Edit struct {
	Off      int
	Removed  []byte
	Inserted []byte
}

type group[S any] struct {
	id            uint64
	key           string
	edits         []Edit
	before, after S
}

// History is an undo/redo stack. Create it with New.
type History[S any] struct {
	undo, redo []group[S]
	limit      int
	nextID     uint64
	sealed     bool
	// savedID is the id of the top undo group when the document was last
	// saved: 0 means "empty undo stack", unreachable means the saved state
	// can no longer be reached.
	savedID uint64
	// grouping is true between BeginGroup and EndGroup; groupID is the
	// group created by the first edit recorded since BeginGroup (0 if none).
	grouping bool
	groupID  uint64
}

const unreachable = ^uint64(0)

// New returns a history keeping at most limit undo steps (limit <= 0 means unlimited).
func New[S any](limit int) *History[S] {
	return &History[S]{limit: limit, nextID: 1}
}

// Record adds an edit that has already been applied to the target. If key is
// non-empty and equals the key of the previous unsealed group, the edit is
// merged into that group (one undo step). Recording clears the redo stack.
func (h *History[S]) Record(e Edit, before, after S, key string) {
	if len(h.redo) > 0 {
		for _, g := range h.redo {
			if g.id == h.savedID {
				h.savedID = unreachable
			}
		}
		h.redo = nil
	}
	if n := len(h.undo); n > 0 {
		last := &h.undo[n-1]
		inGroup := h.grouping && h.groupID == last.id
		coalesce := !h.grouping && !h.sealed && key != "" && last.key == key && last.id != h.savedID
		if inGroup || coalesce {
			last.edits = append(last.edits, e)
			last.after = after
			return
		}
	}
	h.undo = append(h.undo, group[S]{id: h.nextID, key: key, edits: []Edit{e}, before: before, after: after})
	if h.grouping {
		h.groupID = h.nextID
	}
	h.nextID++
	h.sealed = false
	if h.limit > 0 && len(h.undo) > h.limit {
		dropped := h.undo[0]
		h.undo = h.undo[1:]
		switch h.savedID {
		case dropped.id:
			h.savedID = 0
		case 0:
			h.savedID = unreachable
		}
	}
}

// BeginGroup starts an explicit group: every edit recorded until EndGroup
// forms one undo step, regardless of keys and Seal calls in between.
func (h *History[S]) BeginGroup() {
	h.grouping, h.groupID, h.sealed = true, 0, true
}

// EndGroup ends the explicit group started by BeginGroup.
func (h *History[S]) EndGroup() {
	h.grouping, h.groupID, h.sealed = false, 0, true
}

// Seal ends the current group: the next Record starts a new undo step.
func (h *History[S]) Seal() { h.sealed = true }

// CanUndo reports whether there is a step to undo.
func (h *History[S]) CanUndo() bool { return len(h.undo) > 0 }

// CanRedo reports whether there is a step to redo.
func (h *History[S]) CanRedo() bool { return len(h.redo) > 0 }

// Undo reverts the last group on t and returns the state from before it.
// ok is false when there is nothing to undo.
func (h *History[S]) Undo(t Target) (state S, ok bool, err error) {
	n := len(h.undo)
	if n == 0 {
		return state, false, nil
	}
	g := h.undo[n-1]
	for i := len(g.edits) - 1; i >= 0; i-- {
		e := g.edits[i]
		if err := replace(t, e.Off, len(e.Inserted), e.Removed); err != nil {
			return state, false, fmt.Errorf("undo: %w", err)
		}
	}
	h.undo = h.undo[:n-1]
	h.redo = append(h.redo, g)
	h.sealed = true
	return g.before, true, nil
}

// Redo re-applies the last undone group on t and returns the state after it.
// ok is false when there is nothing to redo.
func (h *History[S]) Redo(t Target) (state S, ok bool, err error) {
	n := len(h.redo)
	if n == 0 {
		return state, false, nil
	}
	g := h.redo[n-1]
	for _, e := range g.edits {
		if err := replace(t, e.Off, len(e.Removed), e.Inserted); err != nil {
			return state, false, fmt.Errorf("redo: %w", err)
		}
	}
	h.redo = h.redo[:n-1]
	h.undo = append(h.undo, g)
	h.sealed = true
	return g.after, true, nil
}

// MarkSaved records the current state as the saved one.
func (h *History[S]) MarkSaved() {
	h.savedID = h.topID()
	h.sealed = true
}

// Modified reports whether the current state differs from the last saved one.
func (h *History[S]) Modified() bool { return h.topID() != h.savedID }

func (h *History[S]) topID() uint64 {
	if n := len(h.undo); n > 0 {
		return h.undo[n-1].id
	}
	return 0
}

func replace(t Target, off, n int, text []byte) error {
	if _, err := t.Delete(off, n); err != nil {
		return err
	}
	return t.Insert(off, text)
}

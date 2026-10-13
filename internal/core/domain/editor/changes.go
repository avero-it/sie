package editor

// Change describes one text mutation: at Off, Removed bytes were replaced by
// Inserted bytes. Version is the document version after the change.
type Change struct {
	Version           uint64
	Off               int
	Removed, Inserted int
}

// changeLogSize bounds the change log; older changes are dropped.
const changeLogSize = 1024

// Version increases by one with every text mutation (edits, undo, redo).
// Selection changes do not count.
func (d *Document) Version() uint64 { return d.version }

// ChangesSince returns the changes made after version v, oldest first. ok is
// false when the log no longer reaches back to v (or v is in the future),
// in which case callers must treat everything derived from v as stale.
func (d *Document) ChangesSince(v uint64) (changes []Change, ok bool) {
	switch {
	case v == d.version:
		return nil, true
	case v > d.version:
		return nil, false
	}
	if len(d.changes) == 0 || d.changes[0].Version > v+1 {
		return nil, false
	}
	for i, c := range d.changes {
		if c.Version > v {
			return append([]Change(nil), d.changes[i:]...), true
		}
	}
	return nil, true
}

func (d *Document) logChange(off, removed, inserted int) {
	if removed == 0 && inserted == 0 {
		return
	}
	d.version++
	if len(d.changes) == changeLogSize {
		d.changes = append(d.changes[:0], d.changes[1:]...)
	}
	d.changes = append(d.changes, Change{Version: d.version, Off: off, Removed: removed, Inserted: inserted})
}

// recorder lets undo/redo mutate the buffer while logging the changes.
type recorder struct{ d *Document }

func (r recorder) Insert(off int, text []byte) error {
	if err := r.d.buf.Insert(off, text); err != nil {
		return err
	}
	r.d.logChange(off, 0, len(text))
	return nil
}

func (r recorder) Delete(off, n int) ([]byte, error) {
	removed, err := r.d.buf.Delete(off, n)
	if err != nil {
		return nil, err
	}
	r.d.logChange(off, len(removed), 0)
	return removed, nil
}

package editor

import "slices"

// cursors is the selection state saved in the undo history: the primary
// selection and the secondary cursors.
type cursors struct {
	primary Selection
	extra   []Selection
}

// state returns a copy of the current selections for the history.
func (d *Document) state() cursors {
	if d.op != nil {
		return *d.op
	}
	return cursors{d.sel, slices.Clone(d.extra)}
}

// restore sets the selections from a history state.
func (d *Document) restore(s cursors) {
	d.sel = Selection{d.snap(s.primary.Anchor), d.snap(s.primary.Head)}
	d.extra = d.extra[:0]
	for _, x := range s.extra {
		d.extra = append(d.extra, Selection{d.snap(x.Anchor), d.snap(x.Head)})
	}
	d.goal = -1
	d.mergeCursors()
}

// Selections returns every selection, the primary one first.
func (d *Document) Selections() []Selection {
	return append([]Selection{d.sel}, d.extra...)
}

// CursorCount returns the number of cursors (1 plus the secondary ones).
func (d *Document) CursorCount() int { return 1 + len(d.extra) }

// AddCursor adds a cursor at off, which becomes the primary one; the old
// primary selection is kept as a secondary one. If a cursor (or a selection
// end) is already at off, that cursor is removed instead, unless it is the
// only one.
func (d *Document) AddCursor(off int) {
	d.hist.Seal()
	d.goal = -1
	off = d.snap(off)
	all := d.Selections()
	for i, s := range all {
		if s.Head == off || s.Anchor == off {
			if len(all) == 1 {
				return
			}
			all = slices.Delete(all, i, i+1)
			d.sel, d.extra = all[0], all[1:]
			return
		}
	}
	d.extra = append(d.extra, d.sel)
	d.sel = Cursor(off)
	d.mergeCursors()
}

// ClearSecondary removes the secondary cursors, keeping the primary
// selection. It reports whether there were any.
func (d *Document) ClearSecondary() bool {
	had := len(d.extra) > 0
	d.extra = nil
	return had
}

// forEachCursor runs op once per cursor, with d.sel set to that cursor and
// d.extra holding all the others, from the last cursor in the document to
// the first. Edits made by op go through replace, which shifts the other
// cursors, so each op sees valid offsets. All the edits form one undo step
// (or join the typing group), restoring every cursor.
func (d *Document) forEachCursor(op func() error) error {
	if len(d.extra) == 0 {
		return op()
	}
	before := d.state()
	d.op = &before
	d.opSeq++
	defer func() { d.op, d.rank = nil, 0 }()
	version := d.version

	// Cursors keep their index in all; 0 is the primary one.
	all := d.Selections()
	order := make([]int, len(all))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return cmpSel(all[b], all[a]) })

	var err error
	for k, i := range order {
		d.rank = len(order) - 1 - k
		d.sel = all[i]
		d.extra = slices.Delete(slices.Clone(all), i, i+1)
		err = op()
		all = slices.Insert(d.extra, i, d.sel)
		if err != nil {
			break
		}
	}
	d.sel, d.extra = all[0], all[1:]
	if d.version != version {
		// Inside the run the other cursors are only shifted: snapping each
		// of them after every edit made big rectangles quadratic. Edits go
		// from the last cursor to the first, so the cursors still to be
		// processed are before the edit and can't be affected by a fusion
		// of clusters. Snap once, at the end.
		for i, s := range d.extra {
			d.extra[i] = Selection{d.snapForward(s.Anchor), d.snapForward(s.Head)}
		}
	}
	d.mergeCursors()
	d.op = nil
	if d.version != version {
		d.hist.Amend(d.state())
	}
	return err
}

// mergeCursors drops secondary cursors that coincide with or overlap
// another selection (the primary one wins) and sorts them.
func (d *Document) mergeCursors() {
	if len(d.extra) == 0 {
		return
	}
	slices.SortFunc(d.extra, cmpSel)
	kept := d.extra[:0]
	for _, s := range d.extra {
		if overlaps(s, d.sel) || (len(kept) > 0 && overlaps(s, kept[len(kept)-1])) {
			continue
		}
		kept = append(kept, s)
	}
	d.extra = kept
}

// overlaps reports whether two selections share text or a cursor position.
func overlaps(a, b Selection) bool {
	as, ae := a.Range()
	bs, be := b.Range()
	if as == ae || bs == be {
		return as <= be && bs <= ae
	}
	return as < be && bs < ae
}

func cmpSel(a, b Selection) int {
	as, ae := a.Range()
	bs, be := b.Range()
	if as != bs {
		return as - bs
	}
	return ae - be
}

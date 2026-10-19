package editor

import (
	"bytes"
	"fmt"
	"unicode/utf8"
)

// SearchOptions controls matching.
type SearchOptions struct {
	CaseSensitive bool
}

// Find selects the next match of query after the selection (or the previous
// one before it when forward is false), wrapping around the document. It
// reports whether a match was found.
//
// Case-insensitive matching uses simple Unicode case folding on byte spans of
// len(query); matches whose folded form has a different byte length are missed.
func (d *Document) Find(query string, forward bool, opts SearchOptions) bool {
	if query == "" {
		return false
	}
	q := []byte(query)
	start, end := d.sel.Range()
	n := d.buf.Len()
	var at int
	if forward {
		at = d.scan(q, end, n, opts)
		if at < 0 {
			at = d.scan(q, 0, end, opts)
		}
	} else {
		at = d.scanBack(q, 0, start, opts)
		if at < 0 {
			at = d.scanBack(q, start, n, opts)
		}
	}
	if at < 0 {
		return false
	}
	d.SetSelection(Selection{at, at + len(q)})
	return true
}

// FindNext returns the start of the first match of query at or after from,
// without wrapping and without changing the selection.
func (d *Document) FindNext(query string, from int, opts SearchOptions) (int, bool) {
	if query == "" || from < 0 {
		return 0, false
	}
	at := d.scan([]byte(query), from, d.buf.Len(), opts)
	return at, at >= 0
}

// ReplaceRange replaces [start, end) with text (line breaks normalized to
// the document's line ending), puts the cursor after it and returns the
// inserted length. Inside BeginUndoGroup/EndUndoGroup it joins that step.
func (d *Document) ReplaceRange(start, end int, text string) (int, error) {
	if start < 0 || end < start || end > d.buf.Len() {
		return 0, fmt.Errorf("replace [%d,%d) of %d: out of range", start, end, d.buf.Len())
	}
	norm := d.normalizeEOL(text)
	if err := d.replace(start, end, norm, Cursor(start+len(norm)), ""); err != nil {
		return 0, fmt.Errorf("replace: %w", err)
	}
	return len(norm), nil
}

// ReplaceCurrent replaces the selection with repl if it matches query, then
// selects the next match. It reports whether a replacement happened.
func (d *Document) ReplaceCurrent(query, repl string, opts SearchOptions) (bool, error) {
	start, end := d.sel.Range()
	if query == "" || end-start != len(query) || !matches([]byte(d.SelectedText()), []byte(query), opts) {
		return false, nil
	}
	norm := d.normalizeEOL(repl)
	if err := d.replace(start, end, norm, Cursor(start+len(norm)), ""); err != nil {
		return false, fmt.Errorf("replace: %w", err)
	}
	d.Find(query, true, opts)
	return true, nil
}

// ReplaceAll replaces every match of query with repl as one undo step and
// returns the number of replacements.
func (d *Document) ReplaceAll(query, repl string, opts SearchOptions) (int, error) {
	if query == "" {
		return 0, nil
	}
	q := []byte(query)
	var hits []int
	n := d.buf.Len()
	for at := d.scan(q, 0, n, opts); at >= 0; at = d.scan(q, at+len(q), n, opts) {
		hits = append(hits, at)
	}
	if len(hits) == 0 {
		return 0, nil
	}
	norm := d.normalizeEOL(repl)
	d.hist.Seal()
	// Replace back to front so earlier offsets stay valid; the group is
	// undone in reverse order, front to back, which is equally valid.
	for i := len(hits) - 1; i >= 0; i-- {
		at := hits[i]
		after := d.sel.mapReplace(at, at+len(q), len(norm))
		if err := d.replace(at, at+len(q), norm, after, keyReplace); err != nil {
			return len(hits) - 1 - i, fmt.Errorf("replace all: %w", err)
		}
	}
	d.hist.Seal()
	return len(hits), nil
}

// searchWindow is how many candidate start offsets are read per step, so a
// search holds one window of the document, not all of it.
const searchWindow = 1 << 20

// scan returns the first match start in [from, to) (the match may end past
// to), or -1. A window overlaps the next by len(q)-1 bytes so no match
// falls between two windows.
func (d *Document) scan(q []byte, from, to int, opts SearchOptions) int {
	n := d.buf.Len()
	for lo := max(from, 0); lo < to && lo+len(q) <= n; lo += searchWindow {
		hi := min(lo+searchWindow, to)
		data, err := d.buf.Slice(lo, min(hi+len(q)-1, n))
		if err != nil {
			return -1
		}
		if at := find(data, q, 0, hi-lo, opts); at >= 0 {
			return lo + at
		}
	}
	return -1
}

// scanBack returns the last match start in [from, to), or -1.
func (d *Document) scanBack(q []byte, from, to int, opts SearchOptions) int {
	n := d.buf.Len()
	to = min(to, n-len(q)+1)
	for hi := to; hi > from; hi -= searchWindow {
		lo := max(hi-searchWindow, from)
		data, err := d.buf.Slice(lo, min(hi+len(q)-1, n))
		if err != nil {
			return -1
		}
		if at := findLast(data, q, 0, hi-lo, opts); at >= 0 {
			return lo + at
		}
	}
	return -1
}

// find returns the first match start in [from, to) whose match ends by len(data).
func find(data, q []byte, from, to int, opts SearchOptions) int {
	if opts.CaseSensitive {
		if from > len(data) {
			return -1
		}
		i := bytes.Index(data[from:], q)
		if i < 0 || from+i >= to {
			return -1
		}
		return from + i
	}
	for i := from; i < to && i+len(q) <= len(data); i++ {
		if utf8.RuneStart(data[i]) && bytes.EqualFold(data[i:i+len(q)], q) {
			return i
		}
	}
	return -1
}

// findLast returns the last match start in [from, to).
func findLast(data, q []byte, from, to int, opts SearchOptions) int {
	for i := min(to-1, len(data)-len(q)); i >= from; i-- {
		if utf8.RuneStart(data[i]) && matches(data[i:i+len(q)], q, opts) {
			return i
		}
	}
	return -1
}

func matches(text, q []byte, opts SearchOptions) bool {
	if opts.CaseSensitive {
		return bytes.Equal(text, q)
	}
	return len(text) == len(q) && bytes.EqualFold(text, q)
}

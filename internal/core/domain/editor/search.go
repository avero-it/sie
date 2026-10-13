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
	data := d.buf.Bytes()
	q := []byte(query)
	start, end := d.sel.Range()
	var at int
	if forward {
		at = find(data, q, end, len(data), opts)
		if at < 0 {
			at = find(data, q, 0, end, opts)
		}
	} else {
		at = findLast(data, q, 0, start, opts)
		if at < 0 {
			at = findLast(data, q, start, len(data), opts)
		}
	}
	if at < 0 {
		return false
	}
	d.SetSelection(Selection{at, at + len(q)})
	return true
}

// ReplaceCurrent replaces the selection with repl if it matches query, then
// selects the next match. It reports whether a replacement happened.
func (d *Document) ReplaceCurrent(query, repl string, opts SearchOptions) (bool, error) {
	if query == "" || !matches([]byte(d.SelectedText()), []byte(query), opts) {
		return false, nil
	}
	start, end := d.sel.Range()
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
	data := d.buf.Bytes()
	q := []byte(query)
	var hits []int
	for at := find(data, q, 0, len(data), opts); at >= 0; at = find(data, q, at+len(q), len(data), opts) {
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

package app

import "github.com/avero-it/sie/internal/core/domain/editor"

// maxLiveMatches caps the matches highlighted on one screen.
const maxLiveMatches = 500

// liveFind is the incremental search of the Find prompt: while it is open,
// every edit of the input selects the nearest match from where the search
// started, and the other visible matches are highlighted. Closing the
// prompt, for any reason, puts the cursor and the scroll back; Enter then
// runs the normal Find, which lands on the same match.
type liveFind struct {
	active bool
	sel    editor.Selection // selection when the prompt opened
	top    int              // viewport when the prompt opened
	left   int
	miss   bool // the typed text has no match
}

func (e *Editor) startLive() {
	e.live = liveFind{active: true, sel: e.doc.Selection(), top: e.view.Top, left: e.view.Left}
	e.prompt.onClose = e.endLive
	e.liveSearch() // the pre-filled text counts too
}

func (e *Editor) endLive() {
	if !e.live.active {
		return
	}
	e.restoreLive()
	e.live = liveFind{}
}

func (e *Editor) restoreLive() {
	e.doc.SetSelection(e.live.sel)
	e.view.Top, e.view.Left = e.live.top, e.live.left
}

// liveSearch selects the first match at or after the starting selection,
// wrapping once.
func (e *Editor) liveSearch() {
	q := e.prompt.input.String()
	e.restoreLive()
	e.live.miss = false
	if q == "" {
		return
	}
	if !e.doc.Find(q, true, smartCase(q)) {
		e.live.miss = true
		return
	}
	e.follow()
}

// liveMatches returns the byte ranges of the matches on the visible lines,
// in order.
func (e *Editor) liveMatches(r regions) [][2]int {
	q := e.prompt.input.String()
	if !e.live.active || e.live.miss || q == "" {
		return nil
	}
	opts := smartCase(q)
	last := min(e.view.Top+r.textRows, e.doc.LineCount())
	from := e.doc.OffsetAt(e.view.Top, 0)
	end := e.doc.OffsetAt(last-1, 0) + len(e.doc.Line(last-1))
	var out [][2]int
	for len(out) < maxLiveMatches {
		i, ok := e.doc.FindNext(q, from, opts)
		if !ok || i+len(q) > end {
			break
		}
		out = append(out, [2]int{i, i + len(q)})
		from = i + len(q)
	}
	return out
}

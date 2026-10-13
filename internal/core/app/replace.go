package app

import (
	"errors"
	"fmt"

	"github.com/avero-it/sie/internal/core/domain/editor"
	"github.com/avero-it/sie/internal/core/ports"
)

// replaceSession is an interactive replace: it starts at the cursor, runs
// to the end of the document, wraps to the top and stops before the
// starting point. The whole session is one undo step.
type replaceSession struct {
	query, repl string
	opts        editor.SearchOptions
	pos         int  // where to search next
	origin      int  // where the session started; stop here after wrapping
	wrapped     bool // searching the part before origin
	match       int  // start of the match being asked about
	count       int  // replacements made
}

// cmdReplace starts an interactive replace session ("Replace…").
func (e *Editor) cmdReplace(args []string) error {
	if args[0] == "" {
		return errors.New("nothing to replace")
	}
	e.lastQuery = args[0]
	start, _ := e.doc.Selection().Range() // a selected match is the first candidate
	s := &replaceSession{query: args[0], repl: args[1], opts: smartCase(args[0]), pos: start, origin: start}
	e.doc.BeginUndoGroup()
	e.replaceNext(s)
	return nil
}

// replaceNext selects the next match and asks about it, or ends the session.
func (e *Editor) replaceNext(s *replaceSession) {
	m, ok := e.nextReplaceMatch(s)
	if !ok {
		e.endReplace(s)
		return
	}
	s.match = m
	e.doc.SetSelection(editor.Selection{Anchor: m, Head: m + len(s.query)})
	e.focus = focusPrompt
	e.prompt = promptState{
		mode:     modeReplace,
		question: fmt.Sprintf("Replace %q with %q?", s.query, s.repl),
		replace:  s,
		onClose:  func() { e.endReplace(s) },
	}
}

func (e *Editor) nextReplaceMatch(s *replaceSession) (int, bool) {
	if !s.wrapped {
		if m, ok := e.doc.FindNext(s.query, s.pos, s.opts); ok {
			return m, true
		}
		s.wrapped, s.pos = true, 0
	}
	m, ok := e.doc.FindNext(s.query, s.pos, s.opts)
	if !ok || m+len(s.query) > s.origin {
		return 0, false
	}
	return m, true
}

// replaceKey answers the question: y replace, n skip, a replace all the
// remaining matches, q or Esc stop.
func (e *Editor) replaceKey(k ports.KeyEvent) {
	s := e.prompt.replace
	switch {
	case k.Key == ports.KeyEsc || k.Key == ports.KeyRune && (k.Rune == 'q' || k.Rune == 'Q'):
		e.closePrompt() // ends the session through onClose
	case k.Key == ports.KeyRune && (k.Rune == 'y' || k.Rune == 'Y'):
		if e.replaceMatch(s) {
			e.replaceNext(s)
		}
	case k.Key == ports.KeyRune && (k.Rune == 'n' || k.Rune == 'N'):
		s.pos = s.match + len(s.query)
		e.replaceNext(s)
	case k.Key == ports.KeyRune && (k.Rune == 'a' || k.Rune == 'A'):
		for e.replaceMatch(s) {
			m, ok := e.nextReplaceMatch(s)
			if !ok {
				break
			}
			s.match = m
		}
		e.endReplace(s)
	}
}

// replaceMatch replaces the current match; false means an error ended the session.
func (e *Editor) replaceMatch(s *replaceSession) bool {
	n, err := e.doc.ReplaceRange(s.match, s.match+len(s.query), s.repl)
	if err != nil {
		e.endReplace(s)
		e.setError(err)
		return false
	}
	s.count++
	s.pos = s.match + n
	if s.wrapped {
		// The match was before the starting point, which moves with it.
		s.origin += n - len(s.query)
	}
	return true
}

// endReplace closes the undo group and reports the result. It is safe to
// call more than once.
func (e *Editor) endReplace(s *replaceSession) {
	if s.query == "" {
		return // already ended
	}
	e.doc.EndUndoGroup()
	e.doc.SetSelection(editor.Cursor(e.doc.Selection().Head))
	if e.focus == focusPrompt && e.prompt.mode == modeReplace {
		e.prompt.onClose = nil
		e.closePrompt()
	}
	e.note(fmt.Sprintf("%s replaced", plural(s.count, "match")))
	s.query = ""
}

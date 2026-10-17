package app

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/avero-it/sie/internal/core/ports"
)

const defaultSyntaxMaxSize = 2 << 20 // 2 MB

// highlightState tracks syntax coloring for the current document.
//
// Highlighting runs in the background (see ports.Highlighter). Until a
// result for the current text arrives, the last tokens are kept and mapped
// through the document's change log: tokens before an edit stay, tokens
// after it shift, tokens touched by it are dropped (shown plain).
type highlightState struct {
	language string // canonical name; "" is plain text
	manual   bool   // chosen with "Set language…": not re-detected on Save as

	// epoch changes with the document or language, so results computed for
	// an older document or language are ignored. Job IDs are
	// epoch<<epochShift | document version.
	epoch     uint64
	requested uint64 // ID of the newest job sent
	pending   bool   // requested is valid

	tokens   []ports.Token
	tokensAt uint64 // document version the tokens are mapped to
	source   uint64 // document version the tokens were computed for
	tooLarge bool
}

const epochShift = 40

func jobID(epoch, version uint64) uint64 { return epoch<<epochShift | version }

func splitJobID(id uint64) (epoch, version uint64) {
	return id >> epochShift, id & (1<<epochShift - 1)
}

// highlightActive reports whether the current document should be colored.
func (e *Editor) highlightActive() bool {
	return e.cfg.Highlighter != nil && e.showSyntax && e.hl.language != ""
}

func (e *Editor) syntaxMaxSize() int {
	if e.cfg.SyntaxMaxSize > 0 {
		return e.cfg.SyntaxMaxSize
	}
	return defaultSyntaxMaxSize
}

// resetHighlight drops all tokens and ignores results still in flight.
func (e *Editor) resetHighlight() {
	e.hl.epoch++
	e.hl.pending = false
	e.hl.tokens = nil
	e.hl.tokensAt = e.doc.Version()
	e.hl.source = e.doc.Version()
}

// detectLanguage picks the language from the file name, unless the user
// chose one explicitly for this document.
func (e *Editor) detectLanguage() {
	if e.hl.manual {
		return
	}
	lang := ""
	if e.cfg.Highlighter != nil && e.path != "" {
		lang = e.cfg.Highlighter.Detect(e.path)
	}
	if lang != e.hl.language {
		e.hl.language = lang
		e.resetHighlight()
	}
}

// syncHighlight maps kept tokens to the current text and requests a new
// highlight when the text changed. It runs after every input event.
func (e *Editor) syncHighlight() {
	e.mapTokens()
	e.hl.tooLarge = false
	if !e.highlightActive() {
		return
	}
	if e.doc.Len() > e.syntaxMaxSize() {
		e.hl.tooLarge = true
		e.hl.tokens = nil
		return
	}
	id := jobID(e.hl.epoch, e.doc.Version())
	if e.hl.pending && e.hl.requested == id {
		return
	}
	e.cfg.Highlighter.Highlight(ports.HighlightJob{ID: id, Language: e.hl.language, Text: []byte(e.doc.Text())})
	e.hl.requested, e.hl.pending = id, true
}

// mapTokens moves the kept tokens through the edits made since they were
// mapped last.
func (e *Editor) mapTokens() {
	v := e.doc.Version()
	if e.hl.tokensAt == v {
		return
	}
	changes, ok := e.doc.ChangesSince(e.hl.tokensAt)
	e.hl.tokensAt = v
	if !ok {
		e.hl.tokens = nil
		return
	}
	for _, c := range changes {
		e.hl.tokens = shiftTokens(e.hl.tokens, c.Off, c.Removed, c.Inserted)
	}
}

// shiftTokens applies one change (at off, removed bytes replaced by
// inserted bytes) to sorted tokens, in place.
func shiftTokens(tokens []ports.Token, off, removed, inserted int) []ports.Token {
	out := tokens[:0]
	delta := inserted - removed
	for _, t := range tokens {
		switch {
		case t.End <= off:
			out = append(out, t)
		case t.Start >= off+removed:
			t.Start += delta
			t.End += delta
			out = append(out, t)
		}
		// Tokens overlapping the change are dropped until re-highlighted.
	}
	return out
}

// Poll collects a finished highlight, if any.
func (e *Editor) Poll() {
	e.pruneRipples()
	if e.cfg.Highlighter == nil {
		return
	}
	if res, ok := e.cfg.Highlighter.Result(); ok {
		epoch, version := splitJobID(res.ID)
		fresh := e.hl.tokens == nil || version > e.hl.source
		if res.Err == nil && epoch == e.hl.epoch && version <= e.doc.Version() && fresh {
			e.hl.tokens, e.hl.source, e.hl.tokensAt = res.Tokens, version, version
		}
	}
	e.syncHighlight()
}

// tokenCursor walks sorted tokens alongside increasing offsets.
type tokenCursor struct {
	tokens []ports.Token
	i      int
}

// cursorAt starts a cursor at the first token that ends after off.
func (e *Editor) cursorAt(off int) *tokenCursor {
	if !e.highlightActive() || e.hl.tooLarge || len(e.hl.tokens) == 0 {
		return nil
	}
	t := e.hl.tokens
	return &tokenCursor{tokens: t, i: sort.Search(len(t), func(i int) bool { return t[i].End > off })}
}

// class returns the token class at off; offsets must not decrease.
func (c *tokenCursor) class(off int) ports.TokenClass {
	if c == nil {
		return ports.TokenNone
	}
	for c.i < len(c.tokens) && c.tokens[c.i].End <= off {
		c.i++
	}
	if c.i < len(c.tokens) && c.tokens[c.i].Start <= off {
		return c.tokens[c.i].Class
	}
	return ports.TokenNone
}

// classStyle maps a token class to its style.
func classStyle(c ports.TokenClass) ports.Style {
	if c == ports.TokenNone {
		return ports.StyleDefault
	}
	return ports.StyleKeyword + ports.Style(c-ports.TokenKeyword)
}

// languageLabel is shown in the status line.
func (e *Editor) languageLabel() string {
	switch {
	case e.hl.language == "":
		return "Plain"
	case e.hl.tooLarge:
		return e.hl.language + " (too large)"
	default:
		return e.hl.language
	}
}

var plainNames = map[string]bool{"": true, "plain": true, "none": true, "text": true, "txt": true}

func (e *Editor) cmdSetLanguage(args []string) error {
	name := strings.TrimSpace(args[0])
	lang := ""
	if !plainNames[strings.ToLower(name)] {
		if e.cfg.Highlighter == nil {
			return errors.New("syntax highlighting is not available")
		}
		canonical, ok := e.cfg.Highlighter.Language(name)
		if !ok {
			return fmt.Errorf("unknown language %q", name)
		}
		lang = canonical
	}
	e.hl.language, e.hl.manual = lang, true
	e.resetHighlight()
	e.note(e.languageLabel())
	return nil
}

func (e *Editor) cmdToggleSyntax([]string) error {
	if err := e.toggle(&e.showSyntax); err != nil {
		return err
	}
	e.resetHighlight()
	return nil
}

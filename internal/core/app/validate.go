package app

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/avero-it/sie/internal/core/domain/editor"
	"github.com/avero-it/sie/internal/core/ports"
)

// cmdValidate checks the document as its language (detected or set with
// "Set language…") and reports either that it is valid or the first
// problem, moving the cursor onto it.
func (e *Editor) cmdValidate([]string) error {
	_, p, err := e.runValidate()
	switch {
	case err != nil:
		return err
	case p == nil:
		e.note("✓ valid " + e.hl.language)
		return nil
	case p.Line > 0:
		e.doc.SetSelection(editor.Cursor(e.problemOffset(p)))
	}
	return errors.New(problemText(p))
}

// runValidate checks the document as its language and returns the first
// problem, nil when the document is valid.
func (e *Editor) runValidate() (lang string, p *ports.Problem, err error) {
	v := e.cfg.Validator
	if v == nil {
		return "", nil, errors.New("validation is not available")
	}
	lang = e.hl.language
	if !supports(v, lang) {
		what := lang
		if what == "" {
			what = "plain text"
		}
		return lang, nil, fmt.Errorf("cannot validate %s (supported: %s)", what, strings.Join(v.Languages(), ", "))
	}
	if e.tooLarge() {
		return lang, nil, fmt.Errorf("file too large to validate (%s, limit %s)", formatSize(e.doc.Len()), formatSize(e.largeFileSize()))
	}
	p, err = v.Validate(lang, []byte(e.doc.Text()))
	return lang, p, err
}

// problemOffset converts a problem's line and character column to a
// document offset, clamped to the document.
func (e *Editor) problemOffset(p *ports.Problem) int {
	line := max(0, min(p.Line-1, e.doc.LineCount()-1))
	start, text := e.doc.OffsetAt(line, 0), e.doc.Line(line)
	off := 0
	for n := 1; n < p.Col && off < len(text); n++ {
		_, size := utf8.DecodeRune(text[off:])
		off += size
	}
	return start + off
}

func (e *Editor) cmdClose([]string) error {
	return e.guardUnsaved("Close without saving?", func() error {
		e.setDoc(e.newDoc(nil), "")
		return nil
	})
}

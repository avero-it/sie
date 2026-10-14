package app

import (
	"fmt"
	"strings"

	"github.com/avero-it/sie/internal/core/domain/editor"
	"github.com/avero-it/sie/internal/core/ports"
)

// saveValidated writes the document to path, validating it first when
// "Validate on save" is on and the target's file type can be validated.
// An invalid document is not written: the user chooses between jumping to
// the problem ("validate") and saving anyway.
func (e *Editor) saveValidated(path string) error {
	p := e.saveProblem(path)
	if p == nil {
		return e.write(path)
	}
	e.choose(fmt.Sprintf("Fails validation (%s)", problemText(p)),
		choice{key: 'v', label: "[v]alidate", run: func() error {
			e.showProblem(p)
			return nil
		}},
		choice{key: 's', label: "[s]ave anyway", run: func() error { return e.write(path) }},
	)
	return nil
}

// saveProblem validates the document as the language it will have once
// saved to path: the language chosen by hand, else the one detected from
// path. It returns nil when there is nothing to report or nothing to check
// (option off, unsupported type, validator failure).
func (e *Editor) saveProblem(path string) *ports.Problem {
	v := e.cfg.Validator
	if !e.validateOnSave || v == nil {
		return nil
	}
	lang := e.hl.language
	if !e.hl.manual {
		lang = ""
		if e.cfg.Highlighter != nil {
			lang = e.cfg.Highlighter.Detect(path)
		}
	}
	if !supports(v, lang) {
		return nil
	}
	p, err := v.Validate(lang, []byte(e.doc.Text()))
	if err != nil {
		return nil // could not check: don't block saving
	}
	return p
}

func supports(v ports.Validator, lang string) bool {
	if lang == "" {
		return false
	}
	for _, s := range v.Languages() {
		if strings.EqualFold(s, lang) {
			return true
		}
	}
	return false
}

// problemText describes where a problem is and what it is.
func problemText(p *ports.Problem) string {
	switch {
	case p.Line == 0:
		return p.Message + " (position unknown)"
	case p.Col == 0:
		return fmt.Sprintf("line %d: %s", p.Line, p.Message)
	default:
		return fmt.Sprintf("line %d, col %d: %s", p.Line, p.Col, p.Message)
	}
}

// showProblem moves the cursor onto p (when its position is known) and
// reports it exactly like the Validate command does.
func (e *Editor) showProblem(p *ports.Problem) {
	if p.Line > 0 {
		e.doc.SetSelection(editor.Cursor(e.problemOffset(p)))
	}
	e.msg = message{text: "Validate: " + problemText(p), kind: msgError}
}

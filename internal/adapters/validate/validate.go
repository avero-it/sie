// Package validate implements ports.Validator for JSON (encoding/json),
// YAML (goccy/go-yaml: line and column, duplicate keys), TOML (go-toml/v2)
// and XML (encoding/xml, well-formedness). Each checker reports the first
// problem only, with a 1-based line and a character (rune) column.
package validate

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/avero-it/sie/internal/core/ports"
	"github.com/goccy/go-yaml"
	"github.com/pelletier/go-toml/v2"
)

// Validator is the multi-format validator.
type Validator struct{}

var _ ports.Validator = Validator{}

var checkers = map[string]func([]byte) *ports.Problem{
	"JSON": checkJSON,
	"YAML": checkYAML,
	"TOML": checkTOML,
	"XML":  checkXML,
}

// Languages lists the supported languages.
func (Validator) Languages() []string {
	names := make([]string, 0, len(checkers))
	for n := range checkers {
		names = append(names, n)
	}
	slices.Sort(names)
	return names
}

// Validate checks text as language (case-insensitive name).
func (Validator) Validate(language string, text []byte) (*ports.Problem, error) {
	for name, check := range checkers {
		if strings.EqualFold(name, language) {
			return check(text), nil
		}
	}
	return nil, fmt.Errorf("cannot validate %s", language)
}

func checkJSON(text []byte) *ports.Problem {
	var v any
	err := json.Unmarshal(text, &v)
	if err == nil {
		return nil
	}
	var se *json.SyntaxError
	if !errors.As(err, &se) {
		return &ports.Problem{Line: 1, Col: 1, Message: err.Error()}
	}
	// Offset counts the bytes read, including the offending one.
	off := int(se.Offset) - 1
	if strings.Contains(se.Error(), "unexpected end") {
		off = len(text)
	}
	line, col := position(text, max(0, min(off, len(text))))
	return &ports.Problem{Line: line, Col: col, Message: se.Error()}
}

func checkYAML(text []byte) *ports.Problem {
	dec := yaml.NewDecoder(bytes.NewReader(text))
	for {
		var v any
		err := dec.Decode(&v)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err == nil {
			continue // next document after "---"
		}
		p := &ports.Problem{Line: 1, Message: err.Error()}
		var ye yaml.Error
		if errors.As(err, &ye) {
			p.Message = ye.GetMessage()
			if tok := ye.GetToken(); tok != nil && tok.Position != nil {
				p.Line, p.Col = tok.Position.Line, tok.Position.Column
			}
		}
		return p
	}
}

func checkTOML(text []byte) *ports.Problem {
	var v map[string]any
	err := toml.Unmarshal(text, &v)
	if err == nil {
		return nil
	}
	p := &ports.Problem{Message: strings.TrimPrefix(err.Error(), "toml: ")}
	var de *toml.DecodeError
	if errors.As(err, &de) {
		line, byteCol := de.Position()
		p.Line, p.Col = line, runeColumn(text, line, byteCol)
		return p
	}
	// go-toml reports duplicate keys without a position: point at the
	// second assignment of the key.
	if key, ok := strings.CutSuffix(strings.TrimPrefix(p.Message, "key "), " is already defined"); ok {
		p.Line, p.Col = secondAssignment(text, key)
	}
	return p
}

// secondAssignment finds the line of the second "key = ..." in text.
func secondAssignment(text []byte, key string) (line, col int) {
	seen := 0
	for i, l := range strings.Split(string(text), "\n") {
		trimmed := strings.TrimLeft(l, " \t")
		rest, ok := strings.CutPrefix(trimmed, key)
		if !ok {
			rest, ok = strings.CutPrefix(trimmed, `"`+key+`"`)
		}
		if ok && strings.HasPrefix(strings.TrimLeft(rest, " \t"), "=") {
			if seen++; seen == 2 {
				return i + 1, utf8.RuneCountInString(l[:len(l)-len(trimmed)]) + 1
			}
		}
	}
	return 0, 0
}

func checkXML(text []byte) *ports.Problem {
	dec := xml.NewDecoder(bytes.NewReader(text))
	root := false
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			if !root {
				line, col := position(text, len(text))
				return &ports.Problem{Line: line, Col: col, Message: "no root element"}
			}
			return nil
		}
		if err != nil {
			line, byteCol := dec.InputPos()
			msg := err.Error()
			var se *xml.SyntaxError
			if errors.As(err, &se) {
				msg = se.Msg
			}
			return &ports.Problem{Line: line, Col: runeColumn(text, line, byteCol), Message: msg}
		}
		if _, ok := tok.(xml.StartElement); ok {
			root = true
		}
	}
}

// position converts a byte offset to a 1-based line and rune column.
func position(text []byte, off int) (line, col int) {
	before := text[:off]
	line = bytes.Count(before, []byte{'\n'}) + 1
	lineStart := bytes.LastIndexByte(before, '\n') + 1
	return line, utf8.RuneCount(before[lineStart:]) + 1
}

// runeColumn converts a 1-based byte column on a 1-based line to a rune
// column.
func runeColumn(text []byte, line, byteCol int) int {
	if line < 1 || byteCol < 1 {
		return 0
	}
	lines := bytes.SplitN(text, []byte{'\n'}, line+1)
	if line > len(lines) {
		return byteCol
	}
	l := lines[line-1]
	return utf8.RuneCount(l[:min(byteCol-1, len(l))]) + 1
}

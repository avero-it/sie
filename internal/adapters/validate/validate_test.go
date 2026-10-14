package validate

import (
	"strings"
	"testing"

	"github.com/avero-it/sie/internal/core/ports"
)

func TestLanguages(t *testing.T) {
	if got := strings.Join(Validator{}.Languages(), ","); got != "JSON,TOML,XML,YAML" {
		t.Errorf("languages %s", got)
	}
	if _, err := (Validator{}).Validate("Go", nil); err == nil {
		t.Error("unsupported language accepted")
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name, lang, text string
		ok               bool
		line, col        int
		msg              string // substring of the message
	}{
		{"json ok", "json", `{"a": [1, 2], "b": null}`, true, 0, 0, ""},
		{"json bad char", "JSON", "{\n  \"a\": 1,\n  \"b\" 2\n}", false, 3, 7, "invalid character '2'"},
		{"json trailing comma", "JSON", "{\"a\": 1,}", false, 1, 9, "invalid character '}'"},
		{"json truncated", "JSON", "{\n  \"a\": [1,", false, 2, 11, "unexpected end"},
		{"json empty", "JSON", "", false, 1, 1, "unexpected end"},
		{"json trailing data", "JSON", "{} x", false, 1, 4, "after top-level value"},
		{"json non-ascii column", "JSON", "{\"é\": ü}", false, 1, 7, "invalid character"},
		{"yaml ok multi-doc", "YAML", "a: 1\n---\nb: [x, y]\n", true, 0, 0, ""},
		{"yaml empty", "YAML", "", true, 0, 0, ""},
		{"yaml bad mapping", "YAML", "a: 1\nb: c: d\n", false, 2, 4, "mapping value is not allowed"},
		{"yaml duplicate key", "YAML", "a: 1\na: 2\n", false, 2, 1, "already defined"},
		{"yaml unclosed flow", "YAML", "a: [1, 2\nb: 3\n", false, 2, 1, "must be specified"},
		{"toml ok", "TOML", "title = \"x\"\n[server]\nport = 80\n", true, 0, 0, ""},
		{"toml bad", "TOML", "a = 1\nb = = 2\n", false, 2, 5, ""},
		{"toml duplicate", "TOML", "a = 1\n  a = 2\n", false, 2, 3, "already defined"},
		{"xml ok", "XML", "<?xml version=\"1.0\"?>\n<a><b x=\"1\"/></a>\n", true, 0, 0, ""},
		{"xml mismatched", "XML", "<a>\n  <b></c>\n</a>", false, 2, 0, "element <b> closed by </c>"},
		{"xml unclosed", "XML", "<a>\n<b>", false, 2, 0, "unexpected EOF"},
		{"xml empty", "XML", "  \n", false, 2, 1, "no root element"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := Validator{}.Validate(tt.lang, []byte(tt.text))
			if err != nil {
				t.Fatal(err)
			}
			if tt.ok {
				if p != nil {
					t.Fatalf("unexpected problem %+v", *p)
				}
				return
			}
			if p == nil {
				t.Fatal("no problem reported")
			}
			if p.Line != tt.line || (tt.col != 0 && p.Col != tt.col) {
				t.Errorf("at %d:%d, want %d:%d (%s)", p.Line, p.Col, tt.line, tt.col, p.Message)
			}
			if !strings.Contains(p.Message, tt.msg) || strings.Contains(p.Message, "\n") {
				t.Errorf("message %q, want it to contain %q on one line", p.Message, tt.msg)
			}
		})
	}
}

func TestPosition(t *testing.T) {
	text := []byte("ab\nçd\nx")
	for off, want := range map[int][2]int{0: {1, 1}, 2: {1, 3}, 3: {2, 1}, 5: {2, 2}, 7: {3, 1}, 8: {3, 2}} {
		if l, c := position(text, off); l != want[0] || c != want[1] {
			t.Errorf("position(%d) = %d:%d, want %d:%d", off, l, c, want[0], want[1])
		}
	}
	if got := runeColumn(text, 2, 3); got != 2 { // "ç" is 2 bytes
		t.Errorf("runeColumn = %d", got)
	}
	var _ ports.Validator = Validator{}
}

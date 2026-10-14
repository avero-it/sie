package syntax

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/avero-it/sie/internal/core/ports"
)

// classAt returns the class of the token covering the first occurrence of
// needle in text.
func classAt(t *testing.T, res ports.HighlightResult, text, needle string) ports.TokenClass {
	t.Helper()
	at := strings.Index(text, needle)
	if at < 0 {
		t.Fatalf("%q not in text", needle)
	}
	for _, tok := range res.Tokens {
		if tok.Start <= at && at < tok.End {
			return tok.Class
		}
	}
	return ports.TokenNone
}

func TestTokenizeGo(t *testing.T) {
	src := "package main\n\n// Run it\nfunc Run(n int) error {\n\ts := `raw\nmulti`\n\tif n > 42 {\n\t\treturn nil\n\t}\n\t_ = len(s) + 'r'\n\t/* block */\n\treturn fmt.Errorf(\"%d\", n)\n}\n"
	res := tokenize(ports.HighlightJob{ID: 7, Language: "Go", Text: []byte(src)})
	if res.Err != nil || res.ID != 7 {
		t.Fatalf("err %v id %d", res.Err, res.ID)
	}
	want := map[string]ports.TokenClass{
		"package": ports.TokenKeyword, "func": ports.TokenKeyword, "return": ports.TokenKeyword,
		"// Run it": ports.TokenComment, "/* block */": ports.TokenComment,
		"`raw": ports.TokenString, "multi`": ports.TokenString, `"%d"`: ports.TokenString, "'r'": ports.TokenString,
		"42": ports.TokenNumber, "nil": ports.TokenConstant, "int": ports.TokenType, "error": ports.TokenType,
		"len": ports.TokenBuiltin, "main": ports.TokenNone, "Errorf": ports.TokenNone,
	}
	for needle, class := range want {
		if got := classAt(t, res, src, needle); got != class {
			t.Errorf("%q: class %d, want %d", needle, got, class)
		}
	}
}

func TestTokenizeJSONWithCRLF(t *testing.T) {
	src := "{\r\n  \"name\": \"sie\",\r\n  \"n\": 1.5e3,\r\n  \"ok\": true,\r\n  \"none\": null\r\n}\r\n"
	res := tokenize(ports.HighlightJob{Language: "JSON", Text: []byte(src)})
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	for needle, class := range map[string]ports.TokenClass{
		`"name"`: ports.TokenKey, `"sie"`: ports.TokenString, "1.5e3": ports.TokenNumber,
		"true": ports.TokenConstant, "null": ports.TokenConstant,
	} {
		if got := classAt(t, res, src, needle); got != class {
			t.Errorf("%q: class %d, want %d", needle, got, class)
		}
	}
	// Offsets are exact with CRLF: the last token is "null" at its real position.
	last := res.Tokens[len(res.Tokens)-1]
	if src[last.Start:last.End] != "null" {
		t.Errorf("last token %q", src[last.Start:last.End])
	}
}

func TestTokenizeYAML(t *testing.T) {
	src := "---\n# comment\nname: sie\nport: 8080\nok: true\nanchor: &base\n  k: v\nref: *base\ntagged: !!str x\nblock: |\n  line one\n  line two\n"
	res := tokenize(ports.HighlightJob{Language: "YAML", Text: []byte(src)})
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	for needle, class := range map[string]ports.TokenClass{
		"---": ports.TokenMeta, "# comment": ports.TokenComment, "name": ports.TokenKey, "sie": ports.TokenString,
		"8080": ports.TokenNumber, "true": ports.TokenConstant, "&base": ports.TokenMeta, "*base": ports.TokenMeta,
		"!!str": ports.TokenMeta, "line two": ports.TokenString,
	} {
		if got := classAt(t, res, src, needle); got != class {
			t.Errorf("%q: class %d, want %d", needle, got, class)
		}
	}
}

func TestTokensSortedAndInBounds(t *testing.T) {
	for _, lang := range []string{"Go", "JSON", "YAML", "Python", "Bash"} {
		src := "x = \"a\" # c\n{ \"k\": [1, 2] }\nfunc f() {}\n"
		res := tokenize(ports.HighlightJob{Language: lang, Text: []byte(src)})
		if res.Err != nil {
			t.Fatalf("%s: %v", lang, res.Err)
		}
		prev := 0
		for _, tok := range res.Tokens {
			if tok.Start < prev || tok.End <= tok.Start || tok.End > len(src) {
				t.Fatalf("%s: bad token %+v after %d", lang, tok, prev)
			}
			prev = tok.End
		}
	}
}

func TestDetectAndLanguage(t *testing.T) {
	c := New(nil)
	defer c.Close()
	for file, want := range map[string]string{
		"main.go": "Go", "/x/y/c.json": "JSON", "a.yml": "YAML", "b.yaml": "YAML", "c.toml": "TOML", "d.xml": "XML",
		"README": "", "notes.txt": "", "go.mod": "", "go.sum": "",
	} {
		if got := c.Detect(file); got != want {
			t.Errorf("Detect(%q) = %q, want %q", file, got, want)
		}
	}
	for name, want := range map[string]string{"golang": "Go", "yml": "YAML", "JSON": "JSON"} {
		if got, ok := c.Language(name); !ok || got != want {
			t.Errorf("Language(%q) = %q, %v", name, got, ok)
		}
	}
	for _, name := range []string{"klingon", "plaintext", ""} {
		if _, ok := c.Language(name); ok {
			t.Errorf("Language(%q) accepted", name)
		}
	}
	if len(Languages()) < 100 {
		t.Errorf("only %d languages", len(Languages()))
	}
}

func TestWorkerNewestWins(t *testing.T) {
	woke := make(chan struct{}, 100)
	c := New(func() { woke <- struct{}{} })
	defer c.Close()
	for i := 1; i <= 20; i++ {
		c.Highlight(ports.HighlightJob{ID: uint64(i), Language: "Go", Text: []byte(fmt.Sprintf("var x = %d\n", i))})
	}
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-woke:
			if res, ok := c.Result(); ok && res.ID == 20 {
				if res.Err != nil || len(res.Tokens) == 0 {
					t.Fatalf("result %+v", res)
				}
				return
			}
		case <-deadline:
			t.Fatal("newest job never finished")
		}
	}
}

func TestUnknownLanguageIsAnError(t *testing.T) {
	if res := tokenize(ports.HighlightJob{Language: "klingon", Text: []byte("x")}); res.Err == nil {
		t.Error("no error for unknown language")
	}
}

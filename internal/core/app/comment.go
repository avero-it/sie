package app

import (
	"errors"
	"fmt"
	"strings"
)

// commentPrefixes maps a lower-cased language name (as the highlighter
// reports it) to its line-comment prefix. Languages with only block
// comments (HTML, XML, CSS) and data formats without comments (JSON) are
// left out, so Toggle comment refuses them.
var commentPrefixes = func() map[string]string {
	m := map[string]string{}
	for prefix, langs := range map[string][]string{
		"//": {"go", "c", "c++", "c#", "objective-c", "java", "javascript", "typescript", "jsx", "tsx",
			"rust", "swift", "kotlin", "scala", "php", "dart", "groovy", "d", "zig", "solidity",
			"protocol buffer", "protocol buffers", "protobuf", "glsl", "hlsl", "cuda", "f#", "fsharp",
			"scss", "less", "jsonc", "json5", "jsonnet", "v", "vala", "go template"},
		"#": {"python", "python 3", "python 2", "ruby", "perl", "raku", "bash", "zsh", "fish", "shell",
			"sh", "powershell", "yaml", "toml", "makefile", "docker", "dockerfile", "r", "nim", "elixir",
			"crystal", "julia", "cmake", "tcl", "properties", "hcl", "terraform", "nginx configuration file",
			"gitignore", "git config", "awk", "coffeescript", "mojo", "just", "graphql", "starlark"},
		"--": {"sql", "mysql", "postgresql sql dialect", "pl/pgsql", "sqlite", "lua", "haskell", "elm",
			"ada", "vhdl", "cabal", "purescript", "idris", "agda"},
		";": {"ini", "common lisp", "scheme", "clojure", "emacs lisp", "racket", "nasm", "gas", "assembly"},
		"%": {"erlang", "matlab", "octave", "tex", "latex", "prolog"},
		`"`: {"viml", "vim script", "vim"},
	} {
		for _, l := range langs {
			m[l] = prefix
		}
	}
	return m
}()

// commentPrefix returns the line-comment prefix of a language.
func commentPrefix(language string) (string, bool) {
	p, ok := commentPrefixes[strings.ToLower(language)]
	return p, ok
}

func (e *Editor) cmdToggleComment([]string) error {
	if e.hl.language == "" {
		return errors.New("no language set (see Set language)")
	}
	prefix, ok := commentPrefix(e.hl.language)
	if !ok {
		return fmt.Errorf("no line comment syntax for %s", e.hl.language)
	}
	commented, err := e.doc.ToggleComment(prefix)
	if err != nil {
		return err
	}
	if commented {
		e.note("commented")
	} else {
		e.note("uncommented")
	}
	return nil
}

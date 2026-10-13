package commands

import (
	"slices"
	"testing"
)

type env struct{ ran []string }

func cmd(id, title string, nargs int, aliases ...string) Command[*env] {
	return Command[*env]{
		ID: id, Title: title, Aliases: aliases, Args: make([]Arg[*env], nargs),
		Run: func(e *env, args []string) error { e.ran = append(e.ran, id); return nil },
	}
}

func TestRegisterValidates(t *testing.T) {
	r := NewRegistry[*env]()
	if err := r.Register(cmd("a.b", "AB", 0)); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(cmd("a.b", "Other", 0)); err == nil {
		t.Error("duplicate ID accepted")
	}
	if err := r.Register(Command[*env]{ID: "x", Title: "X"}); err == nil {
		t.Error("command without Run accepted")
	}
}

func TestAllSortedByTitle(t *testing.T) {
	r := NewRegistry[*env]()
	_ = r.RegisterAll(cmd("z", "Zeta", 0), cmd("a", "Alpha", 0))
	var titles []string
	for _, c := range r.All() {
		titles = append(titles, c.Title)
	}
	if !slices.Equal(titles, []string{"Alpha", "Zeta"}) {
		t.Errorf("All() = %v", titles)
	}
}

func TestParseInline(t *testing.T) {
	r := NewRegistry[*env]()
	_ = r.RegisterAll(
		cmd("search.gotoLine", "Go to line", 1, "goto"),
		cmd("search.find", "Find", 1),
		cmd("search.replace", "Replace", 2),
		cmd("file.save", "Save", 0),
		cmd("search.replaceAll", "Replace all", 2),
		cmd("search.findReplace", "Find and replace", 2),
	)
	tests := []struct {
		input  string
		id     string
		args   []string
		wantOK bool
	}{
		{"goto 42", "search.gotoLine", []string{"42"}, true},
		{"Go To Line 7", "search.gotoLine", []string{"7"}, true},
		{"find hello world", "search.find", []string{"hello world"}, true},
		{"replace foo bar baz", "search.replace", []string{"foo", "bar baz"}, true},
		{"replace foo", "search.replace", []string{"foo"}, true},
		{"goto", "search.gotoLine", nil, true}, // whole name: args asked later
		{"save", "file.save", nil, true},
		{"save x", "", nil, false},
		{"gotox 4", "", nil, false},
		{"replace all a b", "search.replaceAll", []string{"a", "b"}, true},
		{"replace all", "search.replaceAll", nil, true},
		{"find and replace", "search.findReplace", nil, true},
		{"find and", "search.find", []string{"and"}, true},
	}
	for _, tt := range tests {
		c, args, ok := r.ParseInline(tt.input)
		if ok != tt.wantOK {
			t.Errorf("%q: ok=%v", tt.input, ok)
			continue
		}
		if ok && (c.ID != tt.id || !slices.Equal(args, tt.args)) {
			t.Errorf("%q: got %s %q, want %s %q", tt.input, c.ID, args, tt.id, tt.args)
		}
	}
}

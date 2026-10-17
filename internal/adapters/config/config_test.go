package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDefaultsWhenNoFiles(t *testing.T) {
	dir := t.TempDir()
	s, used, err := load([]string{filepath.Join(dir, "etc.yaml"), filepath.Join(dir, "user.yaml")}, false)
	if err != nil {
		t.Fatal(err)
	}
	if s != Defaults() || len(used) != 0 {
		t.Errorf("got %+v from %v", s, used)
	}
}

func TestUserOverridesSystemPerKey(t *testing.T) {
	dir := t.TempDir()
	system := write(t, dir, "etc/sie/config.yaml", "tab_width: 8\nshow_invisibles: true\n")
	user := write(t, dir, "home/.sie/config.yaml", "tab_width: 2\nline_ending: CRLF\njump_lines: 10\n")
	s, used, err := load([]string{system, user}, false)
	if err != nil {
		t.Fatal(err)
	}
	want := Defaults()
	want.TabWidth, want.ShowInvisibles, want.LineEnding, want.JumpLines = 2, true, "crlf", 10
	if s != want {
		t.Errorf("got %+v, want %+v", s, want)
	}
	if len(used) != 2 {
		t.Errorf("used %v", used)
	}
}

func TestEnvOverridesFiles(t *testing.T) {
	dir := t.TempDir()
	user := write(t, dir, "config.yaml", "tab_width: 2\n")
	t.Setenv("SIE_TAB_WIDTH", "6")
	t.Setenv("SIE_SHOW_EMPTY_CELLS", "false")
	t.Setenv("SIE_CLICK_RIPPLE", "true")
	s, _, err := load([]string{user}, false)
	if err != nil {
		t.Fatal(err)
	}
	if s.TabWidth != 6 || s.ShowEmptyCells || !s.ClickRipple {
		t.Errorf("got %+v", s)
	}
}

func TestErrors(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name, content, want string
	}{
		{"bad yaml", "tab_width: [", "config"},
		{"unknown key", "tab_widht: 4\n", "tab_widht"},
		{"tab width range", "tab_width: 0\n", "tab_width"},
		{"line ending", "line_ending: cr\n", "line_ending"},
		{"jump lines range", "jump_lines: 0\n", "jump_lines"},
		{"syntax max size", "syntax_max_size: lots\n", "syntax_max_size"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := write(t, dir, tt.name+".yaml", tt.content)
			_, _, err := load([]string{path}, false)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want mention of %q", err, tt.want)
			}
		})
	}
}

func TestExplicitFileMustExist(t *testing.T) {
	if _, _, err := Load(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Error("missing explicit config accepted")
	}
}

func TestSearchPaths(t *testing.T) {
	paths := SearchPaths()
	if !strings.HasSuffix(filepath.ToSlash(paths[0]), "/etc/sie/config.yaml") {
		t.Errorf("system path %q", paths[0])
	}
	if len(paths) == 2 && !strings.HasSuffix(filepath.ToSlash(paths[1]), "/.sie/config.yaml") {
		t.Errorf("user path %q", paths[1])
	}
}

func TestSyntaxSettings(t *testing.T) {
	dir := t.TempDir()
	path := write(t, dir, "c.yaml", "syntax_highlighting: false\nsyntax_max_size: 512KB\n")
	s, _, err := load([]string{path}, false)
	if err != nil {
		t.Fatal(err)
	}
	if s.SyntaxHighlighting || s.SyntaxMaxBytes != 512<<10 {
		t.Errorf("got %+v", s)
	}
	if d := Defaults(); !d.SyntaxHighlighting || d.SyntaxMaxBytes != 2<<20 {
		t.Errorf("defaults %+v", d)
	}
}

func TestValidateOnSaveSetting(t *testing.T) {
	if !Defaults().ValidateOnSave {
		t.Error("validate_on_save should default to true")
	}
	path := write(t, t.TempDir(), "c.yaml", "validate_on_save: false\n")
	s, _, err := load([]string{path}, false)
	if err != nil || s.ValidateOnSave {
		t.Errorf("got %+v, %v", s, err)
	}
}

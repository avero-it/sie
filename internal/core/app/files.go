package app

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/avero-it/sie/internal/core/ports"
)

// The file browser runs inside the prompt for ArgPath arguments (Open, Save
// as). The input is a path; the part after the last separator filters the
// entries of the directory before it. The list above the prompt shows "../"
// and the matching entries, directories first.
//
//	typing     filters by prefix (ignoring case); dotfiles are always listed
//	Tab        completes the longest common prefix, or the single match
//	           (directories get a trailing separator)
//	↑/↓, click highlight; nothing is highlighted until then
//	Enter      acts on the highlighted entry (directory: go in; file: choose);
//	           without a highlight it takes the typed path (a directory: go in)
//	Backspace  right after a separator goes up one directory
//
// Paths are shown relative to the working directory when inside it,
// otherwise with "~" for the home directory, otherwise absolute.

const maxFileRows = 12

const sep = string(filepath.Separator)

// workDir is the directory relative paths are resolved against.
func (e *Editor) workDir() string {
	if e.cfg.WorkDir != "" {
		return e.cfg.WorkDir
	}
	return "."
}

// expandHome replaces a leading "~" with the home directory.
func (e *Editor) expandHome(p string) string {
	if e.cfg.HomeDir != "" && (p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, "~"+sep)) {
		return filepath.Join(e.cfg.HomeDir, p[1:])
	}
	return p
}

// resolvePath turns user input into a clean absolute (or work-dir based) path.
func (e *Editor) resolvePath(p string) string {
	p = e.expandHome(p)
	if !filepath.IsAbs(p) {
		p = filepath.Join(e.workDir(), p)
	}
	return filepath.Clean(p)
}

// displayPath is how a resolved path is shown: relative to the working
// directory when inside it ("" for the working directory itself), else
// "~"-relative, else absolute.
func (e *Editor) displayPath(abs string) string {
	if rel, ok := relInside(e.workDir(), abs); ok {
		if rel == "." {
			return ""
		}
		return rel
	}
	if e.cfg.HomeDir != "" {
		if rel, ok := relInside(e.cfg.HomeDir, abs); ok {
			if rel == "." {
				return "~"
			}
			return "~" + sep + rel
		}
	}
	return abs
}

// dirInput is the prompt text for browsing directory abs.
func (e *Editor) dirInput(abs string) string {
	d := e.displayPath(abs)
	if d == "" || strings.HasSuffix(d, sep) {
		return d
	}
	return d + sep
}

func relInside(base, target string) (string, bool) {
	rel, err := filepath.Rel(base, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+sep) || filepath.IsAbs(rel) {
		return "", false
	}
	return rel, true
}

// splitPathInput splits the input at its last separator ("/" is accepted
// on every platform): the directory part keeps the separator.
func splitPathInput(input string) (dirPart, name string) {
	i := strings.LastIndexAny(input, "/"+sep)
	return input[:i+1], input[i+1:]
}

// browseDir is the directory the current input lists.
func (e *Editor) browseDir() string {
	dirPart, _ := splitPathInput(e.prompt.input.String())
	if dirPart == "" {
		return filepath.Clean(e.workDir())
	}
	return e.resolvePath(dirPart)
}

// currentDir is where browsing starts: the current file's directory.
func (e *Editor) currentDir() string {
	if e.path == "" {
		return filepath.Clean(e.workDir())
	}
	return filepath.Dir(e.resolvePath(e.path))
}

// refreshFiles lists the browsed directory (cached while it doesn't change)
// and filters it by the typed name.
func (e *Editor) refreshFiles() {
	p := &e.prompt
	dir := e.browseDir()
	if !p.listed || p.listDir != dir {
		entries, err := e.cfg.Files.List(dir)
		slices.SortFunc(entries, func(a, b ports.DirEntry) int {
			if a.Dir != b.Dir {
				if a.Dir {
					return -1
				}
				return 1
			}
			return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
		})
		p.entries, p.listErr, p.listDir, p.listed = entries, err, dir, true
	}
	_, name := splitPathInput(p.input.String())
	var items []completion
	if filepath.Dir(dir) != dir && hasPrefixFold("..", name) {
		items = append(items, completion{label: "  .." + sep, dir: true, up: true})
	}
	if p.listErr != nil {
		items = append(items, completion{label: "  (" + p.listErr.Error() + ")", disabled: true})
	}
	var exact []completion
	for _, en := range p.entries {
		if !hasPrefixFold(en.Name, name) {
			continue
		}
		it := completion{label: "  " + en.Name, name: en.Name, dir: en.Dir}
		if en.Dir {
			it.label += sep
		}
		if strings.EqualFold(en.Name, name) {
			exact = append(exact, it) // an exact match is listed first
		} else {
			items = append(items, it)
		}
	}
	items = append(exact, items...)
	p.items, p.selected, p.top = items, -1, 0
}

func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

// browseKey handles the keys that behave differently while browsing. It
// reports whether k was handled.
func (e *Editor) browseKey(k ports.KeyEvent) bool {
	p := &e.prompt
	switch k.Key {
	case ports.KeyTab:
		e.completePath()
	case ports.KeyBackspace:
		if in := p.input.String(); in != "" && strings.ContainsAny(in[len(in)-1:], "/"+sep) {
			e.browseUp()
			return true
		}
		return false
	case ports.KeyUp:
		if p.selected < 0 && len(p.items) > 0 {
			e.selectCompletion(len(p.items) - 1)
			return true
		}
		return false
	case ports.KeyEnter:
		e.browseEnter()
	default:
		return false
	}
	return true
}

// completePath is Tab: the longest common prefix of the matches, or the
// single match itself.
func (e *Editor) completePath() {
	p := &e.prompt
	dirPart, name := splitPathInput(p.input.String())
	if p.input.String() == "~" {
		p.input.Set("~" + sep)
		e.refreshFiles()
		return
	}
	var names []string
	var single completion
	for _, it := range p.items {
		if !it.up && !it.disabled {
			names = append(names, it.name)
			single = it
		}
	}
	switch {
	case len(names) == 1:
		text := dirPart + single.name
		if single.dir {
			text += sep
		}
		p.input.Set(text)
	case len(names) > 1:
		if lcp := commonPrefix(names); len(lcp) > len(name) {
			p.input.Set(dirPart + lcp)
		}
	default:
		return
	}
	e.refreshFiles()
}

func commonPrefix(names []string) string {
	prefix := names[0]
	for _, n := range names[1:] {
		for !strings.HasPrefix(n, prefix) {
			prefix = prefix[:len(prefix)-1]
		}
	}
	return prefix
}

// browseUp shows the parent of the browsed directory.
func (e *Editor) browseUp() {
	dir := e.browseDir()
	if parent := filepath.Dir(dir); parent != dir {
		e.prompt.input.Set(e.dirInput(parent))
	}
	e.refreshFiles()
}

// browseEnter acts on the highlighted entry, or on the typed path.
func (e *Editor) browseEnter() {
	p := &e.prompt
	dirPart, name := splitPathInput(p.input.String())
	if p.selected >= 0 && p.selected < len(p.items) {
		it := p.items[p.selected]
		switch {
		case it.disabled:
			return
		case it.up:
			e.browseUp()
		case it.dir:
			p.input.Set(dirPart + it.name + sep)
			e.refreshFiles()
		default:
			p.input.Set(dirPart + it.name)
			e.submitArg()
		}
		return
	}
	if name == "" {
		return // a directory is shown; nothing to choose yet
	}
	if name == ".." {
		e.browseUp()
		return
	}
	for _, en := range p.entries {
		if en.Dir && en.Name == name {
			p.input.Set(dirPart + name + sep)
			e.refreshFiles()
			return
		}
	}
	e.submitArg()
}

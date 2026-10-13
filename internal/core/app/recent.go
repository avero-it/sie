package app

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/avero-it/sie/internal/core/app/ui"
)

// Recent files: every successful open or save of an existing file is
// recorded (ports.RecentFiles). "Open recent…" lists them most recent first,
// fuzzy-filtered by the typed text with the best match highlighted, so Enter
// opens it right away. The current file and files that no longer exist are
// not listed.

// recentFile is one candidate of the Open recent list.
type recentFile struct {
	abs, display string
}

// recordRecent remembers path; failures are ignored, as the list is a
// convenience and must never get in the way of editing.
func (e *Editor) recordRecent(path string) {
	if e.cfg.Recent != nil && path != "" {
		_ = e.cfg.Recent.Add(e.resolvePath(path))
	}
}

// recentFiles returns the candidates, most recent first.
func (e *Editor) recentFiles() ([]recentFile, error) {
	if e.cfg.Recent == nil {
		return nil, errors.New("recent files are not available")
	}
	paths, err := e.cfg.Recent.List()
	if err != nil {
		return nil, err
	}
	current := ""
	if e.path != "" {
		current = e.resolvePath(e.path)
	}
	var out []recentFile
	for _, p := range paths {
		if p == current {
			continue
		}
		if ok, err := e.cfg.Files.Exists(p); err != nil || !ok {
			continue
		}
		out = append(out, recentFile{abs: p, display: e.displayPath(p)})
	}
	return out, nil
}

// matchRecent orders candidates by fuzzy score against query, keeping
// recency for ties (and everything, in order, for an empty query).
func matchRecent(files []recentFile, query string) []recentFile {
	query = strings.TrimSpace(query)
	if query == "" {
		return files
	}
	type scored struct {
		f     recentFile
		score int
	}
	var ms []scored
	for _, f := range files {
		if s, ok := ui.Score(query, f.display); ok {
			ms = append(ms, scored{f, s})
		}
	}
	slices.SortStableFunc(ms, func(a, b scored) int { return b.score - a.score })
	out := make([]recentFile, len(ms))
	for i, m := range ms {
		out[i] = m.f
	}
	return out
}

// refreshRecent filters the candidates loaded when the prompt opened.
func (e *Editor) refreshRecent() {
	p := &e.prompt
	var items []completion
	for _, f := range matchRecent(p.recentAll, p.input.String()) {
		items = append(items, completion{label: "  " + f.display, name: f.display})
	}
	if p.recentErr != nil {
		items = append(items, completion{label: "  (" + p.recentErr.Error() + ")", disabled: true})
	}
	p.items, p.top = items, 0
	p.selected = 0
	if len(items) == 0 || items[0].disabled {
		p.selected = -1
	}
}

// recentKey handles Tab and Enter in the Open recent list.
func (e *Editor) recentKey(k recentAction) {
	p := &e.prompt
	var chosen *completion
	if p.selected >= 0 && p.selected < len(p.items) && !p.items[p.selected].disabled {
		chosen = &p.items[p.selected]
	}
	switch k {
	case recentComplete:
		if chosen != nil {
			p.input.Set(chosen.name)
			e.refreshRecent()
		}
	case recentChoose:
		if chosen == nil {
			in := strings.TrimSpace(p.input.String())
			e.closePrompt()
			if in == "" {
				e.info("No recent files")
			} else {
				e.setError(fmt.Errorf("no recent file matches %q", in))
			}
			return
		}
		p.input.Set(chosen.name)
		e.submitArg()
	}
}

type recentAction uint8

const (
	recentComplete recentAction = iota
	recentChoose
)

// cmdOpenRecent opens a recent file: an exact entry, or else the best
// fuzzy match (so "recent main" works inline from the command prompt).
func (e *Editor) cmdOpenRecent(args []string) error {
	files, err := e.recentFiles()
	if err != nil {
		return err
	}
	query := strings.TrimSpace(args[0])
	for _, f := range files {
		if f.display == query || f.abs == query {
			return e.cmdOpen([]string{f.display})
		}
	}
	if m := matchRecent(files, query); len(m) > 0 && query != "" {
		return e.cmdOpen([]string{m[0].display})
	}
	return fmt.Errorf("no recent file matches %q", query)
}

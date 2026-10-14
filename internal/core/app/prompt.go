package app

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/avero-it/sie/internal/core/app/commands"
	"github.com/avero-it/sie/internal/core/app/ui"
	"github.com/avero-it/sie/internal/core/ports"
)

type promptMode uint8

const (
	modeCommand promptMode = iota // fuzzy command search
	modeArgs                      // asking for a command's missing arguments
	modeConfirm                   // yes/no question
	modeReplace                   // interactive replace: y/n/a/q
	modeChoice                    // question with several one-key answers
)

// choice is one answer of a modeChoice question.
type choice struct {
	key   rune   // lower-case; the upper-case key works too
	label string // e.g. "[v]alidate"
	run   func() error
}

const (
	maxCompletions   = 8
	historySuggested = 5
)

// completion is one row of the prompt's popup: a command, possibly with
// arguments (from inline parsing or history), or a directory entry while
// browsing files.
type completion struct {
	cmd   *commands.Command[*Editor]
	args  []string
	label string
	hint  string

	name     string // browsing: entry name
	dir      bool   // browsing: a directory
	up       bool   // browsing: the "../" entry
	disabled bool   // browsing: an error line, not selectable
}

type promptState struct {
	mode  promptMode
	input ui.Input

	cmd  *commands.Command[*Editor] // modeArgs: command waiting for args
	args []string                   // modeArgs: arguments collected so far

	question string       // modeConfirm, modeReplace, modeChoice
	onYes    func() error // modeConfirm
	choices  []choice     // modeChoice

	replace *replaceSession // modeReplace
	onClose func()          // runs when the prompt closes for any reason

	items    []completion // modeCommand, or browsing
	selected int          // -1: nothing highlighted (browsing)
	top      int          // first visible completion

	browse  bool             // modeArgs with an ArgPath argument
	entries []ports.DirEntry // browsing: listing of listDir, sorted
	listDir string           // browsing: the directory listed
	listErr error            // browsing: why listDir could not be read
	listed  bool             // browsing: entries are valid

	recent    bool         // modeArgs with an ArgRecent argument
	recentAll []recentFile // candidates loaded when the prompt opened
	recentErr error        // why the recent list could not be read
}

func (e *Editor) openPrompt() {
	e.focus = focusPrompt
	e.prompt = promptState{mode: modeCommand}
	e.refreshCompletions()
}

func (e *Editor) closePrompt() {
	onClose := e.prompt.onClose
	e.focus = focusEditor
	e.prompt = promptState{}
	if onClose != nil {
		onClose()
	}
}

func (e *Editor) askArgs(c *commands.Command[*Editor], args []string) {
	e.focus = focusPrompt
	arg := c.Args[len(args)]
	e.prompt = promptState{mode: modeArgs, cmd: c, args: args,
		browse: arg.Kind == commands.ArgPath, recent: arg.Kind == commands.ArgRecent}
	if arg.Default != nil {
		e.prompt.input.Set(arg.Default(e))
	}
	switch {
	case e.prompt.browse:
		e.refreshFiles()
	case e.prompt.recent:
		e.prompt.recentAll, e.prompt.recentErr = e.recentFiles()
		e.refreshRecent()
	}
}

// choose asks question with one-key answers; Esc or any other key cancels.
func (e *Editor) choose(question string, choices ...choice) {
	e.focus = focusPrompt
	e.prompt = promptState{mode: modeChoice, question: question, choices: choices}
}

func (e *Editor) confirm(question string, onYes func() error) {
	e.focus = focusPrompt
	e.prompt = promptState{mode: modeConfirm, question: question, onYes: onYes}
}

func (e *Editor) promptKey(k ports.KeyEvent) {
	if id, ok := e.cfg.Keymap.Command(k); ok && (id == "ui.prompt" || id == "ui.menu") {
		_ = e.Dispatch(id)
		return
	}
	p := &e.prompt
	if p.mode == modeReplace {
		e.replaceKey(k)
		return
	}
	if p.mode == modeChoice {
		choices := p.choices
		e.closePrompt()
		if k.Key == ports.KeyRune && k.Mod&^ports.ModShift == 0 {
			for _, c := range choices {
				if unicode.ToLower(k.Rune) == c.key {
					e.check(c.run())
					return
				}
			}
		}
		e.info("Cancelled")
		return
	}
	if p.mode == modeConfirm {
		yes := k.Key == ports.KeyRune && (k.Rune == 'y' || k.Rune == 'Y')
		onYes := p.onYes
		e.closePrompt()
		if !yes {
			e.info("Cancelled")
			return
		}
		e.check(onYes())
		return
	}
	if p.browse && e.browseKey(k) {
		return
	}
	if p.recent && (k.Key == ports.KeyTab || k.Key == ports.KeyEnter) {
		action := recentChoose
		if k.Key == ports.KeyTab {
			action = recentComplete
		}
		e.recentKey(action)
		return
	}
	switch k.Key {
	case ports.KeyEsc:
		e.closePrompt()
	case ports.KeyEnter:
		e.submitPrompt()
	case ports.KeyUp:
		e.selectCompletion(p.selected - 1)
	case ports.KeyDown:
		e.selectCompletion(p.selected + 1)
	case ports.KeyPgUp:
		e.selectCompletion(p.selected - maxCompletions)
	case ports.KeyPgDn:
		e.selectCompletion(p.selected + maxCompletions)
	case ports.KeyTab:
		if p.mode == modeCommand && len(p.items) > 0 {
			it := p.items[p.selected]
			text := strings.ToLower(it.cmd.Title)
			if len(it.cmd.Args) > 0 {
				text += " "
			}
			p.input.Set(text)
			e.promptChanged()
		}
	case ports.KeyLeft:
		p.input.Left()
	case ports.KeyRight:
		p.input.Right()
	case ports.KeyHome:
		p.input.Home()
	case ports.KeyEnd:
		p.input.End()
	case ports.KeyBackspace:
		p.input.Backspace()
		e.promptChanged()
	case ports.KeyDelete:
		p.input.Delete()
		e.promptChanged()
	case ports.KeyRune:
		if isText(k) {
			p.input.Insert(string(k.Rune))
			e.promptChanged()
		}
	}
}

func (e *Editor) promptChanged() {
	switch {
	case e.prompt.browse:
		e.refreshFiles()
	case e.prompt.recent:
		e.refreshRecent()
	case e.prompt.mode == modeCommand:
		e.refreshCompletions()
	}
}

// submitArg passes the input as the pending command's next argument.
func (e *Editor) submitArg() {
	p := e.prompt
	args := append(slices.Clone(p.args), p.input.String())
	e.closePrompt()
	_ = e.Dispatch(p.cmd.ID, args...)
}

func (e *Editor) submitPrompt() {
	p := e.prompt
	switch {
	case p.browse:
		e.browseEnter()
	case p.recent:
		e.recentKey(recentChoose)
	case p.mode == modeArgs:
		e.submitArg()
	case p.mode == modeCommand:
		e.closePrompt()
		if len(p.items) == 0 {
			if in := strings.TrimSpace(p.input.String()); in != "" {
				e.setError(fmt.Errorf("no command matches %q", in))
			}
			return
		}
		it := p.items[p.selected]
		_ = e.Dispatch(it.cmd.ID, it.args...)
	}
}

func (e *Editor) selectCompletion(i int) {
	p := &e.prompt
	if len(p.items) == 0 {
		return
	}
	p.selected = max(0, min(i, len(p.items)-1))
	rows := e.completionRows()
	if p.selected < p.top {
		p.top = p.selected
	}
	if rows > 0 && p.selected >= p.top+rows {
		p.top = p.selected - rows + 1
	}
}

// refreshCompletions rebuilds the popup list. With empty input it shows
// recent invocations first, then every command by title; otherwise an
// inline "name args" parse first, then fuzzy matches by score.
func (e *Editor) refreshCompletions() {
	p := &e.prompt
	input := strings.TrimSpace(p.input.String())
	var items []completion
	if input == "" {
		seen := map[string]bool{}
		for _, h := range e.history[:min(len(e.history), historySuggested)] {
			if c, ok := e.reg.Get(h.id); ok {
				items = append(items, e.completion(c, h.args))
				if len(h.args) == 0 {
					seen[c.ID] = true
				}
			}
		}
		for _, c := range e.reg.All() {
			if !c.Silent && !seen[c.ID] {
				items = append(items, e.completion(c, nil))
			}
		}
	} else {
		inline := ""
		if c, args, ok := e.reg.ParseInline(input); ok {
			items = append(items, e.completion(c, args))
			if len(args) == 0 {
				inline = c.ID // a whole-name match; don't list it twice
			}
		}
		type scored struct {
			c     *commands.Command[*Editor]
			score int
		}
		var matches []scored
		for _, c := range e.reg.All() {
			if c.Silent || c.ID == inline {
				continue
			}
			best, found := 0, false
			for _, name := range append([]string{c.Title}, c.Aliases...) {
				if s, ok := ui.Score(input, name); ok && (!found || s > best) {
					best, found = s, true
				}
			}
			if found {
				matches = append(matches, scored{c, best})
			}
		}
		slices.SortStableFunc(matches, func(a, b scored) int { return b.score - a.score })
		for _, m := range matches {
			items = append(items, e.completion(m.c, nil))
		}
	}
	p.items, p.selected, p.top = items, 0, 0
}

func (e *Editor) completion(c *commands.Command[*Editor], args []string) completion {
	label := tickColumn(e.checked(c.ID)) + invocationLabel(c, args)
	if len(args) < len(c.Args) {
		label += "…"
	}
	return completion{cmd: c, args: args, label: label, hint: e.shortcut(c.ID)}
}

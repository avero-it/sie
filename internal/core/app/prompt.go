package app

import (
	"fmt"
	"slices"
	"strings"

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
)

const (
	maxCompletions   = 8
	historySuggested = 5
)

// completion is one row of the prompt's popup: a command, possibly with
// arguments (from inline parsing or history).
type completion struct {
	cmd   *commands.Command[*Editor]
	args  []string
	label string
	hint  string
}

type promptState struct {
	mode  promptMode
	input ui.Input

	cmd  *commands.Command[*Editor] // modeArgs: command waiting for args
	args []string                   // modeArgs: arguments collected so far

	question string       // modeConfirm, modeReplace
	onYes    func() error // modeConfirm

	replace *replaceSession // modeReplace
	onClose func()          // runs when the prompt closes for any reason

	items    []completion // modeCommand
	selected int
	top      int // first visible completion
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
	e.prompt = promptState{mode: modeArgs, cmd: c, args: args}
	if def := c.Args[len(args)].Default; def != nil {
		e.prompt.input.Set(def(e))
	}
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
	if e.prompt.mode == modeCommand {
		e.refreshCompletions()
	}
}

func (e *Editor) submitPrompt() {
	p := e.prompt
	switch p.mode {
	case modeArgs:
		args := append(slices.Clone(p.args), p.input.String())
		e.closePrompt()
		_ = e.Dispatch(p.cmd.ID, args...)
	case modeCommand:
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

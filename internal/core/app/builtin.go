package app

import (
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
	"unicode"

	"github.com/avero-it/sie/internal/core/app/commands"
	"github.com/avero-it/sie/internal/core/domain/editor"
)

type (
	command = commands.Command[*Editor]
	arg     = commands.Arg[*Editor]
)

// builtins is the unique list of v0 commands. Keymaps and menus refer to
// these IDs; keep CLAUDE.md's keymap table in sync.
func builtins() []command {
	return []command{
		{ID: "file.new", Title: "New file", Aliases: []string{"new"}, Run: (*Editor).cmdNew},
		{ID: "file.open", Title: "Open file", Aliases: []string{"open", "edit"},
			Args: []arg{{Prompt: "Open"}}, Run: (*Editor).cmdOpen},
		{ID: "file.save", Title: "Save file", Aliases: []string{"save", "write"}, Run: (*Editor).cmdSave},
		{ID: "file.saveAs", Title: "Save as", Aliases: []string{"saveas"},
			Args: []arg{{Prompt: "Save as", Default: func(e *Editor) string { return e.path }}}, Run: (*Editor).cmdSaveAs},
		{ID: "file.quit", Title: "Quit", Aliases: []string{"exit"}, Run: (*Editor).cmdQuit},

		{ID: "edit.undo", Title: "Undo", Run: (*Editor).cmdUndo},
		{ID: "edit.redo", Title: "Redo", Run: (*Editor).cmdRedo},
		{ID: "edit.cut", Title: "Cut", Run: (*Editor).cmdCut},
		{ID: "edit.copy", Title: "Copy", Run: (*Editor).cmdCopy},
		{ID: "edit.paste", Title: "Paste", Run: (*Editor).cmdPaste},
		{ID: "edit.duplicateLine", Title: "Duplicate line", Run: docOp((*editor.Document).DuplicateLine)},
		{ID: "edit.deleteLine", Title: "Delete line", Run: docOp((*editor.Document).DeleteLine)},
		// Prompt-only: no default key binding, not in any menu.
		{ID: "edit.deleteToLineEnd", Title: "Delete to end of line",
			Aliases: []string{"delete till end of line", "kill line"}, Run: docOp((*editor.Document).DeleteToLineEnd)},

		{ID: "selection.all", Title: "Select all", Run: func(e *Editor, _ []string) error { e.doc.SelectAll(); return nil }},
		{ID: "selection.line", Title: "Select line", Run: func(e *Editor, _ []string) error { e.doc.SelectLine(); return nil }},
		{ID: "selection.trimTrailingSpaces", Title: "Trim trailing spaces",
			Aliases: []string{"trim ending spaces", "trim spaces from selection", "trim"}, Run: (*Editor).cmdTrim},

		{ID: "search.find", Title: "Find", Aliases: []string{"search"},
			Args: []arg{{Prompt: "Find", Default: (*Editor).findDefault}}, Run: (*Editor).cmdFind},
		{ID: "search.next", Title: "Find next", Run: func(e *Editor, _ []string) error { return e.findAgain(true) }},
		{ID: "search.prev", Title: "Find previous", Run: func(e *Editor, _ []string) error { return e.findAgain(false) }},
		{ID: "search.replace", Title: "Replace", Aliases: []string{"find and replace", "query replace"},
			Args: []arg{{Prompt: "Replace", Default: (*Editor).findDefault}, {Prompt: "With"}}, Run: (*Editor).cmdReplace},
		{ID: "search.replaceAll", Title: "Replace all",
			Args: []arg{{Prompt: "Replace all", Default: (*Editor).findDefault}, {Prompt: "With"}}, Run: (*Editor).cmdReplaceAll},
		{ID: "search.gotoLine", Title: "Go to line", Aliases: []string{"goto", "line"},
			Args: []arg{{Prompt: "Line"}}, Run: (*Editor).cmdGotoLine},

		{ID: "view.toggleInvisibles", Title: "Invisible characters",
			Aliases: []string{"show/hide invisible characters", "toggle invisibles", "show invisible chars",
				"hide invisible chars", "whitespace"},
			Checked: func(e *Editor) bool { return e.showInvisibles },
			Run:     func(e *Editor, _ []string) error { return e.toggle(&e.showInvisibles) }},
		{ID: "view.toggleEmptyCells", Title: "Empty cells",
			Aliases: []string{"show/hide empty cells", "toggle empty cells", "show empty cells", "hide empty cells"},
			Checked: func(e *Editor) bool { return e.showEmptyCells },
			Run:     func(e *Editor, _ []string) error { return e.toggle(&e.showEmptyCells) }},

		{ID: "ui.prompt", Title: "Command prompt", Silent: true, Run: (*Editor).cmdTogglePrompt},
		{ID: "ui.menu", Title: "Menu", Silent: true, Run: (*Editor).cmdToggleMenu},
		{ID: "help.about", Title: "About sie", Aliases: []string{"version"}, Run: (*Editor).cmdAbout},
	}
}

func docOp(op func(*editor.Document) error) func(*Editor, []string) error {
	return func(e *Editor, _ []string) error { return op(e.doc) }
}

// guardUnsaved runs action now, or after confirmation if there are unsaved changes.
func (e *Editor) guardUnsaved(question string, action func() error) error {
	if !e.doc.Modified() {
		return action()
	}
	e.confirm(question, action)
	return nil
}

func (e *Editor) cmdNew([]string) error {
	return e.guardUnsaved("Discard unsaved changes?", func() error {
		e.setDoc(e.newDoc(nil), "")
		return nil
	})
}

func (e *Editor) cmdOpen(args []string) error {
	path := strings.TrimSpace(args[0])
	if path == "" {
		return errors.New("no file name")
	}
	return e.guardUnsaved("Discard unsaved changes?", func() error { return e.load(path) })
}

func (e *Editor) load(path string) error {
	data, err := e.cfg.Files.Read(path)
	if errors.Is(err, fs.ErrNotExist) {
		e.setDoc(e.newDoc(nil), path)
		e.note("new file")
		return nil
	}
	if err != nil {
		return err
	}
	e.setDoc(e.newDoc(data), path)
	e.note(plural(e.doc.LineCount(), "line"))
	return nil
}

func (e *Editor) cmdSave([]string) error {
	if e.path == "" {
		_ = e.Dispatch("file.saveAs")
		return nil
	}
	return e.write(e.path)
}

func (e *Editor) cmdSaveAs(args []string) error {
	path := strings.TrimSpace(args[0])
	if path == "" {
		return errors.New("no file name")
	}
	if path != e.path {
		exists, err := e.cfg.Files.Exists(path)
		if err != nil {
			return err
		}
		if exists {
			e.confirm(fmt.Sprintf("Overwrite %s?", path), func() error { return e.write(path) })
			return nil
		}
	}
	return e.write(path)
}

func (e *Editor) write(path string) error {
	if err := e.cfg.Files.Write(path, e.doc.Bytes()); err != nil {
		return err
	}
	e.path = path
	e.doc.MarkSaved()
	e.note(fmt.Sprintf("wrote %s", plural(e.doc.LineCount(), "line")))
	return nil
}

func (e *Editor) cmdQuit([]string) error {
	return e.guardUnsaved("Quit without saving?", func() error {
		e.done = true
		return nil
	})
}

func (e *Editor) cmdUndo([]string) error {
	ok, err := e.doc.Undo()
	if err == nil && !ok {
		e.note("nothing to undo")
	}
	return err
}

func (e *Editor) cmdRedo([]string) error {
	ok, err := e.doc.Redo()
	if err == nil && !ok {
		e.note("nothing to redo")
	}
	return err
}

// copySelection copies the selection, or the whole line when nothing is selected.
func (e *Editor) copySelection() {
	if e.doc.Selection().Empty() {
		e.doc.SelectLine()
	}
	e.clip = e.doc.SelectedText()
	if e.cfg.Clipboard != nil {
		if err := e.cfg.Clipboard.Copy(e.clip); err != nil {
			e.note("system clipboard unavailable")
		}
	}
}

func (e *Editor) cmdCopy([]string) error {
	e.copySelection()
	return nil
}

func (e *Editor) cmdCut([]string) error {
	e.copySelection()
	return e.doc.DeleteSelection()
}

func (e *Editor) cmdPaste([]string) error {
	text := e.clip
	if e.cfg.Clipboard != nil {
		if t, err := e.cfg.Clipboard.Paste(); err == nil && t != "" {
			text = t
		}
	}
	if text == "" {
		e.note("clipboard empty")
		return nil
	}
	return e.doc.Insert(text)
}

func (e *Editor) cmdTrim([]string) error {
	n, err := e.doc.TrimTrailingSpace()
	e.note(fmt.Sprintf("%s trimmed", plural(n, "line")))
	return err
}

// findDefault pre-fills the find prompt with a single-line selection or the last query.
func (e *Editor) findDefault() string {
	if sel := e.doc.SelectedText(); sel != "" && !strings.ContainsAny(sel, "\r\n") {
		return sel
	}
	return e.lastQuery
}

func (e *Editor) cmdFind(args []string) error {
	if args[0] == "" {
		return errors.New("nothing to find")
	}
	e.lastQuery = args[0]
	return e.findAgain(true)
}

func (e *Editor) findAgain(forward bool) error {
	if e.lastQuery == "" {
		_ = e.Dispatch("search.find")
		return nil
	}
	if !e.doc.Find(e.lastQuery, forward, smartCase(e.lastQuery)) {
		return fmt.Errorf("%q not found", e.lastQuery)
	}
	return nil
}

func (e *Editor) cmdReplaceAll(args []string) error {
	if args[0] == "" {
		return errors.New("nothing to replace")
	}
	e.lastQuery = args[0]
	n, err := e.doc.ReplaceAll(args[0], args[1], smartCase(args[0]))
	e.note(fmt.Sprintf("%s replaced", plural(n, "match")))
	return err
}

func (e *Editor) cmdGotoLine(args []string) error {
	n, err := strconv.Atoi(strings.TrimSpace(args[0]))
	if err != nil {
		return fmt.Errorf("invalid line number %q", args[0])
	}
	n = max(1, min(n, e.doc.LineCount()))
	e.doc.SetSelection(editor.Cursor(e.doc.OffsetAt(n-1, 0)))
	return nil
}

// toggle flips a view option and reports the new state.
func (e *Editor) toggle(opt *bool) error {
	*opt = !*opt
	if *opt {
		e.note("shown")
	} else {
		e.note("hidden")
	}
	return nil
}

func (e *Editor) cmdTogglePrompt([]string) error {
	if e.focus == focusPrompt {
		e.closePrompt()
	} else {
		e.openPrompt()
	}
	return nil
}

func (e *Editor) cmdToggleMenu([]string) error {
	if e.focus == focusMenu {
		e.focus = focusEditor
	} else {
		e.openMenu(0)
	}
	return nil
}

func (e *Editor) cmdAbout([]string) error {
	e.note("sie " + e.version())
	return nil
}

// smartCase searches case-sensitively only when the query has upper case.
func smartCase(q string) editor.SearchOptions {
	return editor.SearchOptions{CaseSensitive: strings.IndexFunc(q, unicode.IsUpper) >= 0}
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	if strings.HasSuffix(word, "ch") {
		return fmt.Sprintf("%d %ses", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

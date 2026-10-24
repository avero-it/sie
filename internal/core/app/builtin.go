package app

import (
	"errors"
	"fmt"
	"io/fs"
	"slices"
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
			Args: []arg{{Prompt: "Open", Kind: commands.ArgPath,
				Default: func(e *Editor) string { return e.dirInput(e.currentDir()) }}},
			Run: (*Editor).cmdOpen},
		{ID: "file.openRecent", Title: "Open recent", Aliases: []string{"recent", "recent files"},
			Args: []arg{{Prompt: "Open recent", Kind: commands.ArgRecent}}, Run: (*Editor).cmdOpenRecent},
		{ID: "file.save", Title: "Save file", Aliases: []string{"save", "write"}, Run: (*Editor).cmdSave},
		{ID: "file.saveAs", Title: "Save as", Aliases: []string{"saveas"},
			Args: []arg{{Prompt: "Save as", Kind: commands.ArgPath, Default: func(e *Editor) string {
				if e.path == "" {
					return e.dirInput(e.currentDir())
				}
				return e.displayPath(e.resolvePath(e.path))
			}}},
			Run: (*Editor).cmdSaveAs},
		{ID: "file.close", Title: "Close file", Aliases: []string{"close"}, Run: (*Editor).cmdClose},
		{ID: "file.quit", Title: "Quit", Aliases: []string{"exit"}, Run: (*Editor).cmdQuit},

		{ID: "edit.undo", Title: "Undo", MultiCursor: true, Run: (*Editor).cmdUndo},
		{ID: "edit.redo", Title: "Redo", MultiCursor: true, Run: (*Editor).cmdRedo},
		{ID: "edit.cut", Title: "Cut", MultiCursor: true, Run: (*Editor).cmdCut},
		{ID: "edit.copy", Title: "Copy", MultiCursor: true, Run: (*Editor).cmdCopy},
		{ID: "edit.paste", Title: "Paste", MultiCursor: true, Run: (*Editor).cmdPaste},
		{ID: "edit.validate", Title: "Validate", Aliases: []string{"check", "lint", "validate file"},
			Run: (*Editor).cmdValidate},
		{ID: "edit.duplicateLine", Title: "Duplicate line", Run: docOp((*editor.Document).DuplicateLine)},
		{ID: "edit.deleteLine", Title: "Delete line", Run: docOp((*editor.Document).DeleteLine)},
		{ID: "edit.moveLineUp", Title: "Move line up", Aliases: []string{"move up", "line up"},
			Run: docOp(func(d *editor.Document) error { return d.MoveLines(true) })},
		{ID: "edit.moveLineDown", Title: "Move line down", Aliases: []string{"move down", "line down"},
			Run: docOp(func(d *editor.Document) error { return d.MoveLines(false) })},
		{ID: "edit.toggleComment", Title: "Toggle comment", Aliases: []string{"comment", "uncomment", "comment line"},
			Run: (*Editor).cmdToggleComment},
		// Prompt-only: no default key binding, not in any menu.
		{ID: "edit.deleteToLineEnd", Title: "Delete to end of line",
			Aliases: []string{"delete till end of line", "kill line"}, Run: docOp((*editor.Document).DeleteToLineEnd)},

		{ID: "selection.all", Title: "Select all", Run: func(e *Editor, _ []string) error { e.doc.SelectAll(); return nil }},
		{ID: "selection.line", Title: "Select line", Run: func(e *Editor, _ []string) error { e.doc.SelectLine(); return nil }},
		{ID: "selection.selectNext", Title: "Select next occurrence", MultiCursor: true,
			Aliases: []string{"next occurrence", "add next match"}, Run: occurrenceOp(true)},
		{ID: "selection.selectPrevious", Title: "Select previous occurrence", MultiCursor: true,
			Aliases: []string{"previous occurrence", "add previous match"}, Run: occurrenceOp(false)},
		{ID: "selection.toggleColumnMode", Title: "Column selection",
			Aliases:     []string{"column mode", "block selection", "rectangular selection", "column"},
			MultiCursor: true,
			Checked:     func(e *Editor) bool { return e.columnMode },
			Run:         (*Editor).cmdToggleColumnMode},
		{ID: "selection.trimTrailingSpaces", Title: "Trim trailing spaces",
			Aliases: []string{"trim ending spaces", "trim spaces from selection", "trim"}, Run: (*Editor).cmdTrim},
		{ID: "selection.trimLeadingSpaces", Title: "Trim leading spaces",
			Aliases: []string{"trim starting spaces", "trim beginning spaces", "trim start"},
			Run:     lineCountOp((*editor.Document).TrimLeadingSpace, "trimmed")},
		{ID: "selection.alignPrevious", Title: "Align to previous line", Aliases: []string{"align up"},
			Run: lineCountOp((*editor.Document).AlignToPrevious, "aligned")},
		{ID: "selection.alignNext", Title: "Align to next line", Aliases: []string{"align down"},
			Run: lineCountOp((*editor.Document).AlignToNext, "aligned")},

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
		{ID: "view.toggleClickRipple", Title: "Click ripple",
			Aliases: []string{"ripple", "click effect", "click highlight"},
			Checked: func(e *Editor) bool { return e.clickRipple },
			Run:     func(e *Editor, _ []string) error { return e.toggleOption(&e.clickRipple) }},
		{ID: "view.toggleIndentGuide", Title: "Indent guide",
			Aliases: []string{"alignment guide", "indent flash"},
			Checked: func(e *Editor) bool { return e.indentGuide },
			Run:     func(e *Editor, _ []string) error { return e.toggleOption(&e.indentGuide) }},
		{ID: "view.toggleWrap", Title: "Wrap lines",
			Aliases: []string{"wrap", "word wrap", "soft wrap", "line wrap"},
			Checked: func(e *Editor) bool { return e.wrap },
			Run:     (*Editor).cmdToggleWrap},
		{ID: "view.toggleSyntax", Title: "Syntax highlighting",
			Aliases: []string{"toggle syntax", "highlighting", "colors"},
			Checked: func(e *Editor) bool { return e.showSyntax },
			Run:     (*Editor).cmdToggleSyntax},
		{ID: "view.setLanguage", Title: "Set language",
			Aliases: []string{"language", "set syntax", "syntax", "filetype"},
			Args: []arg{{Prompt: "Language", Default: func(e *Editor) string {
				if e.hl.language == "" {
					return "plain"
				}
				return e.hl.language
			}}},
			Run: (*Editor).cmdSetLanguage},

		{ID: "view.toggleValidateOnSave", Title: "Validate on save",
			Aliases: []string{"toggle validate on save", "validate on save"},
			Checked: func(e *Editor) bool { return e.validateOnSave },
			Run:     func(e *Editor, _ []string) error { return e.toggleOption(&e.validateOnSave) }},

		{ID: "ui.prompt", Title: "Command prompt", Silent: true, Run: (*Editor).cmdTogglePrompt},
		{ID: "ui.menu", Title: "Menu", Silent: true, Run: (*Editor).cmdToggleMenu},
		{ID: "help.about", Title: "About sie", Aliases: []string{"version"}, Run: (*Editor).cmdAbout},
	}
}

// BuiltinIDs returns the IDs of the built-in commands a user can invoke
// (everything but the silent ui.* ones). Every one of them needs a shortcut.
func BuiltinIDs() []string {
	var ids []string
	for _, c := range builtins() {
		if !c.Silent {
			ids = append(ids, c.ID)
		}
	}
	return ids
}

func docOp(op func(*editor.Document) error) func(*Editor, []string) error {
	return func(e *Editor, _ []string) error { return op(e.doc) }
}

// lineCountOp wraps a line operation that returns the number of lines it
// changed and reports it as "N lines <verb>".
func lineCountOp(op func(*editor.Document) (int, error), verb string) func(*Editor, []string) error {
	return func(e *Editor, _ []string) error {
		n, err := op(e.doc)
		if err == nil {
			e.note(fmt.Sprintf("%s %s", plural(n, "line"), verb))
		}
		return err
	}
}

// occurrenceOp adds a cursor on the next (or previous) match of the selection.
func occurrenceOp(forward bool) func(*Editor, []string) error {
	return func(e *Editor, _ []string) error {
		if !e.doc.SelectOccurrence(forward) {
			e.note("no other occurrence")
		}
		return nil
	}
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
	path := e.expandHome(strings.TrimSpace(args[0]))
	if path == "" {
		return errors.New("no file name")
	}
	return e.guardUnsaved("Discard unsaved changes?", func() error { return e.load(path) })
}

func (e *Editor) load(path string) error {
	f, err := e.cfg.Files.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		e.setDoc(e.newDoc(nil), path)
		e.note("new file")
		return nil
	}
	if err != nil {
		return err
	}
	doc, backing, err := e.openDoc(f)
	if err != nil {
		return err
	}
	e.setDoc(doc, path)
	e.file = backing
	e.recordRecent(path)
	e.note(plural(e.doc.LineCount(), "line"))
	return nil
}

func (e *Editor) cmdSave([]string) error {
	if e.path == "" {
		_ = e.Dispatch("file.saveAs")
		return nil
	}
	return e.saveValidated(e.path)
}

func (e *Editor) cmdSaveAs(args []string) error {
	path := e.expandHome(strings.TrimSpace(args[0]))
	if path == "" {
		return errors.New("no file name")
	}
	if path != e.path {
		exists, err := e.cfg.Files.Exists(path)
		if err != nil {
			return err
		}
		if exists {
			e.confirm(fmt.Sprintf("Overwrite %s?", path), func() error { return e.saveValidated(path) })
			return nil
		}
	}
	return e.saveValidated(path)
}

func (e *Editor) write(path string) error {
	if err := e.cfg.Files.WriteFrom(path, e.doc); err != nil {
		return err
	}
	if path != e.path {
		e.path = path
		e.detectLanguage()
	}
	e.doc.MarkSaved()
	e.reopen(path)
	e.recordRecent(path)
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

// copySelection copies the selection, or the whole line when nothing is
// selected. With several cursors (a rectangle, for one) it copies every
// selection, one per line, so the block keeps its shape. It reports whether
// anything was copied, and refuses selections over maxCopySize.
func (e *Editor) copySelection() (bool, error) {
	var text string
	if e.doc.CursorCount() > 1 {
		texts := e.doc.SelectedTexts()
		if !slices.ContainsFunc(texts, func(s string) bool { return s != "" }) {
			e.note("nothing selected")
			return false, nil
		}
		text = strings.Join(texts, "\n")
	} else {
		if e.doc.Selection().Empty() {
			e.doc.SelectLine()
		}
		if s, en := e.doc.Selection().Range(); en-s > maxCopySize {
			return false, fmt.Errorf("selection too large to copy (%s, limit %s)", formatSize(en-s), formatSize(maxCopySize))
		}
		text = e.doc.SelectedText()
	}
	e.clip = text
	if e.cfg.Clipboard != nil {
		if err := e.cfg.Clipboard.Copy(e.clip); err != nil {
			e.note("system clipboard unavailable")
		}
	}
	return true, nil
}

func (e *Editor) cmdCopy([]string) error {
	_, err := e.copySelection()
	return err
}

func (e *Editor) cmdCut([]string) error {
	if ok, err := e.copySelection(); !ok || err != nil {
		return err
	}
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
	if n := e.doc.CursorCount(); n > 1 {
		if parts, ok := splitForCursors(text, n); ok {
			return e.doc.InsertEach(parts)
		}
	}
	return e.doc.Insert(text)
}

// splitForCursors splits clipboard text into one line per cursor, for pasting
// a block onto several cursors. A trailing line break counts as a line of its
// own first (a block whose last line is empty), then as a terminator (whole
// lines copied with one cursor). It reports false when the text has another
// number of lines, in which case the whole text goes to every cursor.
func splitForCursors(text string, n int) ([]string, bool) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if len(lines) == n {
		return lines, true
	}
	if len(lines) == n+1 && lines[n] == "" {
		return lines[:n], true
	}
	return nil, false
}

func (e *Editor) cmdTrim([]string) error {
	n, err := e.doc.TrimTrailingSpace()
	e.note(fmt.Sprintf("%s trimmed", plural(n, "line")))
	return err
}

// findDefault pre-fills the find prompt with a single-line selection or the last query.
func (e *Editor) findDefault() string {
	if s, en := e.doc.Selection().Range(); en-s <= findPrefillMax {
		if sel := e.doc.SelectedText(); sel != "" && !strings.ContainsAny(sel, "\r\n") {
			return sel
		}
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

// toggleOption flips a behavior option and reports it as on/off.
func (e *Editor) toggleOption(opt *bool) error {
	*opt = !*opt
	if *opt {
		e.note("on")
	} else {
		e.note("off")
	}
	return nil
}

// cmdToggleWrap switches line wrapping; the window restarts at the top line.
func (e *Editor) cmdToggleWrap([]string) error {
	if e.columnMode && !e.wrap {
		return errors.New("turn off column selection first")
	}
	e.setWrap(!e.wrap)
	return nil
}

func (e *Editor) setWrap(on bool) {
	e.wrap = on
	e.view.Left, e.view.Skip = 0, 0
	e.doc.SetSelection(e.doc.Selection()) // forget the preferred column
	e.follow()
	if on {
		e.note("on")
	} else {
		e.note("off")
	}
}

// cmdToggleColumnMode switches column selection, which needs unwrapped text.
func (e *Editor) cmdToggleColumnMode([]string) error {
	if err := e.toggleOption(&e.columnMode); err != nil {
		return err
	}
	if e.columnMode && e.wrap {
		e.wrap = false
		e.view.Skip = 0
		e.follow()
		e.note("wrap off")
	}
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

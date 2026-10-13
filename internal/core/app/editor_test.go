package app

import (
	"strings"
	"testing"

	"github.com/avero-it/sie/internal/core/ports"
)

// Rows on the 40x10 test screen.
const (
	rowMenu   = 0
	rowText   = 1
	rowPrompt = 8
	rowStatus = 9
)

func TestInitialScreen(t *testing.T) {
	h := newHarness(t, nil)
	h.wantRow(rowMenu, " File  Edit  Selection  Search  View  He") // clipped at 40 columns
	h.wantRow(rowText, " 1")
	h.Resize(80, 10) // room for the whole status line
	h.wantRowPrefix(rowStatus, " sie test - [No Name]")
	if !strings.HasSuffix(h.row(rowStatus), "Ln 1, Col 1   Plain   UTF-8   LF") {
		t.Errorf("status: %q", h.row(rowStatus))
	}
	if c := h.View().Cursor; !c.Visible || c.X != 3 || c.Y != rowText {
		t.Errorf("cursor %+v", c)
	}
}

func TestTypingAndStatus(t *testing.T) {
	h := newHarness(t, nil)
	h.typeText("héllo\n  日本")
	h.wantRow(rowText, " 1 héllo")
	h.wantRow(rowText+1, " 2   日本")
	h.Resize(80, 10)
	h.wantRowPrefix(rowStatus, " sie test - [No Name] ●")
	if !strings.Contains(h.row(rowStatus), "Ln 2, Col 7") {
		t.Errorf("status: %q", h.row(rowStatus))
	}
}

func TestShortcutIsEchoedWithHint(t *testing.T) {
	h := newHarness(t, map[string]string{"a.txt": "one\ntwo\n"})
	if err := h.Dispatch("file.open", "a.txt"); err != nil {
		t.Fatal(err)
	}
	h.wantRowPrefix(rowPrompt, "> Open file a.txt — 3 lines")
	if !strings.HasSuffix(h.row(rowPrompt), "Ctrl+O") {
		t.Errorf("hint missing: %q", h.row(rowPrompt))
	}
	h.typeText("x")
	h.keys(ports.Ctrl('s'))
	h.wantRowPrefix(rowPrompt, "> Save file — wrote 3 lines")
	if got := string(h.fs.files["a.txt"]); got != "xone\ntwo\n" {
		t.Errorf("saved %q", got)
	}
	h.Resize(80, 10)
	h.wantRowPrefix(rowStatus, " sie test - a.txt   ") // no modified dot
}

func TestSaveWithoutNameAsksForPath(t *testing.T) {
	h := newHarness(t, nil)
	h.typeText("hi")
	h.keys(ports.Ctrl('s'))
	h.wantRow(rowPrompt, "Save as:")
	h.typeText("new.txt")
	h.keys(key(ports.KeyEnter))
	if string(h.fs.files["new.txt"]) != "hi" {
		t.Fatalf("files: %v", h.fs.files)
	}
	h.wantRowPrefix(rowPrompt, "> Save as new.txt — wrote 1 line")
}

func TestSaveAsExistingAsksToOverwrite(t *testing.T) {
	h := newHarness(t, map[string]string{"x": "old"})
	h.typeText("new")
	_ = h.Dispatch("file.saveAs", "x")
	h.wantRow(rowPrompt, "Overwrite x? (y/n)")
	h.typeText("n")
	h.wantRow(rowPrompt, "Cancelled")
	_ = h.Dispatch("file.saveAs", "x")
	h.typeText("y")
	if string(h.fs.files["x"]) != "new" {
		t.Errorf("not overwritten: %q", h.fs.files["x"])
	}
}

func TestWriteErrorIsShown(t *testing.T) {
	h := newHarness(t, map[string]string{"a": ""})
	_ = h.Dispatch("file.open", "a")
	h.fs.writeErr = errDisk
	h.keys(ports.Ctrl('s'))
	h.wantRow(rowPrompt, "Save file: disk full")
	if h.styleAt(0, rowPrompt) != ports.StyleError {
		t.Error("error not styled as error")
	}
}

func TestOpenMissingFileStartsNew(t *testing.T) {
	h := newHarness(t, nil)
	_ = h.Dispatch("file.open", "nope.go")
	h.wantRowPrefix(rowPrompt, "> Open file nope.go — new file")
	h.Resize(80, 10)
	h.wantRowPrefix(rowStatus, " sie test - nope.go")
}

func TestQuitConfirmsUnsavedChanges(t *testing.T) {
	h := newHarness(t, nil)
	h.keys(ports.Ctrl('q'))
	if !h.Done() {
		t.Fatal("clean quit should not ask")
	}
	h = newHarness(t, nil)
	h.typeText("x")
	h.keys(ports.Ctrl('q'))
	h.wantRow(rowPrompt, "Quit without saving? (y/n)")
	h.keys(key(ports.KeyEsc))
	if h.Done() {
		t.Fatal("quit despite cancel")
	}
	h.keys(ports.Ctrl('q'))
	h.typeText("y")
	if !h.Done() {
		t.Fatal("did not quit after confirm")
	}
}

func TestPromptInlineArgsAndHistory(t *testing.T) {
	h := newHarness(t, map[string]string{"f": "1\n2\n3\n4\n"})
	_ = h.Dispatch("file.open", "f")
	h.keys(ports.CtrlAlt('p'))
	h.wantRow(rowPrompt, ">")
	h.typeText("goto 3")
	h.wantRowPrefix(rowPrompt-1, "   Go to line 3") // inline parse is the first completion
	h.keys(key(ports.KeyEnter))
	if line, _, _ := h.doc.CursorPos(); line != 2 {
		t.Fatalf("cursor line %d, want 2", line)
	}
	h.wantRowPrefix(rowPrompt, "> Go to line 3")

	// Reopening with empty input lists recent invocations first.
	h.keys(ports.CtrlAlt('p'))
	rows := h.screen()
	if !strings.HasPrefix(rows[rowPrompt-maxCompletionsOnScreen(h)], "   Go to line 3") {
		t.Errorf("history not first:\n%s", strings.Join(rows, "\n"))
	}
	h.keys(ports.CtrlAlt('p')) // toggles closed
	if h.focus != focusEditor {
		t.Error("Ctrl+Alt+P did not close the prompt")
	}
}

func maxCompletionsOnScreen(h *harness) int { return h.completionRows() }

func TestPromptFuzzyMatchesAliases(t *testing.T) {
	h := newHarness(t, nil)
	h.typeText("a  \nb ")
	h.keys(ports.CtrlAlt('p'))
	h.typeText("trim sel")
	if len(h.prompt.items) == 0 || h.prompt.items[0].cmd.ID != "selection.trimTrailingSpaces" {
		t.Fatalf("first completion: %+v", h.prompt.items)
	}
	h.keys(key(ports.KeyEnter))
	if got := h.doc.Text(); got != "a\nb" {
		t.Errorf("text %q", got)
	}
	h.wantRowPrefix(rowPrompt, "> Trim trailing spaces — 2 lines trimmed")
}

func TestPromptAsksForMissingArgs(t *testing.T) {
	h := newHarness(t, nil)
	h.typeText("foo bar foo")
	h.keys(ports.CtrlAlt('p'))
	h.typeText("replace all")
	h.keys(key(ports.KeyEnter))
	h.wantRow(rowPrompt, "Replace all:")
	h.typeText("foo")
	h.keys(key(ports.KeyEnter))
	h.wantRow(rowPrompt, "With:")
	h.typeText("x")
	h.keys(key(ports.KeyEnter))
	if got := h.doc.Text(); got != "x bar x" {
		t.Errorf("text %q", got)
	}
	h.wantRow(rowPrompt, "> Replace all foo x — 2 matches replaced") // no shortcut, no hint
}

func TestFindPrefillsLastQuery(t *testing.T) {
	h := newHarness(t, nil)
	h.typeText("ab ab ab")
	h.keys(key(ports.KeyHome))
	h.keys(ports.Ctrl('f'))
	h.typeText("ab")
	h.keys(key(ports.KeyEnter))
	h.keys(ports.Ctrl('f'))
	// The current match is selected, so it pre-fills the prompt.
	h.wantRow(rowPrompt, "Find: ab")
	h.keys(key(ports.KeyEnter))
	if s, e := h.doc.Selection().Range(); s != 3 || e != 5 {
		t.Errorf("second match selection %d-%d", s, e)
	}
	h.keys(ports.Ctrl('f'))
	h.keys(key(ports.KeyBackspace), key(ports.KeyBackspace))
	h.typeText("zz")
	h.keys(key(ports.KeyEnter))
	h.wantRow(rowPrompt, `Find zz: "zz" not found`)
}

func TestNoMatchingCommand(t *testing.T) {
	h := newHarness(t, nil)
	h.keys(ports.CtrlAlt('p'))
	h.typeText("qqqq")
	h.keys(key(ports.KeyEnter))
	h.wantRow(rowPrompt, `no command matches "qqqq"`)
}

func TestMenuKeyboard(t *testing.T) {
	h := newHarness(t, nil)
	h.typeText("abc")
	h.keys(ports.CtrlAlt('m'))
	if h.styleAt(1, rowMenu) != ports.StyleMenuTitleActive {
		t.Error("File title not highlighted")
	}
	h.wantRowPrefix(1, "   New file") // dropdown over the text, after the tick column
	h.keys(key(ports.KeyRight))       // Edit
	h.keys(key(ports.KeyEnter))       // Undo (first item)
	if h.doc.Text() != "" {
		t.Errorf("undo from menu: %q", h.doc.Text())
	}
	h.wantRowPrefix(rowPrompt, "> Undo")
	if h.focus != focusEditor {
		t.Error("menu still focused after running an item")
	}
}

func TestMenuSkipsSeparators(t *testing.T) {
	h := newHarness(t, nil)
	h.keys(ports.CtrlAlt('m'))
	for range 4 { // New, Open, Save, Save as → (separator skipped) Quit
		h.keys(key(ports.KeyDown))
	}
	if id := h.menus[0].Items[h.menu.item]; id != "file.quit" {
		t.Errorf("item %q, want file.quit", id)
	}
	h.keys(key(ports.KeyDown)) // wraps
	if h.menu.item != 0 {
		t.Errorf("wrap: item %d", h.menu.item)
	}
}

func TestMenuMouse(t *testing.T) {
	h := newHarness(t, nil)
	h.typeText("abc")
	h.click(8, rowMenu, 0) // "Edit"
	if h.focus != focusMenu || h.menu.open != 1 {
		t.Fatalf("focus %v menu %d", h.focus, h.menu.open)
	}
	h.click(8, 1, 0) // first item: Undo
	if h.doc.Text() != "" || h.focus != focusEditor {
		t.Errorf("text %q focus %v", h.doc.Text(), h.focus)
	}
	h.click(1, rowMenu, 0)
	h.click(30, 6, 0) // outside closes
	if h.focus != focusEditor {
		t.Error("click outside did not close menu")
	}
}

func TestMouseInText(t *testing.T) {
	h := newHarness(t, nil)
	h.typeText("hello\nworld")
	h.click(3+2, rowText+1, 0) // gutter is 3 wide: column 2 of line 2
	if line, col, _ := h.doc.CursorPos(); line != 1 || col != 2 {
		t.Fatalf("click: line %d col %d", line, col)
	}
	h.HandleMouse(ports.MouseEvent{X: 3 + 1, Y: rowText, Button: ports.MouseLeft, Action: ports.MousePress})
	h.HandleMouse(ports.MouseEvent{X: 3 + 4, Y: rowText + 1, Action: ports.MouseDrag})
	h.HandleMouse(ports.MouseEvent{X: 3 + 4, Y: rowText + 1, Action: ports.MouseRelease})
	if got := h.doc.SelectedText(); got != "ello\nworl" {
		t.Errorf("drag selection %q", got)
	}
	if h.styleAt(4, rowText) != ports.StyleSelection {
		t.Error("selection not styled")
	}
}

func TestWheelScrollsWithoutMovingCursor(t *testing.T) {
	h := newHarness(t, nil)
	h.typeText(strings.Repeat("x\n", 30))
	h.keys(ports.KeyEvent{Key: ports.KeyHome, Mod: ports.ModCtrl})
	h.HandleMouse(ports.MouseEvent{Button: ports.WheelDown, X: 5, Y: 3})
	if h.view.Top != 3 {
		t.Errorf("top %d", h.view.Top)
	}
	if line, _, _ := h.doc.CursorPos(); line != 0 {
		t.Errorf("cursor moved to %d", line)
	}
	if h.View().Cursor.Visible {
		t.Error("cursor drawn although scrolled out of view")
	}
}

func TestClipboard(t *testing.T) {
	h := newHarness(t, nil)
	h.typeText("one\ntwo")
	h.keys(ports.Ctrl('x')) // nothing selected: cuts the line
	if h.clip.text != "two" || h.doc.Text() != "one\n" {
		t.Fatalf("cut line: clip %q text %q", h.clip.text, h.doc.Text())
	}
	h.keys(ports.Ctrl('v'))
	if h.doc.Text() != "one\ntwo" {
		t.Errorf("paste: %q", h.doc.Text())
	}
	h.clip.err = errDisk // system clipboard broken: internal fallback still works
	h.keys(ports.Ctrl('a'), ports.Ctrl('c'), key(ports.KeyEnd), ports.Ctrl('v'))
	if h.doc.Text() != "one\ntwoone\ntwo" {
		t.Errorf("fallback paste: %q", h.doc.Text())
	}
}

func TestAltGrRuneIsTyped(t *testing.T) {
	h := newHarness(t, nil)
	h.keys(ports.KeyEvent{Key: ports.KeyRune, Rune: '@', Mod: ports.ModCtrl | ports.ModAlt})
	h.keys(ports.KeyEvent{Key: ports.KeyRune, Rune: 'x', Mod: ports.ModAlt}) // plain Alt+x: unbound, ignored
	if h.doc.Text() != "@" {
		t.Errorf("text %q", h.doc.Text())
	}
}

func TestHorizontalScrollClipsWideGlyph(t *testing.T) {
	h := newHarness(t, nil)
	h.typeText(strings.Repeat("a", 36) + "日本")
	// Text area is 40-3=37 cols; 日 would start at col 36 and is cut.
	h.keys(key(ports.KeyHome), key(ports.KeyHome)) // smart home: indent==0 → stays at 0
	h.wantRow(rowText, " 1 "+strings.Repeat("a", 36))
	h.keys(key(ports.KeyEnd))
	if h.view.Left == 0 {
		t.Fatal("did not scroll horizontally")
	}
	if c := h.View().Cursor; !c.Visible {
		t.Error("cursor not visible after horizontal scroll")
	}
}

func TestPasteIsOneUndoStep(t *testing.T) {
	h := newHarness(t, nil)
	h.HandlePaste("line1\nline2")
	h.keys(ports.Ctrl('z'))
	if h.doc.Text() != "" {
		t.Errorf("text %q", h.doc.Text())
	}
}

func TestTinyScreenDoesNotPanic(t *testing.T) {
	h := newHarness(t, nil)
	h.typeText("hello")
	for _, size := range [][2]int{{0, 0}, {1, 1}, {2, 2}, {5, 3}, {1, 10}} {
		h.Resize(size[0], size[1])
		_ = h.View()
		h.keys(ports.CtrlAlt('m'))
		_ = h.View()
		h.keys(key(ports.KeyEsc), ports.CtrlAlt('p'))
		_ = h.View()
		h.keys(key(ports.KeyEsc))
	}
}

func TestMenusReferenceKnownCommands(t *testing.T) {
	_, err := New(Config{Files: &memFS{}, Keymap: keys, Menus: []Menu{{"X", []string{"no.such"}}}})
	if err == nil {
		t.Error("unknown menu command accepted")
	}
}

func TestWhitespaceMarkers(t *testing.T) {
	h := newHarness(t, nil) // markers are off unless configured
	h.typeText("a b\tc\n\tx")
	h.wantRow(rowText, " 1 a b c")
	_ = h.Dispatch("view.toggleInvisibles")
	h.wantRowPrefix(rowPrompt, "> ✓ Invisible characters — shown")
	h.wantRow(rowText, " 1 a␣b»c")   // tab from col 3 to the stop at 4: one cell
	h.wantRow(rowText+1, " 2 »   x") // full-width tab: marker then blanks
	if h.styleAt(4, rowText) != ports.StyleWhitespace || h.styleAt(6, rowText) != ports.StyleWhitespace {
		t.Error("space/tab markers not styled as whitespace")
	}
	h.keys(ports.Ctrl('a'))
	if h.styleAt(4, rowText) != ports.StyleWhitespaceSelected || h.styleAt(3, rowText) != ports.StyleSelection {
		t.Error("selected markers not styled as selected")
	}
	_ = h.Dispatch("view.toggleInvisibles")
	h.wantRow(rowText, " 1 a b c")
}

func TestEmptyCellMarkers(t *testing.T) {
	h := newHarness(t, nil)
	h.typeText("ab\ncd")
	_ = h.Dispatch("view.toggleEmptyCells")
	h.wantRowPrefix(rowPrompt, "> ✓ Empty cells — shown")
	textCols := 40 - 3
	h.wantRow(rowText, " 1 ab"+strings.Repeat("·", textCols-2))
	h.wantRow(rowText+2, "   "+strings.Repeat("·", textCols)) // below the last line; gutter stays blank
	if h.styleAt(5, rowText) != ports.StyleEmptyCell {
		t.Error("empty cell not styled")
	}
	// A selected line break keeps its selected cell; dots start after it.
	h.keys(ports.Ctrl('a'))
	h.wantRow(rowText, " 1 ab "+strings.Repeat("·", textCols-3))
	if h.styleAt(5, rowText) != ports.StyleSelection {
		t.Error("selected line break lost its cell")
	}
}

func TestMarkersFromConfig(t *testing.T) {
	e, err := New(Config{Files: &memFS{}, Keymap: keys, ShowInvisibles: true, ShowEmptyCells: true})
	if err != nil {
		t.Fatal(err)
	}
	if !e.showInvisibles || !e.showEmptyCells {
		t.Error("config options not applied")
	}
}

func TestPromptOnlyDeleteToLineEnd(t *testing.T) {
	h := newHarness(t, nil)
	h.typeText("keep this\nnext")
	h.keys(ports.KeyEvent{Key: ports.KeyHome, Mod: ports.ModCtrl})
	h.keys(ports.KeyEvent{Key: ports.KeyRight, Mod: ports.ModCtrl}) // after "keep"
	h.keys(ports.CtrlAlt('p'))
	h.typeText("delete till end")
	if len(h.prompt.items) == 0 || h.prompt.items[0].cmd.ID != "edit.deleteToLineEnd" {
		t.Fatalf("first completion: %+v", h.prompt.items)
	}
	h.keys(key(ports.KeyEnter))
	if got := h.doc.Text(); got != "keep\nnext" {
		t.Errorf("text %q", got)
	}
	h.wantRow(rowPrompt, "> Delete to end of line") // no shortcut hint

	for _, m := range DefaultMenus() {
		for _, id := range m.Items {
			if id == "edit.deleteToLineEnd" {
				t.Errorf("prompt-only command is in menu %q", m.Title)
			}
		}
	}
}

func TestTrimEndingSpacesAlias(t *testing.T) {
	h := newHarness(t, nil)
	h.typeText("x  ")
	h.keys(ports.CtrlAlt('p'))
	h.typeText("trim ending spaces")
	if len(h.prompt.items) == 0 || h.prompt.items[0].cmd.ID != "selection.trimTrailingSpaces" {
		t.Fatalf("first completion: %+v", h.prompt.items)
	}
	h.keys(key(ports.KeyEnter))
	if got := h.doc.Text(); got != "x" {
		t.Errorf("text %q", got)
	}
}

func TestWholeNameCompletionNotDuplicated(t *testing.T) {
	h := newHarness(t, nil)
	h.keys(ports.CtrlAlt('p'))
	h.typeText("save")
	n := 0
	for _, it := range h.prompt.items {
		if it.cmd.ID == "file.save" {
			n++
		}
	}
	if n != 1 || h.prompt.items[0].cmd.ID != "file.save" {
		t.Errorf("file.save listed %d times; first %s", n, h.prompt.items[0].cmd.ID)
	}
}

func TestOptionsShowTicks(t *testing.T) {
	h := newHarness(t, nil) // both options off
	viewMenu := -1
	for i, m := range h.menus {
		if m.Title == "View" {
			viewMenu = i
		}
	}
	h.openMenu(viewMenu)
	h.wantRowContains(1, "   Invisible characters")
	h.wantRowContains(2, "   Empty cells")

	h.keys(key(ports.KeyDown), key(ports.KeyEnter)) // turn empty cells on
	h.wantRowPrefix(rowPrompt, "> ✓ Empty cells — shown")
	h.openMenu(viewMenu)
	h.wantRowContains(2, " ✓ Empty cells")
	h.keys(key(ports.KeyEsc))

	// Completions carry the same tick column.
	h.keys(ports.CtrlAlt('p'))
	h.typeText("empty cells")
	if it := h.prompt.items; len(it) == 0 || it[0].label != "✓ Empty cells" {
		t.Fatalf("completion %+v", it)
	}
	h.keys(key(ports.KeyEnter)) // and off again
	h.wantRowPrefix(rowPrompt, ">   Empty cells — hidden")
	h.keys(ports.CtrlAlt('p'))
	h.typeText("empty cells")
	if it := h.prompt.items; it[0].label != "  Empty cells" {
		t.Errorf("completion after turning off: %q", it[0].label)
	}
}

func TestPromptBarIdleAndClearing(t *testing.T) {
	h := newHarness(t, map[string]string{"f": "x"})
	// Idle: placeholder on the left, prompt shortcut on the right, gray bar.
	h.wantRow(rowPrompt, " write a command…            Ctrl+Alt+P")
	if h.styleAt(0, rowPrompt) != ports.StylePromptBar || h.styleAt(39, rowPrompt) != ports.StylePromptBar {
		t.Error("prompt line not filled with the bar style")
	}
	if h.styleAt(1, rowPrompt) != ports.StylePromptIdle {
		t.Error("idle text not styled")
	}
	// An echo stays until the next action in the editor, then idle returns.
	_ = h.Dispatch("file.open", "f")
	h.wantRowPrefix(rowPrompt, "> Open file f")
	_ = h.View()
	h.wantRowPrefix(rowPrompt, "> Open file f") // redraws alone don't clear it
	h.keys(key(ports.KeyRight))
	h.wantRowPrefix(rowPrompt, " write a command…")
	// Errors clear the same way, also on a click or a paste.
	h.fs.writeErr = errDisk
	h.keys(ports.Ctrl('s'))
	h.wantRowPrefix(rowPrompt, "Save file: disk full")
	h.click(5, rowText, 0)
	h.wantRowPrefix(rowPrompt, " write a command…")
	_ = h.Dispatch("help.about")
	h.HandlePaste("y")
	h.wantRowPrefix(rowPrompt, " write a command…")
	// The focused prompt is drawn on the same bar.
	h.keys(ports.CtrlAlt('p'))
	if h.styleAt(20, rowPrompt) != ports.StylePromptBar {
		t.Error("focused prompt lost the bar background")
	}
}

func TestStatusShowsVersionAndTruncates(t *testing.T) {
	h := newHarness(t, nil)
	h.wantRowPrefix(rowStatus, " sie …") // 40 columns: left part cut before the right part
	if !strings.HasSuffix(h.row(rowStatus), "Ln 1, Col 1   Plain   UTF-8   LF") {
		t.Errorf("right part damaged: %q", h.row(rowStatus))
	}
	_ = h.Dispatch("help.about")
	h.wantRowPrefix(rowPrompt, "> About sie — sie test")
}

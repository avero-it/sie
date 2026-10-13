package app

import (
	"github.com/avero-it/sie/internal/core/ports"
)

// separator in Menu.Items draws a divider line.
const separator = "-"

// Menu is a pull-down menu: a title and an ordered list of command IDs.
type Menu struct {
	Title string
	Items []string
}

// DefaultMenus is the v0 menu bar.
func DefaultMenus() []Menu {
	return []Menu{
		{"File", []string{"file.new", "file.open", "file.openRecent", "file.save", "file.saveAs", separator, "file.quit"}},
		{"Edit", []string{"edit.undo", "edit.redo", separator, "edit.cut", "edit.copy", "edit.paste",
			separator, "edit.duplicateLine", "edit.deleteLine"}},
		{"Selection", []string{"selection.all", "selection.line", separator, "selection.trimTrailingSpaces"}},
		{"Search", []string{"search.find", "search.next", "search.prev", separator,
			"search.replace", "search.replaceAll", separator, "search.gotoLine"}},
		{"View", []string{"view.toggleInvisibles", "view.toggleEmptyCells", separator,
			"view.toggleSyntax", "view.setLanguage"}},
		{"Help", []string{"ui.prompt", "help.about"}},
	}
}

type menuState struct {
	open int // index of the open menu
	item int // highlighted item
}

func (e *Editor) openMenu(i int) {
	e.focus = focusMenu
	e.menu = menuState{open: i, item: e.stepItem(i, -1, 1)}
}

func (e *Editor) menuKey(k ports.KeyEvent) {
	if id, ok := e.cfg.Keymap.Command(k); ok && (id == "ui.menu" || id == "ui.prompt") {
		_ = e.Dispatch(id)
		return
	}
	n := len(e.menus)
	switch k.Key {
	case ports.KeyEsc:
		e.focus = focusEditor
	case ports.KeyLeft:
		e.openMenu((e.menu.open + n - 1) % n)
	case ports.KeyRight:
		e.openMenu((e.menu.open + 1) % n)
	case ports.KeyUp:
		e.menu.item = e.stepItem(e.menu.open, e.menu.item, -1)
	case ports.KeyDown:
		e.menu.item = e.stepItem(e.menu.open, e.menu.item, 1)
	case ports.KeyEnter:
		e.runMenuItem(e.menu.item)
	}
}

// stepItem moves from item by dir, skipping separators and wrapping around.
func (e *Editor) stepItem(menu, item, dir int) int {
	items := e.menus[menu].Items
	for range items {
		item = (item + dir + len(items)) % len(items)
		if items[item] != separator {
			return item
		}
	}
	return 0
}

func (e *Editor) runMenuItem(i int) {
	items := e.menus[e.menu.open].Items
	if i < 0 || i >= len(items) || items[i] == separator {
		return
	}
	e.focus = focusEditor
	_ = e.Dispatch(items[i])
}

// menuLabel is how a command appears in a menu: a tick column (✓ for
// options that are on), then the title; commands that ask for arguments get
// an ellipsis.
func (e *Editor) menuLabel(id string) string {
	c, ok := e.reg.Get(id)
	if !ok {
		return id
	}
	label := tickColumn(e.checked(id)) + c.Title
	if len(c.Args) > 0 {
		label += "…"
	}
	return label
}

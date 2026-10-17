// Package app is the application layer of sie: it implements
// ports.EditorService on top of the domain. It owns the command registry
// and Dispatch, the focus state machine (editor, menu, prompt), mouse
// hit-testing and the ViewModel. It never touches the terminal or the OS.
package app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/avero-it/sie/internal/core/app/commands"
	"github.com/avero-it/sie/internal/core/domain/buffer"
	"github.com/avero-it/sie/internal/core/domain/editor"
	"github.com/avero-it/sie/internal/core/ports"
)

// Config wires the editor to its driven ports and settings.
type Config struct {
	Files      ports.FileStore
	Clipboard  ports.Clipboard // optional; an internal clipboard is always kept
	Keymap     ports.Keymap
	Menus      []Menu // nil means DefaultMenus()
	TabWidth   int
	DefaultEOL buffer.LineEnding
	Version    string // shown by "About sie"

	ShowInvisibles bool // mark spaces and tabs
	ShowEmptyCells bool // mark cells holding no character
	JumpLines      int  // lines moved by Alt+Up/Down; <= 0 means defaultJumpLines

	Recent    ports.RecentFiles // optional; nil disables "Open recent…"
	Validator ports.Validator   // optional; nil disables "Validate"

	ValidateOnSave bool   // check supported file types before writing them
	WorkDir        string // relative paths are resolved against it; "" means "."
	HomeDir        string // "~" expands to it; "" disables "~"

	Highlighter        ports.Highlighter // optional; nil means plain text only
	SyntaxHighlighting bool              // color known file types
	SyntaxMaxSize      int               // bytes; larger documents stay plain; <= 0 means 2 MB
}

const defaultJumpLines = 5

type focus uint8

const (
	focusEditor focus = iota
	focusMenu
	focusPrompt
)

type msgKind uint8

const (
	msgNone msgKind = iota
	msgEcho         // "> Title args — note"  with the shortcut as hint
	msgInfo
	msgError
)

type message struct {
	text, hint string
	kind       msgKind
}

type invocation struct {
	id   string
	args []string
}

const historyLimit = 50

// Editor implements ports.EditorService. It is not safe for concurrent use.
type Editor struct {
	cfg   Config
	reg   *commands.Registry[*Editor]
	menus []Menu

	doc  *editor.Document
	path string
	view editor.Viewport

	width, height    int
	focus            focus
	menu             menuState
	prompt           promptState
	msg              message
	history          []invocation // most recent first
	clip             string
	lastQuery        string
	dragging         bool
	dragUnit         dragUnit  // word/line granularity of the current drag
	dragFrom, dragTo int       // the unit first selected by a double or triple click
	rectDrag         *rectDrag // a rectangle being dragged out with the mouse
	done             bool

	showInvisibles bool
	showEmptyCells bool
	showSyntax     bool
	hl             highlightState
	validateOnSave bool
	columnMode     bool // Shift+arrows and plain drags make rectangles
}

var _ ports.EditorService = (*Editor)(nil)

// New creates an editor with an empty, unnamed document.
func New(cfg Config) (*Editor, error) {
	if cfg.Files == nil || cfg.Keymap == nil {
		return nil, errors.New("app: Files and Keymap are required")
	}
	e := &Editor{
		cfg:   cfg,
		reg:   commands.NewRegistry[*Editor](),
		menus: cfg.Menus,
		view:  editor.Viewport{ScrollOff: 2, SideScrollOff: 4},

		showInvisibles: cfg.ShowInvisibles,
		showEmptyCells: cfg.ShowEmptyCells,
		showSyntax:     cfg.SyntaxHighlighting,
		validateOnSave: cfg.ValidateOnSave,
	}
	if e.menus == nil {
		e.menus = DefaultMenus()
	}
	if err := e.reg.RegisterAll(builtins()...); err != nil {
		return nil, fmt.Errorf("app: %w", err)
	}
	for _, m := range e.menus {
		for _, id := range m.Items {
			if _, ok := e.reg.Get(id); id != separator && !ok {
				return nil, fmt.Errorf("app: menu %q references unknown command %q", m.Title, id)
			}
		}
	}
	e.doc = e.newDoc(nil)
	return e, nil
}

// Done reports whether the user asked to quit.
func (e *Editor) Done() bool { return e.done }

// Resize sets the screen size in cells.
func (e *Editor) Resize(width, height int) {
	e.width, e.height = max(0, width), max(0, height)
	e.follow()
}

// HandleKey routes a key to the focused component.
func (e *Editor) HandleKey(k ports.KeyEvent) {
	if e.focus == focusEditor {
		e.clearMessage()
	}
	switch e.focus {
	case focusMenu:
		e.menuKey(k)
	case focusPrompt:
		e.promptKey(k)
	default:
		e.editorKey(k)
	}
	e.follow()
	e.syncHighlight()
}

// HandlePaste inserts pasted text as one edit (one undo step).
func (e *Editor) HandlePaste(text string) {
	if e.focus == focusEditor {
		e.clearMessage()
	}
	switch e.focus {
	case focusPrompt:
		line, _, _ := strings.Cut(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
		e.prompt.input.Insert(line)
		e.promptChanged()
	case focusEditor:
		e.check(e.doc.Insert(text))
		e.follow()
	}
	e.syncHighlight()
}

// Dispatch runs a command; it is the single entry point for shortcuts,
// menus and the prompt. Missing arguments are asked for in the prompt.
// Non-silent commands are echoed in the prompt line and kept in history.
func (e *Editor) Dispatch(id string, args ...string) error {
	defer e.syncHighlight()
	c, ok := e.reg.Get(id)
	if !ok {
		err := fmt.Errorf("unknown command %q", id)
		e.setError(err)
		return err
	}
	if len(args) < len(c.Args) {
		e.askArgs(c, args)
		return nil
	}
	label := invocationLabel(c, args)
	if !c.Silent && !c.MultiCursor {
		e.doc.ClearSecondary()
	}
	if !c.Silent {
		e.msg = message{text: label, hint: e.shortcut(c.ID), kind: msgEcho}
		e.remember(c.ID, args)
	}
	if err := c.Run(e, args); err != nil {
		e.msg = message{text: label + ": " + err.Error(), kind: msgError}
		return err
	}
	if c.Checked != nil && !c.Silent && e.msg.kind == msgEcho {
		e.msg.text = tickColumn(c.Checked(e)) + e.msg.text // reads like the menu item
	}
	return nil
}

// tickMark is shown before options that are on. It is one cell wide in
// every terminal (East Asian width Neutral).
const tickMark = "✓"

// tickColumn is the 2-cell column in front of menu items and completions.
func tickColumn(on bool) string {
	if on {
		return tickMark + " "
	}
	return "  "
}

// checked reports whether the command is an option that is currently on.
func (e *Editor) checked(id string) bool {
	c, ok := e.reg.Get(id)
	return ok && c.Checked != nil && c.Checked(e)
}

func (e *Editor) editorKey(k ports.KeyEvent) {
	if id, ok := e.cfg.Keymap.Command(k); ok {
		_ = e.Dispatch(id) // errors are shown in the prompt line
		return
	}
	if e.navigate(k) {
		return
	}
	switch k.Key {
	case ports.KeyBackspace:
		e.check(e.doc.Backspace())
	case ports.KeyDelete:
		e.check(e.doc.DeleteForward())
	case ports.KeyEnter:
		e.check(e.doc.InsertNewline())
	case ports.KeyTab:
		e.check(e.doc.Insert("\t"))
	case ports.KeyEsc:
		if !e.doc.ClearSecondary() {
			e.doc.SetSelection(editor.Cursor(e.doc.Selection().Head))
		}
	case ports.KeyRune:
		if isText(k) {
			e.check(e.doc.Insert(string(k.Rune)))
		}
	}
}

// navigate handles cursor movement keys; Shift extends the selection with
// every one of them. It reports whether k was a movement key.
//
//	←/→                 character    Ctrl or Alt: word    Cmd: line start/end
//	Alt+B / Alt+F       word left/right (macOS Terminal sends these for Option+←/→)
//	↑/↓                 line         Alt: JumpLines lines  Cmd: document start/end
//	Home/End            line start/end                     Ctrl: document start/end
//	PgUp/PgDn           one screen
func (e *Editor) navigate(k ports.KeyEvent) bool {
	shift := k.Mod&ports.ModShift != 0
	if e.columnMode && k.Mod == ports.ModShift && e.extendRect(k.Key) {
		return true
	}
	cmd := k.Mod&ports.ModSuper != 0
	word := k.Mod&(ports.ModCtrl|ports.ModAlt) != 0
	move := func(m editor.Motion) bool {
		e.doc.Move(m, shift)
		return true
	}
	pick := func(plain, wordwise, withCmd editor.Motion) bool {
		switch {
		case cmd:
			return move(withCmd)
		case word:
			return move(wordwise)
		default:
			return move(plain)
		}
	}
	switch k.Key {
	case ports.KeyLeft:
		return pick(editor.MoveLeft, editor.MoveWordLeft, editor.MoveLineStart)
	case ports.KeyRight:
		return pick(editor.MoveRight, editor.MoveWordRight, editor.MoveLineEnd)
	case ports.KeyUp, ports.KeyDown:
		up := k.Key == ports.KeyUp
		switch {
		case cmd && up:
			return move(editor.MoveDocStart)
		case cmd:
			return move(editor.MoveDocEnd)
		case k.Mod&ports.ModAlt != 0:
			n := e.jumpLines()
			if up {
				n = -n
			}
			e.doc.MovePage(n, shift)
			return true
		case up:
			return move(editor.MoveUp)
		default:
			return move(editor.MoveDown)
		}
	case ports.KeyHome:
		return pick(editor.MoveLineStart, editor.MoveDocStart, editor.MoveLineStart)
	case ports.KeyEnd:
		return pick(editor.MoveLineEnd, editor.MoveDocEnd, editor.MoveLineEnd)
	case ports.KeyPgUp:
		e.doc.MovePage(-max(1, e.view.Rows-1), shift)
		return true
	case ports.KeyPgDn:
		e.doc.MovePage(max(1, e.view.Rows-1), shift)
		return true
	case ports.KeyRune:
		// Readline word moves; Shift arrives as an upper-case rune.
		if k.Mod&^ports.ModShift == ports.ModAlt {
			switch k.Rune {
			case 'b', 'B':
				e.doc.Move(editor.MoveWordLeft, shift || k.Rune == 'B')
				return true
			case 'f', 'F':
				e.doc.Move(editor.MoveWordRight, shift || k.Rune == 'F')
				return true
			}
		}
	}
	return false
}

func (e *Editor) jumpLines() int {
	if e.cfg.JumpLines > 0 {
		return e.cfg.JumpLines
	}
	return defaultJumpLines
}

// isText reports whether an unbound rune key types text: no modifiers
// except Shift, or Ctrl+Alt together (AltGr on Windows). Cmd never types.
func isText(k ports.KeyEvent) bool {
	if k.Mod&ports.ModSuper != 0 {
		return false
	}
	ctrlAlt := k.Mod & (ports.ModCtrl | ports.ModAlt)
	return ctrlAlt == 0 || ctrlAlt == ports.ModCtrl|ports.ModAlt
}

// follow scrolls the viewport so the cursor stays visible.
func (e *Editor) follow() {
	e.layout()
	line, _, vcol := e.doc.CursorPos()
	e.view.Follow(line, vcol)
}

func (e *Editor) newDoc(content []byte) *editor.Document {
	return editor.NewDocument(content, editor.Options{TabWidth: e.cfg.TabWidth, DefaultEOL: e.cfg.DefaultEOL})
}

func (e *Editor) setDoc(d *editor.Document, path string) {
	e.doc, e.path = d, path
	e.view.Top, e.view.Left = 0, 0
	e.hl.manual = false
	e.resetHighlight()
	e.detectLanguage()
}

// note appends a result to the current echo, or shows it on its own.
func (e *Editor) note(s string) {
	if e.msg.kind == msgEcho || e.msg.kind == msgInfo {
		e.msg.text += " — " + s
		return
	}
	e.msg = message{text: s, kind: msgInfo}
}

// clearMessage drops the echo or error, so the prompt line goes back to
// idle. It runs before each editor-focus action, so a message stays visible
// until the user does something else.
func (e *Editor) clearMessage() { e.msg = message{} }

// version is shown in the status line and by "About sie".
func (e *Editor) version() string {
	if e.cfg.Version == "" {
		return "dev"
	}
	return e.cfg.Version
}

func (e *Editor) info(s string) { e.msg = message{text: s, kind: msgInfo} }

func (e *Editor) setError(err error) { e.msg = message{text: err.Error(), kind: msgError} }

// check shows err, if any, in the prompt line.
func (e *Editor) check(err error) {
	if err != nil {
		e.setError(err)
	}
}

func (e *Editor) shortcut(id string) string {
	if k, ok := e.cfg.Keymap.Binding(id); ok {
		return k.String()
	}
	return ""
}

func (e *Editor) remember(id string, args []string) {
	inv := invocation{id: id, args: args}
	out := []invocation{inv}
	for _, h := range e.history {
		if h.id != id || strings.Join(h.args, "\x00") != strings.Join(args, "\x00") {
			out = append(out, h)
		}
	}
	e.history = out[:min(len(out), historyLimit)]
}

func invocationLabel(c *commands.Command[*Editor], args []string) string {
	if len(args) == 0 {
		return c.Title
	}
	return c.Title + " " + strings.Join(args, " ")
}

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
	Version    string
}

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

	width, height int
	focus         focus
	menu          menuState
	prompt        promptState
	msg           message
	history       []invocation // most recent first
	clip          string
	lastQuery     string
	dragging      bool
	done          bool
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
	switch e.focus {
	case focusMenu:
		e.menuKey(k)
	case focusPrompt:
		e.promptKey(k)
	default:
		e.editorKey(k)
	}
	e.follow()
}

// HandlePaste inserts pasted text as one edit (one undo step).
func (e *Editor) HandlePaste(text string) {
	switch e.focus {
	case focusPrompt:
		line, _, _ := strings.Cut(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
		e.prompt.input.Insert(line)
		e.promptChanged()
	case focusEditor:
		e.check(e.doc.Insert(text))
		e.follow()
	}
}

// Dispatch runs a command; it is the single entry point for shortcuts,
// menus and the prompt. Missing arguments are asked for in the prompt.
// Non-silent commands are echoed in the prompt line and kept in history.
func (e *Editor) Dispatch(id string, args ...string) error {
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
	if !c.Silent {
		e.msg = message{text: label, hint: e.shortcut(c.ID), kind: msgEcho}
		e.remember(c.ID, args)
	}
	if err := c.Run(e, args); err != nil {
		e.msg = message{text: label + ": " + err.Error(), kind: msgError}
		return err
	}
	return nil
}

func (e *Editor) editorKey(k ports.KeyEvent) {
	if id, ok := e.cfg.Keymap.Command(k); ok {
		_ = e.Dispatch(id) // errors are shown in the prompt line
		return
	}
	shift := k.Mod&ports.ModShift != 0
	word := k.Mod&ports.ModCtrl != 0
	move := func(plain, ctrl editor.Motion) {
		if word {
			e.doc.Move(ctrl, shift)
		} else {
			e.doc.Move(plain, shift)
		}
	}
	switch k.Key {
	case ports.KeyLeft:
		move(editor.MoveLeft, editor.MoveWordLeft)
	case ports.KeyRight:
		move(editor.MoveRight, editor.MoveWordRight)
	case ports.KeyUp:
		e.doc.Move(editor.MoveUp, shift)
	case ports.KeyDown:
		e.doc.Move(editor.MoveDown, shift)
	case ports.KeyHome:
		move(editor.MoveLineStart, editor.MoveDocStart)
	case ports.KeyEnd:
		move(editor.MoveLineEnd, editor.MoveDocEnd)
	case ports.KeyPgUp:
		e.doc.MovePage(-max(1, e.view.Rows-1), shift)
	case ports.KeyPgDn:
		e.doc.MovePage(max(1, e.view.Rows-1), shift)
	case ports.KeyBackspace:
		e.check(e.doc.Backspace())
	case ports.KeyDelete:
		e.check(e.doc.DeleteForward())
	case ports.KeyEnter:
		e.check(e.doc.InsertNewline())
	case ports.KeyTab:
		e.check(e.doc.Insert("\t"))
	case ports.KeyEsc:
		e.doc.SetSelection(editor.Cursor(e.doc.Selection().Head))
	case ports.KeyRune:
		if isText(k) {
			e.check(e.doc.Insert(string(k.Rune)))
		}
	}
}

// isText reports whether an unbound rune key types text: no modifiers
// except Shift, or Ctrl+Alt together (AltGr on Windows).
func isText(k ports.KeyEvent) bool {
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
}

// note appends a result to the current echo, or shows it on its own.
func (e *Editor) note(s string) {
	if e.msg.kind == msgEcho || e.msg.kind == msgInfo {
		e.msg.text += " — " + s
		return
	}
	e.msg = message{text: s, kind: msgInfo}
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

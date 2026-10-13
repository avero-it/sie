// Package tui is the terminal adapter of sie: it owns the tcell screen,
// normalizes tcell events into ports events, feeds them to the
// EditorService and paints the ViewModel it returns. It contains no editor
// logic and no layout.
package tui

import (
	"fmt"
	"strings"

	"github.com/avero-it/sie/internal/core/ports"
	"github.com/gdamore/tcell/v2"
)

// Terminal owns the tcell screen; it is the only tcell object outside this package's internals.
type Terminal struct {
	screen tcell.Screen
	theme  Theme
}

// New opens the terminal (or Windows console) without initializing it yet.
func New() (*Terminal, error) {
	screen, err := tcell.NewScreen()
	if err != nil {
		return nil, fmt.Errorf("open terminal: %w", err)
	}
	return &Terminal{screen: screen, theme: DefaultTheme()}, nil
}

// Wake makes the event loop run once, so the editor collects results of
// background work (Poll) and redraws. Safe to call from any goroutine; a
// wake-up that cannot be queued is dropped, since the loop polls before
// every redraw anyway.
func (t *Terminal) Wake() { _ = t.screen.PostEvent(tcell.NewEventInterrupt(nil)) }

// SetClipboard sends data to the terminal clipboard via OSC52.
func (t *Terminal) SetClipboard(data []byte) { t.screen.SetClipboard(data) }

// Run runs the editor in the terminal until it reports Done.
func (t *Terminal) Run(svc ports.EditorService) error { return run(t.screen, svc, t.theme) }

// run initializes the screen and runs the event loop until the service
// reports Done. The terminal is always restored, also on panic.
func run(screen tcell.Screen, svc ports.EditorService, theme Theme) (err error) {
	if err := screen.Init(); err != nil {
		return fmt.Errorf("init terminal: %w", err)
	}
	defer func() {
		r := recover()
		screen.Fini()
		if r != nil {
			panic(r) // re-panic after the terminal is restored, so the trace is readable
		}
	}()
	screen.EnableMouse(tcell.MouseButtonEvents | tcell.MouseDragEvents)
	screen.EnablePaste()

	l := &loop{screen: screen, svc: svc, theme: theme}
	svc.Resize(screen.Size())

	events := make(chan tcell.Event, 256)
	quit := make(chan struct{})
	defer close(quit)
	go screen.ChannelEvents(events, quit)

	l.draw()
	for !svc.Done() {
		ev, ok := <-events
		if !ok {
			return nil
		}
		l.handle(ev)
		// Coalesce: handle everything already queued, then draw once.
	drain:
		for !svc.Done() {
			select {
			case ev, ok := <-events:
				if !ok {
					return nil
				}
				l.handle(ev)
			default:
				break drain
			}
		}
		l.draw()
	}
	return nil
}

// loop holds the adapter state between events.
type loop struct {
	screen tcell.Screen
	svc    ports.EditorService
	theme  Theme
	mouse  mouseTracker

	pasting bool
	paste   strings.Builder
}

func (l *loop) handle(ev tcell.Event) {
	switch ev := ev.(type) {
	case *tcell.EventResize:
		l.screen.Sync()
		l.svc.Resize(ev.Size())
	case *tcell.EventPaste:
		if ev.Start() {
			l.pasting = true
			l.paste.Reset()
			return
		}
		l.pasting = false
		if l.paste.Len() > 0 {
			l.svc.HandlePaste(l.paste.String())
		}
	case *tcell.EventKey:
		if l.pasting {
			l.collectPaste(ev)
			return
		}
		if k, ok := normalizeKey(ev); ok {
			l.svc.HandleKey(k)
		}
	case *tcell.EventMouse:
		if m, ok := l.mouse.normalize(ev); ok {
			l.svc.HandleMouse(m)
		}
	case *tcell.EventInterrupt:
		// A Wake from background work: draw (which polls) follows.
	}
}

// collectPaste accumulates the key events of a bracketed paste as text.
func (l *loop) collectPaste(ev *tcell.EventKey) {
	switch ev.Key() {
	case tcell.KeyRune:
		l.paste.WriteRune(ev.Rune())
	case tcell.KeyEnter, tcell.KeyLF:
		l.paste.WriteByte('\n')
	case tcell.KeyTab:
		l.paste.WriteByte('\t')
	}
}

func (l *loop) draw() {
	l.svc.Poll()
	paint(l.screen, l.svc.View(), l.theme)
}

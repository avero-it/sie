// Package tui is the terminal adapter of sie: it owns the tcell screen,
// normalizes tcell events into ports events, feeds them to the
// EditorService and paints the ViewModel it returns. It contains no editor
// logic and no layout.
package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/avero-it/sie/internal/core/ports"
	"github.com/gdamore/tcell/v2"
)

// Terminal owns the tcell screen; it is the only tcell object outside this package's internals.
type Terminal struct {
	screen tcell.Screen
	theme  Theme
	jobs   jobQueue
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

// Submit queues fn to run on the event loop goroutine, which owns the
// editor, just before the next frame is drawn. Safe to call from any
// goroutine. A job submitted before Run starts runs on its first frame;
// one submitted after Run returned never runs, so callers wait for their
// result with a timeout.
func (t *Terminal) Submit(fn func()) {
	t.jobs.add(fn)
	if t.jobs.live.Load() {
		t.Wake()
	}
}

// SetClipboard sends data to the terminal clipboard via OSC52.
func (t *Terminal) SetClipboard(data []byte) { t.screen.SetClipboard(data) }

// Run runs the editor in the terminal until it reports Done.
func (t *Terminal) Run(svc ports.EditorService) error {
	return run(t.screen, svc, t.theme, &t.jobs)
}

// run initializes the screen and runs the event loop until the service
// reports Done. The terminal is always restored, also on panic.
func run(screen tcell.Screen, svc ports.EditorService, theme Theme, jobs *jobQueue) (err error) {
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
	if jobs != nil {
		jobs.live.Store(true)
		defer jobs.live.Store(false)
	}
	screen.EnableMouse(tcell.MouseButtonEvents | tcell.MouseDragEvents)
	screen.EnablePaste()

	l := &loop{screen: screen, svc: svc, theme: theme, jobs: jobs}
	svc.Resize(screen.Size())

	events := make(chan tcell.Event, 256)
	quit := make(chan struct{})
	defer close(quit)
	go screen.ChannelEvents(events, quit)

	// While the view animates, a timer wakes the loop for the next frame.
	// The channel is nil when idle, so an idle editor costs nothing.
	var timer *time.Timer
	var tick <-chan time.Time
	frame := func(animate bool) {
		if timer != nil {
			timer.Stop()
		}
		tick = nil
		if animate {
			timer = time.NewTimer(frameInterval)
			tick = timer.C
		}
	}
	frame(l.draw())
	for !svc.Done() {
		select {
		case ev, ok := <-events:
			if !ok {
				return nil
			}
			l.handle(ev)
		case <-tick:
			frame(l.draw())
			continue
		}
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
		frame(l.draw())
	}
	return nil
}

// frameInterval is the time between frames of an animation (about 30 fps).
const frameInterval = 33 * time.Millisecond

// loop holds the adapter state between events.
type loop struct {
	screen tcell.Screen
	svc    ports.EditorService
	theme  Theme
	mouse  mouseTracker
	jobs   *jobQueue

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

// draw paints one frame and reports whether the view is animating.
func (l *loop) draw() bool {
	if l.jobs != nil {
		l.jobs.run()
	}
	l.svc.Poll()
	vm := l.svc.View()
	paint(l.screen, vm, l.theme)
	return vm.Animate
}

package tui

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/avero-it/sie/internal/core/ports"
	"github.com/gdamore/tcell/v2"
)

func TestNormalizeKey(t *testing.T) {
	type in struct {
		k   tcell.Key
		r   rune
		mod tcell.ModMask
	}
	runeKey := func(r rune, m ports.Mod) ports.KeyEvent { return ports.KeyEvent{Key: ports.KeyRune, Rune: r, Mod: m} }
	tests := []struct {
		name string
		in   in
		want ports.KeyEvent
		ok   bool
	}{
		{"letter", in{tcell.KeyRune, 'a', 0}, runeKey('a', 0), true},
		{"shifted letter drops Shift (Windows)", in{tcell.KeyRune, 'A', tcell.ModShift}, runeKey('A', 0), true},
		{"Ctrl+S as control byte", in{tcell.KeyRune, 0x13, 0}, ports.Ctrl('s'), true},
		{"Ctrl+S as key", in{tcell.KeyCtrlS, 's', tcell.ModCtrl}, ports.Ctrl('s'), true},
		{"Ctrl+Alt+P via ESC prefix", in{tcell.KeyRune, 0x10, tcell.ModAlt}, ports.CtrlAlt('p'), true},
		{"Ctrl+Alt+P via CSI-u", in{tcell.KeyRune, 'p', tcell.ModCtrl | tcell.ModAlt}, ports.CtrlAlt('p'), true},
		{"Ctrl+Alt+M is Alt+Enter", in{tcell.KeyRune, '\r', tcell.ModAlt}, ports.CtrlAlt('m'), true},
		{"Ctrl+M is Enter", in{tcell.KeyRune, 'm', tcell.ModCtrl}, ports.KeyEvent{Key: ports.KeyEnter}, true},
		{"Enter", in{tcell.KeyEnter, 0, 0}, ports.KeyEvent{Key: ports.KeyEnter}, true},
		{"LF is Enter", in{tcell.KeyLF, 0, 0}, ports.KeyEvent{Key: ports.KeyEnter}, true},
		{"Tab", in{tcell.KeyTab, 0, 0}, ports.KeyEvent{Key: ports.KeyTab}, true},
		{"Shift+Tab", in{tcell.KeyTab, 0, tcell.ModShift}, ports.KeyEvent{Key: ports.KeyTab, Mod: ports.ModShift}, true},
		{"Backspace (DEL)", in{tcell.KeyBackspace2, 0, 0}, ports.KeyEvent{Key: ports.KeyBackspace}, true},
		{"Ctrl+H is Backspace", in{tcell.KeyRune, 0x08, 0}, ports.KeyEvent{Key: ports.KeyBackspace}, true},
		{"Ctrl+/ is 0x1F", in{tcell.KeyRune, 0x1f, 0}, runeKey('/', ports.ModCtrl), true},
		{"Ctrl+Shift+7 is 0x1F too", in{tcell.KeyUS, 0, tcell.ModShift}, runeKey('/', ports.ModCtrl), true},
		{"Ctrl+/ via CSI-u", in{tcell.KeyRune, '/', tcell.ModCtrl | tcell.ModShift}, runeKey('/', ports.ModCtrl), true},
		{"Alt+Backspace", in{tcell.KeyBackspace2, 0, tcell.ModAlt}, ports.KeyEvent{Key: ports.KeyBackspace, Mod: ports.ModAlt}, true},
		{"Esc", in{tcell.KeyEsc, 0, 0}, ports.KeyEvent{Key: ports.KeyEsc}, true},
		{"Shift+Ctrl+Right", in{tcell.KeyRight, 0, tcell.ModShift | tcell.ModCtrl},
			ports.KeyEvent{Key: ports.KeyRight, Mod: ports.ModShift | ports.ModCtrl}, true},
		{"Ctrl+Home", in{tcell.KeyHome, 0, tcell.ModCtrl}, ports.KeyEvent{Key: ports.KeyHome, Mod: ports.ModCtrl}, true},
		{"Cmd+Shift+Left (kitty Super)", in{tcell.KeyLeft, 0, tcell.ModMeta | tcell.ModShift},
			ports.KeyEvent{Key: ports.KeyLeft, Mod: ports.ModSuper | ports.ModShift}, true},
		{"Alt+Up", in{tcell.KeyUp, 0, tcell.ModAlt}, ports.KeyEvent{Key: ports.KeyUp, Mod: ports.ModAlt}, true},
		{"Option+Left in Terminal.app is ESC b", in{tcell.KeyRune, 'b', tcell.ModAlt}, runeKey('b', ports.ModAlt), true},
		{"AltGr rune keeps Ctrl+Alt", in{tcell.KeyRune, '@', tcell.ModCtrl | tcell.ModAlt}, runeKey('@', ports.ModCtrl|ports.ModAlt), true},
		{"Ctrl+Space", in{tcell.KeyCtrlSpace, 0, tcell.ModCtrl}, runeKey(' ', ports.ModCtrl), true},
		{"F1 unused", in{tcell.KeyF1, 0, 0}, ports.KeyEvent{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := normalizeKey(tcell.NewEventKey(tt.in.k, tt.in.r, tt.in.mod))
			if ok != tt.ok || got != tt.want {
				t.Errorf("got %v (%+v) ok=%v, want %v (%+v) ok=%v", got, got, ok, tt.want, tt.want, tt.ok)
			}
		})
	}
}

func TestMouseTracker(t *testing.T) {
	var m mouseTracker
	steps := []struct {
		buttons tcell.ButtonMask
		want    ports.MouseEvent
		ok      bool
	}{
		{tcell.ButtonNone, ports.MouseEvent{}, false}, // motion only
		{tcell.Button1, ports.MouseEvent{Button: ports.MouseLeft, Action: ports.MousePress, Clicks: 1}, true},
		{tcell.Button1, ports.MouseEvent{Button: ports.MouseLeft, Action: ports.MouseDrag}, true},
		{tcell.ButtonNone, ports.MouseEvent{Button: ports.MouseLeft, Action: ports.MouseRelease}, true},
		{tcell.WheelDown, ports.MouseEvent{Button: ports.WheelDown, Action: ports.MousePress}, true},
		{tcell.Button2, ports.MouseEvent{Button: ports.MouseRight, Action: ports.MousePress}, true},
	}
	for i, s := range steps {
		got, ok := m.normalize(tcell.NewEventMouse(0, 0, s.buttons, 0))
		if ok != s.ok || (ok && got != s.want) {
			t.Errorf("step %d: got %+v ok=%v, want %+v ok=%v", i, got, ok, s.want, s.ok)
		}
	}
}

func TestCountClick(t *testing.T) {
	var m mouseTracker
	t0 := time.Unix(100, 0)
	steps := []struct {
		x, y int
		at   time.Duration
		want int
	}{
		{1, 1, 0, 1},
		{1, 1, 100 * time.Millisecond, 2},
		{1, 1, 200 * time.Millisecond, 3},
		{1, 1, 300 * time.Millisecond, 1}, // a fourth starts over
		{1, 1, 400 * time.Millisecond, 2},
		{2, 1, 500 * time.Millisecond, 1},  // moved
		{2, 1, 1500 * time.Millisecond, 1}, // too slow
	}
	for i, s := range steps {
		if got := m.countClick(s.x, s.y, t0.Add(s.at)); got != s.want {
			t.Errorf("step %d: got %d, want %d", i, got, s.want)
		}
	}
}

func TestThemeCoversAllStyles(t *testing.T) {
	theme := DefaultTheme()
	for s := ports.Style(0); s < ports.StyleCount; s++ {
		if _, ok := theme[s]; !ok {
			t.Errorf("style %d has no theme entry", s)
		}
	}
}

func newSim(t *testing.T, w, h int) tcell.SimulationScreen {
	t.Helper()
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	s.SetSize(w, h)
	t.Cleanup(s.Fini)
	return s
}

// contents renders the simulation screen as text rows.
func contents(s tcell.SimulationScreen) []string {
	w, h := s.Size()
	rows := make([]string, h)
	for y := range h {
		var b strings.Builder
		for x := 0; x < w; x++ {
			str, _, width := s.Get(x, y)
			b.WriteString(str)
			if width == 2 {
				x++
			}
		}
		rows[y] = strings.TrimRight(b.String(), " ")
	}
	return rows
}

func TestPaint(t *testing.T) {
	s := newSim(t, 12, 2)
	theme := DefaultTheme()
	paint(s, ports.ViewModel{
		Width: 12, Height: 2,
		Runs: []ports.Run{
			{X: 0, Y: 0, Text: "a日éx", Style: ports.StyleDefault},
			{X: 0, Y: 1, Text: strings.Repeat(" ", 12), Style: ports.StyleStatus},
			{X: 1, Y: 1, Text: "status", Style: ports.StyleStatus},
			{X: 10, Y: 1, Text: "clipped", Style: ports.StyleError},
		},
		Cursor: ports.Cursor{X: 4, Y: 0, Visible: true},
	}, theme)
	got := contents(s)
	if got[0] != "a日éx" || got[1] != " status   cl" {
		t.Errorf("screen:\n%q", got)
	}
	if _, st, _ := s.Get(2, 1); st != theme[ports.StyleStatus] {
		t.Error("status style not applied")
	}
	if x, y, vis := s.GetCursor(); !vis || x != 4 || y != 0 {
		t.Errorf("cursor %d,%d visible=%v", x, y, vis)
	}
}

// fakeService records input and quits on Ctrl+Q.
type fakeService struct {
	mu     sync.Mutex
	keys   []ports.KeyEvent
	pastes []string
	mice   []ports.MouseEvent
	w, h   int
	done   bool
	polls  int
	// animateUntil: View asks for another frame while polls < animateUntil.
	animateUntil int
}

func (f *fakeService) HandleKey(k ports.KeyEvent) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.keys = append(f.keys, k)
	if k == ports.Ctrl('q') {
		f.done = true
	}
}
func (f *fakeService) HandleMouse(m ports.MouseEvent) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mice = append(f.mice, m)
}
func (f *fakeService) HandlePaste(s string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pastes = append(f.pastes, s)
}
func (f *fakeService) Resize(w, h int)                  { f.mu.Lock(); f.w, f.h = w, h; f.mu.Unlock() }
func (f *fakeService) Dispatch(string, ...string) error { return nil }
func (f *fakeService) Poll()                            { f.mu.Lock(); f.polls++; f.mu.Unlock() }
func (f *fakeService) Done() bool                       { f.mu.Lock(); defer f.mu.Unlock(); return f.done }
func (f *fakeService) View() ports.ViewModel {
	f.mu.Lock()
	defer f.mu.Unlock()
	return ports.ViewModel{Runs: []ports.Run{{Text: "hello"}}, Animate: f.polls < f.animateUntil}
}

func TestLoopHandlesPasteAsOneEvent(t *testing.T) {
	s := newSim(t, 20, 5)
	svc := &fakeService{}
	l := &loop{screen: s, svc: svc, theme: DefaultTheme()}
	l.handle(tcell.NewEventPaste(true))
	for _, ev := range []*tcell.EventKey{
		tcell.NewEventKey(tcell.KeyRune, 'a', 0),
		tcell.NewEventKey(tcell.KeyEnter, 0, 0),
		tcell.NewEventKey(tcell.KeyTab, 0, 0),
		tcell.NewEventKey(tcell.KeyRune, 'b', 0),
	} {
		l.handle(ev)
	}
	l.handle(tcell.NewEventPaste(false))
	if len(svc.keys) != 0 || len(svc.pastes) != 1 || svc.pastes[0] != "a\n\tb" {
		t.Errorf("keys %v pastes %q", svc.keys, svc.pastes)
	}
}

func TestRunEndToEnd(t *testing.T) {
	s := tcell.NewSimulationScreen("UTF-8")
	svc := &fakeService{}
	errc := make(chan error, 1)
	go func() { errc <- run(s, svc, DefaultTheme()) }()

	// run initializes the screen and then reports the size; injecting
	// events before Init blocks the simulation screen.
	deadline := time.Now().Add(5 * time.Second)
	for {
		svc.mu.Lock()
		ready := svc.w > 0
		svc.mu.Unlock()
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("run never resized the service")
		}
		time.Sleep(5 * time.Millisecond)
	}
	s.InjectKey(tcell.KeyRune, 'x', tcell.ModNone)
	s.InjectKey(tcell.KeyCtrlQ, 'q', tcell.ModCtrl)
	select {
	case err := <-errc:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run did not exit after Ctrl+Q")
	}
	if len(svc.keys) != 2 || svc.keys[0] != (ports.KeyEvent{Key: ports.KeyRune, Rune: 'x'}) {
		t.Errorf("keys %+v", svc.keys)
	}
}

func TestWakePollsBeforeDrawing(t *testing.T) {
	s := tcell.NewSimulationScreen("UTF-8")
	svc := &fakeService{}
	term := &Terminal{screen: s, theme: DefaultTheme()}
	errc := make(chan error, 1)
	go func() { errc <- term.Run(svc) }()
	deadline := time.Now().Add(5 * time.Second)
	polls := func() int { svc.mu.Lock(); defer svc.mu.Unlock(); return svc.polls }
	for polls() == 0 { // the first draw polls
		if time.Now().After(deadline) {
			t.Fatal("never polled")
		}
		time.Sleep(5 * time.Millisecond)
	}
	before := polls()
	term.Wake()
	for polls() == before {
		if time.Now().After(deadline) {
			t.Fatal("Wake did not lead to a poll")
		}
		time.Sleep(5 * time.Millisecond)
	}
	s.InjectKey(tcell.KeyCtrlQ, 'q', tcell.ModCtrl)
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
}

func TestAnimationTicksUntilDone(t *testing.T) {
	s := tcell.NewSimulationScreen("UTF-8")
	svc := &fakeService{animateUntil: 5}
	term := &Terminal{screen: s, theme: DefaultTheme()}
	errc := make(chan error, 1)
	go func() { errc <- term.Run(svc) }()
	deadline := time.Now().Add(5 * time.Second)
	polls := func() int { svc.mu.Lock(); defer svc.mu.Unlock(); return svc.polls }
	for polls() < 5 { // no input at all: only the frame timer can poll
		if time.Now().After(deadline) {
			t.Fatalf("only %d polls without input", polls())
		}
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(3 * frameInterval)
	if n := polls(); n != 5 {
		t.Errorf("polls kept coming after the animation ended: %d", n)
	}
	s.InjectKey(tcell.KeyCtrlQ, 'q', tcell.ModCtrl)
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
}

func TestPaintTintKeepsTextAndForeground(t *testing.T) {
	s := newSim(t, 10, 2)
	theme := DefaultTheme()
	theme[ports.StyleKeyword] = tcell.StyleDefault.Foreground(tcell.ColorPurple).Bold(true)
	vm := ports.ViewModel{
		Runs:  []ports.Run{{X: 0, Y: 0, Text: "a世b", Style: ports.StyleKeyword}},
		Tints: []ports.Tint{{X: 0, Y: 0, Level: 2}, {X: 1, Y: 0, Level: 3}, {X: 3, Y: 0, Level: 1}, {X: 5, Y: 1, Level: 3}, {X: 7, Y: 0, Level: 4}},
	}
	paint(s, vm, theme)
	s.Show()
	cells, _, _ := s.GetContents()
	for x, want := range []rune{'a', '世'} {
		c := cells[x]
		fg, _, _ := c.Style.Decompose()
		if len(c.Runes) == 0 || c.Runes[0] != want {
			t.Errorf("cell %d rune %q, want %q", x, c.Runes, want)
		}
		if fg != tcell.ColorPurple {
			t.Errorf("cell %d lost its foreground: %v", x, fg)
		}
	}
	_, bg, _ := cells[0].Style.Decompose()
	if bg != tintRamp[1] {
		t.Errorf("tinted background %v, want %v", bg, tintRamp[1])
	}
	_, guide, _ := cells[7].Style.Decompose()
	if guide != tintRamp[3] {
		t.Errorf("guide tint background %v, want %v", guide, tintRamp[3])
	}
	_, plain, _ := cells[2].Style.Decompose()
	if plain == tintRamp[0] || plain == tintRamp[1] || plain == tintRamp[2] {
		t.Errorf("untinted cell has a ripple background: %v", plain)
	}
}

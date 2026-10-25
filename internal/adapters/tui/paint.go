package tui

import (
	"github.com/avero-it/sie/internal/core/ports"
	"github.com/gdamore/tcell/v2"
)

// Theme maps semantic styles to terminal styles.
type Theme map[ports.Style]tcell.Style

// DefaultTheme uses the 16 basic ANSI colors and attributes so it works on
// every terminal and follows the user's terminal palette.
func DefaultTheme() Theme {
	d := tcell.StyleDefault
	bar := d.Reverse(true)
	item := d.Background(tcell.ColorSilver).Foreground(tcell.ColorBlack)
	active := d.Background(tcell.ColorTeal).Foreground(tcell.ColorBlack)
	prompt := d.Background(tcell.ColorGray).Foreground(tcell.ColorWhite)
	return Theme{
		ports.StyleDefault:         d,
		ports.StyleSelection:       d.Reverse(true),
		ports.StyleSpecial:         d.Foreground(tcell.ColorOlive),
		ports.StyleSpecialSelected: d.Foreground(tcell.ColorOlive).Reverse(true),
		// Bright black + dim: a barely visible dot on dark and light palettes.
		ports.StyleWhitespace:         d.Foreground(tcell.ColorGray).Dim(true),
		ports.StyleWhitespaceSelected: d.Reverse(true).Dim(true),
		ports.StyleEmptyCell:          d.Foreground(tcell.ColorGray).Dim(true),
		ports.StyleCursorSecondary:    d.Background(tcell.ColorSilver).Foreground(tcell.ColorBlack),
		// Syntax: ANSI colors, so they follow the terminal palette.
		ports.StyleKeyword:          d.Foreground(tcell.ColorPurple).Bold(true),
		ports.StyleString:           d.Foreground(tcell.ColorGreen),
		ports.StyleNumber:           d.Foreground(tcell.ColorTeal),
		ports.StyleComment:          d.Foreground(tcell.ColorGray).Italic(true),
		ports.StyleConstant:         d.Foreground(tcell.ColorTeal).Bold(true),
		ports.StyleKey:              d.Foreground(tcell.ColorBlue),
		ports.StyleType:             d.Foreground(tcell.ColorOlive),
		ports.StyleBuiltin:          d.Foreground(tcell.ColorBlue),
		ports.StyleMeta:             d.Foreground(tcell.ColorFuchsia),
		ports.StyleGutter:           d.Foreground(tcell.ColorGray),
		ports.StyleRuler:            d.Foreground(tcell.ColorGray),
		ports.StyleWrapMark:         d.Foreground(tcell.ColorGray),
		ports.StyleGutterCurrent:    d.Bold(true),
		ports.StyleMenuBar:          bar,
		ports.StyleMenuTitleActive:  active,
		ports.StyleMenuItem:         item,
		ports.StyleMenuItemSelected: active,
		ports.StyleMenuHint:         item.Foreground(tcell.ColorNavy),
		// The prompt line is a gray bar (palette bright black) with white text.
		ports.StylePromptBar:   prompt,
		ports.StylePromptLabel: prompt.Bold(true),
		ports.StylePromptInput: prompt,
		ports.StylePromptIdle:  prompt.Foreground(tcell.ColorSilver),
		ports.StyleEcho:        prompt,
		ports.StyleEchoHint:    prompt.Foreground(tcell.ColorSilver),
		ports.StyleError:       prompt.Foreground(tcell.ColorRed).Bold(true),
		ports.StyleStatus:      bar,
		ports.StyleGuide:       d.Foreground(tcell.ColorGray),
		ports.StyleGuideMatch:  d.Reverse(true),
		ports.StyleMatch:       d.Underline(true).Bold(true),
		ports.StyleDivider:     d.Foreground(tcell.ColorGray).Reverse(true),
	}
}

// tintRamp holds the tint backgrounds: levels 1 to 3 for the ripple, 4 for
// the indent guide. RGB values are
// mapped by tcell to the nearest color on terminals with fewer colors.
var tintRamp = [...]tcell.Color{
	tcell.NewRGBColor(0x1f, 0x3a, 0x5f),
	tcell.NewRGBColor(0x2f, 0x5d, 0x8a),
	tcell.NewRGBColor(0x4a, 0x8b, 0xc2),
	tcell.NewRGBColor(0x44, 0x44, 0x44),
}

// tint repaints one cell with a lighter background, keeping its text,
// foreground and attributes.
func tint(s tcell.Screen, t ports.Tint) {
	if t.Level == 0 || int(t.Level) > len(tintRamp) {
		return
	}
	primary, combining, style, _ := s.GetContent(t.X, t.Y)
	s.SetContent(t.X, t.Y, primary, combining, style.Background(tintRamp[t.Level-1]))
}

// paint draws one frame: clear, runs in order, cursor, Show.
func paint(s tcell.Screen, vm ports.ViewModel, theme Theme) {
	s.Clear()
	for _, r := range vm.Runs {
		s.PutStrStyled(r.X, r.Y, r.Text, theme[r.Style])
	}
	for _, t := range vm.Tints {
		tint(s, t)
	}
	if vm.Cursor.Visible {
		s.ShowCursor(vm.Cursor.X, vm.Cursor.Y)
	} else {
		s.HideCursor()
	}
	s.Show()
}

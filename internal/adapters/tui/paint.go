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
		ports.StyleGutter:             d.Foreground(tcell.ColorGray),
		ports.StyleGutterCurrent:      d.Bold(true),
		ports.StyleMenuBar:            bar,
		ports.StyleMenuTitleActive:    active,
		ports.StyleMenuItem:           item,
		ports.StyleMenuItemSelected:   active,
		ports.StyleMenuHint:           item.Foreground(tcell.ColorNavy),
		// The prompt line is a gray bar (palette bright black) with white text.
		ports.StylePromptBar:   prompt,
		ports.StylePromptLabel: prompt.Bold(true),
		ports.StylePromptInput: prompt,
		ports.StylePromptIdle:  prompt.Foreground(tcell.ColorSilver),
		ports.StyleEcho:        prompt,
		ports.StyleEchoHint:    prompt.Foreground(tcell.ColorSilver),
		ports.StyleError:       prompt.Foreground(tcell.ColorRed).Bold(true),
		ports.StyleStatus:      bar,
	}
}

// paint draws one frame: clear, runs in order, cursor, Show.
func paint(s tcell.Screen, vm ports.ViewModel, theme Theme) {
	s.Clear()
	for _, r := range vm.Runs {
		s.PutStrStyled(r.X, r.Y, r.Text, theme[r.Style])
	}
	if vm.Cursor.Visible {
		s.ShowCursor(vm.Cursor.X, vm.Cursor.Y)
	} else {
		s.HideCursor()
	}
	s.Show()
}

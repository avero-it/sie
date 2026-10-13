package ports

// Style is a semantic style; the TUI adapter maps it to colors (the theme).
type Style uint8

const (
	StyleDefault Style = iota
	StyleSelection
	StyleSpecial // visible controls, invalid bytes, zero-width marks
	StyleSpecialSelected
	StyleWhitespace // faint marker for spaces
	StyleWhitespaceSelected
	StyleGutter
	StyleGutterCurrent
	StyleMenuBar
	StyleMenuTitleActive
	StyleMenuItem
	StyleMenuItemSelected
	StyleMenuHint
	StylePromptLabel
	StylePromptInput
	StyleEcho
	StyleEchoHint // shortcut shown right of the echo
	StyleError
	StyleStatus

	// StyleCount is the number of styles (not a style); themes must cover all.
	StyleCount
)

// Run is text drawn at a screen position. The painter lays it out grapheme
// by grapheme (uniseg widths) starting at X and clips at the screen edge.
type Run struct {
	X, Y  int
	Text  string
	Style Style
}

// Cursor is the terminal cursor position.
type Cursor struct {
	X, Y    int
	Visible bool
}

// ViewModel is everything the painter needs for one frame: it clears the
// screen to StyleDefault, draws Runs in order (later runs paint over earlier
// ones) and places the Cursor. All layout decisions are made by the core.
type ViewModel struct {
	Width, Height int
	Runs          []Run
	Cursor        Cursor
}

package ports

// Style is a semantic style; the TUI adapter maps it to colors (the theme).
type Style uint8

const (
	StyleDefault Style = iota
	StyleSelection
	StyleSpecial // visible controls, invalid bytes, zero-width marks
	StyleSpecialSelected
	StyleWhitespace // faint marker for spaces and tabs
	StyleWhitespaceSelected
	StyleEmptyCell       // faint marker for cells holding no character
	StyleCursorSecondary // the cell under a secondary cursor (multiple cursors)
	// Syntax classes (see TokenClass), in the same order.
	StyleKeyword
	StyleString
	StyleNumber
	StyleComment
	StyleConstant
	StyleKey
	StyleType
	StyleBuiltin
	StyleMeta
	StyleGutter
	StyleGutterCurrent
	StyleMenuBar
	StyleMenuTitleActive
	StyleMenuItem
	StyleMenuItemSelected
	StyleMenuHint
	StylePromptBar // background of the whole prompt line
	StylePromptLabel
	StylePromptInput
	StylePromptIdle // "write a command…" when nothing else is shown
	StyleEcho
	StyleEchoHint // shortcut shown right of the echo
	StyleError
	StyleStatus
	// StyleGuide is the gray bar of the indent guide; StyleGuideMatch the
	// first glyph of a line that starts at the guide column.
	StyleGuide
	StyleGuideMatch
	// StyleMatch marks the other matches of the text typed in the Find prompt.
	StyleMatch

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

// Tint lightens the background of one cell, keeping its text and foreground.
// Levels 1 to 3 are the ripple (faint to strong), 4 is the indent guide.
type Tint struct {
	X, Y  int
	Level uint8
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
	Tints         []Tint // cells whose background is tinted after the runs are drawn
	Cursor        Cursor
	Animate       bool // the next frame will differ with time alone: redraw soon
}

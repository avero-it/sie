package ports

// EditorService is the driving port: the TUI adapter feeds normalized input
// into it and paints the ViewModel it returns. All methods must be called
// from a single goroutine.
type EditorService interface {
	HandleKey(KeyEvent)
	HandleMouse(MouseEvent)
	HandlePaste(text string)
	Resize(width, height int)
	// Dispatch runs a command by ID, e.g. "file.open" with a path at startup.
	Dispatch(id string, args ...string) error
	View() ViewModel
	// Done reports whether the user asked to quit.
	Done() bool
	// Poll collects results of background work (e.g. highlighting). The
	// adapter calls it when woken up by such work.
	Poll()
}

// FileStore reads and writes whole files. A missing file is reported with an
// error wrapping io/fs.ErrNotExist.
type FileStore interface {
	Read(path string) ([]byte, error)
	// Write replaces the file atomically, keeping its permissions if it exists.
	Write(path string, data []byte) error
	Exists(path string) (bool, error)
}

// Clipboard is the system clipboard.
type Clipboard interface {
	Copy(text string) error
	Paste() (string, error)
}

// Keymap maps key events to command IDs.
type Keymap interface {
	// Command returns the command bound to k.
	Command(k KeyEvent) (id string, ok bool)
	// Binding returns the key shown as the shortcut of command id.
	Binding(id string) (KeyEvent, bool)
}

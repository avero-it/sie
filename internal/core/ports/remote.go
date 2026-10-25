package ports

import "time"

// Remote is the driving port of the control API (the local socket, see
// adapters/ipc). Like EditorService, all methods must be called from the
// editor's loop goroutine; the adapter queues each request there.
//
// Lines and columns are 1-based. A column counts grapheme clusters, like
// the status line does for text without tabs or wide characters. A range's
// End is exclusive, and a position one past the last grapheme of a line is
// the line end.
type Remote interface {
	// Info describes the instance; it needs no session.
	Info() RemoteInfo

	// Attach asks the user to let the client control the buffer. It returns
	// at once; reply runs later on the loop goroutine, with the new session
	// ID or a RemoteError (CodeDenied, CodeBusy).
	Attach(client string, reply func(session uint64, err error))
	// CancelAttach withdraws a pending Attach (the adapter's timeout).
	CancelAttach()
	// SessionActive reports whether session is the attached one. Every
	// method below is only called after it returned true.
	SessionActive(session uint64) bool
	Detach()

	State() RemoteState
	Text(startLine, endLine int) (RemoteText, error) // endLine inclusive; 0 means all
	Selections() []RemoteSel
	Goto(RemotePos) error
	Select(RemoteRange) error
	// Replace replaces r with text as one undo step; an empty range inserts.
	Replace(revision uint64, r RemoteRange, text RemoteTextIn) (newRevision uint64, err error)
	// ReplaceSelection replaces the primary selection (or inserts at the cursor).
	ReplaceSelection(revision uint64, text RemoteTextIn) (newRevision uint64, err error)
	Find(RemoteFind) (RemoteMatches, error)
	Validate() (RemoteValidation, error)
	// Run executes a prompt line such as "goto 42" without opening prompts.
	Run(line string) (note string, err error)
	Commands() []RemoteCommand
}

// RemotePos is a 1-based line and grapheme column.
type RemotePos struct {
	Line int `json:"line"`
	Col  int `json:"col"`
}

// RemoteRange is a half-open range of text.
type RemoteRange struct {
	Start RemotePos `json:"start"`
	End   RemotePos `json:"end"`
}

// RemoteInfo is what any local client may learn without attaching.
type RemoteInfo struct {
	Version      string    `json:"version"`
	Path         string    `json:"path"`
	Language     string    `json:"language"`
	Dirty        bool      `json:"dirty"`
	LastActivity time.Time `json:"last_activity"`
	Attached     bool      `json:"attached"`
	Client       string    `json:"client,omitempty"` // who is attached
}

// RemoteState is the buffer's state for the attached client.
type RemoteState struct {
	Path      string      `json:"path"`
	Language  string      `json:"language"`
	Encoding  string      `json:"encoding"`
	BOM       bool        `json:"bom"`
	LineEnd   string      `json:"line_ending"`
	Dirty     bool        `json:"dirty"`
	Lines     int         `json:"lines"`
	Bytes     int         `json:"bytes"`
	Large     bool        `json:"large"` // kept on disk: ranges are required
	Revision  uint64      `json:"revision"`
	Selection []RemoteSel `json:"selections"`
}

// RemoteText is text read from the buffer. Text that is not valid UTF-8 is
// returned in escaped form (Escaped true): an invalid byte becomes \xNN and
// a backslash becomes \\, so that the bytes round-trip.
type RemoteText struct {
	Text      string `json:"text"`
	Escaped   bool   `json:"escaped,omitempty"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Lines     int    `json:"lines"` // in the whole buffer
	Revision  uint64 `json:"revision"`
}

// RemoteTextIn is text sent to the buffer; Escaped says it uses the escaped form.
type RemoteTextIn struct {
	Text    string
	Escaped bool
}

// RemoteSel is one selection; an empty one is a cursor. Anchor is where it
// started, Head where the cursor is.
type RemoteSel struct {
	Anchor    RemotePos `json:"anchor"`
	Head      RemotePos `json:"head"`
	Primary   bool      `json:"primary,omitempty"`
	Text      string    `json:"text,omitempty"`
	Escaped   bool      `json:"escaped,omitempty"`
	Truncated bool      `json:"truncated,omitempty"`
}

// RemoteFind searches the buffer.
type RemoteFind struct {
	Pattern       string
	CaseSensitive bool
	Limit         int // <= 0 means 100
}

// RemoteMatches is the result of RemoteFind.
type RemoteMatches struct {
	Matches   []RemoteRange `json:"matches"`
	Truncated bool          `json:"truncated"`
	Revision  uint64        `json:"revision"`
}

// RemoteValidation is the outcome of validating the buffer.
type RemoteValidation struct {
	Valid    bool   `json:"valid"`
	Language string `json:"language"`
	Line     int    `json:"line,omitempty"`
	Col      int    `json:"col,omitempty"`
	Message  string `json:"message,omitempty"`
}

// RemoteCommand describes a prompt command a client may run.
type RemoteCommand struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Aliases  []string `json:"aliases,omitempty"`
	Args     []string `json:"args,omitempty"` // prompts of the arguments it takes
	Shortcut string   `json:"shortcut,omitempty"`
}

// RemoteError is an error with a stable code.
type RemoteError struct {
	Code    string
	Message string
}

func (e *RemoteError) Error() string { return e.Message }

// Stable error codes of the control API.
const (
	CodeNotAuthorized = "not_authorized"
	CodeDenied        = "denied"
	CodeBusy          = "busy"
	CodeStaleRevision = "stale_revision"
	CodeOutOfRange    = "out_of_range"
	CodeTooLarge      = "too_large"
	CodeInvalid       = "invalid_params"
	CodeFailed        = "failed"
)

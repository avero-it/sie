package ports

// TokenClass is the coarse syntax class the core colors by. Highlighter
// adapters map their own token types onto these.
type TokenClass uint8

const (
	TokenNone     TokenClass = iota
	TokenKeyword             // if, func, package, ...
	TokenString              // string and character literals, YAML plain scalars
	TokenNumber              // numeric literals
	TokenComment             // comments
	TokenConstant            // true, false, nil, null
	TokenKey                 // JSON/YAML mapping keys
	TokenType                // built-in types (int, error)
	TokenBuiltin             // built-in functions (len, append)
	TokenMeta                // YAML anchors, aliases, tags, document markers; preprocessor lines
)

// Token is a classified byte range [Start, End) of the highlighted text.
type Token struct {
	Start, End int
	Class      TokenClass
}

// HighlightJob asks for the tokens of one text.
type HighlightJob struct {
	ID       uint64 // opaque to the adapter; echoed in the result
	Language string // canonical name from Detect or Language
	Text     []byte // the job owns this copy
}

// HighlightResult is a finished job. Tokens are sorted and do not overlap.
type HighlightResult struct {
	ID     uint64
	Tokens []Token
	Err    error
}

// Highlighter colors source code. Highlight is asynchronous: it returns
// immediately, newer jobs supersede older pending ones, and the adapter
// signals completion out of band (the composition root wires a wake-up that
// makes the UI call EditorService.Poll, which collects the Result).
type Highlighter interface {
	// Detect returns the language for a file name, or "" for plain text.
	Detect(filename string) string
	// Language resolves a user-typed name or alias ("yml", "golang") to its
	// canonical name.
	Language(name string) (canonical string, ok bool)
	Highlight(job HighlightJob)
	// Result returns the newest finished result, if any arrived since the
	// last call.
	Result() (HighlightResult, bool)
}

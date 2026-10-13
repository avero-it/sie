// Package syntax implements ports.Highlighter with chroma. Highlighting
// runs on one background goroutine: Highlight only records the newest job,
// the worker tokenizes it, stores the result and calls the notify function
// (wired by cmd/sie to wake the terminal loop), and the editor collects it
// with Result on its own goroutine.
package syntax

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/avero-it/sie/internal/core/ports"
)

// Chroma is a background highlighter backed by chroma's lexers.
type Chroma struct {
	notify func()

	mu      sync.Mutex
	pending *ports.HighlightJob
	result  *ports.HighlightResult

	wake      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
}

var _ ports.Highlighter = (*Chroma)(nil)

// New starts the worker. notify (may be nil) is called from the worker
// goroutine each time a result is ready; it must not block.
func New(notify func()) *Chroma {
	c := &Chroma{notify: notify, wake: make(chan struct{}, 1), done: make(chan struct{})}
	go c.work()
	return c
}

// Close stops the worker.
func (c *Chroma) Close() {
	c.closeOnce.Do(func() { close(c.done) })
}

// plainFiles are names chroma would misdetect (go.mod matches AMPL's *.mod).
var plainFiles = map[string]bool{"go.mod": true, "go.sum": true, "go.work": true, "go.work.sum": true}

// Detect returns the language name for a file, or "" for plain text.
func (c *Chroma) Detect(filename string) string {
	base := filepath.Base(filename)
	if plainFiles[base] {
		return ""
	}
	return canonical(lexers.Match(base))
}

// Language resolves a name or alias ("golang", "yml") to its canonical name.
func (c *Chroma) Language(name string) (string, bool) {
	lang := canonical(lexers.Get(name))
	return lang, lang != ""
}

func canonical(l chroma.Lexer) string {
	if l == nil || l == lexers.Fallback {
		return ""
	}
	name := l.Config().Name
	if name == "plaintext" || name == "fallback" {
		return ""
	}
	return name
}

// Highlight queues a job, replacing any job not yet started.
func (c *Chroma) Highlight(job ports.HighlightJob) {
	c.mu.Lock()
	c.pending = &job
	c.mu.Unlock()
	select {
	case c.wake <- struct{}{}:
	default: // the worker is already signalled
	}
}

// Result returns the newest finished result since the last call.
func (c *Chroma) Result() (ports.HighlightResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.result == nil {
		return ports.HighlightResult{}, false
	}
	r := *c.result
	c.result = nil
	return r, true
}

func (c *Chroma) work() {
	for {
		select {
		case <-c.done:
			return
		case <-c.wake:
		}
		for {
			c.mu.Lock()
			job := c.pending
			c.pending = nil
			c.mu.Unlock()
			if job == nil {
				break
			}
			res := tokenize(*job)
			c.mu.Lock()
			c.result = &res
			c.mu.Unlock()
			if c.notify != nil {
				c.notify()
			}
		}
	}
}

// tokenize lexes a job's text into core tokens with byte offsets.
func tokenize(job ports.HighlightJob) (res ports.HighlightResult) {
	res.ID = job.ID
	defer func() {
		if r := recover(); r != nil {
			res.Tokens, res.Err = nil, fmt.Errorf("highlight %s: lexer panic: %v", job.Language, r)
		}
	}()
	lexer := lexers.Get(job.Language)
	if lexer == nil {
		res.Err = fmt.Errorf("highlight: unknown language %q", job.Language)
		return res
	}
	text := string(job.Text)
	// EnsureLF stays false: CRLF must not be rewritten, or offsets would drift.
	it, err := chroma.Coalesce(lexer).Tokenise(&chroma.TokeniseOptions{State: "root"}, text)
	if err != nil {
		res.Err = fmt.Errorf("highlight %s: %w", job.Language, err)
		return res
	}
	off := 0
	for tok := it(); tok != chroma.EOF; tok = it() {
		start, end := off, min(off+len(tok.Value), len(text))
		off += len(tok.Value)
		class := classify(tok.Type)
		if class == ports.TokenNone || start >= end {
			continue
		}
		if n := len(res.Tokens); n > 0 && res.Tokens[n-1].Class == class && res.Tokens[n-1].End == start {
			res.Tokens[n-1].End = end // merge neighbours, e.g. ` + raw + `
			continue
		}
		res.Tokens = append(res.Tokens, ports.Token{Start: start, End: end, Class: class})
	}
	// Some lexers append a final newline; anything shorter means the
	// lexer rewrote the text and the offsets cannot be trusted.
	if off < len(text) {
		return ports.HighlightResult{ID: job.ID, Err: fmt.Errorf("highlight %s: token offsets do not match the text", job.Language)}
	}
	return res
}

// classify maps chroma token types onto the core's token classes.
func classify(t chroma.TokenType) ports.TokenClass {
	switch {
	case t == chroma.KeywordConstant || t == chroma.NameConstant:
		return ports.TokenConstant
	case t == chroma.KeywordType:
		return ports.TokenType
	case t.InCategory(chroma.Keyword):
		return ports.TokenKeyword
	case t == chroma.NameTag:
		return ports.TokenKey // JSON and YAML keys
	case t == chroma.NameBuiltin || t == chroma.NameBuiltinPseudo:
		return ports.TokenBuiltin
	case t == chroma.CommentPreproc || t == chroma.CommentPreprocFile || t == chroma.NameNamespace ||
		t == chroma.NameLabel || t == chroma.NameDecorator:
		return ports.TokenMeta // YAML anchors/aliases/tags and "---", #include, @decorators
	case t.InCategory(chroma.Comment):
		return ports.TokenComment
	case t.InSubCategory(chroma.LiteralNumber) || t == chroma.LiteralDate:
		return ports.TokenNumber
	case t.InSubCategory(chroma.LiteralString) || t == chroma.Literal:
		return ports.TokenString // Literal: YAML plain scalars
	default:
		return ports.TokenNone
	}
}

// Languages lists all known language names (for documentation and tests).
func Languages() []string {
	names := lexers.Names(false)
	out := names[:0]
	for _, n := range names {
		if !strings.EqualFold(n, "plaintext") && !strings.EqualFold(n, "fallback") {
			out = append(out, n)
		}
	}
	return out
}

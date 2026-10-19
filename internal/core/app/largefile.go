package app

import (
	"errors"
	"fmt"
	"io"

	"github.com/avero-it/sie/internal/core/domain/editor"
	"github.com/avero-it/sie/internal/core/ports"
)

const (
	defaultLargeFileSize = 32 << 20 // 32 MB
	// maxCopySize is the largest selection Copy and Cut put on the clipboard.
	maxCopySize = 64 << 20
	// findPrefillMax is the longest selection the find prompt offers as text.
	findPrefillMax = 4096
)

// largeFileSize is the size above which a file is not loaded: it stays on
// disk, is read in ranges, and features that need the whole text (syntax
// colors, validation) are off.
func (e *Editor) largeFileSize() int {
	if e.cfg.LargeFileSize > 0 {
		return e.cfg.LargeFileSize
	}
	return defaultLargeFileSize
}

func (e *Editor) docOptions() editor.Options {
	return editor.Options{TabWidth: e.cfg.TabWidth, DefaultEOL: e.cfg.DefaultEOL}
}

// openDoc makes a document from f. Small files are read into memory and
// closed (the returned file is nil); large ones stay open and back the
// document, and the caller owns the returned file.
func (e *Editor) openDoc(f ports.File) (*editor.Document, ports.File, error) {
	if f.Len() > e.largeFileSize() {
		d, err := editor.NewDocumentFromSource(f, e.docOptions())
		if err != nil {
			_ = f.Close()
			return nil, nil, err
		}
		return d, f, nil
	}
	defer f.Close()
	data := make([]byte, f.Len())
	if n, err := f.ReadAt(data, 0); n < len(data) {
		if err == nil || errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return nil, nil, fmt.Errorf("read: %w", err)
	}
	return e.newDoc(data), nil, nil
}

// tooLarge reports whether the document is too large for features that
// need its whole text.
func (e *Editor) tooLarge() bool { return e.doc.Len() > e.largeFileSize() }

// checkFile runs after every input event. When the file behind a large
// document can no longer be read (it changed on disk, or vanished), the text
// on screen is not trustworthy and cannot be saved, so it asks to reload or
// quit; any other key asks again.
func (e *Editor) checkFile() {
	if e.doc.TakeUndoLost() {
		e.info("undo history cleared: that change was too large to undo")
	}
	if e.doc.Err() == nil {
		return
	}
	const question = "File changed on disk (or unreadable)"
	if e.focus == focusPrompt && e.prompt.mode == modeChoice && e.prompt.question == question {
		return
	}
	e.choose(question,
		choice{key: 'r', label: "[r]eload", run: func() error { return e.load(e.path) }},
		choice{key: 'q', label: "[q]uit", run: func() error {
			e.done = true
			return nil
		}},
	)
}

// reopen points a document that was just saved at the new file, so that
// large documents keep living on disk and a pasted giant one lets go of its
// memory. Failing is harmless: the file is saved, and if the old view stops
// being valid, checkFile offers a reload.
func (e *Editor) reopen(path string) {
	if e.file == nil && !e.tooLarge() {
		return
	}
	f, err := e.cfg.Files.Open(path)
	if err != nil {
		e.note("saved, but could not reopen the file: " + err.Error())
		return
	}
	if err := e.doc.Rebase(f); err != nil {
		_ = f.Close()
		e.note("saved, but could not reopen the file: " + err.Error())
		return
	}
	if e.file != nil {
		_ = e.file.Close()
	}
	e.file = f
}

// formatSize writes a byte count for messages: "1.5 MB".
func formatSize(n int) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := unit, 0
	for m := n / unit; m >= unit && exp < 3; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGT"[exp])
}

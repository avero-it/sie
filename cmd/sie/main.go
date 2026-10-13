// Command sie is a modeless terminal text editor.
//
// This is the composition root: it is the only package that knows the
// concrete adapters and wires them to the core's ports.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/avero-it/sie/internal/adapters/clipboard"
	fsadapter "github.com/avero-it/sie/internal/adapters/fs"
	"github.com/avero-it/sie/internal/adapters/keymap"
	"github.com/avero-it/sie/internal/adapters/tui"
	"github.com/avero-it/sie/internal/core/app"
	"github.com/avero-it/sie/internal/core/domain/buffer"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("sie", flag.ContinueOnError)
	flags.SetOutput(stderr)
	showVersion := flags.Bool("version", false, "print version and exit")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: sie [-version] [file]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, "sie", version)
		return 0
	}
	if flags.NArg() > 1 {
		flags.Usage()
		return 2
	}

	term, err := tui.New()
	if err != nil {
		fmt.Fprintln(stderr, "sie:", err)
		return 1
	}
	editor, err := app.New(app.Config{
		Files:      fsadapter.Store{},
		Clipboard:  clipboard.New(term.SetClipboard),
		Keymap:     keymap.Default(),
		TabWidth:   4,
		DefaultEOL: buffer.LF,
		Version:    version,
	})
	if err != nil {
		fmt.Fprintln(stderr, "sie:", err)
		return 1
	}
	if flags.NArg() == 1 {
		_ = editor.Dispatch("file.open", flags.Arg(0)) // errors show in the prompt line
	}
	if err := term.Run(editor); err != nil {
		fmt.Fprintln(stderr, "sie:", err)
		return 1
	}
	return 0
}

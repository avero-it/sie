// Command sie is a modeless terminal text editor.
//
// This is the composition root: it is the only package that knows the
// concrete adapters and wires them to the core's ports.
package main

import (
	"flag"
	"fmt"
	"os"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("sie", version)
		return
	}
	fmt.Fprintln(os.Stderr, "sie: editor not wired yet (core under construction)")
	os.Exit(1)
}

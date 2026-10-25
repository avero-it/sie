//go:build windows

package main

import (
	"io"

	"github.com/avero-it/sie/internal/adapters/tui"
	"github.com/avero-it/sie/internal/core/ports"
)

// startControl does nothing on Windows: the control socket is Unix only
// for now (see control_unix.go), so the `mcp` setting has no effect there.
func startControl(ports.Remote, *tui.Terminal, io.Writer) (stop func()) { return func() {} }

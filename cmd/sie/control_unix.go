//go:build !windows

package main

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/avero-it/sie/internal/adapters/ipc"
	"github.com/avero-it/sie/internal/adapters/tui"
	"github.com/avero-it/sie/internal/core/ports"
)

// startControl opens the control socket of this instance (see adapters/ipc)
// and returns the function that closes it. A socket that cannot be opened
// is reported on warn and never stops the editor.
func startControl(remote ports.Remote, term *tui.Terminal, warn io.Writer) (stop func()) {
	dir, err := ipc.DefaultDir()
	if err == nil {
		var srv *ipc.Server
		srv, err = ipc.Listen(ipc.Options{Dir: dir, Remote: remote, Submit: term.Submit})
		if err == nil {
			go srv.Serve()
			// A killed process leaves its socket behind (others sweep it up
			// later); on the common termination signals, remove it, then let
			// the signal do what it would have done.
			sigs := make(chan os.Signal, 1)
			signal.Notify(sigs, syscall.SIGTERM, syscall.SIGHUP)
			go func() {
				sig, ok := <-sigs
				if !ok {
					return
				}
				_ = srv.Close()
				signal.Stop(sigs)
				if p, err := os.FindProcess(os.Getpid()); err == nil {
					_ = p.Signal(sig)
				}
			}()
			return func() {
				signal.Stop(sigs)
				close(sigs)
				_ = srv.Close()
			}
		}
	}
	fmt.Fprintf(warn, "sie: MCP socket disabled: %v\n", err)
	return func() {}
}

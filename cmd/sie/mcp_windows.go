//go:build windows

package main

import (
	"errors"

	"github.com/spf13/cobra"
)

// newMCPCmd explains itself on Windows, where the editors have no control
// socket yet (see mcp_unix.go).
func newMCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Not available on Windows yet",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return errors.New("sie mcp is not available on Windows yet")
		},
	}
}

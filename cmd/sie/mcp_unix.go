//go:build !windows

package main

import (
	"github.com/avero-it/sie/internal/adapters/ipc"
	"github.com/avero-it/sie/internal/buildinfo"
	"github.com/spf13/cobra"
)

// newMCPCmd is the bridge an agent starts: MCP over stdio, forwarded to the
// running sie editors' sockets. It prints nothing but protocol messages.
func newMCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Serve the running editors to an agent over MCP (stdio)",
		Long: "mcp speaks the Model Context Protocol on stdin and stdout and forwards the agent's\n" +
			"tool calls to the sie editors running as the same user, so the agent and the user\n" +
			"work on the same buffer. The user confirms each attach in the editor.\n\n" +
			"Register it once:  claude mcp add sie -- sie mcp",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := ipc.DefaultDir()
			if err != nil {
				return err
			}
			b := &ipc.Bridge{Dir: dir, Version: buildinfo.Get().Display()}
			return b.Run(cmd.InOrStdin(), cmd.OutOrStdout())
		},
	}
}

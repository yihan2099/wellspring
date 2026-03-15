package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	mcpserver "github.com/wellspring-cli/wellspring/mcp"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start as MCP server (stdio transport)",
	Long: `Start Wellspring as an MCP (Model Context Protocol) server.

This exposes all available adapters as MCP tools that LLMs and AI agents
can call directly. Uses stdio transport.

Examples:
  wsp serve                              Start MCP server
  wsp serve --debug                      Start with debug logging`,
	RunE: runServe,
}

func runServe(cmd *cobra.Command, args []string) error {
	rc := getRunContext()

	if !rc.Quiet {
		fmt.Fprintln(os.Stderr, "Starting Wellspring MCP server...")
		fmt.Fprintf(os.Stderr, "Registered %d adapters\n", len(rc.Reg.All()))
	}

	srv := mcpserver.NewServer(rc.Reg, Version, rc.Limiter)

	if rc.Debug {
		fmt.Fprintln(os.Stderr, "[debug] MCP server listening on stdio")
	}

	return srv.ServeStdio()
}

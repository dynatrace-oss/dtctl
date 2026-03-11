package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/commands"
)

var briefMode bool

// commandsCmd outputs a machine-readable listing of all dtctl commands.
var commandsCmd = &cobra.Command{
	Use:   "commands [resource-or-verb]",
	Short: "List all commands as structured JSON for AI agents",
	Long: `Output a machine-readable catalog of dtctl's command tree.

The listing includes all verbs, resources, flags, mutating status, safety
operations, and resource aliases. It is designed for automated consumption
by AI coding agents and MCP servers.

Examples:
  # Full JSON listing
  dtctl commands

  # Brief listing (reduced token count)
  dtctl commands --brief

  # Commands for a specific resource
  dtctl commands workflows
  dtctl commands wf           # alias

  # Commands for a specific verb
  dtctl commands get

  # YAML output
  dtctl commands -o yaml

  # LLM-optimized markdown guide
  dtctl commands howto`,
	Args: cobra.MaximumNArgs(1),
	RunE: runCommandsListing,
}

// howtoCmd outputs an LLM-optimized markdown reference guide.
var howtoCmd = &cobra.Command{
	Use:   "howto",
	Short: "Output an LLM-optimized usage guide in markdown",
	Long: `Output a markdown document optimized for LLM context windows.

The guide includes common workflows, safety levels, time formats, output
formats, patterns, and antipatterns. It is designed to be injected into
an AI agent's system prompt or context.

Examples:
  dtctl commands howto
  dtctl commands howto | pbcopy    # Copy to clipboard on macOS`,
	RunE: runHowto,
}

func runCommandsListing(cmd *cobra.Command, args []string) error {
	listing := commands.Build(rootCmd)

	// Apply resource/verb filter if a positional arg is provided
	if len(args) > 0 {
		filtered, ok := commands.FilterByResource(listing, args[0])
		if !ok {
			return fmt.Errorf("no commands found for %q", args[0])
		}
		listing = filtered
	}

	// Apply brief mode (returns a new copy, original is unchanged)
	output := listing
	if briefMode {
		output = commands.NewBrief(listing)
	}

	return commands.WriteTo(os.Stdout, output, outputFormat)
}

func runHowto(cmd *cobra.Command, args []string) error {
	listing := commands.Build(rootCmd)
	return commands.GenerateHowto(os.Stdout, listing)
}

func init() {
	commandsCmd.Flags().BoolVar(&briefMode, "brief", false, "minimal output (reduced token count for AI agents)")
	commandsCmd.AddCommand(howtoCmd)
	rootCmd.AddCommand(commandsCmd)
}

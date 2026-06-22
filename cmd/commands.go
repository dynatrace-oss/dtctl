package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/dynatrace-oss/dtctl/pkg/commands"
)

var (
	briefMode          bool
	requiredScopesMode bool
)

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

  # Minimal token scope set for a filtered command set
  dtctl commands wf --required-scopes
  dtctl commands --required-scopes        # union across all commands

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

	// --required-scopes: emit the minimal scope union for the requested set.
	if requiredScopesMode {
		if len(args) > 0 {
			// A verb filter unions that verb's scopes across its resources; a
			// resource filter narrows to just that resource's scopes.
			if _, isVerb := listing.Verbs[args[0]]; isVerb {
				filtered, ok := commands.FilterByResource(listing, args[0])
				if !ok {
					return fmt.Errorf("no commands found for %q", args[0])
				}
				return writeRequiredScopes(commands.RequiredScopesUnion(filtered), outputFormat)
			}
			if _, ok := commands.FilterByResource(listing, args[0]); !ok {
				return fmt.Errorf("no commands found for %q", args[0])
			}
			return writeRequiredScopes(commands.RequiredScopesForResource(listing, args[0]), outputFormat)
		}
		return writeRequiredScopes(commands.RequiredScopesUnion(listing), outputFormat)
	}

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

// writeRequiredScopes prints a scope union in the requested output format.
func writeRequiredScopes(scopes []string, format string) error {
	switch format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string][]string{"required_scopes": scopes})
	case "yaml", "yml":
		enc := yaml.NewEncoder(os.Stdout)
		enc.SetIndent(2)
		if err := enc.Encode(map[string][]string{"required_scopes": scopes}); err != nil {
			return err
		}
		return enc.Close()
	default:
		if len(scopes) > 0 {
			fmt.Println(strings.Join(scopes, "\n"))
		}
		return nil
	}
}

func runHowto(cmd *cobra.Command, args []string) error {
	listing := commands.Build(rootCmd)
	return commands.GenerateHowto(os.Stdout, listing)
}

func init() {
	commandsCmd.Flags().BoolVar(&briefMode, "brief", false, "minimal output (reduced token count for AI agents)")
	commandsCmd.Flags().BoolVar(&requiredScopesMode, "required-scopes", false, "print the minimal token scope union for the (optionally filtered) command set")
	commandsCmd.AddCommand(howtoCmd)
	rootCmd.AddCommand(commandsCmd)
}

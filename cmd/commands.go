package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/dynatrace-oss/dtctl/pkg/commands"
	"github.com/dynatrace-oss/dtctl/pkg/stability"
)

var (
	briefMode          bool
	fullMode           bool
	requiredScopesMode bool
)

// commandsCmd outputs a machine-readable listing of all dtctl commands.
var commandsCmd = &cobra.Command{
	Use:   "commands [resource-or-verb]",
	Short: "List commands as a structured, machine-readable catalog for AI agents",
	Long: `Output a machine-readable catalog of dtctl's command tree.

By default this prints a minimal overview — just verbs, their resources, and
nested subcommands — in TOON, the most compact format. Most verb-noun commands
are self-explanatory, so this is usually all an AI agent needs to orient. Use
--brief for mutating status, access levels, flag types, and required scopes, or
--full for the complete catalog (descriptions, flag defaults, global flags,
time formats, and materialized per-resource scopes).

Examples:
  # Minimal overview (default: verbs, resources, subcommands as TOON)
  dtctl commands

  # Brief listing (adds mutating status, access, flag types, scopes)
  dtctl commands --brief

  # Full catalog (everything)
  dtctl commands --full

  # Commands for a specific resource
  dtctl commands workflows
  dtctl commands wf           # alias

  # Commands for a specific verb
  dtctl commands get

  # Minimal token scope set for a filtered command set
  dtctl commands wf --required-scopes
  dtctl commands --required-scopes        # union across all commands

  # JSON or YAML output (default format is TOON)
  dtctl commands -o json
  dtctl commands --full -o yaml

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

	format := commandsFormat(cmd)

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
				return writeRequiredScopes(commands.RequiredScopesUnion(filtered), format)
			}
			if _, ok := commands.FilterByResource(listing, args[0]); !ok {
				return fmt.Errorf("no commands found for %q", args[0])
			}
			return writeRequiredScopes(commands.RequiredScopesForResource(listing, args[0]), format)
		}
		return writeRequiredScopes(commands.RequiredScopesUnion(listing), format)
	}

	// Apply resource/verb filter if a positional arg is provided
	if len(args) > 0 {
		filtered, ok := commands.FilterByResource(listing, args[0])
		if !ok {
			return fmt.Errorf("no commands found for %q", args[0])
		}
		listing = filtered
	}

	// Advertise the two active constraints (profile + safety level) on the base
	// listing before the tier transform, so an agent sees the reduced surface and
	// its permission envelope regardless of detail level.
	annotateListingContext(listing)

	// Select detail level (all transforms leave the original listing intact):
	//   default → minimal overview, --brief → brief, --full → full.
	switch {
	case fullMode:
		return commands.WriteValue(os.Stdout, listing, format)
	case briefMode:
		return commands.WriteValue(os.Stdout, commands.NewBrief(listing), format)
	default:
		return commands.WriteValue(os.Stdout, commands.NewMinimal(listing), format)
	}
}

// commandsFormat returns the output format for the commands catalog. It defaults
// to TOON — the most compact serialization — unless the user explicitly set -o.
func commandsFormat(cmd *cobra.Command) string {
	if cmd.Flags().Changed("output") {
		return outputFormat
	}
	return "toon"
}

// annotateListingContext fills in the three active constraints: the command
// profile (which commands exist here), the effective safety level (what they
// may do), and the stability floor with its exceptions (what is promised about
// their shape). Best-effort: any config error leaves the fields empty, which
// renders as the full surface at default safety and the default floor.
//
// All three are surfaced together because a catalog that showed only one would
// let an agent misread the other two. A command missing from the tree means
// something different under a profile than under a floor, and the fix differs.
func annotateListingContext(l *commands.Listing) {
	cfg, err := LoadConfig()
	if err != nil {
		return
	}
	if p, err := cfg.ResolveProfile(); err == nil && p != nil {
		l.Profile = p.Name
	}
	if _, err := cfg.CurrentContextObj(); err == nil {
		// Config-level, not Context-level: a local .dtctl.yaml has its declared
		// level clamped, and the catalog must advertise what is enforced.
		l.SafetyLevel = cfg.GetEffectiveSafetyLevel().String()
	}
	if floor, err := cfg.ResolveMinStability(); err == nil {
		l.MinStability = floor.String()
	}
	// Rendered from the parsed form rather than echoed from config, so the
	// catalog cannot advertise an exception that the floor stage rejected.
	if parsed, err := stability.ParseExceptions(cfg.StabilityExceptions()); err == nil {
		l.StabilityExceptions = stability.Policy{Exceptions: parsed}.ExceptionStrings()
	}
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
	commandsCmd.Flags().BoolVar(&briefMode, "brief", false, "add mutating status, access levels, flag types, and required scopes to the overview")
	commandsCmd.Flags().BoolVar(&fullMode, "full", false, "emit the complete catalog (descriptions, flag defaults, global flags, time formats, per-resource scopes)")
	commandsCmd.Flags().BoolVar(&requiredScopesMode, "required-scopes", false, "print the minimal token scope union for the (optionally filtered) command set")
	commandsCmd.MarkFlagsMutuallyExclusive("brief", "full")
	commandsCmd.AddCommand(howtoCmd)
	rootCmd.AddCommand(commandsCmd)
}

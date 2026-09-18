package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/appengine"
	"github.com/dynatrace-oss/dtctl/pkg/stability"
)

// describeIntentCmd describes an app intent
var describeIntentCmd = &cobra.Command{
	Use:     "intent <app-id>/<intent-id>",
	Aliases: []string{},
	Short:   "Describe an App Engine intent",
	Long: `Show detailed information about an app intent.

Intents enable inter-app communication by defining entry points
that apps expose for opening resources with contextual data.

Examples:
  # Describe an intent
  dtctl describe intent dynatrace.distributedtracing/view-trace

  # Output as JSON
  dtctl describe intent dynatrace.distributedtracing/view-trace -o json

  # Output as YAML
  dtctl describe intent dynatrace.logs/view-log-entry -o yaml
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		_, c, printer, err := Setup()
		if err != nil {
			return err
		}

		handler := appengine.NewIntentHandler(c)

		// Get intent details
		intent, err := handler.GetIntent(args[0])
		if err != nil {
			return err
		}

		// For table output, show detailed information
		if outputFormat == "table" {
			const w = 14
			output.DescribeKV("Intent:", w, "%s", intent.IntentID)
			output.DescribeKV("Full Name:", w, "%s", intent.FullName)
			if intent.Description != "" {
				output.DescribeKV("Description:", w, "%s", intent.Description)
			}
			output.DescribeKV("App:", w, "%s (%s)", intent.AppName, intent.AppID)

			// Print properties
			if len(intent.Properties) > 0 {
				fmt.Println()
				output.DescribeSection("Properties:")
				for propName, prop := range intent.Properties {
					required := ""
					if prop.Required {
						required = " (required)"
					}
					fmt.Printf("  - %s: %s%s\n", propName, prop.Type, required)
					if prop.Format != "" {
						fmt.Printf("    Format: %s\n", prop.Format)
					}
					if prop.Description != "" {
						fmt.Printf("    Description: %s\n", prop.Description)
					}
				}
			}

			// Show required properties summary
			if len(intent.RequiredProps) > 0 {
				fmt.Println()
				output.DescribeKV("Required:", w, "%s", strings.Join(intent.RequiredProps, ", "))
			}

			// Show usage example
			fmt.Println()
			output.DescribeSection("Usage:")
			fmt.Printf("  dtctl open intent %s --data <key>=<value>\n", intent.FullName)
			fmt.Printf("  dtctl find intents --data <key>=<value>\n")

			return nil
		}

		// For other formats, use standard printer
		return printer.Print(intent)
	},
}

func init() {
	// No flags for this command
}

// Declared stable: the invocation and output contract of this command is
// additive-only. Stable is never implied -- see AGENTS.md "Stability Tiers".
func init() {
	stability.MarkStable(describeIntentCmd)
}

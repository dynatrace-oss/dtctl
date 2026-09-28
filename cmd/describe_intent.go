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
			if intent.Name != "" && intent.Name != intent.Description {
				output.DescribeKV("Name:", w, "%s", intent.Name)
			}
			if intent.Description != "" {
				output.DescribeKV("Description:", w, "%s", intent.Description)
			}
			output.DescribeKV("App:", w, "%s (%s)", intent.AppName, intent.AppID)
			if intent.Deprecated {
				if intent.DeprecationMessage != "" {
					output.DescribeKV("Deprecated:", w, "yes (%s)", intent.DeprecationMessage)
				} else {
					output.DescribeKV("Deprecated:", w, "yes")
				}
			}

			// Print properties, in name order so repeated runs match.
			if len(intent.Properties) > 0 {
				fmt.Println()
				output.DescribeSection("Properties:")
				for _, propName := range intent.SortedPropertyNames() {
					describeIntentProperty(propName, intent.Properties[propName])
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

// describeIntentProperty prints one property of `describe intent`'s table
// view: its declared type (never a guessed one), and the payload keys that
// satisfy it when those are not simply the property name.
func describeIntentProperty(propName string, prop appengine.IntentProperty) {
	required := ""
	if prop.Required {
		required = " (required)"
	}
	propType := prop.Type
	if propType == "" {
		propType = "(type not declared)"
	}
	fmt.Printf("  - %s: %s%s\n", propName, propType, required)
	if prop.Format != "" {
		fmt.Printf("    Format: %s\n", prop.Format)
	}

	keyIsName := len(prop.AcceptedKeys) == 1 && prop.AcceptedKeys[0] == propName
	switch {
	case keyIsName:
		if pattern := prop.KeyPattern(propName, propName); pattern != "" {
			fmt.Printf("    Pattern: %s\n", pattern)
		}
	case len(prop.AcceptedKeys) == 0:
		fmt.Printf("    Accepted keys: none (empty schemas; no payload can satisfy it)\n")
	default:
		fmt.Printf("    Accepted keys:\n")
		for _, key := range prop.AcceptedKeys {
			if pattern := prop.KeyPattern(propName, key); pattern != "" {
				fmt.Printf("      - %s  (pattern: %s)\n", key, pattern)
			} else {
				fmt.Printf("      - %s\n", key)
			}
		}
	}

	if prop.Description != "" {
		fmt.Printf("    Description: %s\n", prop.Description)
	}
}

func init() {
	// No flags for this command
}

// Declared stable: the invocation and output contract of this command is
// additive-only. Stable is never implied -- see AGENTS.md "Stability Tiers".
func init() {
	stability.MarkStable(describeIntentCmd)
}

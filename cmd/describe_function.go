package cmd

import (
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/resources/appengine"
	"github.com/spf13/cobra"
)

// describeFunctionCmd describes an app function
var describeFunctionCmd = &cobra.Command{
	Use:     "function <app-id>/<function-name>",
	Aliases: []string{"fn", "func"},
	Short:   "Describe an App Engine function",
	Long: `Show detailed information about an app function.

Functions are serverless backend functions exposed by installed apps.
Each function can be invoked using 'dtctl exec function'.

Examples:
  # Describe a function
  dtctl describe function dynatrace.automations/execute-dql-query

  # Output as JSON
  dtctl describe function dynatrace.abuseipdb/check-ip -o json

  # Output as YAML
  dtctl describe function dynatrace.slack/slack-send-message -o yaml
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := appengine.NewHandler(c)

		// Get function details
		function, err := handler.GetFunction(args[0])
		if err != nil {
			return err
		}

		// For table output, show detailed information
		if outputFormat == "table" {
			fmt.Printf("Function:     %s\n", function.FunctionName)
			fmt.Printf("Full Name:    %s\n", function.FullName)
			if function.Title != "" {
				fmt.Printf("Title:        %s\n", function.Title)
			}
			if function.Description != "" {
				fmt.Printf("Description:  %s\n", function.Description)
			}
			fmt.Printf("App:          %s (%s)\n", function.AppName, function.AppID)
			fmt.Printf("Resumable:    %t\n", function.Resumable)
			if function.Stateful {
				fmt.Printf("Stateful:     %t\n", function.Stateful)
			}
			fmt.Printf("\nUsage:\n")
			fmt.Printf("  dtctl exec function %s\n", function.FullName)
			if function.Resumable {
				fmt.Printf("  dtctl exec function %s --defer  # For async execution\n", function.FullName)
			}
			return nil
		}

		// For other formats, use standard printer
		printer := NewPrinter()
		return printer.Print(function)
	},
}

func init() {
	// No flags for this command
}

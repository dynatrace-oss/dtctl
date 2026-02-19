package cmd

import (
	"github.com/dynatrace-oss/dtctl/pkg/resources/appengine"
	"github.com/spf13/cobra"
)

var (
	getFunctionsAppFilter string
)

// getFunctionsCmd retrieves App Engine functions
var getFunctionsCmd = &cobra.Command{
	Use:     "functions [app-id/function-name]",
	Aliases: []string{"function", "fn", "func"},
	Short:   "Get App Engine functions",
	Long: `Get app functions from installed apps.

Functions are serverless backend functions exposed by installed apps.
Each function can be invoked using 'dtctl exec function'.

Examples:
  # List all functions across all apps
  dtctl get functions

  # List functions for a specific app
  dtctl get functions --app dynatrace.automations

  # Get a specific function
  dtctl get function dynatrace.automations/execute-dql-query

  # Output as JSON
  dtctl get functions -o json

  # Wide output (shows resumable status)
  dtctl get functions -o wide
`,
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
		printer := NewPrinter()

		// Get specific function if app-id/function-name provided
		if len(args) > 0 {
			function, err := handler.GetFunction(args[0])
			if err != nil {
				return err
			}
			return printer.Print(function)
		}

		// List functions
		functions, err := handler.ListFunctions(getFunctionsAppFilter)
		if err != nil {
			return err
		}

		return printer.PrintList(functions)
	},
}

func init() {
	getFunctionsCmd.Flags().StringVar(&getFunctionsAppFilter, "app", "", "filter by app ID")
}

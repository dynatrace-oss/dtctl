package cmd

import (
	"github.com/spf13/cobra"
)

// openCmd represents the open command
var openCmd = &cobra.Command{
	Use:   "open",
	Short: "Open resources in browser",
	Long: `Open Dynatrace resources in your default web browser.

Constructs the appropriate URL for the resource and opens it in the system
browser. Useful for quickly navigating to a resource's UI from the terminal.

Available resources:
  intent                  Generate and open an intent URL for an app`,
	Example: `  # Open an intent URL in the browser
  dtctl open intent <app-id>/<intent-id>

  # Open an intent with payload data
  dtctl open intent <app-id>/<intent-id> --data '{"key": "value"}'

  # Print the URL without opening a browser
  dtctl open intent <app-id>/<intent-id> --url-only`,
	RunE: requireSubcommand,
}

func init() {
	rootCmd.AddCommand(openCmd)
}

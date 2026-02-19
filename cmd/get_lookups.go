package cmd

import (
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/prompt"
	"github.com/dynatrace-oss/dtctl/pkg/resources/lookup"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/spf13/cobra"
)

// getLookupsCmd retrieves lookup tables
var getLookupsCmd = &cobra.Command{
	Use:     "lookups [path]",
	Aliases: []string{"lookup", "lkup", "lu"},
	Short:   "Get lookup tables",
	Long: `Get lookup tables from Grail Resource Store.

Lookup tables are tabular files stored in Grail that can be loaded and
joined with observability data in DQL queries for data enrichment.

Output formats:
  - List view (no path): Shows lookup table metadata (path, size, records, etc.)
  - Table output (-o table): Shows the actual lookup table data as a table
  - YAML output (-o yaml): Shows both metadata and full data
  - CSV/JSON output: Shows the lookup table data only

Examples:
  # List all lookup tables (shows metadata)
  dtctl get lookups

  # View lookup table data as a table (default)
  dtctl get lookup /lookups/grail/pm/error_codes

  # View with metadata included
  dtctl get lookup /lookups/grail/pm/error_codes -o yaml

  # Export lookup data as CSV
  dtctl get lookup /lookups/grail/pm/error_codes -o csv > error_codes.csv

  # Export lookup data as JSON
  dtctl get lookup /lookups/grail/pm/error_codes -o json

  # List all lookups with additional columns
  dtctl get lookups -o wide
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

		handler := lookup.NewHandler(c)
		printer := NewPrinter()

		// Get specific lookup if path provided
		if len(args) > 0 {
			// For table output, show the actual lookup table data (not metadata)
			if outputFormat == "table" || outputFormat == "wide" {
				data, err := handler.GetData(args[0], 0)
				if err != nil {
					return err
				}
				return printer.PrintList(data)
			}

			// For CSV/JSON output, return full data
			if outputFormat == "csv" || outputFormat == "json" {
				fullData, err := handler.GetData(args[0], 0)
				if err != nil {
					return err
				}
				return printer.PrintList(fullData)
			}

			// For YAML output, return full structure (metadata + data)
			lookupData, err := handler.GetWithData(args[0], 0)
			if err != nil {
				return err
			}
			return printer.Print(lookupData)
		}

		// List all lookups
		list, err := handler.List()
		if err != nil {
			return err
		}

		return printer.PrintList(list)
	},
}

// deleteLookupCmd deletes a lookup table
var deleteLookupCmd = &cobra.Command{
	Use:     "lookup <path>",
	Aliases: []string{"lookups", "lkup", "lu"},
	Short:   "Delete a lookup table",
	Long: `Delete a lookup table from Grail Resource Store.

ATTENTION: This operation is irreversible and will permanently delete the lookup table.

Examples:
  # Delete a lookup table
  dtctl delete lookup /lookups/grail/pm/error_codes

  # Delete without confirmation
  dtctl delete lookup /lookups/grail/pm/error_codes -y
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		// Safety check
		checker, err := NewSafetyChecker(cfg)
		if err != nil {
			return err
		}
		if err := checker.CheckError(safety.OperationDelete, safety.OwnershipUnknown); err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := lookup.NewHandler(c)

		// Get lookup for confirmation
		lu, err := handler.Get(path)
		if err != nil {
			return err
		}

		// Confirm deletion unless --force or --plain
		if !forceDelete && !plainMode {
			displayName := lu.DisplayName
			if displayName == "" {
				displayName = path
			}
			if !prompt.ConfirmDeletion("lookup table", displayName, path) {
				fmt.Println("Deletion cancelled")
				return nil
			}
		}

		if err := handler.Delete(path); err != nil {
			return err
		}

		fmt.Printf("Lookup table %q deleted\n", path)
		return nil
	},
}

func init() {
	// Delete confirmation flags
	deleteLookupCmd.Flags().BoolVarP(&forceDelete, "yes", "y", false, "Skip confirmation prompt")
}

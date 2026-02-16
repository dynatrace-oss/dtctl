package cmd

import (
	"fmt"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/resources/lookup"
	"github.com/spf13/cobra"
)

// describeLookupCmd shows detailed info about a lookup table
var describeLookupCmd = &cobra.Command{
	Use:     "lookup <path>",
	Aliases: []string{"lookups", "lkup", "lu"},
	Short:   "Show details of a lookup table",
	Long: `Show detailed information about a lookup table including metadata and data preview.

Examples:
  # Describe a lookup table
  dtctl describe lookup /lookups/grail/pm/error_codes

  # Output as JSON
  dtctl describe lookup /lookups/grail/pm/error_codes -o json
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := lookup.NewHandler(c)

		// Get lookup metadata
		lu, err := handler.Get(path)
		if err != nil {
			return err
		}

		// Get preview data (first 5 rows)
		data, err := handler.GetData(path, 5)
		if err != nil {
			return err
		}

		// For JSON output, use printer
		if outputFormat == "json" || outputFormat == "yaml" {
			printer := NewPrinter()
			lookupData := struct {
				*lookup.Lookup
				PreviewData []map[string]interface{} `json:"previewData"`
			}{
				Lookup:      lu,
				PreviewData: data,
			}
			return printer.Print(lookupData)
		}

		// Print lookup details
		fmt.Printf("Path:         %s\n", lu.Path)
		if lu.DisplayName != "" {
			fmt.Printf("Display Name: %s\n", lu.DisplayName)
		}
		if lu.Description != "" {
			fmt.Printf("Description:  %s\n", lu.Description)
		}
		if lu.FileSize > 0 {
			fmt.Printf("File Size:    %s\n", formatBytes(lu.FileSize))
		}
		if lu.Records > 0 {
			fmt.Printf("Records:      %d\n", lu.Records)
		}
		if lu.LookupField != "" {
			fmt.Printf("Lookup Field: %s\n", lu.LookupField)
		}
		if len(lu.Columns) > 0 {
			fmt.Printf("Columns:      %s\n", strings.Join(lu.Columns, ", "))
		}
		if lu.Modified != "" {
			fmt.Printf("Modified:     %s\n", lu.Modified)
		}

		// Print data preview
		if len(data) > 0 {
			fmt.Println()
			fmt.Printf("Data Preview (first %d rows):\n", len(data))

			// Create table header
			if len(lu.Columns) > 0 {
				fmt.Println(strings.Join(lu.Columns, "\t"))
			}

			// Print rows
			for _, row := range data {
				var values []string
				for _, col := range lu.Columns {
					val := fmt.Sprintf("%v", row[col])
					values = append(values, val)
				}
				fmt.Println(strings.Join(values, "\t"))
			}
		}

		return nil
	},
}

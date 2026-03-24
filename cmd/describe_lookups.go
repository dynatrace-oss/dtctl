package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/lookup"
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
		dataResult, err := handler.GetData(path, 5)
		if err != nil {
			return err
		}

		// For structured formats, use printer
		if outputFormat != "table" {
			printer := NewPrinter()
			enrichAgent(printer, "describe", "lookup")
			lookupData := struct {
				*lookup.Lookup
				PreviewData []map[string]interface{} `json:"previewData"`
			}{
				Lookup:      lu,
				PreviewData: dataResult.Records,
			}
			return printer.Print(lookupData)
		}

		// Print lookup details
		const w = 14
		output.DescribeKV("Path:", w, "%s", lu.Path)
		if lu.DisplayName != "" {
			output.DescribeKV("Display Name:", w, "%s", lu.DisplayName)
		}
		if lu.Description != "" {
			output.DescribeKV("Description:", w, "%s", lu.Description)
		}
		if lu.FileSize > 0 {
			output.DescribeKV("File Size:", w, "%s", formatBytes(lu.FileSize))
		}
		if lu.Records > 0 {
			output.DescribeKV("Records:", w, "%d", lu.Records)
		}
		if lu.LookupField != "" {
			output.DescribeKV("Lookup Field:", w, "%s", lu.LookupField)
		}
		if len(lu.Columns) > 0 {
			output.DescribeKV("Columns:", w, "%s", strings.Join(lu.Columns, ", "))
		}
		if lu.Modified != "" {
			output.DescribeKV("Modified:", w, "%s", lu.Modified)
		}

		// Print data preview
		if len(dataResult.Records) > 0 {
			fmt.Println()
			output.DescribeSection(fmt.Sprintf("Data Preview (first %d rows):", len(dataResult.Records)))

			// Create table header
			if len(lu.Columns) > 0 {
				fmt.Println(strings.Join(lu.Columns, "\t"))
			}

			// Print rows
			for _, row := range dataResult.Records {
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

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/edgeconnect"
)

// describeEdgeConnectCmd shows detailed info about an EdgeConnect
var describeEdgeConnectCmd = &cobra.Command{
	Use:     "edgeconnect <id>",
	Aliases: []string{"ec"},
	Short:   "Show details of an EdgeConnect configuration",
	Long: `Show detailed information about an EdgeConnect configuration.

Examples:
  # Describe an EdgeConnect
  dtctl describe edgeconnect <id>
  dtctl describe ec <id>
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ecID := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := edgeconnect.NewHandler(c)

		ec, err := handler.Get(ecID)
		if err != nil {
			return err
		}

		// For table output, show detailed human-readable information
		if outputFormat == "table" {
			const w = 10
			output.DescribeKV("ID:", w, "%s", ec.ID)
			output.DescribeKV("Name:", w, "%s", ec.Name)
			output.DescribeKV("Managed:", w, "%v", ec.ManagedByDynatraceOperator)

			if len(ec.HostPatterns) > 0 {
				fmt.Println()
				output.DescribeSection("Host Patterns:")
				for _, pattern := range ec.HostPatterns {
					fmt.Printf("  - %s\n", pattern)
				}
			}

			if ec.OAuthClientID != "" {
				fmt.Println()
				output.DescribeKV("OAuth Client ID:", 0, "%s", ec.OAuthClientID)
			}

			if ec.ModificationInfo != nil {
				fmt.Println()
				if ec.ModificationInfo.CreatedTime != "" {
					output.DescribeKV("Created:", w, "%s (by %s)", ec.ModificationInfo.CreatedTime, ec.ModificationInfo.CreatedBy)
				}
				if ec.ModificationInfo.LastModifiedTime != "" {
					output.DescribeKV("Modified:", w, "%s (by %s)", ec.ModificationInfo.LastModifiedTime, ec.ModificationInfo.LastModifiedBy)
				}
			}

			return nil
		}

		// For other formats, use standard printer
		printer := NewPrinter()
		enrichAgent(printer, "describe", "edgeconnect")
		return printer.Print(ec)
	},
}

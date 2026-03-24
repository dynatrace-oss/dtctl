package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/settings"
	"github.com/dynatrace-oss/dtctl/pkg/resources/slo"
)

// describeSLOCmd shows detailed info about an SLO
var describeSLOCmd = &cobra.Command{
	Use:     "slo <slo-id>",
	Aliases: []string{},
	Short:   "Show details of a service-level objective",
	Long: `Show detailed information about a service-level objective including criteria, tags, and metadata.

Examples:
  # Describe an SLO by ID
  dtctl describe slo <slo-id>
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sloID := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := slo.NewHandler(c)

		// Get SLO details
		s, err := handler.Get(sloID)
		if err != nil {
			return err
		}

		// For table output, show detailed human-readable information
		if outputFormat == "table" {
			const w = 13
			output.DescribeKV("ID:", w, "%s", s.ID)

			// Try to decode the object ID to show the UID
			if decoded, err := settings.DecodeObjectID(s.ID); err == nil && decoded.UID != "" {
				output.DescribeKV("UID:", w, "%s", decoded.UID)
			}

			output.DescribeKV("Name:", w, "%s", s.Name)
			if s.Description != "" {
				output.DescribeKV("Description:", w, "%s", s.Description)
			}
			if s.Version != "" {
				// Try to decode the version to show the modification timestamp
				if decodedVersion, err := settings.DecodeVersion(s.Version); err == nil {
					if decodedVersion.Timestamp != nil {
						output.DescribeKV("Modified:", w, "%s", decodedVersion.Timestamp.Format("2006-01-02 15:04:05 UTC"))
					}
				}
			}
			if s.ExternalID != "" {
				output.DescribeKV("External ID:", w, "%s", s.ExternalID)
			}

			// Print tags
			if len(s.Tags) > 0 {
				output.DescribeKV("Tags:", w, "%s", strings.Join(s.Tags, ", "))
			}

			// Print criteria
			if len(s.Criteria) > 0 {
				fmt.Println()
				output.DescribeSection("Criteria:")
				for _, c := range s.Criteria {
					timeframe := c.TimeframeFrom
					if c.TimeframeTo != "" {
						timeframe = fmt.Sprintf("%s to %s", c.TimeframeFrom, c.TimeframeTo)
					}
					fmt.Printf("  - Timeframe: %s\n", timeframe)
					fmt.Printf("    Target:    %.2f%%\n", c.Target)
					if c.Warning != nil {
						fmt.Printf("    Warning:   %.2f%%\n", *c.Warning)
					}
				}
			}

			// Print custom SLI if present
			if len(s.CustomSli) > 0 {
				fmt.Println()
				output.DescribeSection("Custom SLI:")
				sliJSON, err := json.MarshalIndent(s.CustomSli, "  ", "  ")
				if err == nil {
					fmt.Printf("  %s\n", string(sliJSON))
				}
			}

			return nil
		}

		// For other formats, use standard printer
		printer := NewPrinter()
		enrichAgent(printer, "describe", "slo")
		return printer.Print(s)
	},
}

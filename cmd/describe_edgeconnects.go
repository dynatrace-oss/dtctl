package cmd

import (
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/resources/edgeconnect"
	"github.com/spf13/cobra"
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

		// Print EdgeConnect details
		fmt.Printf("ID:       %s\n", ec.ID)
		fmt.Printf("Name:     %s\n", ec.Name)
		fmt.Printf("Managed:  %v\n", ec.ManagedByDynatraceOperator)

		if len(ec.HostPatterns) > 0 {
			fmt.Println()
			fmt.Println("Host Patterns:")
			for _, pattern := range ec.HostPatterns {
				fmt.Printf("  - %s\n", pattern)
			}
		}

		if ec.OAuthClientID != "" {
			fmt.Printf("\nOAuth Client ID: %s\n", ec.OAuthClientID)
		}

		if ec.ModificationInfo != nil {
			fmt.Println()
			if ec.ModificationInfo.CreatedTime != "" {
				fmt.Printf("Created:  %s (by %s)\n", ec.ModificationInfo.CreatedTime, ec.ModificationInfo.CreatedBy)
			}
			if ec.ModificationInfo.LastModifiedTime != "" {
				fmt.Printf("Modified: %s (by %s)\n", ec.ModificationInfo.LastModifiedTime, ec.ModificationInfo.LastModifiedBy)
			}
		}

		return nil
	},
}

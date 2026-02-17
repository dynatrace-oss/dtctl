package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/resources/edgeconnect"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/dynatrace-oss/dtctl/pkg/util/format"
	"github.com/spf13/cobra"
)

// createEdgeConnectCmd creates an EdgeConnect
var createEdgeConnectCmd = &cobra.Command{
	Use:   "edgeconnect --name <name> [--host-patterns <patterns>]",
	Short: "Create an EdgeConnect configuration",
	Long: `Create a new EdgeConnect configuration.

Examples:
  # Create an EdgeConnect with host patterns
  dtctl create edgeconnect --name my-edgeconnect --host-patterns "*.internal.example.com,api.example.com"

  # Create from a file
  dtctl create edgeconnect -f edgeconnect.yaml

  # Dry run to preview
  dtctl create edgeconnect --name my-edgeconnect --host-patterns "*.example.com" --dry-run
`,
	Aliases: []string{"ec"},
	RunE: func(cmd *cobra.Command, args []string) error {
		file, _ := cmd.Flags().GetString("file")
		name, _ := cmd.Flags().GetString("name")
		hostPatterns, _ := cmd.Flags().GetString("host-patterns")

		var req edgeconnect.EdgeConnectCreate

		if file != "" {
			// Read from file
			fileData, err := os.ReadFile(file)
			if err != nil {
				return fmt.Errorf("failed to read file: %w", err)
			}

			jsonData, err := format.ValidateAndConvert(fileData)
			if err != nil {
				return fmt.Errorf("invalid file format: %w", err)
			}

			if err := json.Unmarshal(jsonData, &req); err != nil {
				return fmt.Errorf("failed to parse EdgeConnect definition: %w", err)
			}
		} else {
			// Use flags
			if name == "" {
				return fmt.Errorf("--name is required (or use -f to specify a file)")
			}

			var patterns []string
			if hostPatterns != "" {
				patterns = strings.Split(hostPatterns, ",")
				for i := range patterns {
					patterns[i] = strings.TrimSpace(patterns[i])
				}
			}

			req = edgeconnect.EdgeConnectCreate{
				Name:         name,
				HostPatterns: patterns,
			}
		}

		// Handle dry-run
		if dryRun {
			fmt.Printf("Dry run: would create EdgeConnect\n")
			fmt.Printf("Name: %s\n", req.Name)
			if len(req.HostPatterns) > 0 {
				fmt.Printf("Host Patterns: %s\n", strings.Join(req.HostPatterns, ", "))
			}
			return nil
		}

		// Load configuration
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		// Safety check
		checker, err := NewSafetyChecker(cfg)
		if err != nil {
			return err
		}
		if err := checker.CheckError(safety.OperationCreate, safety.OwnershipUnknown); err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := edgeconnect.NewHandler(c)

		result, err := handler.Create(req)
		if err != nil {
			return fmt.Errorf("failed to create EdgeConnect: %w", err)
		}

		fmt.Printf("EdgeConnect %q created (ID: %s)\n", result.Name, result.ID)
		if result.OAuthClientSecret != "" {
			fmt.Println("\nOAuth Client Credentials (save these, the secret won't be shown again):")
			fmt.Printf("  Client ID:     %s\n", result.OAuthClientID)
			fmt.Printf("  Client Secret: %s\n", result.OAuthClientSecret)
			if result.OAuthClientResource != "" {
				fmt.Printf("  Resource:      %s\n", result.OAuthClientResource)
			}
		}
		return nil
	},
}

func init() {
	// EdgeConnect flags
	createEdgeConnectCmd.Flags().StringP("file", "f", "", "file containing EdgeConnect definition")
	createEdgeConnectCmd.Flags().String("name", "", "EdgeConnect name (RFC 1123 compliant, max 50 chars)")
	createEdgeConnectCmd.Flags().String("host-patterns", "", "comma-separated list of host patterns")
}

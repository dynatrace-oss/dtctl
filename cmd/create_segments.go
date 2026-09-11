package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/segment"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/dynatrace-oss/dtctl/pkg/util/format"
	"github.com/dynatrace-oss/dtctl/pkg/vfs"
)

// createSegmentCmd creates a Grail filter segment
var createSegmentCmd = &cobra.Command{
	Use:   "segment -f segment.yaml",
	Short: "Create a Grail filter segment",
	Long: `Create a new Grail filter segment from a YAML or JSON file.

Examples:
  # Create a segment from a YAML file
  dtctl create segment -f segment.yaml

  # Create from a JSON file
  dtctl create segment -f segment.json

  # Dry run to preview
  dtctl create segment -f segment.yaml --dry-run
`,
	Aliases: []string{"seg", "filter-segment", "filter-segments"},
	RunE: func(cmd *cobra.Command, args []string) error {
		file, _ := cmd.Flags().GetString("file")

		if file == "" {
			return fmt.Errorf("--file (-f) is required")
		}

		// Read from file
		fileData, err := vfs.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}

		jsonData, err := format.ValidateAndConvert(fileData)
		if err != nil {
			return fmt.Errorf("invalid file format: %w", err)
		}

		// Handle dry-run
		if dryRun {
			var seg map[string]interface{}
			if err := json.Unmarshal(jsonData, &seg); err != nil {
				return fmt.Errorf("failed to parse segment definition: %w", err)
			}

			fmt.Println("Dry run: would create segment")
			if name, ok := seg["name"].(string); ok && name != "" {
				fmt.Printf("  Name: %s\n", name)
			}
			if desc, ok := seg["description"].(string); ok && desc != "" {
				fmt.Printf("  Description: %s\n", desc)
			}
			if includes, ok := seg["includes"].([]interface{}); ok {
				fmt.Printf("  Includes: %d rule(s)\n", len(includes))
			}
			fmt.Println("\nSegment definition parsed successfully")
			return nil
		}

		_, c, err := SetupWithSafety(safety.OperationCreate)
		if err != nil {
			return err
		}

		handler := segment.NewHandler(c)

		result, err := handler.Create(jsonData)
		if err != nil {
			return fmt.Errorf("failed to create segment: %w", err)
		}

		output.PrintSuccess("Segment %q created (UID: %s)", result.Name, result.UID)
		return nil
	},
}

func init() {
	createSegmentCmd.Flags().StringP("file", "f", "", "file containing segment definition (YAML or JSON)")
}

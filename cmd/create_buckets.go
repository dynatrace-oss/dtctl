package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/dynatrace-oss/dtctl/pkg/resources/bucket"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/dynatrace-oss/dtctl/pkg/util/format"
	"github.com/spf13/cobra"
)

// createBucketCmd creates a Grail bucket
var createBucketCmd = &cobra.Command{
	Use:   "bucket --name <name> --table <table> --retention <days>",
	Short: "Create a Grail storage bucket",
	Long: `Create a new Grail storage bucket.

Examples:
  # Create a logs bucket with 35 days retention
  dtctl create bucket --name custom_logs --table logs --retention 35

  # Create with display name
  dtctl create bucket --name custom_logs --table logs --retention 35 --display-name "Custom Logs Bucket"

  # Create from a file
  dtctl create bucket -f bucket.yaml

  # Dry run to preview
  dtctl create bucket --name custom_logs --table logs --retention 35 --dry-run
`,
	Aliases: []string{"bkt"},
	RunE: func(cmd *cobra.Command, args []string) error {
		file, _ := cmd.Flags().GetString("file")
		name, _ := cmd.Flags().GetString("name")
		table, _ := cmd.Flags().GetString("table")
		retention, _ := cmd.Flags().GetInt("retention")
		displayName, _ := cmd.Flags().GetString("display-name")

		var req bucket.BucketCreate

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
				return fmt.Errorf("failed to parse bucket definition: %w", err)
			}
		} else {
			// Use flags
			if name == "" {
				return fmt.Errorf("--name is required (or use -f to specify a file)")
			}
			if table == "" {
				return fmt.Errorf("--table is required (logs, events, or bizevents)")
			}
			if retention == 0 {
				return fmt.Errorf("--retention is required (1-3657 days)")
			}

			req = bucket.BucketCreate{
				BucketName:    name,
				Table:         table,
				RetentionDays: retention,
				DisplayName:   displayName,
			}
		}

		// Handle dry-run
		if dryRun {
			fmt.Printf("Dry run: would create bucket\n")
			fmt.Printf("Name: %s\n", req.BucketName)
			fmt.Printf("Table: %s\n", req.Table)
			fmt.Printf("Retention: %d days\n", req.RetentionDays)
			if req.DisplayName != "" {
				fmt.Printf("Display Name: %s\n", req.DisplayName)
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

		handler := bucket.NewHandler(c)

		result, err := handler.Create(req)
		if err != nil {
			return fmt.Errorf("failed to create bucket: %w", err)
		}

		fmt.Printf("Bucket %q created (status: %s)\n", result.BucketName, result.Status)
		fmt.Println("Note: Bucket creation can take up to 1 minute to complete")
		return nil
	},
}

func init() {
	// Bucket flags
	createBucketCmd.Flags().StringP("file", "f", "", "file containing bucket definition")
	createBucketCmd.Flags().String("name", "", "bucket name (3-100 chars, lowercase alphanumeric, underscores, hyphens)")
	createBucketCmd.Flags().String("table", "", "table type (logs, events, or bizevents)")
	createBucketCmd.Flags().Int("retention", 0, "retention period in days (1-3657)")
	createBucketCmd.Flags().String("display-name", "", "display name for the bucket")
}

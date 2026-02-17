package cmd

import (
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/prompt"
	"github.com/dynatrace-oss/dtctl/pkg/resources/bucket"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/spf13/cobra"
)

// getBucketsCmd retrieves Grail buckets
var getBucketsCmd = &cobra.Command{
	Use:     "buckets [name]",
	Aliases: []string{"bucket", "bkt"},
	Short:   "Get Grail storage buckets",
	Long: `Get Grail storage buckets.

Examples:
  # List all buckets
  dtctl get buckets

  # Get a specific bucket
  dtctl get bucket <bucket-name>

  # Output as JSON
  dtctl get buckets -o json
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

		handler := bucket.NewHandler(c)
		printer := NewPrinter()

		// Get specific bucket if name provided
		if len(args) > 0 {
			b, err := handler.Get(args[0])
			if err != nil {
				return err
			}
			return printer.Print(b)
		}

		// List all buckets
		list, err := handler.List()
		if err != nil {
			return err
		}

		return printer.PrintList(list.Buckets)
	},
}

// deleteBucketCmd deletes a bucket
var deleteBucketCmd = &cobra.Command{
	Use:     "bucket <bucket-name>",
	Aliases: []string{"buckets", "bkt"},
	Short:   "Delete a Grail storage bucket",
	Long: `Delete a Grail storage bucket by name.

WARNING: This operation is irreversible and will delete all data in the bucket.

Examples:
  # Delete a bucket (requires typing the name to confirm)
  dtctl delete bucket <bucket-name>

  # Delete with confirmation flag (non-interactive)
  dtctl delete bucket <bucket-name> --confirm=<bucket-name>

  # Delete without confirmation (use with caution)
  dtctl delete bucket <bucket-name> -y
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		bucketName := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		// Safety check - bucket deletion requires unrestricted level
		checker, err := NewSafetyChecker(cfg)
		if err != nil {
			return err
		}
		if err := checker.CheckError(safety.OperationDeleteBucket, safety.OwnershipUnknown); err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := bucket.NewHandler(c)

		// Verify bucket exists before prompting for confirmation
		if _, err := handler.Get(bucketName); err != nil {
			return err
		}

		// Handle confirmation for data deletion
		confirmFlag, _ := cmd.Flags().GetString("confirm")
		if !forceDelete && !plainMode {
			// If --confirm flag provided, validate it matches the bucket name
			if confirmFlag != "" {
				if !prompt.ValidateConfirmFlag(confirmFlag, bucketName) {
					return fmt.Errorf("confirmation value %q does not match bucket name %q", confirmFlag, bucketName)
				}
			} else {
				// Interactive confirmation - require typing the bucket name
				if !prompt.ConfirmDataDeletion("bucket", bucketName) {
					fmt.Println("Deletion cancelled")
					return nil
				}
			}
		}

		if err := handler.Delete(bucketName); err != nil {
			return err
		}

		fmt.Printf("Bucket %q deletion initiated (async operation)\n", bucketName)
		return nil
	},
}

func init() {
	addWatchFlags(getBucketsCmd)

	// Delete confirmation flags
	deleteBucketCmd.Flags().BoolVarP(&forceDelete, "yes", "y", false, "Skip confirmation prompt")
	deleteBucketCmd.Flags().String("confirm", "", "Confirm deletion by providing the bucket name (for non-interactive use)")
}

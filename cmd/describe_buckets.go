package cmd

import (
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/resources/bucket"
	"github.com/spf13/cobra"
)

// describeBucketCmd shows detailed info about a bucket
var describeBucketCmd = &cobra.Command{
	Use:     "bucket <bucket-name>",
	Aliases: []string{"bkt"},
	Short:   "Show details of a Grail storage bucket",
	Long: `Show detailed information about a Grail storage bucket.

Examples:
  # Describe a bucket
  dtctl describe bucket default_logs
  dtctl describe bkt custom_logs
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		bucketName := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := bucket.NewHandler(c)

		b, err := handler.Get(bucketName)
		if err != nil {
			return err
		}

		// Print bucket details
		fmt.Printf("Name:           %s\n", b.BucketName)
		fmt.Printf("Display Name:   %s\n", b.DisplayName)
		fmt.Printf("Table:          %s\n", b.Table)
		fmt.Printf("Status:         %s\n", b.Status)
		fmt.Printf("Retention:      %d days\n", b.RetentionDays)
		fmt.Printf("Updatable:      %v\n", b.Updatable)
		fmt.Printf("Version:        %d\n", b.Version)
		if b.MetricInterval != "" {
			fmt.Printf("Metric Interval: %s\n", b.MetricInterval)
		}
		if b.Records != nil {
			fmt.Printf("Records:        %d\n", *b.Records)
		}
		if b.EstimatedUncompressedBytes != nil {
			fmt.Printf("Est. Size:      %s\n", formatBytes(*b.EstimatedUncompressedBytes))
		}

		return nil
	},
}

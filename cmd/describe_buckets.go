package cmd

import (
	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/bucket"
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

		// For table output, show detailed human-readable information
		if outputFormat == "table" {
			const w = 16
			output.DescribeKV("Name:", w, "%s", b.BucketName)
			output.DescribeKV("Display Name:", w, "%s", b.DisplayName)
			output.DescribeKV("Table:", w, "%s", b.Table)
			output.DescribeKV("Status:", w, "%s", b.Status)
			output.DescribeKV("Retention:", w, "%d days", b.RetentionDays)
			output.DescribeKV("Updatable:", w, "%v", b.Updatable)
			output.DescribeKV("Version:", w, "%d", b.Version)
			if b.MetricInterval != "" {
				output.DescribeKV("Metric Interval:", w, "%s", b.MetricInterval)
			}
			if b.Records != nil {
				output.DescribeKV("Records:", w, "%d", *b.Records)
			}
			if b.EstimatedUncompressedBytes != nil {
				output.DescribeKV("Est. Size:", w, "%s", formatBytes(*b.EstimatedUncompressedBytes))
			}

			return nil
		}

		// For other formats, use standard printer
		printer := NewPrinter()
		enrichAgent(printer, "describe", "bucket")
		return printer.Print(b)
	},
}

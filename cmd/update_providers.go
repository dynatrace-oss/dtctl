package cmd

import "github.com/spf13/cobra"

var updateAWSProviderCmd = &cobra.Command{
	Use:   "aws",
	Short: "Update AWS resources",
	RunE:  requireSubcommand,
}

var updateGCPProviderCmd = &cobra.Command{
	Use:   "gcp",
	Short: "Update GCP resources (Preview)",
	RunE:  requireSubcommand,
}

func init() {
	updateCmd.AddCommand(updateAWSProviderCmd)
	updateCmd.AddCommand(updateGCPProviderCmd)
	attachPreviewNotice(updateGCPProviderCmd, "GCP")

	updateAWSProviderCmd.AddCommand(newNotImplementedProviderResourceCommand("aws", "connection"))
	updateAWSProviderCmd.AddCommand(newNotImplementedProviderResourceCommand("aws", "monitoring"))
}

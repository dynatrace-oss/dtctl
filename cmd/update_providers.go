package cmd

import "github.com/spf13/cobra"

var updateAWSProviderCmd = &cobra.Command{
	Use:   "aws",
	Short: "Update AWS resources",
	RunE:  requireSubcommand,
}

var updateGCPProviderCmd = &cobra.Command{
	Use:   "gcp",
	Short: "Update GCP resources",
	RunE:  requireSubcommand,
}

func init() {
	updateCmd.AddCommand(updateAWSProviderCmd)
	updateCmd.AddCommand(updateGCPProviderCmd)
}

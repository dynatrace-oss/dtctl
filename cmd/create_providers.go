package cmd

import "github.com/spf13/cobra"

var createAWSProviderCmd = &cobra.Command{
	Use:   "aws",
	Short: "Create AWS resources",
	RunE:  requireSubcommand,
}

var createGCPProviderCmd = &cobra.Command{
	Use:   "gcp",
	Short: "Create GCP resources",
	RunE:  requireSubcommand,
}

func init() {
	createCmd.AddCommand(createAWSProviderCmd)
	createCmd.AddCommand(createGCPProviderCmd)

	createAWSProviderCmd.AddCommand(newNotImplementedProviderResourceCommand("aws", "connection"))
	createAWSProviderCmd.AddCommand(newNotImplementedProviderResourceCommand("aws", "monitoring"))
}

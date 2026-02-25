package cmd

import "github.com/spf13/cobra"

var getAWSProviderCmd = &cobra.Command{
	Use:   "aws",
	Short: "Get AWS resources",
	RunE:  requireSubcommand,
}

var getGCPProviderCmd = &cobra.Command{
	Use:   "gcp",
	Short: "Get GCP resources",
	RunE:  requireSubcommand,
}

func init() {
	getCmd.AddCommand(getAWSProviderCmd)
	getCmd.AddCommand(getGCPProviderCmd)
}

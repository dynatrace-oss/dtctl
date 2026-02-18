package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

const providerAzure = "azure"

func requireAzureProvider(provider string) error {
	if provider != providerAzure {
		return fmt.Errorf("unsupported provider %q (currently supported: azure)", provider)
	}
	return nil
}

func addRequiredProviderFlagVar(cmd *cobra.Command, target *string) {
	cmd.Flags().StringVar(target, "provider", "", "Cloud provider (required): azure")
	_ = cmd.MarkFlagRequired("provider")
	_ = cmd.RegisterFlagCompletionFunc("provider", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{"azure\tAzure provider"}, cobra.ShellCompDirectiveNoFileComp
	})
}

func addRequiredProviderFlag(cmd *cobra.Command) {
	cmd.Flags().String("provider", "", "Cloud provider (required): azure")
	_ = cmd.MarkFlagRequired("provider")
	_ = cmd.RegisterFlagCompletionFunc("provider", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{"azure\tAzure provider"}, cobra.ShellCompDirectiveNoFileComp
	})
}

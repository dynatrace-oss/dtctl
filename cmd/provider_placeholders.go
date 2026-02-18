package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newNotImplementedProviderResourceCommand(provider string, resource string) *cobra.Command {
	return &cobra.Command{
		Use:   resource,
		Short: fmt.Sprintf("Manage %s %s", provider, resource),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("%s %s is not implemented yet", provider, resource)
		},
	}
}

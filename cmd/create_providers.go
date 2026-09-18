package cmd

import (
	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/stability"
)

var createAWSProviderCmd = &cobra.Command{
	Use:   "aws",
	Short: "Create AWS resources",
	RunE:  requireSubcommand,
}

var createGCPProviderCmd = &cobra.Command{
	Use:   "gcp",
	Short: "Create GCP resources (Preview)",
	RunE:  requireSubcommand,
}

func init() {
	createCmd.AddCommand(createAWSProviderCmd)
	createCmd.AddCommand(createGCPProviderCmd)
	attachPreviewNotice(createGCPProviderCmd, "GCP")
}

// Declared stable: the invocation and output contract of these commands is
// additive-only. Stable is never implied -- see AGENTS.md "Stability Tiers".
func init() {
	stability.MarkStable(createAWSProviderCmd)
	stability.MarkStable(createGCPProviderCmd)
}

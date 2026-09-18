package cmd

import (
	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/stability"
)

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
}

// Declared stable: the invocation and output contract of these commands is
// additive-only. Stable is never implied -- see AGENTS.md "Stability Tiers".
func init() {
	stability.MarkStable(updateAWSProviderCmd)
	stability.MarkStable(updateGCPProviderCmd)
}

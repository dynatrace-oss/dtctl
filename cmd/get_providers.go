package cmd

import (
	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/stability"
)

var getAWSProviderCmd = &cobra.Command{
	Use:   "aws",
	Short: "Get AWS resources",
	RunE:  requireSubcommand,
}

var getGCPProviderCmd = &cobra.Command{
	Use:   "gcp",
	Short: "Get GCP resources (Preview)",
	RunE:  requireSubcommand,
}

func init() {
	getCmd.AddCommand(getAWSProviderCmd)
	getCmd.AddCommand(getGCPProviderCmd)
	attachPreviewNotice(getGCPProviderCmd, "GCP")
}

// Declared stable: the invocation and output contract of these commands is
// additive-only. Stable is never implied -- see AGENTS.md "Stability Tiers".
func init() {
	stability.MarkStable(getAWSProviderCmd)
	stability.MarkStable(getGCPProviderCmd)
}

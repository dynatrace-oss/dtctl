package cmd

import (
	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/platform"
)

// usePlatformDescribeTextView reports whether to render the human-readable KV
// text view. Agent mode always takes the structured path (outputFormat stays at
// its "table" default when --agent is set, so the check cannot be outputFormat alone).
func usePlatformDescribeTextView() bool {
	if agentMode {
		return false
	}
	// wide selects extra columns of the describe table, not a different view
	// (same reasoning as describe_api.go:117).
	return outputFormat == "" || outputFormat == "table" || outputFormat == "wide"
}

// describeEnvironmentCmd shows detailed environment information
var describeEnvironmentCmd = &cobra.Command{
	Use:     "environment",
	Aliases: []string{"env"},
	Short:   "Show details of the current environment",
	Args:    cobra.NoArgs,
	Long: `Show detailed information about the current Dynatrace environment.

Examples:
  # Describe the current environment
  dtctl describe environment
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, c, printer, err := Setup()
		if err != nil {
			return err
		}

		h := platform.NewHandler(c)
		info, err := h.GetEnvironment()
		if err != nil {
			return err
		}

		if usePlatformDescribeTextView() {
			const w = 12
			output.DescribeKV("ID:", w, "%s", info.EnvironmentID)
			output.DescribeKV("Type:", w, "%s", info.Type)
			output.DescribeKV("State:", w, "%s", info.State)
			if !info.CreateTime.IsZero() {
				output.DescribeKV("Created:", w, "%s", info.CreateTime.Format("2006-01-02"))
			}
			if !info.BlockTime.IsZero() {
				output.DescribeKV("Block Time:", w, "%s", info.BlockTime.Format("2006-01-02"))
			}
			return nil
		}

		enrichAgent(printer, "describe", "environment")
		return printer.Print(info)
	},
}

// describeLicenseCmd shows detailed license information
var describeLicenseCmd = &cobra.Command{
	Use:   "license",
	Short: "Show details of the environment license",
	Args:  cobra.NoArgs,
	Long: `Show detailed license information for the current Dynatrace environment.

Examples:
  # Describe the environment license
  dtctl describe license
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, c, printer, err := Setup()
		if err != nil {
			return err
		}

		h := platform.NewHandler(c)
		lic, err := h.GetLicense()
		if err != nil {
			return err
		}

		if usePlatformDescribeTextView() {
			const w = 24
			output.DescribeKV("Trial:", w, "%v", lic.Trial)
			output.DescribeKV("Platform Subscription:", w, "%v", lic.PlatformSubscription)
			return nil
		}

		enrichAgent(printer, "describe", "license")
		return printer.Print(lic)
	},
}

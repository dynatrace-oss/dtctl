package cmd

import (
	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/resources/platform"
)

// getEnvironmentCmd retrieves environment information
var getEnvironmentCmd = &cobra.Command{
	Use:     "environment",
	Aliases: []string{"env"},
	Short:   "Get environment information",
	Args:    cobra.NoArgs,
	Long: `Get information about the current Dynatrace environment.

Examples:
  # Get environment info
  dtctl get environment

  # Output as JSON
  dtctl get environment -o json
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
		enrichAgent(printer, "get", "environment")
		return printer.Print(info)
	},
}

// getLicenseSettingsCmd retrieves environment license feature settings
var getLicenseSettingsCmd = &cobra.Command{
	Use:     "license-settings [key...]",
	Aliases: []string{"license-setting"},
	Short:   "Get environment license feature settings",
	Long: `Get the feature settings included in the environment license.

Optionally filter by one or more setting keys.

Examples:
  # List all license feature settings
  dtctl get license-settings

  # Get a specific setting by key
  dtctl get license-settings AUTOMATION

  # Get multiple settings by key
  dtctl get license-settings AUTOMATION AI_FUNCTIONS

  # Output as JSON
  dtctl get license-settings -o json
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, c, printer, err := Setup()
		if err != nil {
			return err
		}

		h := platform.NewHandler(c)
		settings, err := h.GetLicenseSettings(args...)
		if err != nil {
			return err
		}
		enrichAgent(printer, "get", "license-settings")
		return printer.PrintList(settings)
	},
}

// getLicenseCmd retrieves environment license information
var getLicenseCmd = &cobra.Command{
	Use:   "license",
	Short: "Get environment license information",
	Args:  cobra.NoArgs,
	Long: `Get license information for the current Dynatrace environment.

Examples:
  # Get license info
  dtctl get license

  # Output as JSON
  dtctl get license -o json
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
		enrichAgent(printer, "get", "license")
		return printer.Print(lic)
	},
}

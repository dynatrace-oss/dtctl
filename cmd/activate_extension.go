package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/extension"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
)

// activateExtensionCmd activates a specific version of an extension environment-wide.
var activateExtensionCmd = &cobra.Command{
	Use:     "extension <extension-name>",
	Aliases: []string{"ext"},
	Short:   "Activate an Extensions 2.0 extension version",
	Long: `Activate a specific version of an Extensions 2.0 extension environment-wide.

Sends POST /platform/extensions/v2/extensions/{name} with body {"version": "<version>"}.
The server returns 202 Accepted and deploys bundled assets (e.g. OpenPipeline configs)
asynchronously. The version must already be uploaded.

Use 'dtctl create extension -f <zip>' to upload a new version first, then activate
it with this command.

Examples:
  # Activate version 1.0.2 of a custom extension
  dtctl activate extension custom:my-extension --version 1.0.2

  # Activate a specific version of a built-in extension
  dtctl activate extension com.dynatrace.extension.host-monitoring --version 1.2.3
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		extensionName := args[0]
		version, _ := cmd.Flags().GetString("version")

		if version == "" {
			return fmt.Errorf("--version is required")
		}

		if dryRun {
			fmt.Printf("Dry run: would activate extension %q version %s\n", extensionName, version)
			return nil
		}

		_, c, err := SetupWithSafety(safety.OperationUpdate)
		if err != nil {
			return err
		}

		handler := extension.NewHandler(c)
		result, err := handler.SetEnvironmentConfig(extensionName, version)
		if err != nil {
			return err
		}

		output.PrintSuccess("Extension activated (bundled assets deploy asynchronously)")
		output.PrintInfo("  Name:    %s", result.ExtensionName)
		output.PrintInfo("  Version: %s", result.ExtensionVersion)
		return nil
	},
}

func init() {
	activateExtensionCmd.Flags().String("version", "", "version to activate (required)")
}

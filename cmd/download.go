package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/resources/extension"
)

var downloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download raw resource artifacts",
	Long: `Download raw resource artifacts such as extension zip packages.

This command writes binary data directly to stdout. Redirect output to a file.`,
	Example: `  # Download an extension package
  dtctl download extension com.dynatrace.extension.postgres --version 2.9.3 > postgres.zip`,
	RunE: requireSubcommand,
}

var downloadExtensionCmd = &cobra.Command{
	Use:     "extension <extension-name>",
	Aliases: []string{"ext"},
	Short:   "Download an extension zip package",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		extensionName := args[0]
		versionFlag, _ := cmd.Flags().GetString("version")

		if versionFlag == "" {
			return fmt.Errorf("--version is required")
		}
		if outputFormat != "table" {
			return fmt.Errorf("download extension does not support -o output formatting")
		}
		if GetAgentMode() {
			return fmt.Errorf("download extension is incompatible with agent mode (-A): raw binary cannot be wrapped in a JSON envelope")
		}

		_, c, err := SetupClient()
		if err != nil {
			return err
		}

		handler := extension.NewHandler(c)
		data, err := handler.Download(extensionName, versionFlag)
		if err != nil {
			return err
		}

		_, err = os.Stdout.Write(data)
		return err
	},
}

func init() {
	rootCmd.AddCommand(downloadCmd)
	downloadCmd.AddCommand(downloadExtensionCmd)
	downloadExtensionCmd.Flags().String("version", "", "Extension version to download")
}

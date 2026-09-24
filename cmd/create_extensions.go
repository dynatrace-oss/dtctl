package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/extension"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/dynatrace-oss/dtctl/pkg/stability"
)

// createExtensionCmd installs an extension — either a custom zip upload or a Hub extension.
var createExtensionCmd = &cobra.Command{
	Use:     "extension",
	Aliases: []string{"ext"},
	Short:   "Install an Extensions 2.0 extension",
	Long: `Install an Extensions 2.0 extension into the Dynatrace environment.

Two installation modes are supported:

  1. Upload a custom extension from a local zip file:
       dtctl create extension -f custom-extension.zip

  2. Install a Dynatrace Hub extension by its catalog ID:
       dtctl create extension --hub-extension <id> [--version <version>]

     If --version is omitted, the latest available Hub release is installed.

Examples:
  # Upload a custom extension package
  dtctl create extension -f my-extension.zip

  # Install a Hub extension (latest version)
  dtctl create extension --hub-extension com.dynatrace.extension.host-monitoring

  # Install a specific version of a Hub extension
  dtctl create extension --hub-extension com.dynatrace.extension.host-monitoring --version 1.2.3

  # Preview what would be installed (dry run)
  dtctl create extension -f my-extension.zip --dry-run
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		file, _ := cmd.Flags().GetString("file")
		hubExtension, _ := cmd.Flags().GetString("hub-extension")
		version, _ := cmd.Flags().GetString("version")

		// Exactly one of --file or --hub-extension must be provided
		if file == "" && hubExtension == "" {
			return fmt.Errorf("either --file or --hub-extension is required")
		}
		if file != "" && hubExtension != "" {
			return fmt.Errorf("--file and --hub-extension are mutually exclusive")
		}
		if file != "" && version != "" {
			return fmt.Errorf("--version only applies to --hub-extension")
		}

		if file != "" {
			return runUploadExtension(cmd, file)
		}
		return runInstallHubExtension(cmd, hubExtension, version)
	},
}

func runUploadExtension(cmd *cobra.Command, file string) error {
	// Read the zip file
	zipData, err := readFileFlag("file", file)
	if err != nil {
		return fmt.Errorf("failed to read file %q: %w", file, err)
	}

	if dryRun {
		return newDryRunReport(cmd).
			Linef("Dry run: would upload extension from %s (%d bytes)", sourceName(file), len(zipData)).
			Detail("file", "%s", sourceName(file)).
			Detail("size_bytes", "%d", len(zipData)).
			Print()
	}

	_, c, err := SetupWithSafety(safety.OperationCreate)
	if err != nil {
		return err
	}

	handler := extension.NewHandler(c)
	result, err := handler.Upload(filepath.Base(file), zipData)
	if err != nil {
		return err
	}

	output.PrintSuccess("Extension uploaded")
	output.PrintInfo("  Name:    %s", result.ExtensionName)
	output.PrintInfo("  Version: %s", result.Version)
	return nil
}

func runInstallHubExtension(cmd *cobra.Command, extensionID, version string) error {
	if dryRun {
		report := newDryRunReport(cmd).Detail("extension", "%s", extensionID)
		if version != "" {
			report.
				Linef("Dry run: would install Hub extension %q version %s", extensionID, version).
				Detail("version", "%s", version)
		} else {
			report.
				Linef("Dry run: would install Hub extension %q (latest version)", extensionID).
				Detail("version", "latest")
		}
		return report.Print()
	}

	_, c, err := SetupWithSafety(safety.OperationCreate)
	if err != nil {
		return err
	}

	handler := extension.NewHandler(c)
	result, err := handler.InstallFromHub(extensionID, version)
	if err != nil {
		return err
	}

	output.PrintSuccess("Hub extension installed")
	output.PrintInfo("  Name:    %s", result.ExtensionName)
	output.PrintInfo("  Version: %s", result.Version)
	return nil
}

func init() {
	createExtensionCmd.Flags().StringP("file", "f", "", "path to the extension zip file (for custom extension upload), or - for stdin")
	createExtensionCmd.Flags().String("hub-extension", "", "Hub extension catalog ID to install (e.g. com.dynatrace.extension.host-monitoring)")
	createExtensionCmd.Flags().String("version", "", "version to install (only for --hub-extension; defaults to latest)")
}

// Declared stable: the invocation and output contract of this command is
// additive-only. Stable is never implied -- see AGENTS.md "Stability Tiers".
func init() {
	stability.MarkStable(createExtensionCmd)
}

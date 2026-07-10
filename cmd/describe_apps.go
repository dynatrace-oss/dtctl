package cmd

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/appengine"
)

// describeAppCmd shows detailed info about an app
var describeAppCmd = &cobra.Command{
	Use:     "app <app-id>",
	Aliases: []string{"apps"},
	Short:   "Show details of an App Engine app",
	Long: `Show detailed information about an App Engine app.

Examples:
  # Describe an app
  dtctl describe app my.custom-app
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		appID := args[0]

		_, c, printer, err := Setup()
		if err != nil {
			return err
		}

		handler := appengine.NewHandler(c)

		app, err := handler.GetApp(appID)
		if err != nil {
			return err
		}

		// For table output, show detailed human-readable information
		if outputFormat == "table" {
			const w = 13
			output.DescribeKV("ID:", w, "%s", app.ID)
			output.DescribeKV("Name:", w, "%s", app.Name)
			output.DescribeKV("Version:", w, "%s", app.Version)
			output.DescribeKV("Description:", w, "%s", app.Description)
			output.DescribeKV("Builtin:", w, "%v", app.IsBuiltin)

			if app.ResourceStatus != nil {
				output.DescribeKV("Status:", w, "%s", app.ResourceStatus.Status)
				if len(app.ResourceStatus.SubResourceTypes) > 0 {
					output.DescribeKV("Resources:", w, "%s", strings.Join(app.ResourceStatus.SubResourceTypes, ", "))
				}
			}

			if app.ModificationInfo != nil {
				if app.ModificationInfo.CreatedAt != "" {
					output.DescribeKV("Created:", w, "%s (by %s)", app.ModificationInfo.CreatedAt, app.ModificationInfo.CreatedBy)
				}
				if app.ModificationInfo.LastModifiedAt != "" {
					output.DescribeKV("Modified:", w, "%s (by %s)", app.ModificationInfo.LastModifiedAt, app.ModificationInfo.LastModifiedBy)
				}
			}

			return nil
		}

		// For other formats, use standard printer
		enrichAgent(printer, "describe", "app")
		return printer.Print(app)
	},
}

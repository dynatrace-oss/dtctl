package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/prompt"
	"github.com/dynatrace-oss/dtctl/pkg/resources/appengine"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/dynatrace-oss/dtctl/pkg/stability"
)

// getAppsCmd retrieves App Engine apps
var getAppsCmd = &cobra.Command{
	Use:     "apps [id]",
	Aliases: []string{"app"},
	Short:   "Get App Engine apps",
	Long: `Get installed App Engine apps.

Examples:
  # List all apps
  dtctl get apps

  # Get a specific app
  dtctl get app my.custom-app

  # Output as JSON
  dtctl get apps -o json
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, c, printer, err := Setup()
		if err != nil {
			return err
		}

		handler := appengine.NewHandler(c)

		// Get specific app if ID provided
		if len(args) > 0 {
			app, err := handler.GetApp(args[0])
			if err != nil {
				return err
			}
			return printer.Print(app)
		}

		// List all apps
		list, err := handler.ListApps()
		if err != nil {
			return err
		}

		return printer.PrintList(list.Apps)
	},
}

// getSDKVersionsCmd retrieves SDK versions for the function executor
var getSDKVersionsCmd = &cobra.Command{
	Use:     "sdk-versions",
	Aliases: []string{"sdk-version"},
	Short:   "Get available SDK versions for function execution",
	Long: `Get available SDK versions for the function executor.

Examples:
  # List all SDK versions
  dtctl get sdk-versions

  # Output as JSON
  dtctl get sdk-versions -o json
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, c, printer, err := Setup()
		if err != nil {
			return err
		}

		handler := appengine.NewFunctionHandler(c)

		versions, err := handler.GetSDKVersions()
		if err != nil {
			return err
		}

		return printer.PrintList(versions.Versions)
	},
}

// deleteAppCmd deletes an app
var deleteAppCmd = &cobra.Command{
	Use:     "app <app-id>",
	Aliases: []string{"apps"},
	Short:   "Uninstall an App Engine app",
	Long: `Uninstall an App Engine app by ID.

Examples:
  # Uninstall an app
  dtctl delete app my.custom-app

  # Uninstall without confirmation
  dtctl delete app my.custom-app -y
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		appID := args[0]

		_, c, err := SetupWithSafety(safety.OperationDelete)
		if err != nil {
			return err
		}

		handler := appengine.NewHandler(c)

		// Get app for confirmation
		app, err := handler.GetApp(appID)
		if err != nil {
			return err
		}

		if dryRun {
			return deleteDryRun(cmd, "app", app.Name, appID)
		}

		// Confirm deletion unless --force or --plain
		if !forceDelete && !plainMode {
			if !prompt.ConfirmDeletion("app", app.Name, appID) {
				fmt.Println("Deletion cancelled")
				return nil
			}
		}

		if err := handler.DeleteApp(appID); err != nil {
			return err
		}

		output.PrintSuccess("App %q uninstall initiated", appID)
		return nil
	},
}

func init() {
	// Delete confirmation flags
	deleteAppCmd.Flags().BoolVarP(&forceDelete, "yes", "y", false, "Skip confirmation prompt")
}

// Declared stable: the invocation and output contract of these commands is
// additive-only. Stable is never implied -- see AGENTS.md "Stability Tiers".
func init() {
	stability.MarkStable(deleteAppCmd)
	stability.MarkStable(getAppsCmd)
	stability.MarkStable(getSDKVersionsCmd)
}

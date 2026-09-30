package cmd

import (
	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/resources/appengine"
	"github.com/dynatrace-oss/dtctl/pkg/stability"
)

var (
	getActionsAppFilter string
	getActionsFilter    string
)

// getActionsCmd retrieves App Engine actions
var getActionsCmd = &cobra.Command{
	Use:     "actions [app-id/action-name]",
	Aliases: []string{"action"},
	Short:   "Get App Engine actions",
	Long: `Get app actions from installed apps.

Actions are declared in an app's manifest and describe the operations the
app exposes, for example to workflow tasks.

--filter is a whitespace-separated list of search terms, matched
case-insensitively against the app name and description and the action name,
title and description. Each additional term narrows the result further.

Examples:
  # List all actions across all apps
  dtctl get actions

  # List actions for a specific app
  dtctl get actions --app dynatrace.automations

  # Search actions -- each term must match, so terms narrow the result
  dtctl get actions --filter jira
  dtctl get actions --filter 'jira create'

  # Get a specific action
  dtctl get action dynatrace.automations/execute-dql-query

  # Output as JSON
  dtctl get actions -o json

  # Wide output (shows app ID, description and stateful flag)
  dtctl get actions -o wide
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, c, printer, err := Setup()
		if err != nil {
			return err
		}

		handler := appengine.NewHandler(c)

		// Get specific action if app-id/action-name provided
		if len(args) > 0 {
			action, err := handler.GetAction(args[0])
			if err != nil {
				return err
			}
			return printer.Print(action)
		}

		// List actions
		actions, err := handler.ListActions(appengine.ListActionsOptions{
			AppID: getActionsAppFilter,
			Query: getActionsFilter,
		})
		if err != nil {
			return err
		}

		return printer.PrintList(actions)
	},
}

func init() {
	getActionsCmd.Flags().StringVar(&getActionsAppFilter, "app", "", "filter by app ID")
	getActionsCmd.Flags().StringVar(&getActionsFilter, "filter", "", "search terms, matched against app and action name, title and description")
}

// Declared stable: the invocation and output contract of this command is
// additive-only. Stable is never implied -- see AGENTS.md "Stability Tiers".
func init() {
	stability.MarkStable(getActionsCmd)
}

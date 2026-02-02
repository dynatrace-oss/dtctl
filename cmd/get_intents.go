package cmd

import (
	"github.com/dynatrace-oss/dtctl/pkg/resources/appengine"
	"github.com/spf13/cobra"
)

var (
	getIntentsAppFilter string
)

// getIntentsCmd retrieves App Engine intents
var getIntentsCmd = &cobra.Command{
	Use:     "intents [app-id/intent-id]",
	Aliases: []string{"intent"},
	Short:   "Get App Engine intents",
	Long: `Get app intents from installed apps.

Intents enable inter-app communication by defining entry points
that apps expose for opening resources with contextual data.

Examples:
  # List all intents across all apps
  dtctl get intents

  # List intents for a specific app
  dtctl get intents --app dynatrace.distributedtracing

  # Get a specific intent
  dtctl get intent dynatrace.distributedtracing/view-trace

  # Output as JSON
  dtctl get intents -o json

  # Wide output (shows app ID and required properties)
  dtctl get intents -o wide
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := appengine.NewIntentHandler(c)
		printer := NewPrinter()

		// Get specific intent if app-id/intent-id provided
		if len(args) > 0 {
			intent, err := handler.GetIntent(args[0])
			if err != nil {
				return err
			}
			return printer.Print(intent)
		}

		// List intents
		intents, err := handler.ListIntents(getIntentsAppFilter)
		if err != nil {
			return err
		}

		return printer.PrintList(intents)
	},
}

func init() {
	getIntentsCmd.Flags().StringVar(&getIntentsAppFilter, "app", "", "filter by app ID")
}

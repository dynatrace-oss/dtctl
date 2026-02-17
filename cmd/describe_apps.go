package cmd

import (
	"fmt"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/resources/appengine"
	"github.com/spf13/cobra"
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

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := appengine.NewHandler(c)

		app, err := handler.GetApp(appID)
		if err != nil {
			return err
		}

		// Print app details
		fmt.Printf("ID:          %s\n", app.ID)
		fmt.Printf("Name:        %s\n", app.Name)
		fmt.Printf("Version:     %s\n", app.Version)
		fmt.Printf("Description: %s\n", app.Description)
		fmt.Printf("Builtin:     %v\n", app.IsBuiltin)

		if app.ResourceStatus != nil {
			fmt.Printf("Status:      %s\n", app.ResourceStatus.Status)
			if len(app.ResourceStatus.SubResourceTypes) > 0 {
				fmt.Printf("Resources:   %s\n", strings.Join(app.ResourceStatus.SubResourceTypes, ", "))
			}
		}

		if app.ModificationInfo != nil {
			if app.ModificationInfo.CreatedTime != "" {
				fmt.Printf("Created:     %s (by %s)\n", app.ModificationInfo.CreatedTime, app.ModificationInfo.CreatedBy)
			}
			if app.ModificationInfo.LastModifiedTime != "" {
				fmt.Printf("Modified:    %s (by %s)\n", app.ModificationInfo.LastModifiedTime, app.ModificationInfo.LastModifiedBy)
			}
		}

		return nil
	},
}

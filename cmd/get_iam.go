package cmd

import (
	"github.com/dynatrace-oss/dtctl/pkg/resources/iam"
	"github.com/spf13/cobra"
)

// getUsersCmd retrieves IAM users
var getUsersCmd = &cobra.Command{
	Use:     "users [uuid]",
	Aliases: []string{"user"},
	Short:   "Get IAM users",
	Long: `Get users from Identity and Access Management.

Examples:
  # List all users
  dtctl get users

  # Get a specific user by UUID
  dtctl get user <user-uuid>

  # Filter users by email or name
  dtctl get users --filter "john"

  # Output as JSON
  dtctl get users -o json
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

		handler := iam.NewHandler(c)
		printer := NewPrinter()

		// Get specific user if UUID provided
		if len(args) > 0 {
			user, err := handler.GetUser(args[0])
			if err != nil {
				return err
			}
			return printer.Print(user)
		}

		// List all users with optional filter
		filterStr, _ := cmd.Flags().GetString("filter")
		list, err := handler.ListUsers(filterStr, nil, GetChunkSize())
		if err != nil {
			return err
		}

		return printer.PrintList(list.Results)
	},
}

// getGroupsCmd retrieves IAM groups
var getGroupsCmd = &cobra.Command{
	Use:     "groups [uuid]",
	Aliases: []string{"group"},
	Short:   "Get IAM groups",
	Long: `Get groups from Identity and Access Management.

Examples:
  # List all groups
  dtctl get groups

  # Filter groups by name
  dtctl get groups --filter "admin"

  # Output as JSON
  dtctl get groups -o json
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

		handler := iam.NewHandler(c)
		printer := NewPrinter()

		// List all groups with optional filter
		filterStr, _ := cmd.Flags().GetString("filter")
		list, err := handler.ListGroups(filterStr, nil, GetChunkSize())
		if err != nil {
			return err
		}

		return printer.PrintList(list.Results)
	},
}

func init() {
	// IAM flags
	getUsersCmd.Flags().String("filter", "", "Filter users by email or name (partial match)")
	getGroupsCmd.Flags().String("filter", "", "Filter groups by name (partial match)")
}

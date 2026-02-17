package cmd

import (
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/resources/iam"
	"github.com/spf13/cobra"
)

// describeUserCmd shows detailed info about a user
var describeUserCmd = &cobra.Command{
	Use:     "user <user-uuid>",
	Aliases: []string{"users"},
	Short:   "Show details of an IAM user",
	Long: `Show detailed information about an IAM user.

Examples:
  # Describe a user by UUID
  dtctl describe user <user-uuid>
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		userUUID := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := iam.NewHandler(c)

		user, err := handler.GetUser(userUUID)
		if err != nil {
			return err
		}

		// Print user details
		fmt.Printf("UUID:        %s\n", user.UID)
		fmt.Printf("Email:       %s\n", user.Email)
		if user.Name != "" {
			fmt.Printf("Name:        %s\n", user.Name)
		}
		if user.Surname != "" {
			fmt.Printf("Surname:     %s\n", user.Surname)
		}
		if user.Description != "" {
			fmt.Printf("Description: %s\n", user.Description)
		}

		return nil
	},
}

// describeGroupCmd shows detailed info about a group
var describeGroupCmd = &cobra.Command{
	Use:     "group <group-uuid>",
	Aliases: []string{"groups"},
	Short:   "Show details of an IAM group",
	Long: `Show detailed information about an IAM group.

Examples:
  # List all groups to find UUID, then describe
  dtctl get groups
  dtctl describe group <group-uuid>
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		groupUUID := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := iam.NewHandler(c)

		// Since there's no get single group endpoint, we list and filter
		list, err := handler.ListGroups("", []string{groupUUID}, GetChunkSize())
		if err != nil {
			return err
		}

		if len(list.Results) == 0 {
			return fmt.Errorf("group %q not found", groupUUID)
		}

		group := list.Results[0]

		// Print group details
		fmt.Printf("UUID:      %s\n", group.UUID)
		fmt.Printf("Name:      %s\n", group.GroupName)
		fmt.Printf("Type:      %s\n", group.Type)

		return nil
	},
}

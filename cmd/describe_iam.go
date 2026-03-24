package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/iam"
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

		// For table output, show detailed human-readable information
		if outputFormat == "table" {
			const w = 13
			output.DescribeKV("UUID:", w, "%s", user.UID)
			output.DescribeKV("Email:", w, "%s", user.Email)
			if user.Name != "" {
				output.DescribeKV("Name:", w, "%s", user.Name)
			}
			if user.Surname != "" {
				output.DescribeKV("Surname:", w, "%s", user.Surname)
			}
			if user.Description != "" {
				output.DescribeKV("Description:", w, "%s", user.Description)
			}

			return nil
		}

		// For other formats, use standard printer
		printer := NewPrinter()
		enrichAgent(printer, "describe", "user")
		return printer.Print(user)
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

		// For table output, show detailed human-readable information
		if outputFormat == "table" {
			const w = 11
			output.DescribeKV("UUID:", w, "%s", group.UUID)
			output.DescribeKV("Name:", w, "%s", group.GroupName)
			output.DescribeKV("Type:", w, "%s", group.Type)

			return nil
		}

		// For other formats, use standard printer
		printer := NewPrinter()
		enrichAgent(printer, "describe", "group")
		return printer.Print(group)
	},
}

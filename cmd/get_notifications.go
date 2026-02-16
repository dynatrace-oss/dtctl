package cmd

import (
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/prompt"
	"github.com/dynatrace-oss/dtctl/pkg/resources/notification"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/spf13/cobra"
)

// getNotificationsCmd retrieves notifications
var getNotificationsCmd = &cobra.Command{
	Use:     "notifications [id]",
	Aliases: []string{"notification", "notif"},
	Short:   "Get event notifications",
	Long: `Get event notifications.

Examples:
  # List all event notifications
  dtctl get notifications

  # Get a specific notification
  dtctl get notification <notification-id>

  # Filter by notification type
  dtctl get notifications --type my-notification-type

  # Output as JSON
  dtctl get notifications -o json
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		notifType, _ := cmd.Flags().GetString("type")

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := notification.NewHandler(c)
		printer := NewPrinter()

		// Get specific notification if ID provided
		if len(args) > 0 {
			n, err := handler.GetEventNotification(args[0])
			if err != nil {
				return err
			}
			return printer.Print(n)
		}

		// List all notifications
		list, err := handler.ListEventNotifications(notifType)
		if err != nil {
			return err
		}

		return printer.PrintList(list.Results)
	},
}

// deleteNotificationCmd deletes a notification
var deleteNotificationCmd = &cobra.Command{
	Use:   "notification <notification-id>",
	Short: "Delete an event notification",
	Long: `Delete an event notification by ID.

Examples:
  # Delete a notification
  dtctl delete notification <notification-id>

  # Delete without confirmation
  dtctl delete notification <notification-id> -y
`,
	Aliases: []string{"notif"},
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		notifID := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := notification.NewHandler(c)

		// Get notification for confirmation and ownership check
		n, err := handler.GetEventNotification(notifID)
		if err != nil {
			return err
		}

		// Safety check with actual ownership
		checker, err := NewSafetyChecker(cfg)
		if err != nil {
			return err
		}
		currentUserID, _ := c.CurrentUserID()
		ownership := safety.DetermineOwnership(n.Owner, currentUserID)
		if err := checker.CheckError(safety.OperationDelete, ownership); err != nil {
			return err
		}

		// Confirm deletion unless --force or --plain
		if !forceDelete && !plainMode {
			if !prompt.ConfirmDeletion("notification", n.NotificationType, notifID) {
				fmt.Println("Deletion cancelled")
				return nil
			}
		}

		if err := handler.DeleteEventNotification(notifID); err != nil {
			return err
		}

		fmt.Printf("Notification %q deleted\n", notifID)
		return nil
	},
}

func init() {
	addWatchFlags(getNotificationsCmd)

	// Notification flags
	getNotificationsCmd.Flags().String("type", "", "Filter by notification type")

	// Delete confirmation flags
	deleteNotificationCmd.Flags().BoolVarP(&forceDelete, "yes", "y", false, "Skip confirmation prompt")
}

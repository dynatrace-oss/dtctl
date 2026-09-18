package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// validateSnapshotFlags rejects --snapshot-description without --create-snapshot.
// The description is inert on its own (the API only reads it alongside the
// create-snapshot parameter), and silently dropping it would leave the user
// without the snapshot they were trying to label.
func validateSnapshotFlags(cmd *cobra.Command) error {
	createSnapshot, _ := cmd.Flags().GetBool("create-snapshot")
	snapshotDescription, _ := cmd.Flags().GetString("snapshot-description")
	if snapshotDescription != "" && !createSnapshot {
		return fmt.Errorf("--snapshot-description requires --create-snapshot")
	}
	return nil
}

package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/watch"
	"github.com/spf13/cobra"
)

// getCmd represents the get command
var getCmd = &cobra.Command{
	Use:   "get",
	Short: "Display one or many resources",
	Long:  `Display one or many resources such as workflows, dashboards, notebooks, SLOs, etc.`,
	RunE:  requireSubcommand,
}

// deleteCmd represents the delete command
var deleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete resources",
	Long:  `Delete one or more resources.`,
	RunE:  requireSubcommand,
}

// updateCmd represents the update command
var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update resources",
	Long:  `Update resources from files.`,
	RunE:  requireSubcommand,
}

// forceDelete skips confirmation prompts
var forceDelete bool

// executeWithWatch wraps a fetcher function with watch mode support
func executeWithWatch(cmd *cobra.Command, fetcher watch.ResourceFetcher, printer interface{}) error {
	watchMode, _ := cmd.Flags().GetBool("watch")
	if !watchMode {
		return nil
	}

	interval, _ := cmd.Flags().GetDuration("interval")
	watchOnly, _ := cmd.Flags().GetBool("watch-only")

	if interval < time.Second {
		interval = 2 * time.Second
	}

	cfg, err := LoadConfig()
	if err != nil {
		return err
	}

	c, err := NewClientFromConfig(cfg)
	if err != nil {
		return err
	}

	// Wrap the printer with WatchPrinter to maintain output format
	basePrinter := printer.(output.Printer)
	watchPrinter := output.NewWatchPrinter(basePrinter)

	watcher := watch.NewWatcher(watch.WatcherOptions{
		Interval:    interval,
		Client:      c,
		Fetcher:     fetcher,
		Printer:     watchPrinter,
		ShowInitial: !watchOnly,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	return watcher.Start(ctx)
}

// addWatchFlags adds watch-related flags to a command
func addWatchFlags(cmd *cobra.Command) {
	cmd.Flags().Bool("watch", false, "Watch for changes")
	cmd.Flags().Duration("interval", 2*time.Second, "Polling interval (minimum: 1s)")
	cmd.Flags().Bool("watch-only", false, "Only show changes, not initial state")
}

func init() {
	rootCmd.AddCommand(getCmd)
	rootCmd.AddCommand(deleteCmd)
	rootCmd.AddCommand(updateCmd)

	// Get subcommands (command definitions live in get_*.go files)
	getCmd.AddCommand(getWorkflowsCmd)
	getCmd.AddCommand(getWorkflowExecutionsCmd)
	getCmd.AddCommand(getDashboardsCmd)
	getCmd.AddCommand(getNotebooksCmd)
	getCmd.AddCommand(getTrashCmd)
	getCmd.AddCommand(getSLOsCmd)
	getCmd.AddCommand(getSLOTemplatesCmd)
	getCmd.AddCommand(getNotificationsCmd)
	getCmd.AddCommand(getBucketsCmd)
	getCmd.AddCommand(getLookupsCmd)
	getCmd.AddCommand(getAppsCmd)
	getCmd.AddCommand(getFunctionsCmd)
	getCmd.AddCommand(getIntentsCmd)
	getCmd.AddCommand(getEdgeConnectsCmd)
	getCmd.AddCommand(getUsersCmd)
	getCmd.AddCommand(getGroupsCmd)
	getCmd.AddCommand(getSDKVersionsCmd)
	getCmd.AddCommand(getAnalyzersCmd)
	getCmd.AddCommand(getCopilotSkillsCmd)
	getCmd.AddCommand(getSettingsSchemasCmd)
	getCmd.AddCommand(getSettingsCmd)

	// Delete subcommands (command definitions live in get_*.go files)
	deleteCmd.AddCommand(deleteWorkflowCmd)
	deleteCmd.AddCommand(deleteDashboardCmd)
	deleteCmd.AddCommand(deleteNotebookCmd)
	deleteCmd.AddCommand(deleteTrashCmd)
	deleteCmd.AddCommand(deleteSLOCmd)
	deleteCmd.AddCommand(deleteNotificationCmd)
	deleteCmd.AddCommand(deleteBucketCmd)
	deleteCmd.AddCommand(deleteLookupCmd)
	deleteCmd.AddCommand(deleteSettingsCmd)
	deleteCmd.AddCommand(deleteAppCmd)
	deleteCmd.AddCommand(deleteEdgeConnectCmd)
}

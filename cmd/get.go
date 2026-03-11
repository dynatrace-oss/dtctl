package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/watch"
)

// getCmd represents the get command
var getCmd = &cobra.Command{
	Use:   "get",
	Short: "Display one or many resources",
	Long: `Display one or many resources from the Dynatrace platform.

When called with a resource type only, lists all resources of that type.
When called with a resource type and ID or name, retrieves that specific resource.
Results can be filtered with --mine and formatted with -o (json, yaml, wide, chart).

Supported resources:
  workflows (wf)          dashboards (dash, db)     notebooks (nb)
  slos                    slo-templates             settings
  settings-schemas        buckets (bkt)             apps
  functions               intents                   notifications
  users                   groups                    edgeconnect (ec)
  sdk-versions            analyzers                 copilot-skills
  lookup-tables (lu)      trash                     workflow-executions (wfe)

Use 'dtctl get <resource> --help' for resource-specific options.`,
	Example: `  # List all workflows
  dtctl get workflows

  # Get a specific workflow by name or ID
  dtctl get workflow my-workflow

  # List workflows as JSON
  dtctl get workflows -o json

  # List only your own workflows
  dtctl get workflows --mine

  # Watch workflows for changes in real-time
  dtctl get workflows --watch

  # List with wide output (extra columns)
  dtctl get workflows -o wide`,
	RunE: requireSubcommand,
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
	getCmd.AddCommand(getExtensionsCmd)
	getCmd.AddCommand(getExtensionConfigsCmd)
	getCmd.AddCommand(getDocumentsCmd)

	// Delete subcommands (command definitions live in get_*.go files)
	updateCmd.AddCommand(updateExtensionConfigCmd)

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
	deleteCmd.AddCommand(deleteDocumentCmd)
}

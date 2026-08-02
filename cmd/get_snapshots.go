package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/exec"
	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/livedebugger"
)

var getSnapshotsCmd = &cobra.Command{
	Use:   "snapshots <breakpoint>",
	Short: "Get Live Debugger snapshots for a breakpoint (experimental)",
	Long: `Fetch application snapshots captured by a Live Debugger breakpoint.

BREAKPOINT can be specified as:
  - filename:line  e.g. OrderController.java:306
  - stable rule ID e.g. dtctl-rule-5bfb45a29fce7a46

The command looks up the breakpoint's internal DQL ID, then executes:
  fetch application.snapshots | filter breakpoint.id == toUid("<id>")

Use --decode-snapshots to decode snapshot payloads (same as dtctl query).
Use -o json / -o yaml for structured output.

Note: Live Debugger support is experimental. The underlying APIs and query
behavior may change in future releases.`,
	Example: `  # Get snapshots for a breakpoint by location
  dtctl get snapshots OrderController.java:306

  # Get snapshots by stable rule ID
  dtctl get snapshots dtctl-rule-5bfb45a29fce7a46

  # Decode snapshot payloads into readable fields
  dtctl get snapshots OrderController.java:306 --decode-snapshots

  # Output as JSON
  dtctl get snapshots OrderController.java:306 -o json

  # Limit number of results
  dtctl get snapshots OrderController.java:306 --limit 50

  # Scope to a time window
  dtctl get snapshots OrderController.java:306 \
    --default-timeframe-start 2024-01-01T00:00:00Z \
    --default-timeframe-end   2024-01-02T00:00:00Z`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, c, err := SetupClient()
		if err != nil {
			return err
		}

		ctx, ctxErr := cfg.CurrentContextObj()
		if ctxErr != nil {
			return ctxErr
		}

		handler, err := livedebugger.NewHandler(c, ctx.Environment)
		if err != nil {
			return err
		}

		workspaceResp, workspaceID, err := handler.GetOrCreateWorkspace(currentProjectPath())
		if err != nil {
			if isDebugVerbose() {
				_ = printGraphQLResponse("getOrCreateWorkspaceV2", workspaceResp)
			}
			return err
		}

		workspaceRulesResp, err := handler.GetWorkspaceRules(workspaceID)
		if err != nil {
			return err
		}

		rules, err := livedebugger.ExtractWorkspaceRules(workspaceRulesResp)
		if err != nil {
			return err
		}

		identifier := args[0]
		matchedRules, _, _, err := resolveBreakpointRulesForEdit(rules, identifier)
		if err != nil {
			return err
		}
		if len(matchedRules) == 0 {
			return fmt.Errorf("no breakpoint found with identifier %q", identifier)
		}

		rule := matchedRules[0]
		if rule.ImmutableID == "" {
			return fmt.Errorf("breakpoint %q has no snapshot ID", identifier)
		}
		dqlID := padBreakpointID(rule.ImmutableID)
		dqlQuery := fmt.Sprintf(`fetch application.snapshots | filter breakpoint.id == toUid("%s")`, dqlID)
		if limit, _ := cmd.Flags().GetInt("limit"); limit > 0 {
			dqlQuery += fmt.Sprintf(" | limit %d", limit)
		}

		fmt.Fprintln(os.Stderr, "Note: Snapshots from the latest version of the breakpoint will be shown")

		decodeVal, _ := cmd.Flags().GetString("decode-snapshots")
		var decodeMode exec.DecodeMode
		if cmd.Flags().Changed("decode-snapshots") {
			switch decodeVal {
			case "", "simplified":
				decodeMode = exec.DecodeSimplified
			case "full":
				decodeMode = exec.DecodeFull
			default:
				return fmt.Errorf("unsupported --decode-snapshots value %q (use \"simplified\" or \"full\")", decodeVal)
			}
		}

		maxResultRecords, _ := cmd.Flags().GetInt64("max-result-records")
		defaultTimeframeStart, _ := cmd.Flags().GetString("default-timeframe-start")
		defaultTimeframeEnd, _ := cmd.Flags().GetString("default-timeframe-end")
		noProgress, _ := cmd.Flags().GetBool("no-progress")

		// In agent mode, always include metadata unless explicitly disabled.
		var metadataFields []string
		if agentMode && !cmd.Flags().Changed("metadata") {
			metadataFields = []string{"all"}
		} else if cmd.Flags().Changed("metadata") {
			metaVal, _ := cmd.Flags().GetString("metadata")
			metadataFields, err = output.ParseMetadataFields(metaVal)
			if err != nil {
				return err
			}
		}

		executor := NewDQLExecutorFromConfig(cfg, c)

		cancelCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(sigCh)
		go func() {
			<-sigCh
			cancel()
		}()

		opts := exec.DQLExecuteOptions{
			OutputFormat:          outputFormat,
			AgentMode:             agentMode,
			Decode:                decodeMode,
			MaxResultRecords:      maxResultRecords,
			DefaultTimeframeStart: defaultTimeframeStart,
			DefaultTimeframeEnd:   defaultTimeframeEnd,
			MetadataFields:        metadataFields,
			ShowProgress:          !noProgress,
		}

		return executor.ExecuteWithContext(cancelCtx, dqlQuery, opts)
	},
}

func init() {
	getSnapshotsCmd.Flags().Int("limit", 0, "maximum number of snapshots to return (0 = no limit)")

	getSnapshotsCmd.Flags().String("decode-snapshots", "", `(experimental) decode Live Debugger snapshot payloads in query results
bare --decode-snapshots simplifies variant wrappers to plain values;
--decode-snapshots=full preserves the full decoded tree with type annotations`)
	getSnapshotsCmd.Flags().Lookup("decode-snapshots").NoOptDefVal = "simplified"

	getSnapshotsCmd.Flags().Int64("max-result-records", 0, "maximum number of result records to return (0 = use default, typically 1000)")
	getSnapshotsCmd.Flags().String("default-timeframe-start", "", "query timeframe start timestamp (ISO-8601/RFC3339, e.g., '2022-04-20T12:10:04.123Z')")
	getSnapshotsCmd.Flags().String("default-timeframe-end", "", "query timeframe end timestamp (ISO-8601/RFC3339, e.g., '2022-04-20T13:10:04.123Z')")
	getSnapshotsCmd.Flags().Bool("no-progress", false, "disable the live progress bar shown on stderr for long queries")
	getSnapshotsCmd.Flags().StringP("metadata", "M", "", `include query metadata in output (use = for field selection)
bare --metadata or -M shows all fields; --metadata=field1,field2 selects specific fields`)
	getSnapshotsCmd.Flags().Lookup("metadata").NoOptDefVal = "all"
}

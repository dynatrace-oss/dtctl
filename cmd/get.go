package cmd

import (
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/prompt"
	"github.com/dynatrace-oss/dtctl/pkg/resources/analyzer"
	"github.com/dynatrace-oss/dtctl/pkg/resources/appengine"
	"github.com/dynatrace-oss/dtctl/pkg/resources/bucket"
	"github.com/dynatrace-oss/dtctl/pkg/resources/copilot"
	"github.com/dynatrace-oss/dtctl/pkg/resources/document"
	"github.com/dynatrace-oss/dtctl/pkg/resources/edgeconnect"
	"github.com/dynatrace-oss/dtctl/pkg/resources/iam"
	"github.com/dynatrace-oss/dtctl/pkg/resources/lookup"
	"github.com/dynatrace-oss/dtctl/pkg/resources/notification"
	"github.com/dynatrace-oss/dtctl/pkg/resources/resolver"
	"github.com/dynatrace-oss/dtctl/pkg/resources/settings"
	"github.com/dynatrace-oss/dtctl/pkg/resources/slo"
	"github.com/dynatrace-oss/dtctl/pkg/resources/workflow"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/spf13/cobra"
)

// getCmd represents the get command
var getCmd = &cobra.Command{
	Use:   "get",
	Short: "Display one or many resources",
	Long:  `Display one or many resources such as workflows, dashboards, notebooks, SLOs, etc.`,
	RunE:  requireSubcommand,
}

// getWorkflowsCmd retrieves workflows
var getWorkflowsCmd = &cobra.Command{
	Use:     "workflows [id]",
	Aliases: []string{"workflow", "wf"},
	Short:   "Get workflows",
	Long: `Get one or more workflows.

Examples:
  # List all workflows
  dtctl get workflows

  # Get a specific workflow
  dtctl get workflow <workflow-id>

  # Output as JSON
  dtctl get workflows -o json

  # List only my workflows
  dtctl get workflows --mine
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

		handler := workflow.NewHandler(c)
		printer := NewPrinter()

		// Get specific workflow if ID provided
		if len(args) > 0 {
			wf, err := handler.Get(args[0])
			if err != nil {
				return err
			}
			return printer.Print(wf)
		}

		// List workflows with filters
		mineOnly, _ := cmd.Flags().GetBool("mine")

		filters := workflow.WorkflowFilters{}

		// If --mine flag is set, get current user ID and filter by owner
		if mineOnly {
			userID, err := c.CurrentUserID()
			if err != nil {
				return fmt.Errorf("failed to get current user ID for --mine filter: %w", err)
			}
			filters.Owner = userID
		}

		list, err := handler.List(filters)
		if err != nil {
			return err
		}

		return printer.PrintList(list.Results)
	},
}

// workflowFilter holds the workflow ID filter for executions
var workflowFilter string

// forceDelete skips confirmation prompts
var forceDelete bool

// getWorkflowExecutionsCmd retrieves workflow executions
var getWorkflowExecutionsCmd = &cobra.Command{
	Use:     "workflow-executions [id]",
	Aliases: []string{"workflow-execution", "wfe"},
	Short:   "Get workflow executions",
	Long: `Get one or more workflow executions.

Examples:
  # List all workflow executions
  dtctl get workflow-executions
  dtctl get wfe

  # List executions for a specific workflow
  dtctl get wfe --workflow <workflow-id>
  dtctl get wfe -w <workflow-id>

  # Get a specific execution
  dtctl get wfe <execution-id>

  # Output as JSON
  dtctl get wfe -o json
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

		handler := workflow.NewExecutionHandler(c)
		printer := NewPrinter()

		// Get specific execution if ID provided
		if len(args) > 0 {
			exec, err := handler.Get(args[0])
			if err != nil {
				return err
			}
			return printer.Print(exec)
		}

		// List executions (optionally filtered by workflow)
		list, err := handler.List(workflowFilter)
		if err != nil {
			return err
		}

		return printer.PrintList(list.Results)
	},
}

// getDashboardsCmd retrieves dashboards
var getDashboardsCmd = &cobra.Command{
	Use:     "dashboards [id]",
	Aliases: []string{"dashboard", "dash", "db"},
	Short:   "Get dashboards",
	Long: `Get one or more dashboards.

Examples:
  # List all dashboards
  dtctl get dashboards
  dtctl get dash

  # Get a specific dashboard
  dtctl get dashboard <dashboard-id>

  # Output as JSON
  dtctl get dashboards -o json

  # Filter by name
  dtctl get dashboards --name "production"

  # List only my dashboards
  dtctl get dashboards --mine
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

		handler := document.NewHandler(c)
		printer := NewPrinter()

		// Get specific dashboard if ID provided
		if len(args) > 0 {
			doc, err := handler.Get(args[0])
			if err != nil {
				return err
			}
			return printer.Print(doc)
		}

		// List all dashboards
		nameFilter, _ := cmd.Flags().GetString("name")
		mineOnly, _ := cmd.Flags().GetBool("mine")

		filters := document.DocumentFilters{
			Type:      "dashboard",
			Name:      nameFilter,
			ChunkSize: GetChunkSize(),
		}

		// If --mine flag is set, get current user ID and filter by owner
		if mineOnly {
			userID, err := c.CurrentUserID()
			if err != nil {
				return fmt.Errorf("failed to get current user ID for --mine filter: %w", err)
			}
			filters.Owner = userID
		}

		list, err := handler.List(filters)
		if err != nil {
			return err
		}

		// Convert metadata list to documents for table display
		docs := document.ConvertToDocuments(list)
		return printer.PrintList(docs)
	},
}

// getNotebooksCmd retrieves notebooks
var getNotebooksCmd = &cobra.Command{
	Use:     "notebooks [id]",
	Aliases: []string{"notebook", "nb"},
	Short:   "Get notebooks",
	Long: `Get one or more notebooks.

Examples:
  # List all notebooks
  dtctl get notebooks
  dtctl get nb

  # Get a specific notebook
  dtctl get notebook <notebook-id>

  # Output as JSON
  dtctl get notebooks -o json

  # Filter by name
  dtctl get notebooks --name "analysis"

  # List only my notebooks
  dtctl get notebooks --mine
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

		handler := document.NewHandler(c)
		printer := NewPrinter()

		// Get specific notebook if ID provided
		if len(args) > 0 {
			doc, err := handler.Get(args[0])
			if err != nil {
				return err
			}
			return printer.Print(doc)
		}

		// List all notebooks
		nameFilter, _ := cmd.Flags().GetString("name")
		mineOnly, _ := cmd.Flags().GetBool("mine")

		filters := document.DocumentFilters{
			Type:      "notebook",
			Name:      nameFilter,
			ChunkSize: GetChunkSize(),
		}

		// If --mine flag is set, get current user ID and filter by owner
		if mineOnly {
			userID, err := c.CurrentUserID()
			if err != nil {
				return fmt.Errorf("failed to get current user ID for --mine filter: %w", err)
			}
			filters.Owner = userID
		}

		list, err := handler.List(filters)
		if err != nil {
			return err
		}

		// Convert metadata list to documents for table display
		docs := document.ConvertToDocuments(list)
		return printer.PrintList(docs)
	},
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

// deleteWorkflowCmd deletes a workflow
var deleteWorkflowCmd = &cobra.Command{
	Use:     "workflow <workflow-id-or-name>",
	Aliases: []string{"workflows", "wf"},
	Short:   "Delete a workflow",
	Long: `Delete a workflow by ID or name.

Examples:
  # Delete by ID
  dtctl delete workflow a1b2c3d4-e5f6-7890-abcd-ef1234567890

  # Delete by name (interactive disambiguation if multiple matches)
  dtctl delete workflow "My Workflow"

  # Delete without confirmation
  dtctl delete workflow "My Workflow" -y
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		identifier := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		// Safety check
		checker, err := NewSafetyChecker(cfg)
		if err != nil {
			return err
		}
		if err := checker.CheckError(safety.OperationDelete, safety.OwnershipUnknown); err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		// Resolve name to ID
		res := resolver.NewResolver(c)
		workflowID, err := res.ResolveID(resolver.TypeWorkflow, identifier)
		if err != nil {
			return err
		}

		handler := workflow.NewHandler(c)

		// Get workflow details for confirmation
		wf, err := handler.Get(workflowID)
		if err != nil {
			return err
		}

		// Confirm deletion unless --force or --plain
		if !forceDelete && !plainMode {
			if !prompt.ConfirmDeletion("workflow", wf.Title, workflowID) {
				fmt.Println("Deletion cancelled")
				return nil
			}
		}

		if err := handler.Delete(workflowID); err != nil {
			return err
		}

		fmt.Printf("Workflow %q deleted\n", wf.Title)
		return nil
	},
}

// deleteDashboardCmd deletes a dashboard
var deleteDashboardCmd = &cobra.Command{
	Use:     "dashboard <dashboard-id-or-name>",
	Aliases: []string{"dashboards", "dash", "db"},
	Short:   "Delete a dashboard",
	Long: `Delete a dashboard by ID or name.

Examples:
  # Delete by ID
  dtctl delete dashboard a1b2c3d4-e5f6-7890-abcd-ef1234567890

  # Delete by name (interactive disambiguation if multiple matches)
  dtctl delete dashboard "Production Dashboard"

  # Delete without confirmation
  dtctl delete dashboard "Production Dashboard" -y
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		identifier := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		// Safety check
		checker, err := NewSafetyChecker(cfg)
		if err != nil {
			return err
		}
		if err := checker.CheckError(safety.OperationDelete, safety.OwnershipUnknown); err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		// Resolve name to ID
		res := resolver.NewResolver(c)
		dashboardID, err := res.ResolveID(resolver.TypeDashboard, identifier)
		if err != nil {
			return err
		}

		handler := document.NewHandler(c)

		// Get current version for optimistic locking and details for confirmation
		metadata, err := handler.GetMetadata(dashboardID)
		if err != nil {
			return err
		}

		// Confirm deletion unless --force or --plain
		if !forceDelete && !plainMode {
			if !prompt.ConfirmDeletion("dashboard", metadata.Name, dashboardID) {
				fmt.Println("Deletion cancelled")
				return nil
			}
		}

		if err := handler.Delete(dashboardID, metadata.Version); err != nil {
			return err
		}

		fmt.Printf("Dashboard %q deleted (moved to trash)\n", metadata.Name)
		return nil
	},
}

// deleteNotebookCmd deletes a notebook
var deleteNotebookCmd = &cobra.Command{
	Use:     "notebook <notebook-id-or-name>",
	Aliases: []string{"notebooks", "nb"},
	Short:   "Delete a notebook",
	Long: `Delete a notebook by ID or name.

Examples:
  # Delete by ID
  dtctl delete notebook a1b2c3d4-e5f6-7890-abcd-ef1234567890

  # Delete by name (interactive disambiguation if multiple matches)
  dtctl delete notebook "Analysis Notebook"

  # Delete without confirmation
  dtctl delete notebook "Analysis Notebook" -y
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		identifier := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		// Safety check
		checker, err := NewSafetyChecker(cfg)
		if err != nil {
			return err
		}
		if err := checker.CheckError(safety.OperationDelete, safety.OwnershipUnknown); err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		// Resolve name to ID
		res := resolver.NewResolver(c)
		notebookID, err := res.ResolveID(resolver.TypeNotebook, identifier)
		if err != nil {
			return err
		}

		handler := document.NewHandler(c)

		// Get current version for optimistic locking and details for confirmation
		metadata, err := handler.GetMetadata(notebookID)
		if err != nil {
			return err
		}

		// Confirm deletion unless --force or --plain
		if !forceDelete && !plainMode {
			if !prompt.ConfirmDeletion("notebook", metadata.Name, notebookID) {
				fmt.Println("Deletion cancelled")
				return nil
			}
		}

		if err := handler.Delete(notebookID, metadata.Version); err != nil {
			return err
		}

		fmt.Printf("Notebook %q deleted (moved to trash)\n", metadata.Name)
		return nil
	},
}

// getSLOsCmd retrieves SLOs
var getSLOsCmd = &cobra.Command{
	Use:     "slos [id]",
	Aliases: []string{"slo"},
	Short:   "Get service-level objectives",
	Long: `Get service-level objectives.

Examples:
  # List all SLOs
  dtctl get slos

  # Get a specific SLO
  dtctl get slo <slo-id>

  # Filter SLOs by name
  dtctl get slos --filter "name~'production'"

  # Output as JSON
  dtctl get slos -o json
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		filter, _ := cmd.Flags().GetString("filter")

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := slo.NewHandler(c)
		printer := NewPrinter()

		// Get specific SLO if ID provided
		if len(args) > 0 {
			s, err := handler.Get(args[0])
			if err != nil {
				return err
			}
			return printer.Print(s)
		}

		// List all SLOs
		list, err := handler.List(filter, GetChunkSize())
		if err != nil {
			return err
		}

		return printer.PrintList(list.SLOs)
	},
}

// getSLOTemplatesCmd retrieves SLO templates
var getSLOTemplatesCmd = &cobra.Command{
	Use:     "slo-templates [id]",
	Aliases: []string{"slo-template"},
	Short:   "Get SLO objective templates",
	Long: `Get SLO objective templates.

Examples:
  # List all SLO templates
  dtctl get slo-templates

  # Get a specific template
  dtctl get slo-template <template-id>

  # Filter templates
  dtctl get slo-templates --filter "builtIn==true"

  # Output as JSON
  dtctl get slo-templates -o json
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		filter, _ := cmd.Flags().GetString("filter")

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := slo.NewHandler(c)
		printer := NewPrinter()

		// Get specific template if ID provided
		if len(args) > 0 {
			t, err := handler.GetTemplate(args[0])
			if err != nil {
				return err
			}
			return printer.Print(t)
		}

		// List all templates
		list, err := handler.ListTemplates(filter)
		if err != nil {
			return err
		}

		return printer.PrintList(list.Items)
	},
}

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

		// Safety check
		checker, err := NewSafetyChecker(cfg)
		if err != nil {
			return err
		}
		if err := checker.CheckError(safety.OperationDelete, safety.OwnershipUnknown); err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := notification.NewHandler(c)

		// Get notification for confirmation
		n, err := handler.GetEventNotification(notifID)
		if err != nil {
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

// deleteSLOCmd deletes an SLO
var deleteSLOCmd = &cobra.Command{
	Use:   "slo <slo-id>",
	Short: "Delete a service-level objective",
	Long: `Delete a service-level objective by ID.

Examples:
  # Delete an SLO
  dtctl delete slo <slo-id>

  # Delete without confirmation
  dtctl delete slo <slo-id> -y
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sloID := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		// Safety check
		checker, err := NewSafetyChecker(cfg)
		if err != nil {
			return err
		}
		if err := checker.CheckError(safety.OperationDelete, safety.OwnershipUnknown); err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := slo.NewHandler(c)

		// Get current version for optimistic locking
		s, err := handler.Get(sloID)
		if err != nil {
			return err
		}

		// Confirm deletion unless --force or --plain
		if !forceDelete && !plainMode {
			if !prompt.ConfirmDeletion("SLO", s.Name, sloID) {
				fmt.Println("Deletion cancelled")
				return nil
			}
		}

		if err := handler.Delete(sloID, s.Version); err != nil {
			return err
		}

		fmt.Printf("SLO %q deleted\n", s.Name)
		return nil
	},
}

// getBucketsCmd retrieves Grail buckets
var getBucketsCmd = &cobra.Command{
	Use:     "buckets [name]",
	Aliases: []string{"bucket", "bkt"},
	Short:   "Get Grail storage buckets",
	Long: `Get Grail storage buckets.

Examples:
  # List all buckets
  dtctl get buckets

  # Get a specific bucket
  dtctl get bucket <bucket-name>

  # Output as JSON
  dtctl get buckets -o json
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

		handler := bucket.NewHandler(c)
		printer := NewPrinter()

		// Get specific bucket if name provided
		if len(args) > 0 {
			b, err := handler.Get(args[0])
			if err != nil {
				return err
			}
			return printer.Print(b)
		}

		// List all buckets
		list, err := handler.List()
		if err != nil {
			return err
		}

		return printer.PrintList(list.Buckets)
	},
}

// getLookupsCmd retrieves lookup tables
var getLookupsCmd = &cobra.Command{
	Use:     "lookups [path]",
	Aliases: []string{"lookup", "lkup", "lu"},
	Short:   "Get lookup tables",
	Long: `Get lookup tables from Grail Resource Store.

Lookup tables are tabular files stored in Grail that can be loaded and
joined with observability data in DQL queries for data enrichment.

Output formats:
  - List view (no path): Shows lookup table metadata (path, size, records, etc.)
  - Table output (-o table): Shows the actual lookup table data as a table
  - YAML output (-o yaml): Shows both metadata and full data
  - CSV/JSON output: Shows the lookup table data only

Examples:
  # List all lookup tables (shows metadata)
  dtctl get lookups

  # View lookup table data as a table (default)
  dtctl get lookup /lookups/grail/pm/error_codes

  # View with metadata included
  dtctl get lookup /lookups/grail/pm/error_codes -o yaml

  # Export lookup data as CSV
  dtctl get lookup /lookups/grail/pm/error_codes -o csv > error_codes.csv

  # Export lookup data as JSON
  dtctl get lookup /lookups/grail/pm/error_codes -o json

  # List all lookups with additional columns
  dtctl get lookups -o wide
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

		handler := lookup.NewHandler(c)
		printer := NewPrinter()

		// Get specific lookup if path provided
		if len(args) > 0 {
			// For table output, show the actual lookup table data (not metadata)
			if outputFormat == "table" || outputFormat == "wide" {
				data, err := handler.GetData(args[0], 0)
				if err != nil {
					return err
				}
				return printer.PrintList(data)
			}

			// For CSV/JSON output, return full data
			if outputFormat == "csv" || outputFormat == "json" {
				fullData, err := handler.GetData(args[0], 0)
				if err != nil {
					return err
				}
				return printer.PrintList(fullData)
			}

			// For YAML output, return full structure (metadata + data)
			lookupData, err := handler.GetWithData(args[0], 0)
			if err != nil {
				return err
			}
			return printer.Print(lookupData)
		}

		// List all lookups
		list, err := handler.List()
		if err != nil {
			return err
		}

		return printer.PrintList(list)
	},
}

// getAppsCmd retrieves App Engine apps
var getAppsCmd = &cobra.Command{
	Use:     "apps [id]",
	Aliases: []string{"app"},
	Short:   "Get App Engine apps",
	Long: `Get installed App Engine apps.

Examples:
  # List all apps
  dtctl get apps

  # Get a specific app
  dtctl get app my.custom-app

  # Output as JSON
  dtctl get apps -o json
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

		handler := appengine.NewHandler(c)
		printer := NewPrinter()

		// Get specific app if ID provided
		if len(args) > 0 {
			app, err := handler.GetApp(args[0])
			if err != nil {
				return err
			}
			return printer.Print(app)
		}

		// List all apps
		list, err := handler.ListApps()
		if err != nil {
			return err
		}

		return printer.PrintList(list.Apps)
	},
}

// getEdgeConnectsCmd retrieves EdgeConnect configurations
var getEdgeConnectsCmd = &cobra.Command{
	Use:     "edgeconnects [id]",
	Aliases: []string{"edgeconnect", "ec"},
	Short:   "Get EdgeConnect configurations",
	Long: `Get EdgeConnect configurations.

Examples:
  # List all EdgeConnects
  dtctl get edgeconnects

  # Get a specific EdgeConnect
  dtctl get edgeconnect <id>

  # Output as JSON
  dtctl get edgeconnects -o json
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

		handler := edgeconnect.NewHandler(c)
		printer := NewPrinter()

		// Get specific EdgeConnect if ID provided
		if len(args) > 0 {
			ec, err := handler.Get(args[0])
			if err != nil {
				return err
			}
			return printer.Print(ec)
		}

		// List all EdgeConnects
		list, err := handler.List()
		if err != nil {
			return err
		}

		return printer.PrintList(list.EdgeConnects)
	},
}

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

// getSDKVersionsCmd retrieves SDK versions for the function executor
var getSDKVersionsCmd = &cobra.Command{
	Use:     "sdk-versions",
	Aliases: []string{"sdk-version"},
	Short:   "Get available SDK versions for function execution",
	Long: `Get available SDK versions for the function executor.

Examples:
  # List all SDK versions
  dtctl get sdk-versions

  # Output as JSON
  dtctl get sdk-versions -o json
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

		handler := appengine.NewFunctionHandler(c)
		printer := NewPrinter()

		versions, err := handler.GetSDKVersions()
		if err != nil {
			return err
		}

		return printer.PrintList(versions.Versions)
	},
}

// deleteAppCmd deletes an app
var deleteAppCmd = &cobra.Command{
	Use:     "app <app-id>",
	Aliases: []string{"apps"},
	Short:   "Uninstall an App Engine app",
	Long: `Uninstall an App Engine app by ID.

Examples:
  # Uninstall an app
  dtctl delete app my.custom-app

  # Uninstall without confirmation
  dtctl delete app my.custom-app -y
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		appID := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		// Safety check
		checker, err := NewSafetyChecker(cfg)
		if err != nil {
			return err
		}
		if err := checker.CheckError(safety.OperationDelete, safety.OwnershipUnknown); err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := appengine.NewHandler(c)

		// Get app for confirmation
		app, err := handler.GetApp(appID)
		if err != nil {
			return err
		}

		// Confirm deletion unless --force or --plain
		if !forceDelete && !plainMode {
			if !prompt.ConfirmDeletion("app", app.Name, appID) {
				fmt.Println("Deletion cancelled")
				return nil
			}
		}

		if err := handler.DeleteApp(appID); err != nil {
			return err
		}

		fmt.Printf("App %q uninstall initiated\n", appID)
		return nil
	},
}

// deleteEdgeConnectCmd deletes an EdgeConnect
var deleteEdgeConnectCmd = &cobra.Command{
	Use:     "edgeconnect <id>",
	Aliases: []string{"ec"},
	Short:   "Delete an EdgeConnect configuration",
	Long: `Delete an EdgeConnect configuration by ID.

Examples:
  # Delete an EdgeConnect
  dtctl delete edgeconnect <id>

  # Delete without confirmation
  dtctl delete edgeconnect <id> -y
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ecID := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		// Safety check
		checker, err := NewSafetyChecker(cfg)
		if err != nil {
			return err
		}
		if err := checker.CheckError(safety.OperationDelete, safety.OwnershipUnknown); err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := edgeconnect.NewHandler(c)

		// Get EdgeConnect for confirmation
		ec, err := handler.Get(ecID)
		if err != nil {
			return err
		}

		// Confirm deletion unless --force or --plain
		if !forceDelete && !plainMode {
			if !prompt.ConfirmDeletion("EdgeConnect", ec.Name, ecID) {
				fmt.Println("Deletion cancelled")
				return nil
			}
		}

		if err := handler.Delete(ecID); err != nil {
			return err
		}

		fmt.Printf("EdgeConnect %q deleted\n", ec.Name)
		return nil
	},
}

// deleteBucketCmd deletes a bucket
var deleteBucketCmd = &cobra.Command{
	Use:     "bucket <bucket-name>",
	Aliases: []string{"buckets", "bkt"},
	Short:   "Delete a Grail storage bucket",
	Long: `Delete a Grail storage bucket by name.

WARNING: This operation is irreversible and will delete all data in the bucket.

Examples:
  # Delete a bucket (requires typing the name to confirm)
  dtctl delete bucket <bucket-name>

  # Delete with confirmation flag (non-interactive)
  dtctl delete bucket <bucket-name> --confirm=<bucket-name>

  # Delete without confirmation (use with caution)
  dtctl delete bucket <bucket-name> -y
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		bucketName := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		// Safety check - bucket deletion requires unrestricted level
		checker, err := NewSafetyChecker(cfg)
		if err != nil {
			return err
		}
		if err := checker.CheckError(safety.OperationDeleteBucket, safety.OwnershipUnknown); err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := bucket.NewHandler(c)

		// Verify bucket exists before prompting for confirmation
		if _, err := handler.Get(bucketName); err != nil {
			return err
		}

		// Handle confirmation for data deletion
		confirmFlag, _ := cmd.Flags().GetString("confirm")
		if !forceDelete && !plainMode {
			// If --confirm flag provided, validate it matches the bucket name
			if confirmFlag != "" {
				if !prompt.ValidateConfirmFlag(confirmFlag, bucketName) {
					return fmt.Errorf("confirmation value %q does not match bucket name %q", confirmFlag, bucketName)
				}
			} else {
				// Interactive confirmation - require typing the bucket name
				if !prompt.ConfirmDataDeletion("bucket", bucketName) {
					fmt.Println("Deletion cancelled")
					return nil
				}
			}
		}

		if err := handler.Delete(bucketName); err != nil {
			return err
		}

		fmt.Printf("Bucket %q deletion initiated (async operation)\n", bucketName)
		return nil
	},
}

// deleteLookupCmd deletes a lookup table
var deleteLookupCmd = &cobra.Command{
	Use:     "lookup <path>",
	Aliases: []string{"lookups", "lkup", "lu"},
	Short:   "Delete a lookup table",
	Long: `Delete a lookup table from Grail Resource Store.

ATTENTION: This operation is irreversible and will permanently delete the lookup table.

Examples:
  # Delete a lookup table
  dtctl delete lookup /lookups/grail/pm/error_codes

  # Delete without confirmation
  dtctl delete lookup /lookups/grail/pm/error_codes -y
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		// Safety check
		checker, err := NewSafetyChecker(cfg)
		if err != nil {
			return err
		}
		if err := checker.CheckError(safety.OperationDelete, safety.OwnershipUnknown); err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := lookup.NewHandler(c)

		// Get lookup for confirmation
		lu, err := handler.Get(path)
		if err != nil {
			return err
		}

		// Confirm deletion unless --force or --plain
		if !forceDelete && !plainMode {
			displayName := lu.DisplayName
			if displayName == "" {
				displayName = path
			}
			if !prompt.ConfirmDeletion("lookup table", displayName, path) {
				fmt.Println("Deletion cancelled")
				return nil
			}
		}

		if err := handler.Delete(path); err != nil {
			return err
		}

		fmt.Printf("Lookup table %q deleted\n", path)
		return nil
	},
}

// deleteSettingsCmd deletes a settings object
var deleteSettingsCmd = &cobra.Command{
	Use:   "settings <object-id-or-uid>",
	Short: "Delete a settings object",
	Long: `Delete a settings object by objectId or UID.

You can specify either the full objectId or the UID (UUID format).
When using a UID, you MUST specify --schema.

Examples:
  # Delete by objectId
  dtctl delete settings vu9U3hXa3q0AAAABABRidWlsdGluOnJ1bS53ZWIubmFtZQ...

  # Delete by UID (requires --schema)
  dtctl delete settings e1cd3543-8603-3895-bcee-34d20c700074 --schema builtin:openpipeline.logs.pipelines

  # Delete without confirmation
  dtctl delete settings <object-id-or-uid> -y
`,
	Aliases: []string{"setting"},
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		objectID := args[0]
		schemaID, _ := cmd.Flags().GetString("schema")
		scope, _ := cmd.Flags().GetString("scope")

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		// Safety check
		checker, err := NewSafetyChecker(cfg)
		if err != nil {
			return err
		}
		if err := checker.CheckError(safety.OperationDelete, safety.OwnershipUnknown); err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := settings.NewHandler(c)

		// Get current settings object for confirmation
		obj, err := handler.GetWithContext(objectID, schemaID, scope)
		if err != nil {
			return err
		}

		// Confirm deletion unless --force or --plain
		if !forceDelete && !plainMode {
			summary := obj.Summary
			if summary == "" {
				summary = obj.SchemaID
			}
			if !prompt.ConfirmDeletion("settings object", summary, objectID) {
				fmt.Println("Deletion cancelled")
				return nil
			}
		}

		if err := handler.DeleteWithContext(objectID, schemaID, scope); err != nil {
			return err
		}

		fmt.Printf("Settings object %q deleted\n", objectID)
		return nil
	},
}

// getAnalyzersCmd retrieves Davis analyzers
var getAnalyzersCmd = &cobra.Command{
	Use:     "analyzers [name]",
	Aliases: []string{"analyzer", "az"},
	Short:   "Get Davis AI analyzers",
	Long: `Get available Davis AI analyzers.

Examples:
  # List all analyzers
  dtctl get analyzers

  # Get a specific analyzer definition
  dtctl get analyzer dt.statistics.GenericForecastAnalyzer

  # Filter analyzers
  dtctl get analyzers --filter "name contains 'forecast'"

  # Output as JSON
  dtctl get analyzers -o json
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

		handler := analyzer.NewHandler(c)
		printer := NewPrinter()

		// Get specific analyzer if name provided
		if len(args) > 0 {
			az, err := handler.Get(args[0])
			if err != nil {
				return err
			}
			return printer.Print(az)
		}

		// List all analyzers
		filter, _ := cmd.Flags().GetString("filter")
		list, err := handler.List(filter)
		if err != nil {
			return err
		}

		return printer.PrintList(list.Analyzers)
	},
}

// getCopilotSkillsCmd retrieves Davis CoPilot skills
var getCopilotSkillsCmd = &cobra.Command{
	Use:     "copilot-skills",
	Aliases: []string{"copilot-skill", "skills"},
	Short:   "Get Davis CoPilot skills",
	Long: `Get available Davis CoPilot skills.

Examples:
  # List all CoPilot skills
  dtctl get copilot-skills

  # Output as JSON
  dtctl get copilot-skills -o json
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

		handler := copilot.NewHandler(c)
		printer := NewPrinter()

		list, err := handler.ListSkills()
		if err != nil {
			return err
		}

		return printer.PrintList(list.Skills)
	},
}

// getSettingsSchemasCmd retrieves settings schemas
var getSettingsSchemasCmd = &cobra.Command{
	Use:     "settings-schemas [schema-id]",
	Aliases: []string{"settings-schema", "schemas", "schema"},
	Short:   "Get settings schemas",
	Long: `Get available settings schemas.

Examples:
  # List all settings schemas
  dtctl get settings-schemas

  # Get a specific schema definition
  dtctl get settings-schema builtin:openpipeline.logs.pipelines

  # Output as JSON
  dtctl get settings-schemas -o json
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

		handler := settings.NewHandler(c)
		printer := NewPrinter()

		// Get specific schema if ID provided
		if len(args) > 0 {
			schema, err := handler.GetSchema(args[0])
			if err != nil {
				return err
			}
			return printer.Print(schema)
		}

		// List all schemas
		list, err := handler.ListSchemas()
		if err != nil {
			return err
		}

		return printer.PrintList(list.Items)
	},
}

// getSettingsCmd retrieves settings objects
var getSettingsCmd = &cobra.Command{
	Use:     "settings [object-id-or-uid]",
	Aliases: []string{"setting"},
	Short:   "Get settings objects",
	Long: `Get settings objects for a schema.

You can retrieve a specific settings object by providing either:
- The full objectId (base64-encoded composite key) - no flags needed
- The UID (UUID format) - REQUIRES --schema flag

When using a UID, you MUST specify --schema to narrow the search. This prevents
expensive operations that could search through thousands of objects and put load
on the Dynatrace backend.

Examples:
  # List settings objects for a schema
  dtctl get settings --schema builtin:openpipeline.logs.pipelines

  # List settings with a specific scope
  dtctl get settings --schema builtin:openpipeline.logs.pipelines --scope environment

  # Get by objectId (direct API call, no flags needed)
  dtctl get settings vu9U3hXa3q0AAAABABRidWlsdGluOnJ1bS53ZWIubmFtZQ...

  # Get by UID (requires --schema flag)
  dtctl get settings e1cd3543-8603-3895-bcee-34d20c700074 --schema builtin:openpipeline.logs.pipelines

  # Get by UID with custom scope
  dtctl get settings e1cd3543-8603-3895-bcee-34d20c700074 --schema builtin:openpipeline.logs.pipelines --scope environment

  # Output as JSON
  dtctl get settings --schema builtin:openpipeline.logs.pipelines -o json
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		schemaID, _ := cmd.Flags().GetString("schema")
		scope, _ := cmd.Flags().GetString("scope")

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := settings.NewHandler(c)
		printer := NewPrinter()

		// Get specific object if ID provided
		if len(args) > 0 {
			obj, err := handler.GetWithContext(args[0], schemaID, scope)
			if err != nil {
				return err
			}
			return printer.Print(obj)
		}

		// List objects for schema
		if schemaID == "" {
			return fmt.Errorf("--schema is required when listing settings objects")
		}

		list, err := handler.ListObjects(schemaID, scope, GetChunkSize())
		if err != nil {
			return err
		}

		return printer.PrintList(list.Items)
	},
}

func init() {
	rootCmd.AddCommand(getCmd)
	rootCmd.AddCommand(deleteCmd)
	rootCmd.AddCommand(updateCmd)

	getCmd.AddCommand(getWorkflowsCmd)
	getCmd.AddCommand(getWorkflowExecutionsCmd)
	getCmd.AddCommand(getDashboardsCmd)
	getCmd.AddCommand(getNotebooksCmd)
	getCmd.AddCommand(getSLOsCmd)
	getCmd.AddCommand(getSLOTemplatesCmd)
	getCmd.AddCommand(getNotificationsCmd)
	getCmd.AddCommand(getBucketsCmd)
	getCmd.AddCommand(getLookupsCmd)
	getCmd.AddCommand(getAppsCmd)
	getCmd.AddCommand(getEdgeConnectsCmd)
	getCmd.AddCommand(getUsersCmd)
	getCmd.AddCommand(getGroupsCmd)
	getCmd.AddCommand(getSDKVersionsCmd)
	getCmd.AddCommand(getAnalyzersCmd)
	getCmd.AddCommand(getCopilotSkillsCmd)
	getCmd.AddCommand(getSettingsSchemasCmd)
	getCmd.AddCommand(getSettingsCmd)

	deleteCmd.AddCommand(deleteWorkflowCmd)
	deleteCmd.AddCommand(deleteDashboardCmd)
	deleteCmd.AddCommand(deleteNotebookCmd)
	deleteCmd.AddCommand(deleteSLOCmd)
	deleteCmd.AddCommand(deleteNotificationCmd)
	deleteCmd.AddCommand(deleteBucketCmd)
	deleteCmd.AddCommand(deleteLookupCmd)
	deleteCmd.AddCommand(deleteSettingsCmd)
	deleteCmd.AddCommand(deleteAppCmd)
	deleteCmd.AddCommand(deleteEdgeConnectCmd)

	getWorkflowExecutionsCmd.Flags().StringVarP(&workflowFilter, "workflow", "w", "", "Filter executions by workflow ID")
	getWorkflowsCmd.Flags().Bool("mine", false, "Show only workflows owned by current user")
	getDashboardsCmd.Flags().String("name", "", "Filter by dashboard name (partial match, case-insensitive)")
	getDashboardsCmd.Flags().Bool("mine", false, "Show only dashboards owned by current user")
	getNotebooksCmd.Flags().String("name", "", "Filter by notebook name (partial match, case-insensitive)")
	getNotebooksCmd.Flags().Bool("mine", false, "Show only notebooks owned by current user")

	// SLO flags
	getSLOsCmd.Flags().String("filter", "", "Filter SLOs (e.g., \"name~'production'\")")
	getSLOTemplatesCmd.Flags().String("filter", "", "Filter templates (e.g., \"builtIn==true\")")

	// Notification flags
	getNotificationsCmd.Flags().String("type", "", "Filter by notification type")

	// IAM flags
	getUsersCmd.Flags().String("filter", "", "Filter users by email or name (partial match)")
	getGroupsCmd.Flags().String("filter", "", "Filter groups by name (partial match)")

	// Analyzer flags
	getAnalyzersCmd.Flags().String("filter", "", "Filter analyzers (e.g., \"name contains 'forecast'\")")

	// Settings flags
	getSettingsCmd.Flags().String("schema", "", "Schema ID (required when listing or using UID)")
	getSettingsCmd.Flags().String("scope", "", "Scope to filter settings (e.g., 'environment')")

	// Delete settings flags
	deleteSettingsCmd.Flags().String("schema", "", "Schema ID (required when using UID)")
	deleteSettingsCmd.Flags().String("scope", "", "Scope for UID resolution (optional, defaults to 'environment')")

	// Add --force flag to all delete commands
	deleteWorkflowCmd.Flags().BoolVarP(&forceDelete, "yes", "y", false, "Skip confirmation prompt")
	deleteDashboardCmd.Flags().BoolVarP(&forceDelete, "yes", "y", false, "Skip confirmation prompt")
	deleteNotebookCmd.Flags().BoolVarP(&forceDelete, "yes", "y", false, "Skip confirmation prompt")
	deleteSLOCmd.Flags().BoolVarP(&forceDelete, "yes", "y", false, "Skip confirmation prompt")
	deleteNotificationCmd.Flags().BoolVarP(&forceDelete, "yes", "y", false, "Skip confirmation prompt")
	deleteBucketCmd.Flags().BoolVarP(&forceDelete, "yes", "y", false, "Skip confirmation prompt")
	deleteBucketCmd.Flags().String("confirm", "", "Confirm deletion by providing the bucket name (for non-interactive use)")
	deleteLookupCmd.Flags().BoolVarP(&forceDelete, "yes", "y", false, "Skip confirmation prompt")
	deleteSettingsCmd.Flags().BoolVarP(&forceDelete, "yes", "y", false, "Skip confirmation prompt")
	deleteAppCmd.Flags().BoolVarP(&forceDelete, "yes", "y", false, "Skip confirmation prompt")
	deleteEdgeConnectCmd.Flags().BoolVarP(&forceDelete, "yes", "y", false, "Skip confirmation prompt")
}

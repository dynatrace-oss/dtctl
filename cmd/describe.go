package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/resources/appengine"
	"github.com/dynatrace-oss/dtctl/pkg/resources/azureconnection"
	"github.com/dynatrace-oss/dtctl/pkg/resources/azuremonitoringconfig"
	"github.com/dynatrace-oss/dtctl/pkg/resources/bucket"
	"github.com/dynatrace-oss/dtctl/pkg/resources/document"
	"github.com/dynatrace-oss/dtctl/pkg/resources/edgeconnect"
	"github.com/dynatrace-oss/dtctl/pkg/resources/iam"
	"github.com/dynatrace-oss/dtctl/pkg/resources/lookup"
	"github.com/dynatrace-oss/dtctl/pkg/resources/resolver"
	"github.com/dynatrace-oss/dtctl/pkg/resources/settings"
	"github.com/dynatrace-oss/dtctl/pkg/resources/slo"
	"github.com/dynatrace-oss/dtctl/pkg/resources/workflow"
	"github.com/spf13/cobra"
)

// describeCmd represents the describe command
var describeCmd = &cobra.Command{
	Use:   "describe",
	Short: "Show details of a specific resource",
	Long:  `Show detailed information about a specific resource.`,
	RunE:  requireSubcommand,
}

// describeWorkflowExecutionCmd shows detailed info about a workflow execution
var describeWorkflowExecutionCmd = &cobra.Command{
	Use:     "workflow-execution <execution-id>",
	Aliases: []string{"wfe"},
	Short:   "Show details of a workflow execution",
	Long: `Show detailed information about a workflow execution including task states.

Examples:
  # Describe a workflow execution
  dtctl describe workflow-execution <execution-id>
  dtctl describe wfe <execution-id>
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		executionID := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := workflow.NewExecutionHandler(c)

		// Get execution details
		exec, err := handler.Get(executionID)
		if err != nil {
			return err
		}

		// Get task executions
		tasks, err := handler.ListTasks(executionID)
		if err != nil {
			return err
		}

		// Print execution details
		fmt.Printf("ID:         %s\n", exec.ID)
		fmt.Printf("Workflow:   %s\n", exec.Workflow)
		fmt.Printf("Title:      %s\n", exec.Title)
		fmt.Printf("State:      %s\n", exec.State)
		fmt.Printf("Started:    %s\n", exec.StartedAt.Format("2006-01-02 15:04:05"))
		if exec.EndedAt != nil {
			fmt.Printf("Ended:      %s\n", exec.EndedAt.Format("2006-01-02 15:04:05"))
		}
		fmt.Printf("Duration:   %s\n", formatDuration(exec.Runtime))
		fmt.Printf("Trigger:    %s\n", exec.TriggerType)
		if exec.StateInfo != nil && *exec.StateInfo != "" {
			fmt.Printf("State Info: %s\n", *exec.StateInfo)
		}

		// Print tasks table
		if len(tasks) > 0 {
			fmt.Println()
			fmt.Println("Tasks:")

			// Find max name length for alignment
			maxNameLen := 4 // "NAME"
			for _, t := range tasks {
				if len(t.Name) > maxNameLen {
					maxNameLen = len(t.Name)
				}
			}

			// Print header
			fmt.Printf("  %-*s  %-10s  %s\n", maxNameLen, "NAME", "STATE", "DURATION")

			// Print tasks
			for _, t := range tasks {
				duration := formatDuration(t.Runtime)
				fmt.Printf("  %-*s  %-10s  %s\n", maxNameLen, t.Name, t.State, duration)
			}
		}

		return nil
	},
}

// describeDashboardCmd shows detailed info about a dashboard
var describeDashboardCmd = &cobra.Command{
	Use:     "dashboard <dashboard-id-or-name>",
	Aliases: []string{"dash", "db"},
	Short:   "Show details of a dashboard",
	Long: `Show detailed information about a dashboard including metadata and sharing info.

Examples:
  # Describe a dashboard by ID
  dtctl describe dashboard <dashboard-id>
  dtctl describe dash <dashboard-id>

  # Describe a dashboard by name
  dtctl describe dashboard "Production Dashboard"
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		identifier := args[0]

		cfg, err := LoadConfig()
		if err != nil {
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

		// Get full metadata
		metadata, err := handler.GetMetadata(dashboardID)
		if err != nil {
			return err
		}

		// Print detailed information
		fmt.Printf("ID:          %s\n", metadata.ID)
		fmt.Printf("Name:        %s\n", metadata.Name)
		fmt.Printf("Type:        %s\n", metadata.Type)
		if metadata.Description != "" {
			fmt.Printf("Description: %s\n", metadata.Description)
		}
		fmt.Printf("Version:     %d\n", metadata.Version)
		fmt.Printf("Owner:       %s\n", metadata.Owner)
		fmt.Printf("Private:     %v\n", metadata.IsPrivate)
		fmt.Printf("Created:     %s (by %s)\n",
			metadata.ModificationInfo.CreatedTime.Format("2006-01-02 15:04:05"),
			metadata.ModificationInfo.CreatedBy)
		fmt.Printf("Modified:    %s (by %s)\n",
			metadata.ModificationInfo.LastModifiedTime.Format("2006-01-02 15:04:05"),
			metadata.ModificationInfo.LastModifiedBy)
		if len(metadata.Access) > 0 {
			fmt.Printf("Access:      %s\n", strings.Join(metadata.Access, ", "))
		}

		return nil
	},
}

// describeNotebookCmd shows detailed info about a notebook
var describeNotebookCmd = &cobra.Command{
	Use:     "notebook <notebook-id-or-name>",
	Aliases: []string{"nb"},
	Short:   "Show details of a notebook",
	Long: `Show detailed information about a notebook including metadata and sharing info.

Examples:
  # Describe a notebook by ID
  dtctl describe notebook <notebook-id>
  dtctl describe nb <notebook-id>

  # Describe a notebook by name
  dtctl describe notebook "Analysis Notebook"
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		identifier := args[0]

		cfg, err := LoadConfig()
		if err != nil {
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

		// Get full metadata
		metadata, err := handler.GetMetadata(notebookID)
		if err != nil {
			return err
		}

		// Print detailed information
		fmt.Printf("ID:          %s\n", metadata.ID)
		fmt.Printf("Name:        %s\n", metadata.Name)
		fmt.Printf("Type:        %s\n", metadata.Type)
		if metadata.Description != "" {
			fmt.Printf("Description: %s\n", metadata.Description)
		}
		fmt.Printf("Version:     %d\n", metadata.Version)
		fmt.Printf("Owner:       %s\n", metadata.Owner)
		fmt.Printf("Private:     %v\n", metadata.IsPrivate)
		fmt.Printf("Created:     %s (by %s)\n",
			metadata.ModificationInfo.CreatedTime.Format("2006-01-02 15:04:05"),
			metadata.ModificationInfo.CreatedBy)
		fmt.Printf("Modified:    %s (by %s)\n",
			metadata.ModificationInfo.LastModifiedTime.Format("2006-01-02 15:04:05"),
			metadata.ModificationInfo.LastModifiedBy)
		if len(metadata.Access) > 0 {
			fmt.Printf("Access:      %s\n", strings.Join(metadata.Access, ", "))
		}

		return nil
	},
}

// describeTrashCmd shows detailed info about a trashed document
var describeTrashCmd = &cobra.Command{
	Use:     "trash <document-id>",
	Aliases: []string{"deleted"},
	Short:   "Show details of a trashed document",
	Long: `Show detailed information about a trashed document.

Examples:
  # Describe a trashed document by ID
  dtctl describe trash <document-id>
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		documentID := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := document.NewTrashHandler(c)

		// Get trashed document details
		doc, err := handler.Get(documentID)
		if err != nil {
			return err
		}

		// Print detailed information
		fmt.Printf("ID:                 %s\n", doc.ID)
		fmt.Printf("Name:               %s\n", doc.Name)
		fmt.Printf("Type:               %s\n", doc.Type)
		fmt.Printf("Version:            %d\n", doc.Version)
		fmt.Printf("Owner:              %s\n", doc.Owner)
		fmt.Printf("Deleted By:         %s\n", doc.DeletedBy)
		fmt.Printf("Deleted At:         %s\n", doc.DeletedAt.Format("2006-01-02 15:04:05"))

		// Show modification info if available
		if !doc.ModificationInfo.LastModifiedTime.IsZero() {
			fmt.Printf("Last Modified:      %s\n", doc.ModificationInfo.LastModifiedTime.Format("2006-01-02 15:04:05"))
		}
		if doc.ModificationInfo.LastModifiedBy != "" {
			fmt.Printf("Last Modified By:   %s\n", doc.ModificationInfo.LastModifiedBy)
		}

		return nil
	},
}

// formatDuration formats seconds into a human-readable duration
func formatDuration(seconds int) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	if seconds < 3600 {
		m := seconds / 60
		s := seconds % 60
		if s == 0 {
			return fmt.Sprintf("%dm", m)
		}
		return fmt.Sprintf("%dm%ds", m, s)
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	if m == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh%dm", h, m)
}

// describeWorkflowCmd shows detailed info about a workflow
var describeWorkflowCmd = &cobra.Command{
	Use:     "workflow <workflow-id>",
	Aliases: []string{"wf"},
	Short:   "Show details of a workflow",
	Long: `Show detailed information about a workflow including triggers, tasks, and recent executions.

Examples:
  # Describe a workflow
  dtctl describe workflow <workflow-id>
  dtctl describe wf <workflow-id>
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		workflowID := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := workflow.NewHandler(c)
		execHandler := workflow.NewExecutionHandler(c)

		// Get workflow details
		wf, err := handler.Get(workflowID)
		if err != nil {
			return err
		}

		// Print workflow details
		fmt.Printf("ID:          %s\n", wf.ID)
		fmt.Printf("Title:       %s\n", wf.Title)
		if wf.Description != "" {
			fmt.Printf("Description: %s\n", wf.Description)
		}
		fmt.Printf("Owner:       %s (%s)\n", wf.Owner, wf.OwnerType)
		fmt.Printf("Private:     %v\n", wf.Private)
		fmt.Printf("Deployed:    %v\n", wf.IsDeployed)

		// Print trigger info
		if wf.Trigger != nil {
			fmt.Println()
			fmt.Println("Trigger:")
			printTriggerInfo(wf.Trigger)
		}

		// Print tasks
		if len(wf.Tasks) > 0 {
			fmt.Println()
			fmt.Println("Tasks:")
			for name, task := range wf.Tasks {
				taskMap, ok := task.(map[string]interface{})
				if ok {
					action := ""
					if a, exists := taskMap["action"]; exists {
						action = fmt.Sprintf("%v", a)
					}
					fmt.Printf("  - %s", name)
					if action != "" {
						fmt.Printf(" (%s)", action)
					}
					fmt.Println()
				} else {
					fmt.Printf("  - %s\n", name)
				}
			}
		}

		// Get recent executions
		execList, err := execHandler.List(workflowID)
		if err == nil && execList.Count > 0 {
			fmt.Println()
			fmt.Println("Recent Executions:")

			// Show up to 5 recent executions
			limit := 5
			if execList.Count < limit {
				limit = execList.Count
			}

			for i := 0; i < limit; i++ {
				exec := execList.Results[i]
				fmt.Printf("  - %s  %-10s  %s  %s\n",
					exec.ID[:8]+"...",
					exec.State,
					exec.StartedAt.Format("2006-01-02 15:04"),
					formatDuration(exec.Runtime))
			}

			if execList.Count > limit {
				fmt.Printf("  ... and %d more\n", execList.Count-limit)
			}
		}

		return nil
	},
}

// describeBucketCmd shows detailed info about a bucket
var describeBucketCmd = &cobra.Command{
	Use:     "bucket <bucket-name>",
	Aliases: []string{"bkt"},
	Short:   "Show details of a Grail storage bucket",
	Long: `Show detailed information about a Grail storage bucket.

Examples:
  # Describe a bucket
  dtctl describe bucket default_logs
  dtctl describe bkt custom_logs
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		bucketName := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := bucket.NewHandler(c)

		b, err := handler.Get(bucketName)
		if err != nil {
			return err
		}

		// Print bucket details
		fmt.Printf("Name:           %s\n", b.BucketName)
		fmt.Printf("Display Name:   %s\n", b.DisplayName)
		fmt.Printf("Table:          %s\n", b.Table)
		fmt.Printf("Status:         %s\n", b.Status)
		fmt.Printf("Retention:      %d days\n", b.RetentionDays)
		fmt.Printf("Updatable:      %v\n", b.Updatable)
		fmt.Printf("Version:        %d\n", b.Version)
		if b.MetricInterval != "" {
			fmt.Printf("Metric Interval: %s\n", b.MetricInterval)
		}
		if b.Records != nil {
			fmt.Printf("Records:        %d\n", *b.Records)
		}
		if b.EstimatedUncompressedBytes != nil {
			fmt.Printf("Est. Size:      %s\n", formatBytes(*b.EstimatedUncompressedBytes))
		}

		return nil
	},
}

// describeLookupCmd shows detailed info about a lookup table
var describeLookupCmd = &cobra.Command{
	Use:     "lookup <path>",
	Aliases: []string{"lookups", "lkup", "lu"},
	Short:   "Show details of a lookup table",
	Long: `Show detailed information about a lookup table including metadata and data preview.

Examples:
  # Describe a lookup table
  dtctl describe lookup /lookups/grail/pm/error_codes

  # Output as JSON
  dtctl describe lookup /lookups/grail/pm/error_codes -o json
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := lookup.NewHandler(c)

		// Get lookup metadata
		lu, err := handler.Get(path)
		if err != nil {
			return err
		}

		// Get preview data (first 5 rows)
		data, err := handler.GetData(path, 5)
		if err != nil {
			return err
		}

		// For JSON output, use printer
		if outputFormat == "json" || outputFormat == "yaml" {
			printer := NewPrinter()
			lookupData := struct {
				*lookup.Lookup
				PreviewData []map[string]interface{} `json:"previewData"`
			}{
				Lookup:      lu,
				PreviewData: data,
			}
			return printer.Print(lookupData)
		}

		// Print lookup details
		fmt.Printf("Path:         %s\n", lu.Path)
		if lu.DisplayName != "" {
			fmt.Printf("Display Name: %s\n", lu.DisplayName)
		}
		if lu.Description != "" {
			fmt.Printf("Description:  %s\n", lu.Description)
		}
		if lu.FileSize > 0 {
			fmt.Printf("File Size:    %s\n", formatBytes(lu.FileSize))
		}
		if lu.Records > 0 {
			fmt.Printf("Records:      %d\n", lu.Records)
		}
		if lu.LookupField != "" {
			fmt.Printf("Lookup Field: %s\n", lu.LookupField)
		}
		if len(lu.Columns) > 0 {
			fmt.Printf("Columns:      %s\n", strings.Join(lu.Columns, ", "))
		}
		if lu.Modified != "" {
			fmt.Printf("Modified:     %s\n", lu.Modified)
		}

		// Print data preview
		if len(data) > 0 {
			fmt.Println()
			fmt.Printf("Data Preview (first %d rows):\n", len(data))

			// Create table header
			if len(lu.Columns) > 0 {
				fmt.Println(strings.Join(lu.Columns, "\t"))
			}

			// Print rows
			for _, row := range data {
				var values []string
				for _, col := range lu.Columns {
					val := fmt.Sprintf("%v", row[col])
					values = append(values, val)
				}
				fmt.Println(strings.Join(values, "\t"))
			}
		}

		return nil
	},
}

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

// describeEdgeConnectCmd shows detailed info about an EdgeConnect
var describeEdgeConnectCmd = &cobra.Command{
	Use:     "edgeconnect <id>",
	Aliases: []string{"ec"},
	Short:   "Show details of an EdgeConnect configuration",
	Long: `Show detailed information about an EdgeConnect configuration.

Examples:
  # Describe an EdgeConnect
  dtctl describe edgeconnect <id>
  dtctl describe ec <id>
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ecID := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := edgeconnect.NewHandler(c)

		ec, err := handler.Get(ecID)
		if err != nil {
			return err
		}

		// Print EdgeConnect details
		fmt.Printf("ID:       %s\n", ec.ID)
		fmt.Printf("Name:     %s\n", ec.Name)
		fmt.Printf("Managed:  %v\n", ec.ManagedByDynatraceOperator)

		if len(ec.HostPatterns) > 0 {
			fmt.Println()
			fmt.Println("Host Patterns:")
			for _, pattern := range ec.HostPatterns {
				fmt.Printf("  - %s\n", pattern)
			}
		}

		if ec.OAuthClientID != "" {
			fmt.Printf("\nOAuth Client ID: %s\n", ec.OAuthClientID)
		}

		if ec.ModificationInfo != nil {
			fmt.Println()
			if ec.ModificationInfo.CreatedTime != "" {
				fmt.Printf("Created:  %s (by %s)\n", ec.ModificationInfo.CreatedTime, ec.ModificationInfo.CreatedBy)
			}
			if ec.ModificationInfo.LastModifiedTime != "" {
				fmt.Printf("Modified: %s (by %s)\n", ec.ModificationInfo.LastModifiedTime, ec.ModificationInfo.LastModifiedBy)
			}
		}

		return nil
	},
}

// formatBytes formats bytes into a human-readable string
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// printTriggerInfo prints trigger configuration
func printTriggerInfo(trigger map[string]interface{}) {
	if triggerType, ok := trigger["type"].(string); ok {
		fmt.Printf("  Type: %s\n", triggerType)
	}

	// Handle schedule trigger
	if schedule, ok := trigger["schedule"].(map[string]interface{}); ok {
		if rule, exists := schedule["rule"]; exists {
			fmt.Printf("  Schedule: %v\n", rule)
		}
		if tz, exists := schedule["timezone"]; exists {
			fmt.Printf("  Timezone: %v\n", tz)
		}
	}

	// Handle event trigger
	if eventTrigger, ok := trigger["eventTrigger"].(map[string]interface{}); ok {
		if triggerConfig, exists := eventTrigger["triggerConfiguration"].(map[string]interface{}); exists {
			if eventType, exists := triggerConfig["type"]; exists {
				fmt.Printf("  Event Type: %v\n", eventType)
			}
		}
	}
}

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

// describeSettingsCmd shows detailed info about a settings object
var describeSettingsCmd = &cobra.Command{
	Use:     "settings <object-id-or-uid>",
	Aliases: []string{"setting", "set"},
	Short:   "Show details of a settings object",
	Long: `Show detailed information about a settings object including its value, scope, and metadata.

You can specify either:
  - ObjectID: The full base64-encoded object identifier
  - UID: A human-readable UUID (requires --schema-id and/or --scope for disambiguation)

Examples:
  # Describe a settings object by ObjectID
  dtctl describe settings vu9U3hXa3q0AAAABABlidWlsdGluOnJ1bS5mcm9...

  # Describe a settings object by UID
  dtctl describe settings b396f4-ec8f-3e02-bcef-0328b86a63cc --schema-id builtin:rum.frontend.name
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		idOrUID := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := settings.NewHandler(c)

		// Get optional context flags for UID resolution
		schemaID, _ := cmd.Flags().GetString("schema-id")
		scope, _ := cmd.Flags().GetString("scope")

		// Get settings object
		obj, err := handler.GetWithContext(idOrUID, schemaID, scope)
		if err != nil {
			return err
		}

		// Print settings object details
		fmt.Printf("Object ID:    %s\n", obj.ObjectID)
		if obj.UID != "" {
			fmt.Printf("UID:          %s\n", obj.UID)
		}
		fmt.Printf("Schema ID:    %s\n", obj.SchemaID)
		if obj.SchemaVersion != "" {
			fmt.Printf("Version:      %s\n", obj.SchemaVersion)
		}
		fmt.Printf("Scope:        %s\n", obj.Scope)
		if obj.ScopeType != "" {
			fmt.Printf("Scope Type:   %s\n", obj.ScopeType)
		}
		if obj.ScopeID != "" {
			fmt.Printf("Scope ID:     %s\n", obj.ScopeID)
		}
		if obj.ExternalID != "" {
			fmt.Printf("External ID:  %s\n", obj.ExternalID)
		}
		if obj.Summary != "" {
			fmt.Printf("Summary:      %s\n", obj.Summary)
		}

		// Print modification info
		if obj.ModificationInfo != nil {
			fmt.Println()
			if obj.ModificationInfo.CreatedTime != "" {
				fmt.Printf("Created:      %s", obj.ModificationInfo.CreatedTime)
				if obj.ModificationInfo.CreatedBy != "" {
					fmt.Printf(" (by %s)", obj.ModificationInfo.CreatedBy)
				}
				fmt.Println()
			}
			if obj.ModificationInfo.LastModifiedTime != "" {
				fmt.Printf("Modified:     %s", obj.ModificationInfo.LastModifiedTime)
				if obj.ModificationInfo.LastModifiedBy != "" {
					fmt.Printf(" (by %s)", obj.ModificationInfo.LastModifiedBy)
				}
				fmt.Println()
			}
		}

		// Print value as JSON
		if len(obj.Value) > 0 {
			fmt.Println()
			fmt.Println("Value:")
			valueJSON, err := json.MarshalIndent(obj.Value, "  ", "  ")
			if err == nil {
				fmt.Printf("  %s\n", string(valueJSON))
			}
		}

		return nil
	},
}

// describeSettingsSchemaCmd shows detailed info about a settings schema
var describeSettingsSchemaCmd = &cobra.Command{
	Use:     "settings-schema <schema-id>",
	Aliases: []string{"schema"},
	Short:   "Show details of a settings schema",
	Long: `Show detailed information about a settings schema including properties and validation rules.

Examples:
  # Describe a settings schema
  dtctl describe settings-schema builtin:openpipeline.logs.pipelines
  dtctl describe schema builtin:anomaly-detection.infrastructure
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		schemaID := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := settings.NewHandler(c)

		schema, err := handler.GetSchema(schemaID)
		if err != nil {
			return err
		}

		// Extract and print key schema information
		if schemaID, ok := schema["schemaId"].(string); ok {
			fmt.Printf("Schema ID:        %s\n", schemaID)
		}
		if displayName, ok := schema["displayName"].(string); ok {
			fmt.Printf("Display Name:     %s\n", displayName)
		}
		if description, ok := schema["description"].(string); ok && description != "" {
			fmt.Printf("Description:      %s\n", description)
		}
		if version, ok := schema["version"].(string); ok {
			fmt.Printf("Version:          %s\n", version)
		}
		if multiObj, ok := schema["multiObject"].(bool); ok {
			fmt.Printf("Multi-Object:     %v\n", multiObj)
		}
		if ordered, ok := schema["ordered"].(bool); ok {
			fmt.Printf("Ordered:          %v\n", ordered)
		}

		// Print properties if available
		if properties, ok := schema["properties"].(map[string]any); ok && len(properties) > 0 {
			fmt.Println()
			fmt.Printf("Properties:       %d defined\n", len(properties))
		}

		// Print scopes if available
		if scopesRaw, ok := schema["scopes"].([]any); ok && len(scopesRaw) > 0 {
			fmt.Println()
			fmt.Println("Scopes:")
			for _, s := range scopesRaw {
				if scope, ok := s.(string); ok {
					fmt.Printf("  - %s\n", scope)
				}
			}
		}

		return nil
	},
}

// describeSLOCmd shows detailed info about an SLO
var describeSLOCmd = &cobra.Command{
	Use:     "slo <slo-id>",
	Aliases: []string{},
	Short:   "Show details of a service-level objective",
	Long: `Show detailed information about a service-level objective including criteria, tags, and metadata.

Examples:
  # Describe an SLO by ID
  dtctl describe slo <slo-id>
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sloID := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := slo.NewHandler(c)

		// Get SLO details
		s, err := handler.Get(sloID)
		if err != nil {
			return err
		}

		// Print SLO details
		fmt.Printf("ID:          %s\n", s.ID)

		// Try to decode the object ID to show the UID
		if decoded, err := settings.DecodeObjectID(s.ID); err == nil && decoded.UID != "" {
			fmt.Printf("UID:         %s\n", decoded.UID)
		}

		fmt.Printf("Name:        %s\n", s.Name)
		if s.Description != "" {
			fmt.Printf("Description: %s\n", s.Description)
		}
		if s.Version != "" {
			// Try to decode the version to show the modification timestamp
			if decodedVersion, err := settings.DecodeVersion(s.Version); err == nil {
				if decodedVersion.Timestamp != nil {
					fmt.Printf("Modified:    %s\n", decodedVersion.Timestamp.Format("2006-01-02 15:04:05 UTC"))
				}
			}
		}
		if s.ExternalID != "" {
			fmt.Printf("External ID: %s\n", s.ExternalID)
		}

		// Print tags
		if len(s.Tags) > 0 {
			fmt.Printf("Tags:        %s\n", strings.Join(s.Tags, ", "))
		}

		// Print criteria
		if len(s.Criteria) > 0 {
			fmt.Println()
			fmt.Println("Criteria:")
			for _, c := range s.Criteria {
				timeframe := c.TimeframeFrom
				if c.TimeframeTo != "" {
					timeframe = fmt.Sprintf("%s to %s", c.TimeframeFrom, c.TimeframeTo)
				}
				fmt.Printf("  - Timeframe: %s\n", timeframe)
				fmt.Printf("    Target:    %.2f%%\n", c.Target)
				if c.Warning != nil {
					fmt.Printf("    Warning:   %.2f%%\n", *c.Warning)
				}
			}
		}

		// Print custom SLI if present
		if len(s.CustomSli) > 0 {
			fmt.Println()
			fmt.Println("Custom SLI:")
			sliJSON, err := json.MarshalIndent(s.CustomSli, "  ", "  ")
			if err == nil {
				fmt.Printf("  %s\n", string(sliJSON))
			}
		}

		return nil
	},
}

// describeAzureConnectionCmd shows details of an Azure connection (credential)
var describeAzureConnectionCmd = &cobra.Command{
	Use:     "azure_connection <id>",
	Aliases: []string{"azure_connections", "azconn"},
	Short:   "Show details of an Azure connection (credential)",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}
		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		h := azureconnection.NewHandler(c)
		item, err := h.Get(args[0])
		if err != nil {
			return err
		}

		fmt.Printf("ID:   %s\n", item.ObjectID)
		fmt.Printf("Name: %s\n", item.Value.Name)
		fmt.Printf("Type: %s\n", item.Value.Type)

		if item.Value.ClientSecret != nil {
			fmt.Println("Client Secret Config:")
			fmt.Printf("  Application ID: %s\n", item.Value.ClientSecret.ApplicationID)
			fmt.Printf("  Directory ID:   %s\n", item.Value.ClientSecret.DirectoryID)
			fmt.Printf("  Consumers:      %v\n", item.Value.ClientSecret.Consumers)
		}

		if item.Value.FederatedIdentityCredential != nil {
			fmt.Println("Federated Identity Config:")
			fmt.Printf("  Consumers: %v\n", item.Value.FederatedIdentityCredential.Consumers)
		}

		return nil
	},
}

// describeAzureMonitoringConfigCmd shows details of an Azure monitoring configuration
var describeAzureMonitoringConfigCmd = &cobra.Command{
	Use:     "azure_monitoring_config <id>",
	Aliases: []string{"azure_monitoring", "azmon"},
	Short:   "Show details of an Azure monitoring configuration",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}
		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		h := azuremonitoringconfig.NewHandler(c)
		item, err := h.Get(args[0])
		if err != nil {
			return err
		}

		fmt.Printf("ID:          %s\n", item.ObjectID)
		fmt.Printf("Description: %s\n", item.Value.Description)
		fmt.Printf("Enabled:     %v\n", item.Value.Enabled)
		fmt.Printf("Version:     %s\n", item.Value.Version)
		fmt.Println("Azure Config:")
		fmt.Printf("  Deployment Scope:            %s\n", item.Value.Azure.DeploymentScope)
		fmt.Printf("  Subscription Filtering Mode: %s\n", item.Value.Azure.SubscriptionFilteringMode)
		fmt.Printf("  Configuration Mode:          %s\n", item.Value.Azure.ConfigurationMode)
		fmt.Printf("  Deployment Mode:             %s\n", item.Value.Azure.DeploymentMode)
		
		if len(item.Value.Azure.Credentials) > 0 {
			fmt.Println("  Credentials:")
			for _, cred := range item.Value.Azure.Credentials {
				fmt.Printf("    - Description:   %s\n", cred.Description)
				fmt.Printf("      Connection ID: %s\n", cred.ConnectionId)
				fmt.Printf("      Type:          %s\n", cred.Type)
			}
		}

		return nil
	},
}

func init() {
	describeCmd.AddCommand(describeAzureConnectionCmd)
	describeCmd.AddCommand(describeAzureMonitoringConfigCmd)
	rootCmd.AddCommand(describeCmd)
	describeCmd.AddCommand(describeWorkflowCmd)
	describeCmd.AddCommand(describeWorkflowExecutionCmd)
	describeCmd.AddCommand(describeDashboardCmd)
	describeCmd.AddCommand(describeNotebookCmd)
	describeCmd.AddCommand(describeTrashCmd)
	describeCmd.AddCommand(describeBucketCmd)
	describeCmd.AddCommand(describeLookupCmd)
	describeCmd.AddCommand(describeAppCmd)
	describeCmd.AddCommand(describeFunctionCmd)
	describeCmd.AddCommand(describeIntentCmd)
	describeCmd.AddCommand(describeEdgeConnectCmd)
	describeCmd.AddCommand(describeUserCmd)
	describeCmd.AddCommand(describeGroupCmd)
	describeCmd.AddCommand(describeSettingsCmd)
	describeCmd.AddCommand(describeSettingsSchemaCmd)
	describeCmd.AddCommand(describeSLOCmd)

	// Add flags for settings command
	describeSettingsCmd.Flags().String("schema-id", "", "Schema ID to use for UID resolution")
	describeSettingsCmd.Flags().String("scope", "", "Scope to use for UID resolution")
}

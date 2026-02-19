package cmd

import (
	"fmt"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/prompt"
	"github.com/dynatrace-oss/dtctl/pkg/resources/document"
	"github.com/dynatrace-oss/dtctl/pkg/resources/resolver"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/spf13/cobra"
)

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

		// Check if watch mode is enabled
		watchMode, _ := cmd.Flags().GetBool("watch")
		if watchMode {
			fetcher := func() (interface{}, error) {
				list, err := handler.List(filters)
				if err != nil {
					return nil, err
				}
				return document.ConvertToDocuments(list), nil
			}
			return executeWithWatch(cmd, fetcher, printer)
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

		// Check if watch mode is enabled
		watchMode, _ := cmd.Flags().GetBool("watch")
		if watchMode {
			fetcher := func() (interface{}, error) {
				list, err := handler.List(filters)
				if err != nil {
					return nil, err
				}
				return document.ConvertToDocuments(list), nil
			}
			return executeWithWatch(cmd, fetcher, printer)
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

// getTrashCmd retrieves trashed documents
var getTrashCmd = &cobra.Command{
	Use:     "trash",
	Aliases: []string{"deleted"},
	Short:   "Get trashed documents",
	Long: `List or get trashed documents (dashboards and notebooks).

Documents are soft-deleted and kept in trash for 30 days before permanent deletion.

Examples:
  # List all trashed documents
  dtctl get trash

  # List only trashed dashboards
  dtctl get trash --type dashboard

  # List only trashed notebooks
  dtctl get trash --type notebook

  # Filter by who deleted it
  dtctl get trash --deleted-by user@example.com

  # Filter by deletion date
  dtctl get trash --deleted-after 2024-01-01
  dtctl get trash --deleted-before 2024-12-31

  # Output as JSON
  dtctl get trash -o json
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

		handler := document.NewTrashHandler(c)
		printer := NewPrinter()

		// Build filter options from flags
		typeFilter, _ := cmd.Flags().GetString("type")
		deletedBy, _ := cmd.Flags().GetString("deleted-by")
		deletedAfter, _ := cmd.Flags().GetString("deleted-after")
		deletedBefore, _ := cmd.Flags().GetString("deleted-before")

		opts := document.TrashListOptions{
			Type:      typeFilter,
			DeletedBy: deletedBy,
			ChunkSize: GetChunkSize(),
		}

		// Parse date filters if provided
		if deletedAfter != "" {
			t, err := time.Parse("2006-01-02", deletedAfter)
			if err != nil {
				return fmt.Errorf("invalid deleted-after date format (use YYYY-MM-DD): %w", err)
			}
			opts.DeletedAfter = t
		}
		if deletedBefore != "" {
			t, err := time.Parse("2006-01-02", deletedBefore)
			if err != nil {
				return fmt.Errorf("invalid deleted-before date format (use YYYY-MM-DD): %w", err)
			}
			opts.DeletedBefore = t
		}

		// Check if watch mode is enabled
		watchMode, _ := cmd.Flags().GetBool("watch")
		if watchMode {
			fetcher := func() (interface{}, error) {
				return handler.List(opts)
			}
			return executeWithWatch(cmd, fetcher, printer)
		}

		// List trash
		docs, err := handler.List(opts)
		if err != nil {
			return err
		}

		return printer.PrintList(docs)
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

		// Safety check with actual ownership
		checker, err := NewSafetyChecker(cfg)
		if err != nil {
			return err
		}
		currentUserID, _ := c.CurrentUserID()
		ownership := safety.DetermineOwnership(metadata.Owner, currentUserID)
		if err := checker.CheckError(safety.OperationDelete, ownership); err != nil {
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

		// Safety check with actual ownership
		checker, err := NewSafetyChecker(cfg)
		if err != nil {
			return err
		}
		currentUserID, _ := c.CurrentUserID()
		ownership := safety.DetermineOwnership(metadata.Owner, currentUserID)
		if err := checker.CheckError(safety.OperationDelete, ownership); err != nil {
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

// deleteTrashCmd permanently deletes documents from trash
var deleteTrashCmd = &cobra.Command{
	Use:     "trash <document-id> [document-id...]",
	Aliases: []string{"deleted"},
	Short:   "Permanently delete document(s) from trash",
	Long: `Permanently delete one or more documents from trash.

WARNING: This operation cannot be undone. Documents will be permanently deleted
and cannot be recovered.

The --permanent flag is required to prevent accidental deletion.

Examples:
  # Permanently delete a single document
  dtctl delete trash a1b2c3d4-e5f6-7890-abcd-ef1234567890 --permanent

  # Permanently delete multiple documents
  dtctl delete trash <id1> <id2> <id3> --permanent -y
`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		// Check for --permanent flag
		permanent, _ := cmd.Flags().GetBool("permanent")
		if !permanent {
			return fmt.Errorf("--permanent flag is required to delete from trash")
		}

		handler := document.NewTrashHandler(c)

		// Confirm deletion unless --force or --plain or deleting multiple
		if !forceDelete && !plainMode {
			var docNames []string
			for _, docID := range args {
				doc, err := handler.Get(docID)
				if err != nil {
					fmt.Printf("Warning: Could not get document %s: %v\n", docID, err)
					docNames = append(docNames, docID)
				} else {
					docNames = append(docNames, fmt.Sprintf("%s %q", doc.Type, doc.Name))
				}
			}

			confirmMsg := fmt.Sprintf("PERMANENTLY DELETE %d document(s) from trash? This cannot be undone.", len(args))
			if !prompt.Confirm(confirmMsg) {
				fmt.Println("Deletion cancelled")
				return nil
			}
		}

		// Delete each document
		successCount := 0
		for _, docID := range args {
			err := handler.Delete(docID)
			if err != nil {
				fmt.Printf("Failed to delete document %s: %v\n", docID, err)
				continue
			}

			fmt.Printf("Permanently deleted document %s\n", docID)
			successCount++
		}

		if successCount == 0 && len(args) > 0 {
			return fmt.Errorf("failed to delete any documents")
		}

		if len(args) > 1 {
			fmt.Printf("\nDeleted %d of %d documents\n", successCount, len(args))
		}

		return nil
	},
}

func init() {
	// Watch flags
	addWatchFlags(getDashboardsCmd)
	addWatchFlags(getNotebooksCmd)
	addWatchFlags(getTrashCmd)

	// Dashboard flags
	getDashboardsCmd.Flags().String("name", "", "Filter by dashboard name (partial match, case-insensitive)")
	getDashboardsCmd.Flags().Bool("mine", false, "Show only dashboards owned by current user")

	// Notebook flags
	getNotebooksCmd.Flags().String("name", "", "Filter by notebook name (partial match, case-insensitive)")
	getNotebooksCmd.Flags().Bool("mine", false, "Show only notebooks owned by current user")

	// Trash flags
	getTrashCmd.Flags().String("type", "", "Filter by type: dashboard, notebook")
	getTrashCmd.Flags().String("deleted-by", "", "Filter by who deleted it")
	getTrashCmd.Flags().String("deleted-after", "", "Show documents deleted after date (YYYY-MM-DD)")
	getTrashCmd.Flags().String("deleted-before", "", "Show documents deleted before date (YYYY-MM-DD)")

	// Delete confirmation flags
	deleteDashboardCmd.Flags().BoolVarP(&forceDelete, "yes", "y", false, "Skip confirmation prompt")
	deleteNotebookCmd.Flags().BoolVarP(&forceDelete, "yes", "y", false, "Skip confirmation prompt")
	deleteTrashCmd.Flags().Bool("permanent", false, "Permanently delete (required)")
	deleteTrashCmd.Flags().BoolVarP(&forceDelete, "yes", "y", false, "Skip confirmation prompt")
}

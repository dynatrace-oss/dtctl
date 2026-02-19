package cmd

import (
	"fmt"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/resources/document"
	"github.com/dynatrace-oss/dtctl/pkg/resources/resolver"
	"github.com/spf13/cobra"
)

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

package cmd

import (
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/prompt"
	"github.com/dynatrace-oss/dtctl/pkg/resources/slo"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/spf13/cobra"
)

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

func init() {
	addWatchFlags(getSLOsCmd)

	// SLO flags
	getSLOsCmd.Flags().String("filter", "", "Filter SLOs (e.g., \"name~'production'\")")
	getSLOTemplatesCmd.Flags().String("filter", "", "Filter templates (e.g., \"builtIn==true\")")

	// Delete confirmation flags
	deleteSLOCmd.Flags().BoolVarP(&forceDelete, "yes", "y", false, "Skip confirmation prompt")
}

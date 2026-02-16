package cmd

import (
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/prompt"
	"github.com/dynatrace-oss/dtctl/pkg/resources/edgeconnect"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/spf13/cobra"
)

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

func init() {
	// Delete confirmation flags
	deleteEdgeConnectCmd.Flags().BoolVarP(&forceDelete, "yes", "y", false, "Skip confirmation prompt")
}

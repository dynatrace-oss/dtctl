package cmd

import (
	"github.com/dynatrace-oss/dtctl/pkg/resources/analyzer"
	"github.com/dynatrace-oss/dtctl/pkg/resources/copilot"
	"github.com/spf13/cobra"
)

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

func init() {
	// Analyzer flags
	getAnalyzersCmd.Flags().String("filter", "", "Filter analyzers (e.g., \"name contains 'forecast'\")")
}

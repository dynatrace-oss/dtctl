package cmd

import (
	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/schedulingrule"
)

// describeSchedulingRuleCmd shows detailed info about a scheduling rule
var describeSchedulingRuleCmd = &cobra.Command{
	Use:     "scheduling-rule <id>",
	Aliases: []string{"sr"},
	Short:   "Show details of a scheduling rule",
	Long: `Show detailed information about a scheduling rule.

Examples:
  # Describe a scheduling rule
  dtctl describe scheduling-rule <scheduling-rule-id>
  dtctl describe sr <scheduling-rule-id>
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]

		_, c, printer, err := Setup()
		if err != nil {
			return err
		}

		handler := schedulingrule.NewHandler(c)

		rule, err := handler.Get(id)
		if err != nil {
			return err
		}

		if outputFormat == "table" {
			const kw = 13
			output.DescribeKV("ID:", kw, "%s", rule.ID)
			output.DescribeKV("Title:", kw, "%s", rule.Title)
			if rule.Description != "" {
				output.DescribeKV("Description:", kw, "%s", rule.Description)
			}
			output.DescribeKV("Timezone:", kw, "%s", rule.Timezone)
			output.DescribeKV("Rule:", kw, "%s", rule.Rule)
			if rule.Owner != "" {
				output.DescribeKV("Owner:", kw, "%s (%s)", rule.Owner, rule.OwnerType)
			}
			return nil
		}

		enrichAgent(printer, "describe", "scheduling-rule")
		return printer.Print(rule)
	},
}

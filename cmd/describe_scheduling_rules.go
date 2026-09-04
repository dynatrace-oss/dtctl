package cmd

import (
	"fmt"
	"sort"

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

		if useSchedulingRuleDescribeTextView() {
			const kw = 18
			output.DescribeKV("ID:", kw, "%s", rule.ID)
			output.DescribeKV("Title:", kw, "%s", rule.Title)
			if rule.Description != "" {
				output.DescribeKV("Description:", kw, "%s", rule.Description)
			}
			output.DescribeKV("Rule Type:", kw, "%s", rule.RuleType)
			if rule.BusinessCalendar != "" {
				output.DescribeKV("Business Calendar:", kw, "%s", rule.BusinessCalendar)
			}
			if rule.Version != 0 {
				output.DescribeKV("Version:", kw, "%d", rule.Version)
			}
			if body := schedulingRuleBody(rule); body != nil {
				fmt.Println()
				fmt.Println("Rule:")
				for _, k := range sortedKeys(body) {
					output.DescribeKV("  "+k+":", kw, "%v", body[k])
				}
			}
			if rule.ModificationInfo != nil {
				fmt.Println()
				fmt.Println("Modification Info:")
				output.DescribeKV("  Created By:", kw, "%s", rule.ModificationInfo.CreatedBy)
				output.DescribeKV("  Created:", kw, "%s", rule.ModificationInfo.CreatedTime)
				output.DescribeKV("  Modified By:", kw, "%s", rule.ModificationInfo.LastModifiedBy)
				output.DescribeKV("  Modified:", kw, "%s", rule.ModificationInfo.LastModifiedTime)
			}
			return nil
		}

		enrichAgent(printer, "describe", "scheduling-rule")
		return printer.Print(rule)
	},
}

// useSchedulingRuleDescribeTextView reports whether to render the human-readable
// text view. Agent mode always takes the structured envelope path — note that
// agent mode leaves outputFormat at its "table" default, so a bare format check
// would wrongly emit human text into an agent session.
func useSchedulingRuleDescribeTextView() bool {
	if agentMode {
		return false
	}
	return outputFormat == "" || outputFormat == "table"
}

// schedulingRuleBody returns the rule body matching the rule's type, or nil.
func schedulingRuleBody(r *schedulingrule.SchedulingRule) map[string]interface{} {
	switch r.RuleType {
	case "rrule":
		return r.RRule
	case "grouping":
		return r.GroupingRule
	case "fixed_offset":
		return r.FixedOffsetRule
	case "relative_offset":
		return r.RelativeOffsetRule
	}
	return nil
}

// sortedKeys keeps the describe output stable across runs.
func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

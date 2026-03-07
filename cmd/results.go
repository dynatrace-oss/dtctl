package cmd

import (
	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/resources/workflow"
)

// resultsCmd represents the results command
var resultsCmd = &cobra.Command{
	Use:   "results",
	Short: "Print structured return values for resources",
	Long:  `Print structured return values (task results) for various resources.`,
}

// resultsWorkflowExecutionCmd prints the return value of a workflow execution task
var resultsWorkflowExecutionCmd = &cobra.Command{
	Use:     "workflow-execution <execution-id>",
	Aliases: []string{"wfe"},
	Short:   "Print the return value of a workflow execution task",
	Long: `Print the structured return value produced by a task in a workflow execution.

Unlike 'dtctl logs wfe', which prints stdout/stderr log output, this command
retrieves the structured data returned by the task (e.g. the object returned
by a JavaScript task's default export function).

Examples:
  # Get the return value of a specific task
  dtctl results workflow-execution <execution-id> --task <task-name>
  dtctl results wfe <execution-id> --task <task-name>
  dtctl results wfe <execution-id> -t <task-name>

  # Output as JSON
  dtctl results wfe <execution-id> --task <task-name> -o json

  # Output as YAML
  dtctl results wfe <execution-id> --task <task-name> -o yaml
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		executionID := args[0]

		task, _ := cmd.Flags().GetString("task")

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := workflow.NewExecutionHandler(c)
		result, err := handler.GetTaskResult(executionID, task)
		if err != nil {
			return err
		}

		printer := NewPrinter()
		return printer.Print(result)
	},
}

func init() {
	rootCmd.AddCommand(resultsCmd)
	resultsCmd.AddCommand(resultsWorkflowExecutionCmd)
	resultsWorkflowExecutionCmd.Flags().StringP("task", "t", "", "Task name to retrieve the result for (required)")
	if err := resultsWorkflowExecutionCmd.MarkFlagRequired("task"); err != nil {
		panic(err)
	}
}

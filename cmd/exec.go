package cmd

import (
	"github.com/spf13/cobra"
)

// execCmd represents the exec command
var execCmd = &cobra.Command{
	Use:   "exec",
	Short: "Execute queries, workflows, or functions",
	Long: `Execute operations on the Dynatrace platform: run workflows, invoke
serverless functions, evaluate SLOs, or chat with Davis CoPilot.

For DQL queries, use 'dtctl query' instead (exec dql is deprecated).

Available operations:
  workflow (wf)           Trigger a workflow execution and poll for results
  function (fn, func)     Invoke an app function or run ad-hoc JavaScript
  analyzer (az)           Run a Davis AI analyzer
  slo                     Evaluate a service-level objective
  copilot (cp, chat)      Chat with Davis CoPilot interactively`,
	Example: `  # Execute a workflow and wait for completion
  dtctl exec workflow <workflow-id>

  # Execute a workflow with input parameters
  dtctl exec workflow <workflow-id> --set key=value

  # Invoke a serverless function
  dtctl exec function <app-id>/<function-name>

  # Evaluate an SLO
  dtctl exec slo <slo-id>

  # Chat with Davis CoPilot
  dtctl exec copilot "What happened in the last hour?"`,
	RunE: requireSubcommand,
}

func init() {
	rootCmd.AddCommand(execCmd)

	execCmd.AddCommand(execDQLCmd)
	execCmd.AddCommand(execWorkflowCmd)
	execCmd.AddCommand(execFunctionCmd)
	execCmd.AddCommand(execAnalyzerCmd)
	execCmd.AddCommand(execCopilotCmd)
	execCmd.AddCommand(execSLOCmd)
}

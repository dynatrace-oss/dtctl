package cmd

import (
	"github.com/spf13/cobra"
)

// execCmd represents the exec command
var execCmd = &cobra.Command{
	Use:   "exec",
	Short: "Execute queries, workflows, or functions",
	Long:  `Execute DQL queries, workflows, SLO evaluations, or functions.`,
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

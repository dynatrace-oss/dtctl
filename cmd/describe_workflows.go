package cmd

import (
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/resources/workflow"
	"github.com/spf13/cobra"
)

// describeWorkflowCmd shows detailed info about a workflow
var describeWorkflowCmd = &cobra.Command{
	Use:     "workflow <workflow-id>",
	Aliases: []string{"wf"},
	Short:   "Show details of a workflow",
	Long: `Show detailed information about a workflow including triggers, tasks, and recent executions.

Examples:
  # Describe a workflow
  dtctl describe workflow <workflow-id>
  dtctl describe wf <workflow-id>
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		workflowID := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := workflow.NewHandler(c)
		execHandler := workflow.NewExecutionHandler(c)

		// Get workflow details
		wf, err := handler.Get(workflowID)
		if err != nil {
			return err
		}

		// Print workflow details
		fmt.Printf("ID:          %s\n", wf.ID)
		fmt.Printf("Title:       %s\n", wf.Title)
		if wf.Description != "" {
			fmt.Printf("Description: %s\n", wf.Description)
		}
		fmt.Printf("Owner:       %s (%s)\n", wf.Owner, wf.OwnerType)
		fmt.Printf("Private:     %v\n", wf.Private)
		fmt.Printf("Deployed:    %v\n", wf.IsDeployed)

		// Print trigger info
		if wf.Trigger != nil {
			fmt.Println()
			fmt.Println("Trigger:")
			printTriggerInfo(wf.Trigger)
		}

		// Print tasks
		if len(wf.Tasks) > 0 {
			fmt.Println()
			fmt.Println("Tasks:")
			for name, task := range wf.Tasks {
				taskMap, ok := task.(map[string]interface{})
				if ok {
					action := ""
					if a, exists := taskMap["action"]; exists {
						action = fmt.Sprintf("%v", a)
					}
					fmt.Printf("  - %s", name)
					if action != "" {
						fmt.Printf(" (%s)", action)
					}
					fmt.Println()
				} else {
					fmt.Printf("  - %s\n", name)
				}
			}
		}

		// Get recent executions
		execList, err := execHandler.List(workflowID)
		if err == nil && execList.Count > 0 {
			fmt.Println()
			fmt.Println("Recent Executions:")

			// Show up to 5 recent executions
			limit := 5
			if execList.Count < limit {
				limit = execList.Count
			}

			for i := 0; i < limit; i++ {
				exec := execList.Results[i]
				fmt.Printf("  - %s  %-10s  %s  %s\n",
					exec.ID[:8]+"...",
					exec.State,
					exec.StartedAt.Format("2006-01-02 15:04"),
					formatDuration(exec.Runtime))
			}

			if execList.Count > limit {
				fmt.Printf("  ... and %d more\n", execList.Count-limit)
			}
		}

		return nil
	},
}

// describeWorkflowExecutionCmd shows detailed info about a workflow execution
var describeWorkflowExecutionCmd = &cobra.Command{
	Use:     "workflow-execution <execution-id>",
	Aliases: []string{"wfe"},
	Short:   "Show details of a workflow execution",
	Long: `Show detailed information about a workflow execution including task states.

Examples:
  # Describe a workflow execution
  dtctl describe workflow-execution <execution-id>
  dtctl describe wfe <execution-id>
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		executionID := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := workflow.NewExecutionHandler(c)

		// Get execution details
		exec, err := handler.Get(executionID)
		if err != nil {
			return err
		}

		// Get task executions
		tasks, err := handler.ListTasks(executionID)
		if err != nil {
			return err
		}

		// Print execution details
		fmt.Printf("ID:         %s\n", exec.ID)
		fmt.Printf("Workflow:   %s\n", exec.Workflow)
		fmt.Printf("Title:      %s\n", exec.Title)
		fmt.Printf("State:      %s\n", exec.State)
		fmt.Printf("Started:    %s\n", exec.StartedAt.Format("2006-01-02 15:04:05"))
		if exec.EndedAt != nil {
			fmt.Printf("Ended:      %s\n", exec.EndedAt.Format("2006-01-02 15:04:05"))
		}
		fmt.Printf("Duration:   %s\n", formatDuration(exec.Runtime))
		fmt.Printf("Trigger:    %s\n", exec.TriggerType)
		if exec.StateInfo != nil && *exec.StateInfo != "" {
			fmt.Printf("State Info: %s\n", *exec.StateInfo)
		}

		// Print tasks table
		if len(tasks) > 0 {
			fmt.Println()
			fmt.Println("Tasks:")

			// Find max name length for alignment
			maxNameLen := 4 // "NAME"
			for _, t := range tasks {
				if len(t.Name) > maxNameLen {
					maxNameLen = len(t.Name)
				}
			}

			// Print header
			fmt.Printf("  %-*s  %-10s  %s\n", maxNameLen, "NAME", "STATE", "DURATION")

			// Print tasks
			for _, t := range tasks {
				duration := formatDuration(t.Runtime)
				fmt.Printf("  %-*s  %-10s  %s\n", maxNameLen, t.Name, t.State, duration)
			}
		}

		return nil
	},
}

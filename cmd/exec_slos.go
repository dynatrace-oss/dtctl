package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/slo"
	"github.com/spf13/cobra"
)

// execSLOCmd evaluates an SLO
var execSLOCmd = &cobra.Command{
	Use:     "slo <slo-id>",
	Aliases: []string{"service-level-objective"},
	Short:   "Evaluate a service-level objective",
	Long: `Evaluate an SLO and retrieve its current status.

This command starts an SLO evaluation and polls for the results. The evaluation
assesses the SLO against its defined criteria and returns the current status,
value, and error budget for each criterion.

Examples:
  # Evaluate an SLO
  dtctl exec slo my-slo-id

  # Evaluate with custom timeout (default: 30s)
  dtctl exec slo my-slo-id --timeout 60

  # Output as JSON
  dtctl exec slo my-slo-id -o json

  # Output as YAML
  dtctl exec slo my-slo-id -o yaml
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sloID := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := slo.NewHandler(c)

		// Start the evaluation
		fmt.Printf("Starting SLO evaluation for %q...\n", sloID)
		evalResult, err := handler.Evaluate(sloID)
		if err != nil {
			return err
		}

		evaluationToken := evalResult.EvaluationToken
		if evaluationToken == "" {
			return fmt.Errorf("no evaluation token returned")
		}

		// Get timeout from flags
		timeoutSeconds, _ := cmd.Flags().GetInt("timeout")
		timeoutMs := timeoutSeconds * 1000

		// Poll for results with exponential backoff
		fmt.Printf("Polling for evaluation results...\n")

		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
		defer cancel()

		pollInterval := 2 * time.Second
		maxPollInterval := 10 * time.Second

		for {
			select {
			case <-ctx.Done():
				return fmt.Errorf("timeout waiting for SLO evaluation to complete")
			default:
				result, err := handler.PollEvaluation(evaluationToken, timeoutMs)
				if err != nil {
					// Check if it's a timeout or other error
					if ctx.Err() != nil {
						return fmt.Errorf("timeout waiting for SLO evaluation to complete")
					}
					return err
				}

				// Check if we have results
				if len(result.EvaluationResults) > 0 {
					fmt.Printf("\nSLO Evaluation Complete\n")

					// Check output format
					outputFmt, _ := cmd.Flags().GetString("output")
					if outputFmt == "" || outputFmt == "table" {
						// Print in table format
						printer := output.NewPrinter("table")
						return printer.Print(result.EvaluationResults)
					}

					// Print in requested format (json/yaml)
					printer := output.NewPrinter(outputFmt)
					return printer.Print(result)
				}

				// Wait before next poll with exponential backoff
				time.Sleep(pollInterval)
				if pollInterval < maxPollInterval {
					pollInterval = pollInterval * 2
					if pollInterval > maxPollInterval {
						pollInterval = maxPollInterval
					}
				}
			}
		}
	},
}

func init() {
	// SLO flags
	execSLOCmd.Flags().Int("timeout", 30, "timeout in seconds when polling for evaluation results")
}

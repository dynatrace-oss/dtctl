package wait

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/exec"
	"github.com/dynatrace-oss/dtctl/pkg/output"
)

// WaitConfig configures the wait operation
type WaitConfig struct {
	Query        string
	Condition    Condition
	Timeout      time.Duration
	MaxAttempts  int // 0 = unlimited
	Backoff      BackoffConfig
	QueryOptions exec.DQLExecuteOptions
	OutputFormat string
	Quiet        bool
	Verbose      bool
	ProgressOut  io.Writer // Where to write progress messages (default: stderr)
}

// QueryWaiter polls a query until a condition is met
type QueryWaiter struct {
	executor *exec.DQLExecutor
	config   WaitConfig
}

// Result contains the result of a wait operation
type Result struct {
	Success       bool
	Attempts      int
	Elapsed       time.Duration
	RecordCount   int64
	Records       []map[string]any
	FailureReason string
}

// NewQueryWaiter creates a new query waiter
func NewQueryWaiter(executor *exec.DQLExecutor, config WaitConfig) *QueryWaiter {
	// Set default progress output to stderr if not specified
	if config.ProgressOut == nil {
		config.ProgressOut = os.Stderr
	}
	return &QueryWaiter{
		executor: executor,
		config:   config,
	}
}

// Wait executes the wait operation
func (w *QueryWaiter) Wait(ctx context.Context) (*Result, error) {
	startTime := time.Now()

	// Apply timeout to context
	if w.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, w.config.Timeout)
		defer cancel()
	}

	// Print initial message (unless quiet)
	if !w.config.Quiet {
		fmt.Fprintf(w.config.ProgressOut, "Waiting for condition: %s\n", w.config.Condition.String())
		if w.config.Verbose {
			fmt.Fprintf(w.config.ProgressOut, "Query: %s\n", w.config.Query)
			timeoutStr := "none"
			if w.config.Timeout > 0 {
				timeoutStr = w.config.Timeout.String()
			}
			attemptsStr := "unlimited"
			if w.config.MaxAttempts > 0 {
				attemptsStr = fmt.Sprintf("%d", w.config.MaxAttempts)
			}
			fmt.Fprintf(w.config.ProgressOut, "Timeout: %s, Max attempts: %s\n", timeoutStr, attemptsStr)
			fmt.Fprintln(w.config.ProgressOut)
		}
	}

	// Initial delay if configured
	if w.config.Backoff.InitialDelay > 0 {
		if w.config.Verbose {
			fmt.Fprintf(w.config.ProgressOut, "Initial delay: %s\n", w.config.Backoff.InitialDelay)
		}
		select {
		case <-time.After(w.config.Backoff.InitialDelay):
		case <-ctx.Done():
			return &Result{
				Success:       false,
				Attempts:      0,
				Elapsed:       time.Since(startTime),
				FailureReason: "timeout during initial delay",
			}, ctx.Err()
		}
	}

	attempt := 0
	for {
		// Check max attempts
		if w.config.MaxAttempts > 0 && attempt >= w.config.MaxAttempts {
			elapsed := time.Since(startTime)
			if !w.config.Quiet {
				fmt.Fprintf(w.config.ProgressOut, "\nMax attempts (%d) exceeded\n", w.config.MaxAttempts)
				fmt.Fprintf(w.config.ProgressOut, "Elapsed: %s\n", elapsed.Round(100*time.Millisecond))
			}
			return &Result{
				Success:       false,
				Attempts:      attempt,
				Elapsed:       elapsed,
				FailureReason: "max attempts exceeded",
			}, nil
		}

		attemptStartTime := time.Now()

		// Show attempt number (unless quiet)
		attemptsDisplay := "∞"
		if w.config.MaxAttempts > 0 {
			attemptsDisplay = fmt.Sprintf("%d", w.config.MaxAttempts)
		}

		if w.config.Verbose {
			fmt.Fprintf(w.config.ProgressOut, "Attempt %d/%s at %s\n",
				attempt+1, attemptsDisplay, attemptStartTime.Format(time.RFC3339))
		}

		// Execute query
		result, err := w.executor.ExecuteQueryWithOptions(w.config.Query, w.config.QueryOptions)
		if err != nil {
			// Query execution error - retry unless context cancelled
			if ctx.Err() != nil {
				elapsed := time.Since(startTime)
				return &Result{
					Success:       false,
					Attempts:      attempt + 1,
					Elapsed:       elapsed,
					FailureReason: "timeout",
				}, ctx.Err()
			}

			// Transient error - log and retry
			if w.config.Verbose {
				fmt.Fprintf(w.config.ProgressOut, "  Query error: %v (will retry)\n", err)
			}
		} else {
			// Extract records
			records := result.Records
			if result.Result != nil && len(result.Result.Records) > 0 {
				records = result.Result.Records
			}
			recordCount := int64(len(records))

			queryDuration := time.Since(attemptStartTime)
			if w.config.Verbose {
				fmt.Fprintf(w.config.ProgressOut, "  Query executed in %s\n", queryDuration.Round(time.Millisecond))
				fmt.Fprintf(w.config.ProgressOut, "  Result: %d record(s)\n", recordCount)
			}

			// Evaluate condition
			if w.config.Condition.Evaluate(recordCount) {
				elapsed := time.Since(startTime)
				if !w.config.Quiet {
					if w.config.Verbose {
						fmt.Fprintf(w.config.ProgressOut, "  Condition met!\n\n")
					}
					recordStr := "records"
					if recordCount == 1 {
						recordStr = "record"
					}
					fmt.Fprintf(w.config.ProgressOut, "Success! Condition '%s' satisfied after %d attempt(s)\n",
						w.config.Condition.String(), attempt+1)
					fmt.Fprintf(w.config.ProgressOut, "Found %d %s in %s\n",
						recordCount, recordStr, elapsed.Round(100*time.Millisecond))
				}
				return &Result{
					Success:     true,
					Attempts:    attempt + 1,
					Elapsed:     elapsed,
					RecordCount: recordCount,
					Records:     records,
				}, nil
			}

			// Condition not met - continue retrying
			if w.config.Verbose {
				fmt.Fprintf(w.config.ProgressOut, "  Condition not met")
			} else if !w.config.Quiet {
				recordStr := "records"
				if recordCount == 1 {
					recordStr = "record"
				}
				fmt.Fprintf(w.config.ProgressOut, "Attempt %d/%s: %d %s found",
					attempt+1, attemptsDisplay, recordCount, recordStr)
			}
		}

		// Calculate next interval
		interval := CalculateNextInterval(attempt, w.config.Backoff)

		if !w.config.Quiet {
			fmt.Fprintf(w.config.ProgressOut, ", retrying in %s...\n", interval.Round(100*time.Millisecond))
		}
		if w.config.Verbose {
			fmt.Fprintln(w.config.ProgressOut)
		}

		// Wait for next attempt
		select {
		case <-time.After(interval):
			// Continue to next attempt
		case <-ctx.Done():
			elapsed := time.Since(startTime)
			if !w.config.Quiet {
				fmt.Fprintf(w.config.ProgressOut, "\nTimeout reached\n")
				fmt.Fprintf(w.config.ProgressOut, "Elapsed: %s\n", elapsed.Round(100*time.Millisecond))
			}
			return &Result{
				Success:       false,
				Attempts:      attempt + 1,
				Elapsed:       elapsed,
				FailureReason: "timeout",
			}, ctx.Err()
		}

		attempt++
	}
}

// PrintResults prints the query results if output format is specified
func (w *QueryWaiter) PrintResults(result *Result) error {
	if w.config.OutputFormat == "" || result.Records == nil {
		return nil
	}

	printer := output.NewPrinter(w.config.OutputFormat)
	if w.config.OutputFormat == "table" {
		if len(result.Records) == 0 {
			return nil
		}
		return printer.PrintList(result.Records)
	}

	// For JSON/YAML, wrap in records object
	return printer.Print(map[string]any{"records": result.Records})
}

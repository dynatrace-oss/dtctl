package cmd

import (
	"fmt"
	"os"

	"github.com/dynatrace-oss/dtctl/pkg/exec"
	"github.com/spf13/cobra"
)

// execDQLCmd executes a DQL query (DEPRECATED)
var execDQLCmd = &cobra.Command{
	Use:    "dql [query]",
	Short:  "Execute a DQL query (DEPRECATED: use 'dtctl query')",
	Hidden: true, // Hide from help output
	Long: `Execute a DQL query against Grail storage.

DEPRECATED: This command is deprecated. Use 'dtctl query' instead.
The 'dtctl query' command provides the same functionality with additional
features like template variables.

Examples:
  # Execute inline query (use 'dtctl query' instead)
  dtctl query "fetch logs | limit 10"

  # Execute from file (use 'dtctl query -f' instead)
  dtctl query -f query.dql

  # Output as JSON (use 'dtctl query -o json' instead)
  dtctl query "fetch logs" -o json
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Show deprecation warning
		fmt.Fprintln(os.Stderr, "Warning: 'dtctl exec dql' is deprecated. Use 'dtctl query' instead.")
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		executor := exec.NewDQLExecutor(c)

		queryFile, _ := cmd.Flags().GetString("file")

		if queryFile != "" {
			return executor.ExecuteFromFile(queryFile, outputFormat)
		}

		if len(args) == 0 {
			return fmt.Errorf("query string or --file is required")
		}

		query := args[0]
		return executor.Execute(query, outputFormat)
	},
}

func init() {
	// DQL flags
	execDQLCmd.Flags().StringP("file", "f", "", "read query from file")
}

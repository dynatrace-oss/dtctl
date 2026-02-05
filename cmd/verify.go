package cmd

import (
	"github.com/spf13/cobra"
)

// verifyCmd represents the verify command
var verifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Verify resources without executing them",
	Long: `Verify resources without executing them.

The verify command validates resources before execution, checking for syntax errors,
semantic issues, and configuration problems. This is useful for:

- CI/CD pipelines: validate queries before deployment
- Development: catch errors early without executing
- Configuration validation: ensure correctness before applying

Examples:
  # Verify a DQL query from file
  dtctl verify query -f query.dql

  # Verify inline DQL query
  dtctl verify query "fetch logs | summarize count()"

  # Verify with template variables
  dtctl verify query -f query.dql --set env=prod --set timeframe=1h

  # Verify and fail on warnings (strict mode for CI/CD)
  dtctl verify query -f query.dql --fail-on-warn

Exit Codes:
  0 - Verification successful
  1 - Verification failed (errors found)
  2 - Authentication/permission error
  3 - Network/server error

Use "dtctl verify <command> --help" for more information about a command.
`,
}

func init() {
	rootCmd.AddCommand(verifyCmd)
}

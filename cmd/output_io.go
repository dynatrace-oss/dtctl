package cmd

import (
	"fmt"
	"io"
)

func commandStdout() io.Writer {
	return rootCmd.OutOrStdout()
}

func printOutln(args ...interface{}) {
	_, _ = fmt.Fprintln(commandStdout(), args...)
}

func printOutf(format string, args ...interface{}) {
	_, _ = fmt.Fprintf(commandStdout(), format, args...)
}

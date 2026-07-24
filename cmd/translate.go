package cmd

import "github.com/spf13/cobra"

var translateCmd = &cobra.Command{
	Use:   "translate",
	Short: "Translate expressions between formats",
	Long: `Translate expressions and configurations between formats.

Available subcommands:
  lql-to-dql   Translate an LQL matcher expression into a DQL matcher expression`,
}

func init() {
	rootCmd.AddCommand(translateCmd)
	translateCmd.AddCommand(translateLqlToDqlCmd)
	translateCmd.AddCommand(translateClassicPipelinesCmd)
}

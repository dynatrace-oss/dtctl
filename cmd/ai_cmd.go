package cmd

import (
	"github.com/dynatrace-oss/dtctl/cmd/ai"
)

func init() {
	rootCmd.AddCommand(ai.AiCmd)
}

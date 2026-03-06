package cmd

import (
	"fmt"
	"os"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/spf13/cobra"
)

var previewNoticeShown = map[string]bool{}

func attachPreviewNotice(cmd *cobra.Command, area string) {
	prev := cmd.PersistentPreRunE
	cmd.PersistentPreRunE = func(c *cobra.Command, args []string) error {
		printPreviewNotice(area)
		if prev != nil {
			return prev(c, args)
		}
		return nil
	}
}

func printPreviewNotice(area string) {
	if previewNoticeShown[area] {
		return
	}
	previewNoticeShown[area] = true

	message := fmt.Sprintf("%s commands are in Preview and may change in future releases.", area)
	tag := output.Colorize(output.Yellow, "[Preview]")
	fmt.Fprintf(os.Stderr, "%s %s\n", tag, message)
}

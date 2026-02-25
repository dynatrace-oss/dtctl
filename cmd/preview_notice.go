package cmd

import (
	"fmt"
	"os"

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
	if plainMode {
		fmt.Fprintf(os.Stderr, "[Preview] %s\n", message)
		return
	}

	fmt.Fprintf(os.Stderr, "\x1b[33m[Preview]\x1b[0m %s\n", message)
}

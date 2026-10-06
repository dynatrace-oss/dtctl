package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
)

// previewNoticeShown records the notices this process has printed. A
// concurrent invocation keeps its own record (invocation.previewNoticeShown):
// shared, this map would be written by several requests at once — a fatal
// error in Go, not a recoverable panic — and one tenant's request would
// silence the notice for every later one.
var previewNoticeShown = map[string]bool{}

func attachPreviewNotice(cmd *cobra.Command, area string) {
	prev := cmd.PersistentPreRunE
	cmd.PersistentPreRunE = func(c *cobra.Command, args []string) error {
		printPreviewNotice(cmdContext(c), area)
		if prev != nil {
			return prev(c, args)
		}
		return nil
	}
}

func printPreviewNotice(ctx context.Context, area string) {
	shown := previewNoticeShown
	if inv := concurrentInvocation(ctx); inv != nil {
		if inv.previewNoticeShown == nil {
			inv.previewNoticeShown = map[string]bool{}
		}
		shown = inv.previewNoticeShown
	}
	if shown[area] {
		return
	}
	shown[area] = true

	message := fmt.Sprintf("%s commands are in Preview and may change in future releases.", area)
	tag := output.Colorize(output.Yellow, "[Preview]")
	fmt.Fprintf(currentStderr(ctx), "%s %s\n", tag, message)
}

package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/resources/document"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/dynatrace-oss/dtctl/pkg/stability"
	"github.com/dynatrace-oss/dtctl/sdk/urls"
)

// claimCmd represents the claim command
var claimCmd = &cobra.Command{
	Use:   "claim",
	Short: "Claim access to a shared resource",
	Long:  `Claim access to a resource that was shared with you.`,
}

// claimEnvironmentShareCmd claims an environment share, mirroring the
// browser's "Copy link to share" -> open flow.
var claimEnvironmentShareCmd = &cobra.Command{
	Use:     "environment-share <share-id | share-url>",
	Aliases: []string{"share", "envshare"},
	Short:   "Claim an environment share by ID or URL",
	Long: `Claim an environment share and learn which document it points to.

A share link copied from the browser carries only the share ID:

  https://<environment>.apps.dynatrace.com/ui/document/v0/#share=<share-id>

Claiming is the resolution step: only the share's owner can look a share up,
so a recipient claims it and gets back the document ID, type and the access
now held. Accepts the bare ID or the pasted URL.

Claiming cannot be undone by the claimer (only the owner can delete the
share). Claiming your own share fails; claiming twice changes nothing.

Examples:
  dtctl claim environment-share 018f1234-abcd-7000-8000-000000000000
  dtctl claim environment-share 'https://xyz.apps.dynatrace.com/ui/document/v0/#share=018f1234-abcd-7000-8000-000000000000'
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		shareID, host, err := document.ParseShareRef(args[0])
		if err != nil {
			return err
		}

		cfg, c, printer, err := SetupWithSafetyAndPrinter(safety.OperationCreate)
		if err != nil {
			return err
		}

		if host != "" {
			if ctx, cerr := cfg.CurrentContextObj(); cerr == nil && !sameEnvironment(host, ctx.Environment) {
				return fmt.Errorf("share link is for %s, but the current context targets %s", host, urls.Host(ctx.Environment))
			}
		}

		if dryRun {
			return newDryRunReport(cmd).
				Linef("Dry run: would claim environment share %q", shareID).
				Detail("share_id", "%s", shareID).
				Print()
		}

		claim, err := document.NewHandler(c).ClaimEnvironmentShare(shareID)
		if err != nil {
			return err
		}

		if ap := enrichAgent(printer, "claim", "environment-share"); ap != nil {
			ap.SetSuggestions([]string{claimFollowUp(claim)})
		}
		return printer.Print(claim)
	},
}

// sameEnvironment reports whether a share link's host belongs to the
// environment: equal hosts, or equal tenant ID (first label), since the same
// tenant is reachable under both its apps and live hostnames.
func sameEnvironment(linkHost, environmentURL string) bool {
	envHost := urls.Host(environmentURL)
	if envHost == "" {
		return true
	}
	link := strings.ToLower(linkHost)
	if i := strings.IndexByte(link, ':'); i >= 0 {
		link = link[:i]
	}
	if link == envHost {
		return true
	}
	first := func(h string) string { return strings.SplitN(h, ".", 2)[0] }
	return first(link) == first(envHost)
}

// claimFollowUp is the agent-mode suggestion for reading the claimed document.
func claimFollowUp(c *document.EnvironmentShareClaim) string {
	switch c.DocumentType {
	case "dashboard", "notebook":
		return fmt.Sprintf("dtctl get %s %s", c.DocumentType, c.DocumentID)
	}
	return fmt.Sprintf("dtctl get document %s", c.DocumentID)
}

func init() {
	rootCmd.AddCommand(claimCmd)
	claimCmd.AddCommand(claimEnvironmentShareCmd)

	stability.Mark(claimCmd, stability.Experimental, "0.41.0")
	stability.Mark(claimEnvironmentShareCmd, stability.Experimental, "0.41.0")
}

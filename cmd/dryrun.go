package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/suggest"
)

// dryRunPlan is the agent-mode result of a dry run: the mutation the command
// would have performed, and the exact text a human would have been shown.
//
// Message is deliberately part of the payload. A dry run's prose often carries
// detail that no structured field captures ("can take up to a minute", "also
// re-scopes existing breakpoints"), and dropping it in agent mode would make the
// envelope a lossy rendering of the human output rather than a second view of it.
type dryRunPlan struct {
	DryRun   bool              `json:"dry_run"`
	Verb     string            `json:"verb"`
	Resource string            `json:"resource,omitempty"`
	Details  map[string]string `json:"details,omitempty"`
	Payload  json.RawMessage   `json:"payload,omitempty"`
	Message  string            `json:"message"`
}

// dryRunReport collects what a command would have done, so a single call site
// renders both audiences: the plain lines a dry run has always printed, and the
// agent envelope.
//
// Dry-run branches used to print straight to stdout with fmt.Print*. In agent
// mode that put prose on the one stream the caller parses as JSON — while an
// error from the same command arrived correctly enveloped, so the only outcome an
// agent could not read was the successful one. The human rendering is unchanged:
// the lines, their order, and their wording are the contract this type preserves.
type dryRunReport struct {
	cmd     *cobra.Command
	lines   []string
	details map[string]string
	payload json.RawMessage
}

// newDryRunReport starts a report for a command's dry-run branch. The verb and
// resource in the envelope come from the command itself, so they cannot drift
// from what the caller typed.
func newDryRunReport(cmd *cobra.Command) *dryRunReport {
	return &dryRunReport{cmd: cmd, details: make(map[string]string)}
}

// Linef adds a line of prose. It contributes no structured field — use Field for
// anything a caller might want to read back.
func (r *dryRunReport) Linef(format string, args ...interface{}) *dryRunReport {
	r.lines = append(r.lines, fmt.Sprintf(format, args...))
	return r
}

// Field adds a "Label: value" line and records the value under a snake_case key,
// so the same value is both readable and machine-readable.
func (r *dryRunReport) Field(label, format string, args ...interface{}) *dryRunReport {
	value := fmt.Sprintf(format, args...)
	r.lines = append(r.lines, label+": "+value)
	r.details[detailKey(label)] = value
	return r
}

// Detail records a structured field without printing a line, for values whose
// human rendering is not "Label: value" — an indented list item, or a line
// padded to align with its neighbours. Field is the common case; this keeps the
// envelope complete for the rest without reformatting what a human sees.
func (r *dryRunReport) Detail(key, format string, args ...interface{}) *dryRunReport {
	r.details[key] = fmt.Sprintf(format, args...)
	return r
}

// Payload records the request body the command would have sent. It is embedded
// as JSON rather than as a string, so an agent can diff a dry run against the
// real request instead of parsing the human preview back out of the message.
// raw must be valid JSON; anything else is dropped rather than risking an
// envelope a caller cannot decode.
func (r *dryRunReport) Payload(raw []byte) *dryRunReport {
	if json.Valid(raw) {
		r.payload = json.RawMessage(raw)
	}
	return r
}

// Print renders the report. It is the return value of a dry-run branch.
func (r *dryRunReport) Print() error {
	if !agentMode {
		for _, line := range r.lines {
			fmt.Println(line)
		}
		return nil
	}

	verb, resource := verbResource(r.cmd)
	plan := dryRunPlan{
		DryRun:   true,
		Verb:     verb,
		Resource: resource,
		Message:  strings.Join(r.lines, "\n"),
	}
	if len(r.details) > 0 {
		plan.Details = r.details
	}
	plan.Payload = r.payload
	// EncodeEnvelope, not a local encoder: it is the one place that decides an
	// envelope's wire form, and it pretty-prints only for a human running --agent
	// at a terminal. A dry run piped to an agent must be compact like every other
	// envelope dtctl emits — indenting it would spend a third more tokens on
	// whitespace, for the one audience this rendering exists to serve.
	return output.EncodeEnvelope(os.Stdout, output.Response{
		OK:      true,
		Result:  plan,
		Context: &output.ResponseContext{Verb: verb, Resource: resource},
	})
}

// detailKey turns a human label into a stable snake_case JSON key ("Display
// Name" -> "display_name"), so renaming a label's capitalisation or spacing does
// not change the payload's shape.
func detailKey(label string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(label)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ', r == '-', r == '_', r == '.':
			b.WriteByte('_')
		}
	}
	return b.String()
}

// deleteDryRun is the dry-run branch of a delete command: it names the object
// the command would delete. name may be empty when the ID is all there is.
func deleteDryRun(cmd *cobra.Command, kind, name, id string) error {
	report := newDryRunReport(cmd)
	if name == "" || name == id {
		report.Linef("Dry run: would delete %s %q", kind, id)
	} else {
		report.Linef("Dry run: would delete %s %q (%s)", kind, name, id).Detail("name", "%s", name)
	}
	return report.Detail("id", "%s", id).Print()
}

// dryRunCommands are the commands that implement --dry-run: they resolve the
// target, print what they would do and send no mutating request. The flag is
// registered on these commands only, so every other command rejects it
// instead of ignoring it and doing the real work (#477).
//
// Commands that share a RunE must appear together (share/unshare document and
// their dashboard/notebook aliases run the same function, dry-run branch
// included), or the same implementation would accept the flag under one name
// and reject it under another. Guard: TestDryRunFlagFollowsSharedRunE.
//
// query is deliberately absent: it has no dry run, only a warning that the flag
// is meaningless in live mode. `dtctl verify query` is the check-without-running
// command for DQL.
//
// apply and update document define their own --dry-run flag.
var dryRunCommands = []*cobra.Command{
	accountCreateTokenCmd,
	accountDeleteTokenCmd,
	applyExtensionConfigCmd,
	configDeleteContextCmd,
	configDeleteCredentialsCmd,
	createAnomalyDetectorCmd,
	createAWSConnectionCmd,
	createAWSMonitoringConfigCmd,
	createAzureConnectionCmd,
	createAzureMonitoringConfigCmd,
	createBreakpointCmd,
	createBucketCmd,
	createDashboardCmd,
	createDocumentCmd,
	createEdgeConnectCmd,
	createExtensionCmd,
	createGCPConnectionCmd,
	createGCPMonitoringConfigCmd,
	createLookupCmd,
	createNotebookCmd,
	createSchedulingRuleCmd,
	createSegmentCmd,
	createSettingsCmd,
	createSLOCmd,
	createWorkflowCmd,
	ctxDeleteCmd,
	deleteAnomalyDetectorCmd,
	deleteAppCmd,
	deleteAWSConnectionCmd,
	deleteAWSMonitoringConfigCmd,
	deleteAzureConnectionCmd,
	deleteAzureMonitoringConfigCmd,
	deleteBreakpointCmd,
	deleteBucketCmd,
	deleteDashboardCmd,
	deleteDocumentCmd,
	deleteEdgeConnectCmd,
	deleteGCPConnectionCmd,
	deleteGCPMonitoringConfigCmd,
	deleteLookupCmd,
	deleteNotebookCmd,
	deleteNotificationCmd,
	deleteSchedulingRuleCmd,
	deleteSegmentCmd,
	deleteSettingsCmd,
	deleteSLOCmd,
	deleteTrashCmd,
	deleteWorkflowCmd,
	disableAWSMonitoringCmd,
	disableAzureMonitoringCmd,
	disableGCPMonitoringCmd,
	enableAWSMonitoringCmd,
	enableAzureMonitoringCmd,
	enableGCPMonitoringCmd,
	execAPICmd,
	restoreDashboardCmd,
	restoreDocumentCmd,
	restoreNotebookCmd,
	restoreTrashCmd,
	restoreWorkflowCmd,
	shareDashboardCmd,
	shareDocumentCmd,
	shareNotebookCmd,
	unshareDashboardCmd,
	unshareDocumentCmd,
	unshareNotebookCmd,
	updateAWSConnectionCmd,
	updateAWSMonitoringConfigCmd,
	updateAzureConnectionCmd,
	updateAzureMonitoringConfigCmd,
	updateBreakpointCmd,
	updateExtensionCmd,
	updateExtensionsCmd,
	updateGCPConnectionCmd,
	updateGCPMonitoringConfigCmd,
}

func init() {
	for _, c := range dryRunCommands {
		c.Flags().BoolVar(&dryRun, "dry-run", false, "print what would be done without doing it")
	}

	// A hidden root declaration, so that Cobra still knows --dry-run is a
	// boolean when it appears *before* the subcommand. Command lookup strips
	// flags without parsing them, and an undeclared `--dry-run` is assumed to
	// take a value — which swallows the next word: `dtctl --dry-run delete
	// workflow x` resolved to the command "workflow", and `dtctl --dry-run
	// apply -f x.yaml` reported that the root has no dry run. Both spellings
	// worked while the flag was global, so both must keep working.
	//
	// An opted-in command's own --dry-run shadows this one, so it is only ever
	// parsed for a command that has no dry run — which is what
	// rejectUnimplementedDryRun turns into the usage error. Hidden keeps it out
	// of --help and out of the `commands` catalog's global flags.
	rootCmd.PersistentFlags().Bool("dry-run", false, "print what would be done without doing it")
	_ = rootCmd.PersistentFlags().MarkHidden("dry-run")
	rootDryRunFlag = rootCmd.PersistentFlags().Lookup("dry-run")
}

// rootDryRunFlag is the hidden root declaration registered above. Held as a
// variable rather than looked up through rootCmd, because rootCmd's
// PersistentPreRunE calls the function that reads it.
var rootDryRunFlag *pflag.Flag

// dryRunUnavailableMessage is the one wording for "this command has no dry
// run", whether the flag was rejected at parse time or at run time.
func dryRunUnavailableMessage(cmd *cobra.Command) string {
	msg := fmt.Sprintf("unknown flag --dry-run — '%s' has no dry run", cmd.CommandPath())
	if alt := verifyAlternativeFor(cmd); alt != "" {
		msg += fmt.Sprintf("; to check it without running it, use '%s'", alt)
	}
	return msg
}

// verifyAlternativeFor names the `dtctl verify` subcommand that checks what
// this command runs, or "" when none does.
//
// Every verify subcommand is named after the thing it checks, so the typed
// command's own name is the lookup key: `query` and `exec analyzer` resolve to
// `verify query` and `verify analyzer`, and a verify subcommand added later is
// found without touching this function.
//
// The pointer is omitted rather than generalised because a wrong one costs the
// caller a second failed command to discover it was wrong: `verify` has no
// subcommand for a workflow or a login, so "use 'dtctl verify'" dead-ended
// every caller it was written for.
func verifyAlternativeFor(cmd *cobra.Command) string {
	for _, sub := range verifyCmd.Commands() {
		if sub == cmd || !sub.HasAlias(cmd.Name()) && sub.Name() != cmd.Name() {
			continue
		}
		return sub.CommandPath()
	}
	return ""
}

// rejectUnimplementedDryRun fails a command that was given --dry-run but does
// not implement one. The flag reaches the root declaration only when the
// command has none of its own, so its presence there *is* the error.
func rejectUnimplementedDryRun(cmd *cobra.Command) error {
	if rootDryRunFlag == nil || !rootDryRunFlag.Changed {
		return nil
	}
	if own := cmd.Flags().Lookup("dry-run"); own != nil && own != rootDryRunFlag {
		return nil
	}
	return &suggest.FlagError{Flag: "dry-run", Message: dryRunUnavailableMessage(cmd)}
}

package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
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
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(output.Response{
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

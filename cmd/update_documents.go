package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/apply"
	"github.com/dynatrace-oss/dtctl/pkg/util/template"
)

// updateDocumentCmd updates an existing document of any type from a file.
//
// It is the update-only counterpart to 'create document': it accepts the same
// --type + --id handling but fails instead of creating when the target document
// does not exist. For create-or-update semantics use 'dtctl apply' instead.
var updateDocumentCmd = &cobra.Command{
	Use:   "document -f <file> --id <id> [--type <type>]",
	Short: "Update an existing document of any type from a file",
	Long: `Update an existing document from a YAML or JSON file.

This is the update-only counterpart to 'create document'. It works for any
document type — dashboard, notebook, launchpad, or custom app documents such as
acme:config — and completes the round-trip that 'create document' starts:

  dtctl get document acme-config -o json > doc.json
  # edit doc.json...
  dtctl update document -f doc.json                # type + id read from the file

The document type is resolved from --type, falling back to the "type" field in
the payload. The target ID is resolved from --id, falling back to the "id" field
in the payload. Unlike 'apply', this command fails if the document does not
already exist, so a typo in the ID never silently creates a new document.

Labels are set with repeatable --label flags, or carried in the payload under a
"labels" array (as produced by 'dtctl get document -o json'). Providing labels
replaces the document's entire label set; omitting them leaves labels unchanged.

Examples:
  # Round-trip update (type and id come from the exported file)
  dtctl update document -f doc.json

  # Update a custom document by explicit type and id
  dtctl update document -f content.json --type acme:config --id acme-config

  # Replace the document's labels
  dtctl update document -f doc.json --label team-a --label env:prod

  # Preview the change without applying it
  dtctl update document -f doc.json --dry-run

  # Show what changed
  dtctl update document -f doc.json --show-diff

See also:
  dtctl create document --help   # create a new document of any type
  dtctl apply --help             # create-or-update semantics
`,
	Aliases: []string{"doc"},
	RunE:    updateDocumentRunE,
}

// updateDocumentRunE updates a document by delegating to the shared applier with
// update-only semantics (RequireExisting). Reusing the applier keeps document
// content extraction, ownership-aware safety checks, dry-run, and diff rendering
// identical to 'apply'.
func updateDocumentRunE(cmd *cobra.Command, _ []string) error {
	file, _ := cmd.Flags().GetString("file")
	if file == "" {
		return fmt.Errorf("--file is required")
	}

	docType, _ := cmd.Flags().GetString("type")
	id, _ := cmd.Flags().GetString("id")
	setFlags, _ := cmd.Flags().GetStringArray("set")
	labels, _ := cmd.Flags().GetStringArray("label")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	showDiff, _ := cmd.Flags().GetBool("show-diff")

	fileData, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	var templateVars map[string]interface{}
	if len(setFlags) > 0 {
		templateVars, err = template.ParseSetFlags(setFlags)
		if err != nil {
			return fmt.Errorf("invalid --set flag: %w", err)
		}
	}

	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	c, err := NewClientFromConfig(cfg)
	if err != nil {
		return err
	}

	applier := apply.NewApplier(c)
	if !dryRun {
		checker, err := NewSafetyChecker(cfg)
		if err != nil {
			return err
		}
		applier = applier.WithSafetyChecker(checker)
	}

	results, err := applier.Apply(fileData, apply.ApplyOptions{
		TemplateVars:    templateVars,
		DryRun:          dryRun,
		ShowDiff:        showDiff,
		OverrideID:      id,
		Type:            docType,
		Labels:          labels,
		RequireExisting: true,
	})
	if err != nil {
		return err
	}

	printer := NewPrinter()
	enrichAgent(printer, "update", "document")
	if len(results) == 1 {
		return printer.Print(results[0])
	}
	items := make([]interface{}, len(results))
	for i, r := range results {
		items[i] = r
	}
	return printer.PrintList(items)
}

func init() {
	updateDocumentCmd.Flags().StringP("file", "f", "", "file containing the document definition (required)")
	updateDocumentCmd.Flags().String("type", "", "document type (e.g. launchpad, acme:config); read from payload if not provided")
	updateDocumentCmd.Flags().String("id", "", "ID of the document to update; read from payload if not provided")
	updateDocumentCmd.Flags().StringArray("set", []string{}, "set template variable (key=value)")
	updateDocumentCmd.Flags().StringArray("label", []string{}, "classification label to set (repeatable); replaces the document's labels. Falls back to labels in the payload")
	updateDocumentCmd.Flags().Bool("dry-run", false, "preview the update without applying it")
	updateDocumentCmd.Flags().Bool("show-diff", false, "show a diff of the change")
	_ = updateDocumentCmd.MarkFlagRequired("file")
}

package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/classicpipelinestranslate"
)

// getClassicPipelinesTranslationCmd translates a Classic pipeline into an
// OpenPipeline configuration pipeline.
var getClassicPipelinesTranslationCmd = &cobra.Command{
	Use:   "classic-pipelines-translation <logs|bizevents>",
	Short: "Translate a Classic pipeline into an OpenPipeline configuration pipeline",
	Long: `Translate the tenant's Classic pipeline for a configuration scope into an
OpenPipeline configuration pipeline (Settings shape).

This is a read-only call that returns the translated pipeline verbatim. The
translation is deterministic where possible; processing rules whose definition
script could not be translated automatically are reported via a warning, and
need a manual rewrite.

The scope is a positional argument and must be one of: logs, bizevents.

Note: the underlying API is public but early-adopter and may change.

Examples:
  # Translate the logs Classic pipeline (pretty-printed pipeline document)
  dtctl get classic-pipelines-translation logs

  # Translate bizevents and export the document as YAML for review/editing
  dtctl get classic-pipelines-translation bizevents -o yaml > reference-pipeline.yaml

  # Full result including the withWarning flag
  dtctl get classic-pipelines-translation logs -o json

  # Keep disabled rules in the translation
  dtctl get classic-pipelines-translation logs --skip-disabled-rules=false`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		scope := args[0]
		if !classicpipelinestranslate.IsValidConfiguration(scope) {
			return fmt.Errorf(
				"invalid configuration scope %q: must be one of %s",
				scope, strings.Join(classicpipelinestranslate.ValidConfigurations, ", "),
			)
		}

		includeSampleData, _ := cmd.Flags().GetBool("include-sample-data")
		skipDisabledRules, _ := cmd.Flags().GetBool("skip-disabled-rules")
		skipBuiltinProcessingRules, _ := cmd.Flags().GetBool("skip-builtin-processing-rules")

		_, c, printer, err := Setup()
		if err != nil {
			return err
		}

		handler := classicpipelinestranslate.NewHandler(c)
		result, err := handler.Translate(classicpipelinestranslate.TranslateOptions{
			Configuration:              scope,
			IncludeSampleData:          includeSampleData,
			SkipDisabledRules:          skipDisabledRules,
			SkipBuiltinProcessingRules: skipBuiltinProcessingRules,
		})
		if err != nil {
			return err
		}

		ap := enrichAgent(printer, "get", "classic-pipelines-translation")

		// Surface the partial-translation warning where it won't corrupt piped
		// output: on stderr for humans, via the agent envelope for agents.
		if result.WithWarning {
			const warn = "some processing rules could not be translated automatically and need a manual rewrite (withWarning=true)"
			if ap != nil {
				ap.SetWarnings([]string{warn})
			} else {
				output.PrintWarning("%s", warn)
			}
		}

		if ap != nil {
			ap.SetSuggestions([]string{
				"Review the translated pipeline, then apply it with 'dtctl create settings --schema builtin:openpipeline.* -f <file>'",
			})
			return printer.Print(result)
		}

		// In an explicitly requested structured format, print the full result
		// ({value, withWarning}). Otherwise the translated pipeline is the
		// deliverable, so print just the document as indented JSON.
		switch outputFormat {
		case "json", "yaml", "yml":
			return printer.Print(result)
		default:
			return printIndentedJSON(result.Value)
		}
	},
}

// printIndentedJSON writes v to stdout as indented JSON followed by a newline.
func printIndentedJSON(v any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
}

func init() {
	getClassicPipelinesTranslationCmd.Flags().Bool("include-sample-data", false, "Include processor sample data in the translation")
	getClassicPipelinesTranslationCmd.Flags().Bool("skip-disabled-rules", true, "Skip disabled rules during translation")
	getClassicPipelinesTranslationCmd.Flags().Bool("skip-builtin-processing-rules", false, "Skip built-in processing rules during translation")
}

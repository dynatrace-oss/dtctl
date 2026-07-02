package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/analyzer"
)

// describeAnalyzerCmd shows details of a Davis analyzer, including its resolved
// input and result JSON Schemas.
var describeAnalyzerCmd = &cobra.Command{
	Use:     "analyzer <name>",
	Aliases: []string{"analyzers", "az"},
	Short:   "Show details of a Davis AI analyzer",
	Long: `Show detailed information about a Davis AI analyzer, including its input and
result schemas so you know what to pass to 'dtctl exec analyzer'.

Unlike 'get analyzer', which returns the raw analyzer definition, 'describe'
resolves the analyzer's JSON Schemas and renders the required and optional
input fields. The schemas are included in JSON/YAML output as well.

Examples:
  # Describe an analyzer and see its input fields
  dtctl describe analyzer dt.statistics.GenericForecastAnalyzer

  # Include the full markdown documentation
  dtctl describe analyzer dt.statistics.GenericForecastAnalyzer --doc

  # Structured output (includes inputSchema and resultSchema)
  dtctl describe analyzer dt.statistics.GenericForecastAnalyzer -o json
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		_, c, printer, err := Setup()
		if err != nil {
			return err
		}

		handler := analyzer.NewHandler(c)

		// --doc short-circuits to raw markdown documentation.
		if doc, _ := cmd.Flags().GetBool("doc"); doc {
			md, err := handler.GetDocumentation(name)
			if err != nil {
				return err
			}
			fmt.Println(md)
			return nil
		}

		def, err := handler.Get(name)
		if err != nil {
			return err
		}

		// Schema calls are best-effort: an analyzer without a published schema
		// should still describe successfully.
		inputSchema, _ := handler.GetInputSchema(name)
		resultSchema, _ := handler.GetResultSchema(name)

		desc := &analyzer.AnalyzerDescription{
			Name:         def.Name,
			DisplayName:  def.DisplayName,
			Description:  def.Description,
			Type:         def.Type,
			Labels:       def.Labels,
			InputSchema:  inputSchema,
			ResultSchema: resultSchema,
		}
		if def.Category != nil {
			desc.Category = def.Category.DisplayName
		}

		if useAnalyzerDescribeTextView() {
			printAnalyzerDescribe(desc)
			return nil
		}

		ap := enrichAgent(printer, "describe", "analyzer")
		if ap != nil {
			ap.Context().Suggestions = []string{
				fmt.Sprintf("dtctl exec analyzer %s --query <dql>  -- run this analyzer", name),
				fmt.Sprintf("dtctl verify analyzer %s -f input.json  -- validate an input", name),
				fmt.Sprintf("dtctl describe analyzer %s --doc  -- full markdown docs", name),
			}
		}
		return printer.Print(desc)
	},
}

// useAnalyzerDescribeTextView reports whether to render the human-readable text
// view. Agent mode always takes the structured envelope path — note that agent
// mode leaves outputFormat at its "table" default, so a bare format check would
// wrongly emit human text into an agent session.
func useAnalyzerDescribeTextView() bool {
	if agentMode {
		return false
	}
	return outputFormat == "" || outputFormat == "table"
}

// printAnalyzerDescribe renders the human-readable table view.
func printAnalyzerDescribe(d *analyzer.AnalyzerDescription) {
	const w = 14
	output.DescribeKV("Name:", w, "%s", d.Name)
	output.DescribeKV("Display Name:", w, "%s", d.DisplayName)
	if d.Category != "" {
		output.DescribeKV("Category:", w, "%s", d.Category)
	}
	if d.Type != "" {
		output.DescribeKV("Type:", w, "%s", d.Type)
	}
	if d.Description != "" {
		output.DescribeKV("Description:", w, "%s", d.Description)
	}
	if len(d.Labels) > 0 {
		output.DescribeKV("Labels:", w, "%s", strings.Join(d.Labels, ", "))
	}

	printSchemaSection("Input", d.InputSchema)
	printSchemaSection("Output", d.ResultSchema)

	fmt.Println()
	fmt.Printf("  Run it:  dtctl exec analyzer %s --query <dql>\n", d.Name)
	fmt.Printf("  Docs:    dtctl describe analyzer %s --doc\n", d.Name)
}

// printSchemaSection prints a flattened schema. "Input" splits into required and
// optional groups; other sections list all fields together.
func printSchemaSection(title string, schema map[string]interface{}) {
	fields, ok := analyzer.FlattenSchema(schema)
	if !ok {
		fmt.Println()
		output.DescribeSection(title + ":")
		fmt.Println("  (schema not introspectable — use -o json or --doc)")
		return
	}

	if title == "Input" {
		printSchemaFields("Input (required)", filterFields(fields, true))
		printSchemaFields("Input (optional)", filterFields(fields, false))
		return
	}
	printSchemaFields(title, fields)
}

func printSchemaFields(title string, fields []analyzer.SchemaField) {
	if len(fields) == 0 {
		return
	}
	fmt.Println()
	output.DescribeSection(title + ":")
	// Column-align the name and type.
	nameW := 0
	for _, f := range fields {
		if len(f.Name) > nameW {
			nameW = len(f.Name)
		}
	}
	for _, f := range fields {
		desc := f.Description
		if f.Composite {
			desc = "(composite — see -o json or --doc)"
		}
		fmt.Printf("  %-*s  %-9s  %s\n", nameW, f.Name, f.Type, desc)
	}
}

func filterFields(fields []analyzer.SchemaField, required bool) []analyzer.SchemaField {
	var out []analyzer.SchemaField
	for _, f := range fields {
		if f.Required == required {
			out = append(out, f)
		}
	}
	return out
}

func init() {
	describeAnalyzerCmd.Flags().Bool("doc", false, "print the analyzer's markdown documentation")
}

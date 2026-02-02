package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/resources/appengine"
	"github.com/spf13/cobra"
)

var (
	findIntentsData     string
	findIntentsDataFile string
	findIntentsLimit    int
)

// findIntentsCmd finds intents that match given data
var findIntentsCmd = &cobra.Command{
	Use:   "intents",
	Short: "Find intents that match given data",
	Long: `Find app intents that can handle the provided data.

This command matches the provided data against all available intents
and returns intents that can handle the data, sorted by match quality.

Match quality is calculated based on property coverage:
  - 0%: Intent has required properties missing from data
  - 1-100%: Percentage of intent properties present in data

Examples:
  # Find intents for trace data
  dtctl find intents --data trace_id=abc123,timestamp=2026-02-02T10:00:00Z

  # Find intents from JSON file
  dtctl find intents --data-file payload.json

  # Find intents from JSON stdin
  echo '{"trace_id":"abc123"}' | dtctl find intents --data-file -

  # Limit results
  dtctl find intents --data trace_id=abc123 --limit 5

  # Output as JSON
  dtctl find intents --data trace_id=abc123 -o json
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if findIntentsData == "" && findIntentsDataFile == "" {
			return fmt.Errorf("either --data or --data-file must be specified")
		}

		if findIntentsData != "" && findIntentsDataFile != "" {
			return fmt.Errorf("--data and --data-file are mutually exclusive")
		}

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := appengine.NewIntentHandler(c)

		// Parse data
		var data map[string]interface{}

		if findIntentsDataFile != "" {
			// Read from file
			var content []byte
			if findIntentsDataFile == "-" {
				content, err = os.ReadFile("/dev/stdin")
			} else {
				content, err = os.ReadFile(findIntentsDataFile)
			}
			if err != nil {
				return fmt.Errorf("failed to read data file: %w", err)
			}

			if err := json.Unmarshal(content, &data); err != nil {
				return fmt.Errorf("failed to parse JSON data: %w", err)
			}
		} else {
			// Parse key=value pairs
			data = make(map[string]interface{})
			pairs := strings.Split(findIntentsData, ",")
			for _, pair := range pairs {
				parts := strings.SplitN(pair, "=", 2)
				if len(parts) != 2 {
					return fmt.Errorf("invalid data format, expected key=value, got %q", pair)
				}
				data[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
		}

		// Find matching intents
		matches, err := handler.FindIntentsForData(data)
		if err != nil {
			return err
		}

		// Apply limit
		if findIntentsLimit > 0 && len(matches) > findIntentsLimit {
			matches = matches[:findIntentsLimit]
		}

		// Print results
		printer := NewPrinter()
		return printer.PrintList(matches)
	},
}

func init() {
	findIntentsCmd.Flags().StringVar(&findIntentsData, "data", "", "data as comma-separated key=value pairs")
	findIntentsCmd.Flags().StringVar(&findIntentsDataFile, "data-file", "", "JSON file containing data (use - for stdin)")
	findIntentsCmd.Flags().IntVar(&findIntentsLimit, "limit", 0, "limit number of results (0 for unlimited)")
}

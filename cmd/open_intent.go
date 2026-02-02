package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/resources/appengine"
	"github.com/spf13/cobra"
)

var (
	openIntentData     string
	openIntentDataFile string
	openIntentBrowser  bool
)

// openIntentCmd generates and optionally opens an intent URL
var openIntentCmd = &cobra.Command{
	Use:   "intent <app-id>/<intent-id>",
	Short: "Generate and open an intent URL",
	Long: `Generate an intent URL for opening a resource in a Dynatrace app.

Intent URLs enable navigation to specific app views with contextual data.
The URL can be printed or opened directly in a browser.

Examples:
  # Generate intent URL
  dtctl open intent dynatrace.distributedtracing/view-trace --data trace_id=abc123

  # Generate with multiple properties
  dtctl open intent dynatrace.distributedtracing/view-trace-addon \\
    --data trace_id=abc123,timestamp=2026-02-02T16:04:19.947000000Z

  # Generate from JSON file
  dtctl open intent dynatrace.logs/view-log-entry --data-file payload.json

  # Generate and open in browser
  dtctl open intent dynatrace.distributedtracing/view-trace \\
    --data trace_id=abc123 --browser
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if openIntentData == "" && openIntentDataFile == "" {
			return fmt.Errorf("either --data or --data-file must be specified")
		}

		if openIntentData != "" && openIntentDataFile != "" {
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

		// Parse app-id/intent-id
		fullName := args[0]
		parts := strings.SplitN(fullName, "/", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid format, expected 'app-id/intent-id', got %q", fullName)
		}
		appID := parts[0]
		intentID := parts[1]

		// Parse data
		var data map[string]interface{}

		if openIntentDataFile != "" {
			// Read from file
			var content []byte
			if openIntentDataFile == "-" {
				content, err = os.ReadFile("/dev/stdin")
			} else {
				content, err = os.ReadFile(openIntentDataFile)
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
			pairs := strings.Split(openIntentData, ",")
			for _, pair := range pairs {
				parts := strings.SplitN(pair, "=", 2)
				if len(parts) != 2 {
					return fmt.Errorf("invalid data format, expected key=value, got %q", pair)
				}
				data[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
		}

		// Generate URL
		intentURL, err := handler.GenerateIntentURL(appID, intentID, data)
		if err != nil {
			return err
		}

		// Print URL
		fmt.Println(intentURL)

		// Open in browser if requested
		if openIntentBrowser {
			if err := openBrowser(intentURL); err != nil {
				return fmt.Errorf("failed to open browser: %w", err)
			}
		}

		return nil
	},
}

// openBrowser opens a URL in the default browser
func openBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	return cmd.Start()
}

func init() {
	openCmd.AddCommand(openIntentCmd)
	openIntentCmd.Flags().StringVar(&openIntentData, "data", "", "data as comma-separated key=value pairs")
	openIntentCmd.Flags().StringVar(&openIntentDataFile, "data-file", "", "JSON file containing data (use - for stdin)")
	openIntentCmd.Flags().BoolVar(&openIntentBrowser, "browser", false, "open URL in browser")
}

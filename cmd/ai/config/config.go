package aiconfig_cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	aiconfig "github.com/dynatrace-oss/dtctl/pkg/ai/config"
	"github.com/dynatrace-oss/dtctl/pkg/output"
)

// ConfigEntry represents a single AI configuration key-value pair.
type ConfigEntry struct {
	Key   string `table:"KEY"   json:"key"   yaml:"key"`
	Value string `table:"VALUE" json:"value" yaml:"value"`
}

// newPrinterFromCmd builds a Printer that respects --output, --plain, --agent, and --jq
// flags from the root command, mirroring the behaviour of cmd.NewPrinter().
func newPrinterFromCmd(cmd *cobra.Command, verb, resource string) output.Printer {
	flags := cmd.Root().PersistentFlags()

	format, _ := flags.GetString("output")
	plain, _ := flags.GetBool("plain")
	agentMode, _ := flags.GetBool("agent")
	jq, _ := flags.GetString("jq")

	if agentMode {
		ctx := &output.ResponseContext{Verb: verb, Resource: resource}
		ap := output.NewAgentPrinter(os.Stdout, ctx)
		ap.SetJQFilter(jq)
		ap.SetResultFormat(format)
		return ap
	}

	return output.NewPrinterWithOpts(output.PrinterOptions{
		Format:    format,
		Writer:    os.Stdout,
		PlainMode: plain,
		JQFilter:  jq,
	})
}

// ConfigCmd is the root "dtctl ai config" command.
var ConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage dtctl AI configuration",
	Long: `View and modify dtctl AI configuration.

Configuration is stored in ~/.dtctl/ai.yml and can be overridden per-variable
using environment variables of the form DTCTL_AI_<KEY_UPPERCASE>.

Examples:
  dtctl ai config set openai_baseurl "https://api.openai.com/v1"
  dtctl ai config set openai_token sk-...
  dtctl ai config get openai_baseurl
  dtctl ai config ls`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set an AI configuration value",
	Long: `Set an AI configuration value in ~/.dtctl/ai.yml.

Available keys:
  openai_baseurl, openai_token
  openrouter_baseurl, openrouter_token
  deepseek_baseurl, deepseek_token
  anthropic_baseurl, anthropic_token
  google_baseurl, google_token
  mistral_baseurl, mistral_token
  gitlab_baseurl, gitlab_token, gitlab_webhook_secret
  agent_name, default_provider, default_model

Examples:
  dtctl ai config set openai_token sk-...
  dtctl ai config set default_provider anthropic
  dtctl ai config set google_baseurl "https://generativelanguage.googleapis.com/v1beta"`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := strings.TrimSpace(args[0])
		value := args[1]

		if err := aiconfig.Set(key, value); err != nil {
			return err
		}

		fmt.Printf("Set %s in %s\n", key, aiconfig.ConfigFilePath())
		return nil
	},
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get an AI configuration value",
	Long: `Get an AI configuration value. Environment variable overrides (DTCTL_AI_<KEY>) are applied.

Examples:
  dtctl ai config get openai_baseurl
  dtctl ai config get default_provider`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := strings.TrimSpace(args[0])
		value, err := aiconfig.Get(key)
		if err != nil {
			return err
		}

		printer := newPrinterFromCmd(cmd, "get", "ai-config")
		return printer.Print(ConfigEntry{Key: key, Value: value})
	},
}

var configLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List all AI configuration values",
	Long: `List all AI configuration values. Token values are masked.
Environment variable overrides (DTCTL_AI_<KEY>) are applied.

Examples:
  dtctl ai config ls`,
	RunE: func(cmd *cobra.Command, args []string) error {
		all, err := aiconfig.ListAll()
		if err != nil {
			return err
		}

		keys := make([]string, 0, len(all))
		for k := range all {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		printer := newPrinterFromCmd(cmd, "get", "ai-config")
		if err := printer.PrintList(keys); err != nil {
			return err
		}

		return nil
	},
}

func init() {
	ConfigCmd.DisableFlagsInUseLine = true
	ConfigCmd.AddCommand(configSetCmd)
	ConfigCmd.AddCommand(configGetCmd)
	ConfigCmd.AddCommand(configLsCmd)
}

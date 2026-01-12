package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

// completionCmd represents the completion command
var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion scripts",
	Long: `Generate shell completion scripts for dtctl.

Examples:
  # bash (temporary)
  source <(dtctl completion bash)

  # bash (permanent)
  sudo cp <(dtctl completion bash) /etc/bash_completion.d/dtctl

  # zsh
  mkdir -p ~/.zsh/completions
  dtctl completion zsh > ~/.zsh/completions/_dtctl
  # Add to ~/.zshrc: fpath=(~/.zsh/completions $fpath)
  # Then: rm -f ~/.zcompdump* && autoload -U compinit && compinit

  # fish
  dtctl completion fish > ~/.config/fish/completions/dtctl.fish

  # powershell
  dtctl completion powershell | Out-String | Invoke-Expression
`,
	ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
	Args:      cobra.ExactValidArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		switch args[0] {
		case "bash":
			cmd.Root().GenBashCompletion(os.Stdout)
		case "zsh":
			cmd.Root().GenZshCompletion(os.Stdout)
		case "fish":
			cmd.Root().GenFishCompletion(os.Stdout, true)
		case "powershell":
			cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
		}
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
}

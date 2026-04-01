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

  # zsh (with alias, e.g. "dt")
  # After installing completions as above, add to ~/.zshrc:
  #   compdef dt=dtctl

  # fish
  dtctl completion fish > ~/.config/fish/completions/dtctl.fish

  # powershell
  dtctl completion powershell | Out-String | Invoke-Expression

Note:
  If you previously generated completions with an older version of dtctl,
  remove the old completion files and regenerate them:
    rm -f ~/.zcompdump* ~/.zsh/completions/_dtctl
    dtctl completion zsh > ~/.zsh/completions/_dtctl
`,
	ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
	Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	Run: func(cmd *cobra.Command, args []string) {
		switch args[0] {
		case "bash":
			_ = cmd.Root().GenBashCompletionV2(os.Stdout, true)
		case "zsh":
			_ = cmd.Root().GenZshCompletion(os.Stdout)
		case "fish":
			_ = cmd.Root().GenFishCompletion(os.Stdout, true)
		case "powershell":
			_ = cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
		}
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
}

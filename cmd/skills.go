package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/skills"
	"github.com/dynatrace-oss/dtctl/pkg/suggest"
)

// skillsCmd is the parent command for AI assistant skill management.
var skillsCmd = &cobra.Command{
	Use:   "skills",
	Short: "Manage AI coding assistant skill files",
	Long: `Manage dtctl skill files for AI coding assistants.

Skill files teach your AI assistant how to use dtctl effectively.
Supported agents: claude, copilot, cursor, kiro, opencode, openclaw.`,
	Example: `  # Auto-detect agent and install skill file
  dtctl skills install

  # Install for a specific agent
  dtctl skills install --for claude

  # List all supported agents
  dtctl skills install --list

  # Check what's installed
  dtctl skills status`,
	RunE: requireSkillsSubcommand,
}

// requireSkillsSubcommand returns a helpful error when no subcommand is given.
// Unlike requireSubcommand (which says "resource type"), this uses wording
// appropriate for a utility command whose subcommands are verbs.
func requireSkillsSubcommand(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		var subs []string
		for _, sub := range cmd.Commands() {
			if sub.IsAvailableCommand() {
				subs = append(subs, sub.Name())
			}
		}
		return fmt.Errorf("requires a subcommand\n\nAvailable subcommands:\n  %s\n\nUsage:\n  %s <subcommand> [flags]",
			strings.Join(subs, "\n  "), cmd.CommandPath())
	}

	subcommands := collectSubcommands(cmd)
	if suggestion := suggest.FindClosest(args[0], subcommands); suggestion != nil {
		return fmt.Errorf("unknown subcommand %q, did you mean %q?", args[0], suggestion.Value)
	}

	return fmt.Errorf("unknown subcommand %q\nRun '%s --help' for available subcommands", args[0], cmd.CommandPath())
}

// skillsInstallCmd installs skill files for an AI coding assistant.
var skillsInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install skill file for an AI coding assistant",
	Long: `Install a dtctl skill file for the specified AI coding assistant.

If no agent is specified with --for, the command auto-detects the current
agent from environment variables. Use --global to install to the user-wide
location instead of the project directory.

Examples:
  # Auto-detect and install
  dtctl skills install

  # Install for Claude Code
  dtctl skills install --for claude

  # Install globally (Claude Code only)
  dtctl skills install --for claude --global

  # Overwrite existing file
  dtctl skills install --for claude --force

  # List supported agents
  dtctl skills install --list`,
	RunE: runSkillsInstall,
}

// skillsUninstallCmd removes installed skill files.
var skillsUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove installed skill files",
	Long: `Remove dtctl skill files installed for an AI coding assistant.

If no agent is specified with --for, the command auto-detects the current
agent. Removes skill files from both project-local and global locations.

Examples:
  # Auto-detect and uninstall
  dtctl skills uninstall

  # Uninstall for a specific agent
  dtctl skills uninstall --for claude`,
	RunE: runSkillsUninstall,
}

// skillsStatusCmd shows the installation state of skill files.
var skillsStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show installation status of skill files",
	Long: `Show the current installation status of dtctl skill files.

Checks both project-local and global locations for all supported agents.

Examples:
  # Check all agents
  dtctl skills status

  # Check a specific agent
  dtctl skills status --for claude`,
	RunE: runSkillsStatus,
}

// agentCompletionFunc provides shell completion for the --for flag.
func agentCompletionFunc(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	var completions []string
	for _, a := range skills.AllAgents() {
		completions = append(completions, a.Name+"\t"+a.DisplayName)
	}
	return completions, cobra.ShellCompDirectiveNoFileComp
}

func init() {
	rootCmd.AddCommand(skillsCmd)

	skillsCmd.AddCommand(skillsInstallCmd)
	skillsCmd.AddCommand(skillsUninstallCmd)
	skillsCmd.AddCommand(skillsStatusCmd)

	// Flags for install
	skillsInstallCmd.Flags().String("for", "", "install for a specific agent (claude, copilot, cursor, kiro, opencode, openclaw)")
	skillsInstallCmd.Flags().Bool("global", false, "install to user-wide location instead of project directory")
	skillsInstallCmd.Flags().Bool("force", false, "overwrite existing files without prompting")
	skillsInstallCmd.Flags().Bool("list", false, "list all supported agents")
	_ = skillsInstallCmd.RegisterFlagCompletionFunc("for", agentCompletionFunc)

	// Flags for uninstall
	skillsUninstallCmd.Flags().String("for", "", "uninstall for a specific agent")
	_ = skillsUninstallCmd.RegisterFlagCompletionFunc("for", agentCompletionFunc)

	// Flags for status
	skillsStatusCmd.Flags().String("for", "", "check status for a specific agent")
	_ = skillsStatusCmd.RegisterFlagCompletionFunc("for", agentCompletionFunc)
}

// skillsInstallAgentResult is the structured result for agent-mode output.
type skillsInstallAgentResult struct {
	Action string `json:"action"`
	Agent  string `json:"agent"`
	Path   string `json:"path"`
	Scope  string `json:"scope"`
}

// skillsUninstallAgentResult is the structured result for agent-mode output.
type skillsUninstallAgentResult struct {
	Agent   string   `json:"agent"`
	Removed []string `json:"removed"`
}

// skillsStatusAgentEntry is a single agent's status for agent-mode output.
type skillsStatusAgentEntry struct {
	Agent     string `json:"agent"`
	Installed bool   `json:"installed"`
	Path      string `json:"path,omitempty"`
	Scope     string `json:"scope,omitempty"`
}

// skillsListAgentEntry is a single agent's info for agent-mode --list output.
type skillsListAgentEntry struct {
	Name           string `json:"name"`
	DisplayName    string `json:"display_name"`
	ProjectPath    string `json:"project_path"`
	SupportsGlobal bool   `json:"supports_global"`
}

// runSkillsInstall installs skill files.
func runSkillsInstall(cmd *cobra.Command, _ []string) error {
	listFlag, _ := cmd.Flags().GetBool("list")
	if listFlag {
		return runSkillsList()
	}

	forFlag, _ := cmd.Flags().GetString("for")
	globalFlag, _ := cmd.Flags().GetBool("global")
	forceFlag, _ := cmd.Flags().GetBool("force")

	agent, err := resolveAgent(forFlag)
	if err != nil {
		return err
	}

	baseDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to determine working directory: %w", err)
	}

	result, err := skills.Install(agent, baseDir, globalFlag, forceFlag)
	if err != nil {
		return err
	}

	scope := "project"
	if result.Global {
		scope = "global"
	}

	printer := NewPrinter()
	if ap := enrichAgent(printer, "install", "skills"); ap != nil {
		ap.SetSuggestions([]string{
			"Run 'dtctl skills status' to verify installation",
		})
	}

	if agentMode {
		action := "installed"
		if result.Replaced {
			action = "updated"
		}
		return printer.Print(skillsInstallAgentResult{
			Action: action,
			Agent:  result.Agent.Name,
			Path:   result.Path,
			Scope:  scope,
		})
	}

	if result.Replaced {
		output.PrintSuccess("Updated %s skill file: %s", result.Agent.DisplayName, result.Path)
	} else {
		output.PrintSuccess("Installed %s skill file: %s", result.Agent.DisplayName, result.Path)
	}
	output.PrintInfo("Scope: %s", scope)

	return nil
}

// runSkillsList lists all supported agents.
func runSkillsList() error {
	printer := NewPrinter()
	if ap := enrichAgent(printer, "list", "skills"); ap != nil {
		ap.SetTotal(len(skills.AllAgents()))
	}

	if agentMode {
		var entries []skillsListAgentEntry
		for _, a := range skills.AllAgents() {
			entries = append(entries, skillsListAgentEntry{
				Name:           a.Name,
				DisplayName:    a.DisplayName,
				ProjectPath:    a.ProjectPath,
				SupportsGlobal: a.GlobalPath != "",
			})
		}
		return printer.PrintList(entries)
	}

	fmt.Println("Supported agents:")
	for _, a := range skills.AllAgents() {
		globalNote := ""
		if a.GlobalPath != "" {
			globalNote = " (supports --global)"
		}
		fmt.Printf("  %-10s %s%s\n", a.Name, a.DisplayName, globalNote)
		fmt.Printf("             Project path: %s\n", a.ProjectPath)
	}
	return nil
}

// runSkillsUninstall removes installed skill files.
func runSkillsUninstall(cmd *cobra.Command, _ []string) error {
	forFlag, _ := cmd.Flags().GetString("for")
	agent, err := resolveAgent(forFlag)
	if err != nil {
		return err
	}

	baseDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to determine working directory: %w", err)
	}

	removed, err := skills.Uninstall(agent, baseDir)
	if err != nil {
		return err
	}

	printer := NewPrinter()
	if ap := enrichAgent(printer, "uninstall", "skills"); ap != nil {
		ap.SetSuggestions([]string{
			"Run 'dtctl skills status' to verify removal",
		})
	}

	if agentMode {
		return printer.Print(skillsUninstallAgentResult{
			Agent:   agent.Name,
			Removed: removed,
		})
	}

	if len(removed) == 0 {
		output.PrintInfo("No %s skill files found to remove.", agent.DisplayName)
		return nil
	}

	for _, path := range removed {
		output.PrintSuccess("Removed: %s", path)
	}

	return nil
}

// runSkillsStatus shows installation status.
func runSkillsStatus(cmd *cobra.Command, _ []string) error {
	baseDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to determine working directory: %w", err)
	}

	forFlag, _ := cmd.Flags().GetString("for")

	printer := NewPrinter()
	ap := enrichAgent(printer, "status", "skills")

	if forFlag != "" {
		agent, err := resolveAgent(forFlag)
		if err != nil {
			return err
		}

		result := skills.Status(agent, baseDir)
		if agentMode {
			return printer.Print(statusToAgentEntry(result))
		}
		// Detect agent only for human-readable "(detected via ...)" annotation
		detectedAgent, detected := skills.DetectAgent()
		printStatus(result, detectedAgent, detected)
		return nil
	}

	// Show all agents
	results := skills.StatusAll(baseDir)

	if agentMode {
		var entries []skillsStatusAgentEntry
		for _, r := range results {
			entries = append(entries, statusToAgentEntry(r))
		}
		if ap != nil {
			ap.SetTotal(len(entries))
		}
		return printer.PrintList(entries)
	}

	// Detect agent only for human-readable "(detected via ...)" annotation
	detectedAgent, detected := skills.DetectAgent()
	anyInstalled := false
	for _, r := range results {
		if r.Installed {
			anyInstalled = true
			printStatus(r, detectedAgent, detected)
			output.PrintInfo("")
		}
	}

	if !anyInstalled {
		output.PrintInfo("No skill files installed.")
		output.PrintInfo("Run 'dtctl skills install' to get started.")
	}

	return nil
}

// statusToAgentEntry converts a StatusResult to the agent-mode JSON entry.
func statusToAgentEntry(r *skills.StatusResult) skillsStatusAgentEntry {
	entry := skillsStatusAgentEntry{
		Agent:     r.Agent.Name,
		Installed: r.Installed,
	}
	if r.Installed {
		entry.Path = r.Path
		scope := "project"
		if r.Global {
			scope = "global"
		}
		entry.Scope = scope
	}
	return entry
}

// printStatus prints a single agent's status in human-readable format.
func printStatus(r *skills.StatusResult, detectedAgent skills.Agent, detected bool) {
	suffix := ""
	if detected && detectedAgent.Name == r.Agent.Name {
		suffix = fmt.Sprintf(" (detected via %s env)", r.Agent.EnvVar)
	}
	output.PrintInfo("Agent:     %s%s", r.Agent.DisplayName, suffix)

	if r.Installed {
		scope := "project"
		if r.Global {
			scope = "global"
		}
		output.PrintInfo("Installed: %s (%s)", r.Path, scope)
	} else {
		output.PrintInfo("Installed: no")
	}
}

// resolveAgent resolves the target agent from --for flag or auto-detection.
func resolveAgent(forFlag string) (skills.Agent, error) {
	if forFlag != "" {
		agent, ok := skills.FindAgent(forFlag)
		if !ok {
			return skills.Agent{}, fmt.Errorf(
				"unknown agent %q\nSupported agents: %s",
				forFlag, strings.Join(skills.SupportedAgents(), ", "),
			)
		}
		return agent, nil
	}

	// Auto-detect
	agent, detected := skills.DetectAgent()
	if !detected {
		return skills.Agent{}, fmt.Errorf(
			"no AI agent detected\nUse --for to specify an agent: %s",
			strings.Join(skills.SupportedAgents(), ", "),
		)
	}

	return agent, nil
}

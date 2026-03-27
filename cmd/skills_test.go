package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/skills"
)

// --- helper: reset skills subcommand flags between tests ---

func resetSkillsFlags(t *testing.T) {
	t.Helper()
	for _, cmd := range []*struct{ name, flag, def string }{
		{"install-for", "for", ""},
		{"install-global", "global", "false"},
		{"install-force", "force", "false"},
		{"install-list", "list", "false"},
		{"install-cross-client", "cross-client", "false"},
	} {
		_ = skillsInstallCmd.Flags().Set(cmd.flag, cmd.def)
	}
	_ = skillsUninstallCmd.Flags().Set("for", "")
	_ = skillsUninstallCmd.Flags().Set("cross-client", "false")
	_ = skillsStatusCmd.Flags().Set("for", "")
}

// clearAgentEnvVars unsets all AI-agent env vars to ensure test isolation.
func clearAgentEnvVars(t *testing.T) {
	t.Helper()
	for _, env := range []string{
		"CLAUDECODE", "CURSOR_AGENT", "GITHUB_COPILOT", "JUNIE", "KIRO", "OPENCODE", "OPENCLAW",
		"CODEIUM_AGENT", "TABNINE_AGENT", "AMAZON_Q", "AI_AGENT",
	} {
		t.Setenv(env, "")
	}
}

// --- requireSkillsSubcommand tests ---

func TestSkillsCmd_NoArgs(t *testing.T) {
	err := requireSkillsSubcommand(skillsCmd, []string{})
	if err == nil {
		t.Fatal("expected error when no subcommand given")
	}
	if !strings.Contains(err.Error(), "requires a subcommand") {
		t.Errorf("error should say 'requires a subcommand', got: %v", err)
	}
	if !strings.Contains(err.Error(), "install") {
		t.Errorf("error should list 'install' subcommand, got: %v", err)
	}
	if !strings.Contains(err.Error(), "uninstall") {
		t.Errorf("error should list 'uninstall' subcommand, got: %v", err)
	}
	if !strings.Contains(err.Error(), "status") {
		t.Errorf("error should list 'status' subcommand, got: %v", err)
	}
	// Must NOT say "resource type" (that's the old requireSubcommand wording)
	if strings.Contains(err.Error(), "resource type") {
		t.Errorf("error should NOT say 'resource type', got: %v", err)
	}
}

func TestSkillsCmd_UnknownSubcommand(t *testing.T) {
	err := requireSkillsSubcommand(skillsCmd, []string{"bogus"})
	if err == nil {
		t.Fatal("expected error for unknown subcommand")
	}
	if !strings.Contains(err.Error(), "unknown subcommand") {
		t.Errorf("error should say 'unknown subcommand', got: %v", err)
	}
}

func TestSkillsCmd_TypoSuggestion(t *testing.T) {
	err := requireSkillsSubcommand(skillsCmd, []string{"instal"})
	if err == nil {
		t.Fatal("expected error for typo")
	}
	if !strings.Contains(err.Error(), "did you mean") {
		t.Errorf("error should suggest correction, got: %v", err)
	}
}

// --- resolveAgent tests ---

func TestResolveAgent_Valid(t *testing.T) {
	clearAgentEnvVars(t)

	for _, name := range skills.SupportedAgents() {
		t.Run(name, func(t *testing.T) {
			agent, err := resolveAgent(name)
			if err != nil {
				t.Fatalf("resolveAgent(%q) error: %v", name, err)
			}
			if agent.Name != name {
				t.Errorf("expected agent %q, got %q", name, agent.Name)
			}
		})
	}
}

func TestResolveAgent_Unknown(t *testing.T) {
	clearAgentEnvVars(t)

	agent, err := resolveAgent("vim-copilot")
	if err == nil {
		t.Fatalf("expected error for unknown agent, got: %v", agent)
	}
	if !strings.Contains(err.Error(), "unknown agent") {
		t.Errorf("error should say 'unknown agent', got: %v", err)
	}
	if !strings.Contains(err.Error(), "vim-copilot") {
		t.Errorf("error should include the bad name, got: %v", err)
	}
	// Should list supported agents
	for _, name := range skills.SupportedAgents() {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error should list supported agent %q, got: %v", name, err)
		}
	}
}

func TestResolveAgent_AutoDetect(t *testing.T) {
	clearAgentEnvVars(t)
	t.Setenv("CLAUDECODE", "1")

	agent, err := resolveAgent("")
	if err != nil {
		t.Fatalf("resolveAgent auto-detect error: %v", err)
	}
	if agent.Name != "claude" {
		t.Errorf("expected claude, got %q", agent.Name)
	}
}

func TestResolveAgent_NoDetection(t *testing.T) {
	clearAgentEnvVars(t)

	_, err := resolveAgent("")
	if err == nil {
		t.Fatal("expected error when no agent detected")
	}
	if !strings.Contains(err.Error(), "no AI agent detected") {
		t.Errorf("error should say 'no AI agent detected', got: %v", err)
	}
	if !strings.Contains(err.Error(), "--for") {
		t.Errorf("error should mention --for flag, got: %v", err)
	}
}

// --- statusToAgentEntry tests ---

func TestStatusToAgentEntry_Installed(t *testing.T) {
	agent, _ := skills.FindAgent("claude")
	result := &skills.StatusResult{
		Agent:     agent,
		Installed: true,
		Path:      "/tmp/project/.claude/skills/dtctl",
		Global:    false,
	}

	entry := statusToAgentEntry(result)
	if entry.Agent != "claude" {
		t.Errorf("Agent = %q, want %q", entry.Agent, "claude")
	}
	if !entry.Installed {
		t.Error("expected Installed=true")
	}
	if entry.Path != "/tmp/project/.claude/skills/dtctl" {
		t.Errorf("Path = %q, unexpected", entry.Path)
	}
	if entry.Scope != "project" {
		t.Errorf("Scope = %q, want %q", entry.Scope, "project")
	}
}

func TestStatusToAgentEntry_InstalledGlobal(t *testing.T) {
	agent, _ := skills.FindAgent("claude")
	result := &skills.StatusResult{
		Agent:     agent,
		Installed: true,
		Path:      "/home/user/.claude/skills/dtctl",
		Global:    true,
	}

	entry := statusToAgentEntry(result)
	if entry.Scope != "global" {
		t.Errorf("Scope = %q, want %q", entry.Scope, "global")
	}
}

func TestStatusToAgentEntry_NotInstalled(t *testing.T) {
	agent, _ := skills.FindAgent("cursor")
	result := &skills.StatusResult{
		Agent:     agent,
		Installed: false,
	}

	entry := statusToAgentEntry(result)
	if entry.Installed {
		t.Error("expected Installed=false")
	}
	if entry.Path != "" {
		t.Errorf("Path should be empty, got %q", entry.Path)
	}
	if entry.Scope != "" {
		t.Errorf("Scope should be empty, got %q", entry.Scope)
	}
}

// --- RunE integration tests ---

func TestSkillsInstall_RunE(t *testing.T) {
	clearAgentEnvVars(t)
	origAgentMode := agentMode
	defer func() { agentMode = origAgentMode }()
	agentMode = false

	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	resetSkillsFlags(t)
	_ = skillsInstallCmd.Flags().Set("for", "claude")

	err = skillsInstallCmd.RunE(skillsInstallCmd, []string{})
	if err != nil {
		t.Fatalf("RunE error: %v", err)
	}

	// Verify SKILL.md was created in the skill directory
	expectedPath := filepath.Join(tmpDir, ".claude", "skills", "dtctl", "SKILL.md")
	data, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("SKILL.md not created: %v", err)
	}
	if !strings.Contains(string(data), "dtctl") {
		t.Error("SKILL.md should contain 'dtctl'")
	}

	// Verify references/ directory exists
	refsDir := filepath.Join(tmpDir, ".claude", "skills", "dtctl", "references")
	if info, err := os.Stat(refsDir); err != nil || !info.IsDir() {
		t.Error("references/ directory should exist")
	}
}

func TestSkillsInstall_AllAgents(t *testing.T) {
	clearAgentEnvVars(t)
	origAgentMode := agentMode
	defer func() { agentMode = origAgentMode }()
	agentMode = false

	for _, agentName := range skills.SupportedAgents() {
		t.Run(agentName, func(t *testing.T) {
			tmpDir := t.TempDir()
			origDir, _ := os.Getwd()
			defer func() { _ = os.Chdir(origDir) }()
			_ = os.Chdir(tmpDir)

			resetSkillsFlags(t)
			_ = skillsInstallCmd.Flags().Set("for", agentName)

			err := skillsInstallCmd.RunE(skillsInstallCmd, []string{})
			if err != nil {
				t.Fatalf("RunE error for %s: %v", agentName, err)
			}

			agent, _ := skills.FindAgent(agentName)
			skillFile := filepath.Join(tmpDir, agent.ProjectPath, "SKILL.md")
			if _, err := os.Stat(skillFile); os.IsNotExist(err) {
				t.Errorf("SKILL.md not found at %s", skillFile)
			}
		})
	}
}

func TestSkillsInstall_RefusesOverwrite(t *testing.T) {
	clearAgentEnvVars(t)
	origAgentMode := agentMode
	defer func() { agentMode = origAgentMode }()
	agentMode = false

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	resetSkillsFlags(t)
	_ = skillsInstallCmd.Flags().Set("for", "copilot")

	// First install
	err := skillsInstallCmd.RunE(skillsInstallCmd, []string{})
	if err != nil {
		t.Fatalf("first install error: %v", err)
	}

	// Second install without --force
	resetSkillsFlags(t)
	_ = skillsInstallCmd.Flags().Set("for", "copilot")

	err = skillsInstallCmd.RunE(skillsInstallCmd, []string{})
	if err == nil {
		t.Fatal("expected error on overwrite without --force")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error should mention 'already exists', got: %v", err)
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("error should mention '--force', got: %v", err)
	}
}

func TestSkillsInstall_OverwriteWithForce(t *testing.T) {
	clearAgentEnvVars(t)
	origAgentMode := agentMode
	defer func() { agentMode = origAgentMode }()
	agentMode = false

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	resetSkillsFlags(t)
	_ = skillsInstallCmd.Flags().Set("for", "cursor")

	// First install
	err := skillsInstallCmd.RunE(skillsInstallCmd, []string{})
	if err != nil {
		t.Fatalf("first install error: %v", err)
	}

	// Second install with --force
	resetSkillsFlags(t)
	_ = skillsInstallCmd.Flags().Set("for", "cursor")
	_ = skillsInstallCmd.Flags().Set("force", "true")

	err = skillsInstallCmd.RunE(skillsInstallCmd, []string{})
	if err != nil {
		t.Fatalf("force install error: %v", err)
	}
}

func TestSkillsInstall_GlobalUnsupported(t *testing.T) {
	clearAgentEnvVars(t)
	origAgentMode := agentMode
	defer func() { agentMode = origAgentMode }()
	agentMode = false

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	resetSkillsFlags(t)
	_ = skillsInstallCmd.Flags().Set("for", "cursor")
	_ = skillsInstallCmd.Flags().Set("global", "true")

	err := skillsInstallCmd.RunE(skillsInstallCmd, []string{})
	if err == nil {
		t.Fatal("expected error for unsupported global install")
	}
	if !strings.Contains(err.Error(), "does not support global") {
		t.Errorf("error should mention 'does not support global', got: %v", err)
	}
}

func TestSkillsInstall_AutoDetect(t *testing.T) {
	clearAgentEnvVars(t)
	t.Setenv("OPENCODE", "1")

	origAgentMode := agentMode
	defer func() { agentMode = origAgentMode }()
	agentMode = false

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	resetSkillsFlags(t)
	// No --for flag, should auto-detect

	err := skillsInstallCmd.RunE(skillsInstallCmd, []string{})
	if err != nil {
		t.Fatalf("RunE error: %v", err)
	}

	expectedPath := filepath.Join(tmpDir, ".opencode", "skills", "dtctl", "SKILL.md")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("SKILL.md not found at %s", expectedPath)
	}
}

func TestSkillsInstall_NoAgentDetected(t *testing.T) {
	clearAgentEnvVars(t)
	origAgentMode := agentMode
	defer func() { agentMode = origAgentMode }()
	agentMode = false

	resetSkillsFlags(t)
	// No --for flag, no env var

	err := skillsInstallCmd.RunE(skillsInstallCmd, []string{})
	if err == nil {
		t.Fatal("expected error when no agent detected")
	}
	if !strings.Contains(err.Error(), "no AI agent detected") {
		t.Errorf("error should say 'no AI agent detected', got: %v", err)
	}
}

func TestSkillsUninstall_RunE(t *testing.T) {
	clearAgentEnvVars(t)
	origAgentMode := agentMode
	defer func() { agentMode = origAgentMode }()
	agentMode = false

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	// Install first
	resetSkillsFlags(t)
	_ = skillsInstallCmd.Flags().Set("for", "claude")
	if err := skillsInstallCmd.RunE(skillsInstallCmd, []string{}); err != nil {
		t.Fatalf("install error: %v", err)
	}

	agent, _ := skills.FindAgent("claude")
	skillDir := filepath.Join(tmpDir, agent.ProjectPath)
	skillFile := filepath.Join(skillDir, "SKILL.md")
	if _, err := os.Stat(skillFile); os.IsNotExist(err) {
		t.Fatal("SKILL.md should exist before uninstall")
	}

	// Uninstall
	resetSkillsFlags(t)
	_ = skillsUninstallCmd.Flags().Set("for", "claude")
	if err := skillsUninstallCmd.RunE(skillsUninstallCmd, []string{}); err != nil {
		t.Fatalf("uninstall error: %v", err)
	}

	// Verify the entire skill directory is gone
	if _, err := os.Stat(skillDir); !os.IsNotExist(err) {
		t.Error("skill directory should not exist after uninstall")
	}
}

func TestSkillsUninstall_NothingInstalled(t *testing.T) {
	clearAgentEnvVars(t)
	origAgentMode := agentMode
	defer func() { agentMode = origAgentMode }()
	agentMode = false

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	resetSkillsFlags(t)
	_ = skillsUninstallCmd.Flags().Set("for", "copilot")

	// Should succeed with no error even when nothing to remove
	err := skillsUninstallCmd.RunE(skillsUninstallCmd, []string{})
	if err != nil {
		t.Fatalf("uninstall error: %v", err)
	}
}

func TestSkillsStatus_RunE(t *testing.T) {
	clearAgentEnvVars(t)
	origAgentMode := agentMode
	defer func() { agentMode = origAgentMode }()
	agentMode = false

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	// Install one agent
	resetSkillsFlags(t)
	_ = skillsInstallCmd.Flags().Set("for", "cursor")
	if err := skillsInstallCmd.RunE(skillsInstallCmd, []string{}); err != nil {
		t.Fatalf("install error: %v", err)
	}

	// Check status for that agent
	resetSkillsFlags(t)
	_ = skillsStatusCmd.Flags().Set("for", "cursor")
	err := skillsStatusCmd.RunE(skillsStatusCmd, []string{})
	if err != nil {
		t.Fatalf("status error: %v", err)
	}
}

func TestSkillsStatus_AllAgents(t *testing.T) {
	clearAgentEnvVars(t)
	origAgentMode := agentMode
	defer func() { agentMode = origAgentMode }()
	agentMode = false

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	// No --for flag → shows all agents
	resetSkillsFlags(t)
	err := skillsStatusCmd.RunE(skillsStatusCmd, []string{})
	if err != nil {
		t.Fatalf("status all error: %v", err)
	}
}

func TestSkillsStatus_UnknownAgent(t *testing.T) {
	clearAgentEnvVars(t)
	origAgentMode := agentMode
	defer func() { agentMode = origAgentMode }()
	agentMode = false

	resetSkillsFlags(t)
	_ = skillsStatusCmd.Flags().Set("for", "vim-copilot")
	err := skillsStatusCmd.RunE(skillsStatusCmd, []string{})
	if err == nil {
		t.Fatal("expected error for unknown agent")
	}
	if !strings.Contains(err.Error(), "unknown agent") {
		t.Errorf("error should say 'unknown agent', got: %v", err)
	}
}

// --- Agent-mode output tests ---

func TestSkillsInstall_AgentMode(t *testing.T) {
	clearAgentEnvVars(t)
	origAgentMode := agentMode
	defer func() { agentMode = origAgentMode }()
	agentMode = true

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	resetSkillsFlags(t)
	_ = skillsInstallCmd.Flags().Set("for", "claude")

	err := skillsInstallCmd.RunE(skillsInstallCmd, []string{})
	if err != nil {
		t.Fatalf("RunE error: %v", err)
	}

	// Verify SKILL.md was also created on disk
	agent, _ := skills.FindAgent("claude")
	expectedPath := filepath.Join(tmpDir, agent.ProjectPath, "SKILL.md")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("SKILL.md not created in agent mode")
	}
}

func TestSkillsInstall_AgentModeUpdated(t *testing.T) {
	clearAgentEnvVars(t)
	origAgentMode := agentMode
	defer func() { agentMode = origAgentMode }()
	agentMode = true

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	// First install
	resetSkillsFlags(t)
	_ = skillsInstallCmd.Flags().Set("for", "claude")
	_ = skillsInstallCmd.Flags().Set("force", "true")
	if err := skillsInstallCmd.RunE(skillsInstallCmd, []string{}); err != nil {
		t.Fatalf("first install error: %v", err)
	}

	// Second install with force → should report "updated"
	resetSkillsFlags(t)
	_ = skillsInstallCmd.Flags().Set("for", "claude")
	_ = skillsInstallCmd.Flags().Set("force", "true")
	err := skillsInstallCmd.RunE(skillsInstallCmd, []string{})
	if err != nil {
		t.Fatalf("force install error: %v", err)
	}
}

func TestSkillsUninstall_AgentMode(t *testing.T) {
	clearAgentEnvVars(t)
	origAgentMode := agentMode
	defer func() { agentMode = origAgentMode }()

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	// Install first (non-agent mode)
	agentMode = false
	resetSkillsFlags(t)
	_ = skillsInstallCmd.Flags().Set("for", "copilot")
	if err := skillsInstallCmd.RunE(skillsInstallCmd, []string{}); err != nil {
		t.Fatalf("install error: %v", err)
	}

	// Uninstall in agent mode
	agentMode = true
	resetSkillsFlags(t)
	_ = skillsUninstallCmd.Flags().Set("for", "copilot")
	err := skillsUninstallCmd.RunE(skillsUninstallCmd, []string{})
	if err != nil {
		t.Fatalf("uninstall error: %v", err)
	}
}

func TestSkillsStatus_AgentModeSingleAgent(t *testing.T) {
	clearAgentEnvVars(t)
	origAgentMode := agentMode
	defer func() { agentMode = origAgentMode }()
	agentMode = true

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	resetSkillsFlags(t)
	_ = skillsStatusCmd.Flags().Set("for", "cursor")
	err := skillsStatusCmd.RunE(skillsStatusCmd, []string{})
	if err != nil {
		t.Fatalf("status error: %v", err)
	}
}

func TestSkillsStatus_AgentModeAllAgents(t *testing.T) {
	clearAgentEnvVars(t)
	origAgentMode := agentMode
	defer func() { agentMode = origAgentMode }()
	agentMode = true

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	resetSkillsFlags(t)
	err := skillsStatusCmd.RunE(skillsStatusCmd, []string{})
	if err != nil {
		t.Fatalf("status all error: %v", err)
	}
}

func TestSkillsList_AgentMode(t *testing.T) {
	clearAgentEnvVars(t)
	origAgentMode := agentMode
	defer func() { agentMode = origAgentMode }()
	agentMode = true

	resetSkillsFlags(t)
	_ = skillsInstallCmd.Flags().Set("list", "true")
	err := skillsInstallCmd.RunE(skillsInstallCmd, []string{})
	if err != nil {
		t.Fatalf("list error: %v", err)
	}
}

// --- Agent-mode enrichAgent tests ---

func TestSkillsInstall_EnrichAgent(t *testing.T) {
	var buf bytes.Buffer
	ctx := &output.ResponseContext{}
	printer := output.NewAgentPrinter(&buf, ctx)

	ap := enrichAgent(printer, "install", "skills")
	if ap == nil {
		t.Fatal("expected non-nil AgentPrinter")
	}
	if ap.Context().Verb != "install" {
		t.Errorf("Verb = %q, want %q", ap.Context().Verb, "install")
	}
}

func TestSkillsUninstall_EnrichAgent(t *testing.T) {
	var buf bytes.Buffer
	ctx := &output.ResponseContext{}
	printer := output.NewAgentPrinter(&buf, ctx)

	ap := enrichAgent(printer, "uninstall", "skills")
	if ap == nil {
		t.Fatal("expected non-nil AgentPrinter")
	}
	if ap.Context().Verb != "uninstall" {
		t.Errorf("Verb = %q, want %q", ap.Context().Verb, "uninstall")
	}
}

func TestSkillsStatus_EnrichAgent(t *testing.T) {
	var buf bytes.Buffer
	ctx := &output.ResponseContext{}
	printer := output.NewAgentPrinter(&buf, ctx)

	ap := enrichAgent(printer, "status", "skills")
	if ap == nil {
		t.Fatal("expected non-nil AgentPrinter")
	}
	if ap.Context().Verb != "status" {
		t.Errorf("Verb = %q, want %q", ap.Context().Verb, "status")
	}
}

func TestSkillsList_EnrichAgent(t *testing.T) {
	var buf bytes.Buffer
	ctx := &output.ResponseContext{}
	printer := output.NewAgentPrinter(&buf, ctx)

	ap := enrichAgent(printer, "list", "skills")
	if ap == nil {
		t.Fatal("expected non-nil AgentPrinter")
	}
	if ap.Context().Verb != "list" {
		t.Errorf("Verb = %q, want %q", ap.Context().Verb, "list")
	}
}

// --- Agent-mode result struct serialization tests ---

func TestSkillsInstallAgentResult_JSON(t *testing.T) {
	result := skillsInstallAgentResult{
		Action: "installed",
		Agent:  "claude",
		Path:   "/tmp/project/.claude/skills/dtctl",
		Scope:  "project",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}

	if m["action"] != "installed" {
		t.Errorf("action = %v, want %v", m["action"], "installed")
	}
	if m["agent"] != "claude" {
		t.Errorf("agent = %v, want %v", m["agent"], "claude")
	}
	if m["scope"] != "project" {
		t.Errorf("scope = %v, want %v", m["scope"], "project")
	}
}

func TestSkillsInstallAgentResult_UpdatedAction(t *testing.T) {
	result := skillsInstallAgentResult{
		Action: "updated",
		Agent:  "cursor",
		Path:   "/tmp/.cursor/skills/dtctl",
		Scope:  "project",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}

	var m map[string]interface{}
	_ = json.Unmarshal(data, &m)
	if m["action"] != "updated" {
		t.Errorf("action = %v, want %v", m["action"], "updated")
	}
}

func TestSkillsUninstallAgentResult_JSON(t *testing.T) {
	result := skillsUninstallAgentResult{
		Agent:   "copilot",
		Removed: []string{"/tmp/.github/skills/dtctl"},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}

	var m map[string]interface{}
	_ = json.Unmarshal(data, &m)
	if m["agent"] != "copilot" {
		t.Errorf("agent = %v, want %v", m["agent"], "copilot")
	}
	removed, ok := m["removed"].([]interface{})
	if !ok || len(removed) != 1 {
		t.Errorf("removed = %v, want array of length 1", m["removed"])
	}
}

func TestSkillsStatusAgentEntry_JSON(t *testing.T) {
	tests := []struct {
		name      string
		entry     skillsStatusAgentEntry
		wantPath  bool
		wantScope bool
	}{
		{
			name: "installed project",
			entry: skillsStatusAgentEntry{
				Agent:     "claude",
				Installed: true,
				Path:      "/tmp/.claude/skills/dtctl",
				Scope:     "project",
			},
			wantPath:  true,
			wantScope: true,
		},
		{
			name: "not installed omits path and scope",
			entry: skillsStatusAgentEntry{
				Agent:     "cursor",
				Installed: false,
			},
			wantPath:  false,
			wantScope: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.entry)
			if err != nil {
				t.Fatalf("json.Marshal error: %v", err)
			}

			var m map[string]interface{}
			_ = json.Unmarshal(data, &m)

			_, hasPath := m["path"]
			_, hasScope := m["scope"]

			if hasPath != tt.wantPath {
				t.Errorf("has path = %v, want %v (json: %s)", hasPath, tt.wantPath, data)
			}
			if hasScope != tt.wantScope {
				t.Errorf("has scope = %v, want %v (json: %s)", hasScope, tt.wantScope, data)
			}
		})
	}
}

func TestSkillsListAgentEntry_JSON(t *testing.T) {
	entry := skillsListAgentEntry{
		Name:           "claude",
		DisplayName:    "Claude Code",
		ProjectPath:    ".claude/skills/dtctl",
		SupportsGlobal: true,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}

	var m map[string]interface{}
	_ = json.Unmarshal(data, &m)

	if m["name"] != "claude" {
		t.Errorf("name = %v, want %v", m["name"], "claude")
	}
	if m["display_name"] != "Claude Code" {
		t.Errorf("display_name = %v, want %v", m["display_name"], "Claude Code")
	}
	if m["supports_global"] != true {
		t.Errorf("supports_global = %v, want %v", m["supports_global"], true)
	}
}

// --- Shell completion tests ---

func TestAgentCompletionFunc(t *testing.T) {
	completions, directive := agentCompletionFunc(nil, nil, "")

	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
	}

	if len(completions) != len(skills.AllAgents()) {
		t.Fatalf("expected %d completions, got %d", len(skills.AllAgents()), len(completions))
	}

	for _, c := range completions {
		// Each completion should be "name\tDisplayName"
		parts := strings.SplitN(c, "\t", 2)
		if len(parts) != 2 {
			t.Errorf("completion %q should have tab separator", c)
			continue
		}
		_, ok := skills.FindAgent(parts[0])
		if !ok {
			t.Errorf("completion %q has unknown agent name", c)
		}
	}
}

// --- runSkillsList (non-agent mode, smoke test) ---

func TestSkillsList_RunE(t *testing.T) {
	origAgentMode := agentMode
	defer func() { agentMode = origAgentMode }()
	agentMode = false

	resetSkillsFlags(t)
	_ = skillsInstallCmd.Flags().Set("list", "true")
	err := skillsInstallCmd.RunE(skillsInstallCmd, []string{})
	if err != nil {
		t.Fatalf("list error: %v", err)
	}
}

// --- Cross-client CLI tests ---

func TestSkillsInstall_CrossClient(t *testing.T) {
	clearAgentEnvVars(t)
	origAgentMode := agentMode
	defer func() { agentMode = origAgentMode }()
	agentMode = false

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	resetSkillsFlags(t)
	_ = skillsInstallCmd.Flags().Set("cross-client", "true")

	err := skillsInstallCmd.RunE(skillsInstallCmd, []string{})
	if err != nil {
		t.Fatalf("RunE error: %v", err)
	}

	// Verify SKILL.md was created in the cross-client directory
	expectedPath := filepath.Join(tmpDir, ".agents", "skills", "dtctl", "SKILL.md")
	data, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("SKILL.md not created: %v", err)
	}
	if !strings.Contains(string(data), "dtctl") {
		t.Error("SKILL.md should contain 'dtctl'")
	}

	// Verify references/ directory exists
	refsDir := filepath.Join(tmpDir, ".agents", "skills", "dtctl", "references")
	if info, err := os.Stat(refsDir); err != nil || !info.IsDir() {
		t.Error("references/ directory should exist")
	}
}

func TestSkillsInstall_CrossClient_DoesNotRequireAgentDetection(t *testing.T) {
	clearAgentEnvVars(t) // No agent env vars set
	origAgentMode := agentMode
	defer func() { agentMode = origAgentMode }()
	agentMode = false

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	resetSkillsFlags(t)
	_ = skillsInstallCmd.Flags().Set("cross-client", "true")

	// Should succeed even though no agent is detected
	err := skillsInstallCmd.RunE(skillsInstallCmd, []string{})
	if err != nil {
		t.Fatalf("cross-client install should not require agent detection: %v", err)
	}
}

func TestSkillsInstall_CrossClientAndForConflict(t *testing.T) {
	clearAgentEnvVars(t)
	origAgentMode := agentMode
	defer func() { agentMode = origAgentMode }()
	agentMode = false

	resetSkillsFlags(t)
	_ = skillsInstallCmd.Flags().Set("cross-client", "true")
	_ = skillsInstallCmd.Flags().Set("for", "claude")

	err := skillsInstallCmd.RunE(skillsInstallCmd, []string{})
	if err == nil {
		t.Fatal("expected error when both --cross-client and --for are used")
	}
	if !strings.Contains(err.Error(), "cannot be used together") {
		t.Errorf("error should mention 'cannot be used together', got: %v", err)
	}
}

func TestSkillsInstall_CrossClient_Overwrite(t *testing.T) {
	clearAgentEnvVars(t)
	origAgentMode := agentMode
	defer func() { agentMode = origAgentMode }()
	agentMode = false

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	// First install
	resetSkillsFlags(t)
	_ = skillsInstallCmd.Flags().Set("cross-client", "true")
	if err := skillsInstallCmd.RunE(skillsInstallCmd, []string{}); err != nil {
		t.Fatalf("first install error: %v", err)
	}

	// Second install without force should fail
	resetSkillsFlags(t)
	_ = skillsInstallCmd.Flags().Set("cross-client", "true")
	err := skillsInstallCmd.RunE(skillsInstallCmd, []string{})
	if err == nil {
		t.Fatal("expected error on overwrite without --force")
	}

	// Third install with force should succeed
	resetSkillsFlags(t)
	_ = skillsInstallCmd.Flags().Set("cross-client", "true")
	_ = skillsInstallCmd.Flags().Set("force", "true")
	err = skillsInstallCmd.RunE(skillsInstallCmd, []string{})
	if err != nil {
		t.Fatalf("force install error: %v", err)
	}
}

func TestSkillsInstall_CrossClient_AgentMode(t *testing.T) {
	clearAgentEnvVars(t)
	origAgentMode := agentMode
	defer func() { agentMode = origAgentMode }()
	agentMode = true

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	resetSkillsFlags(t)
	_ = skillsInstallCmd.Flags().Set("cross-client", "true")

	err := skillsInstallCmd.RunE(skillsInstallCmd, []string{})
	if err != nil {
		t.Fatalf("RunE error: %v", err)
	}

	// Verify files exist on disk
	expectedPath := filepath.Join(tmpDir, ".agents", "skills", "dtctl", "SKILL.md")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Error("SKILL.md not created in agent mode")
	}
}

func TestSkillsUninstall_CrossClient(t *testing.T) {
	clearAgentEnvVars(t)
	origAgentMode := agentMode
	defer func() { agentMode = origAgentMode }()
	agentMode = false

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	// Install cross-client first
	resetSkillsFlags(t)
	_ = skillsInstallCmd.Flags().Set("cross-client", "true")
	if err := skillsInstallCmd.RunE(skillsInstallCmd, []string{}); err != nil {
		t.Fatalf("install error: %v", err)
	}

	skillDir := filepath.Join(tmpDir, ".agents", "skills", "dtctl")
	if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); os.IsNotExist(err) {
		t.Fatal("SKILL.md should exist before uninstall")
	}

	// Uninstall
	resetSkillsFlags(t)
	_ = skillsUninstallCmd.Flags().Set("cross-client", "true")
	if err := skillsUninstallCmd.RunE(skillsUninstallCmd, []string{}); err != nil {
		t.Fatalf("uninstall error: %v", err)
	}

	// Verify the directory is gone
	if _, err := os.Stat(skillDir); !os.IsNotExist(err) {
		t.Error("skill directory should not exist after uninstall")
	}
}

func TestSkillsUninstall_CrossClientAndForConflict(t *testing.T) {
	clearAgentEnvVars(t)
	origAgentMode := agentMode
	defer func() { agentMode = origAgentMode }()
	agentMode = false

	resetSkillsFlags(t)
	_ = skillsUninstallCmd.Flags().Set("cross-client", "true")
	_ = skillsUninstallCmd.Flags().Set("for", "claude")

	err := skillsUninstallCmd.RunE(skillsUninstallCmd, []string{})
	if err == nil {
		t.Fatal("expected error when both --cross-client and --for are used")
	}
	if !strings.Contains(err.Error(), "cannot be used together") {
		t.Errorf("error should mention 'cannot be used together', got: %v", err)
	}
}

func TestSkillsStatus_CrossClient_ViaForFlag(t *testing.T) {
	clearAgentEnvVars(t)
	origAgentMode := agentMode
	defer func() { agentMode = origAgentMode }()
	agentMode = false

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	// Install cross-client
	resetSkillsFlags(t)
	_ = skillsInstallCmd.Flags().Set("cross-client", "true")
	if err := skillsInstallCmd.RunE(skillsInstallCmd, []string{}); err != nil {
		t.Fatalf("install error: %v", err)
	}

	// Check status via --for cross-client
	resetSkillsFlags(t)
	_ = skillsStatusCmd.Flags().Set("for", "cross-client")
	err := skillsStatusCmd.RunE(skillsStatusCmd, []string{})
	if err != nil {
		t.Fatalf("status error: %v", err)
	}
}

func TestSkillsStatus_ShowsCrossClient(t *testing.T) {
	clearAgentEnvVars(t)
	origAgentMode := agentMode
	defer func() { agentMode = origAgentMode }()
	agentMode = true

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	// Install cross-client
	resetSkillsFlags(t)
	_ = skillsInstallCmd.Flags().Set("cross-client", "true")
	if err := skillsInstallCmd.RunE(skillsInstallCmd, []string{}); err != nil {
		t.Fatalf("install error: %v", err)
	}

	// Status for all agents (should include cross-client)
	resetSkillsFlags(t)
	err := skillsStatusCmd.RunE(skillsStatusCmd, []string{})
	if err != nil {
		t.Fatalf("status error: %v", err)
	}
}

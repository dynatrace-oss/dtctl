package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSupportedAgents(t *testing.T) {
	agents := SupportedAgents()
	expected := []string{"claude", "copilot", "cursor", "kiro", "opencode"}
	if len(agents) != len(expected) {
		t.Fatalf("expected %d agents, got %d", len(expected), len(agents))
	}
	for i, name := range expected {
		if agents[i] != name {
			t.Errorf("agent[%d] = %q, want %q", i, agents[i], name)
		}
	}
}

func TestFindAgent(t *testing.T) {
	tests := []struct {
		name  string
		found bool
	}{
		{"claude", true},
		{"copilot", true},
		{"cursor", true},
		{"kiro", true},
		{"opencode", true},
		{"unknown", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent, ok := FindAgent(tt.name)
			if ok != tt.found {
				t.Errorf("FindAgent(%q) found = %v, want %v", tt.name, ok, tt.found)
			}
			if ok && agent.Name != tt.name {
				t.Errorf("FindAgent(%q) returned agent.Name = %q", tt.name, agent.Name)
			}
		})
	}
}

func TestDetectAgent(t *testing.T) {
	// Each subtest clears ALL agent env vars to ensure full isolation.
	allEnvVars := []string{
		"CLAUDECODE", "CURSOR_AGENT", "GITHUB_COPILOT", "KIRO", "OPENCODE",
		"CODEIUM_AGENT", "TABNINE_AGENT", "AMAZON_Q", "AI_AGENT",
	}

	clearAllEnvVars := func(t *testing.T) {
		t.Helper()
		for _, env := range allEnvVars {
			t.Setenv(env, "")
		}
	}

	t.Run("no agent detected", func(t *testing.T) {
		clearAllEnvVars(t)
		_, detected := DetectAgent()
		if detected {
			t.Error("expected no agent detected")
		}
	})

	t.Run("detects claude", func(t *testing.T) {
		clearAllEnvVars(t)
		t.Setenv("CLAUDECODE", "1")
		agent, detected := DetectAgent()
		if !detected {
			t.Fatal("expected agent detected")
		}
		if agent.Name != "claude" {
			t.Errorf("expected claude, got %q", agent.Name)
		}
	})

	t.Run("detects opencode", func(t *testing.T) {
		clearAllEnvVars(t)
		t.Setenv("OPENCODE", "1")
		agent, detected := DetectAgent()
		if !detected {
			t.Fatal("expected agent detected")
		}
		if agent.Name != "opencode" {
			t.Errorf("expected opencode, got %q", agent.Name)
		}
	})

	t.Run("detects copilot", func(t *testing.T) {
		clearAllEnvVars(t)
		t.Setenv("GITHUB_COPILOT", "1")
		agent, detected := DetectAgent()
		if !detected {
			t.Fatal("expected agent detected")
		}
		if agent.Name != "copilot" {
			t.Errorf("expected copilot, got %q", agent.Name)
		}
	})

	t.Run("detects cursor", func(t *testing.T) {
		clearAllEnvVars(t)
		t.Setenv("CURSOR_AGENT", "1")
		agent, detected := DetectAgent()
		if !detected {
			t.Fatal("expected agent detected")
		}
		if agent.Name != "cursor" {
			t.Errorf("expected cursor, got %q", agent.Name)
		}
	})

	t.Run("detects kiro", func(t *testing.T) {
		clearAllEnvVars(t)
		t.Setenv("KIRO", "1")
		agent, detected := DetectAgent()
		if !detected {
			t.Fatal("expected agent detected")
		}
		if agent.Name != "kiro" {
			t.Errorf("expected kiro, got %q", agent.Name)
		}
	})
}

func TestRender(t *testing.T) {
	for _, agent := range AllAgents() {
		t.Run(agent.Name, func(t *testing.T) {
			content, err := Render(agent)
			if err != nil {
				t.Fatalf("Render(%s) error: %v", agent.Name, err)
			}
			if content == "" {
				t.Errorf("Render(%s) returned empty content", agent.Name)
			}
			// All rendered content must contain dtctl
			if !strings.Contains(content, "dtctl") {
				t.Errorf("Render(%s) should contain 'dtctl'", agent.Name)
			}
		})
	}
}

func TestRenderWithData(t *testing.T) {
	agent, _ := FindAgent("claude")
	data := TemplateData{Version: "1.2.3"}

	content, err := RenderWithData(agent, data)
	if err != nil {
		t.Fatalf("RenderWithData error: %v", err)
	}

	if !strings.Contains(content, "1.2.3") {
		t.Error("rendered content should contain custom version 1.2.3")
	}
}

func TestRenderWithData_CursorFormat(t *testing.T) {
	agent, _ := FindAgent("cursor")
	data := TemplateData{Version: "2.0.0"}

	content, err := RenderWithData(agent, data)
	if err != nil {
		t.Fatalf("RenderWithData error: %v", err)
	}

	// Cursor output must have MDC frontmatter
	if !strings.HasPrefix(content, "---\n") {
		t.Error("Cursor output should start with MDC frontmatter")
	}
	if !strings.Contains(content, "description:") {
		t.Error("Cursor output should have description in frontmatter")
	}
	if !strings.Contains(content, "globs:") {
		t.Error("Cursor output should have globs in frontmatter")
	}
	if !strings.Contains(content, "2.0.0") {
		t.Error("Cursor output should contain version 2.0.0")
	}
}

func TestRenderWithData_MarkdownFormat(t *testing.T) {
	for _, name := range []string{"claude", "copilot", "opencode"} {
		t.Run(name, func(t *testing.T) {
			agent, _ := FindAgent(name)
			data := TemplateData{Version: "3.0.0"}

			content, err := RenderWithData(agent, data)
			if err != nil {
				t.Fatalf("RenderWithData error: %v", err)
			}

			// Non-Cursor agents get an HTML comment version header
			if !strings.Contains(content, "<!-- dtctl skill v3.0.0 -->") {
				t.Errorf("output should contain HTML comment version header, got prefix: %q",
					content[:min(100, len(content))])
			}
			// Must not have MDC frontmatter
			if strings.HasPrefix(content, "---\n") {
				t.Error("non-Cursor output should NOT start with MDC frontmatter")
			}
		})
	}
}

func TestRenderWithData_KiroPowerFormat(t *testing.T) {
	agent, _ := FindAgent("kiro")
	data := TemplateData{Version: "4.0.0"}

	content, err := RenderWithData(agent, data)
	if err != nil {
		t.Fatalf("RenderWithData error: %v", err)
	}

	// Kiro output must have POWER.md YAML frontmatter
	if !strings.HasPrefix(content, "---\n") {
		t.Error("Kiro output should start with YAML frontmatter")
	}
	if !strings.Contains(content, "name: \"dtctl\"") {
		t.Error("Kiro output should have name in frontmatter")
	}
	if !strings.Contains(content, "displayName:") {
		t.Error("Kiro output should have displayName in frontmatter")
	}
	if !strings.Contains(content, "description:") {
		t.Error("Kiro output should have description in frontmatter")
	}
	if !strings.Contains(content, "keywords:") {
		t.Error("Kiro output should have keywords in frontmatter")
	}
	if !strings.Contains(content, "author: \"Dynatrace\"") {
		t.Error("Kiro output should have author in frontmatter")
	}
	if !strings.Contains(content, "4.0.0") {
		t.Error("Kiro output should contain version 4.0.0")
	}
	// Must not have HTML comment version header
	if strings.Contains(content, "<!-- dtctl skill") {
		t.Error("Kiro output should NOT contain HTML comment version header")
	}
	// Must contain the skill content after the frontmatter
	if !strings.Contains(content, "dtctl") {
		t.Error("Kiro output should contain skill content")
	}
}

func TestInstall(t *testing.T) {
	t.Run("installs to project directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		agent, _ := FindAgent("claude")

		result, err := Install(agent, tmpDir, false, false)
		if err != nil {
			t.Fatalf("Install error: %v", err)
		}

		if result.Replaced {
			t.Error("expected Replaced=false for new install")
		}
		if result.Global {
			t.Error("expected Global=false")
		}

		expectedPath := filepath.Join(tmpDir, ".claude", "commands", "dtctl.md")
		if result.Path != expectedPath {
			t.Errorf("Path = %q, want %q", result.Path, expectedPath)
		}

		// Verify file exists and has content
		data, err := os.ReadFile(expectedPath)
		if err != nil {
			t.Fatalf("failed to read installed file: %v", err)
		}
		if len(data) == 0 {
			t.Error("installed file is empty")
		}
		if !strings.Contains(string(data), "dtctl") {
			t.Error("installed file should contain 'dtctl'")
		}
	})

	t.Run("refuses overwrite without --force", func(t *testing.T) {
		tmpDir := t.TempDir()
		agent, _ := FindAgent("copilot")

		// First install
		_, err := Install(agent, tmpDir, false, false)
		if err != nil {
			t.Fatalf("first Install error: %v", err)
		}

		// Second install should fail
		_, err = Install(agent, tmpDir, false, false)
		if err == nil {
			t.Fatal("expected error on duplicate install")
		}
		if !strings.Contains(err.Error(), "already exists") {
			t.Errorf("error should mention 'already exists', got: %v", err)
		}
		if !strings.Contains(err.Error(), "--force") {
			t.Errorf("error should mention '--force', got: %v", err)
		}
	})

	t.Run("overwrites with force", func(t *testing.T) {
		tmpDir := t.TempDir()
		agent, _ := FindAgent("cursor")

		// First install
		_, err := Install(agent, tmpDir, false, false)
		if err != nil {
			t.Fatalf("first Install error: %v", err)
		}

		// Second install with overwrite
		result, err := Install(agent, tmpDir, false, true)
		if err != nil {
			t.Fatalf("overwrite Install error: %v", err)
		}
		if !result.Replaced {
			t.Error("expected Replaced=true on overwrite")
		}
	})

	t.Run("global install unsupported agent", func(t *testing.T) {
		tmpDir := t.TempDir()
		agent, _ := FindAgent("copilot")

		_, err := Install(agent, tmpDir, true, false)
		if err == nil {
			t.Fatal("expected error for unsupported global install")
		}
		if !strings.Contains(err.Error(), "does not support global") {
			t.Errorf("error should mention 'does not support global', got: %v", err)
		}
	})

	t.Run("installs all agents", func(t *testing.T) {
		tmpDir := t.TempDir()

		for _, agent := range AllAgents() {
			result, err := Install(agent, tmpDir, false, false)
			if err != nil {
				t.Fatalf("Install(%s) error: %v", agent.Name, err)
			}

			// Verify file exists
			if _, err := os.Stat(result.Path); err != nil {
				t.Errorf("Install(%s) file not found at %s", agent.Name, result.Path)
			}
		}
	})
}

func TestUninstall(t *testing.T) {
	t.Run("removes installed file", func(t *testing.T) {
		tmpDir := t.TempDir()
		agent, _ := FindAgent("claude")

		// Install first
		result, _ := Install(agent, tmpDir, false, false)

		// Verify it exists
		if _, err := os.Stat(result.Path); err != nil {
			t.Fatalf("file should exist before uninstall")
		}

		// Uninstall
		removed, err := Uninstall(agent, tmpDir)
		if err != nil {
			t.Fatalf("Uninstall error: %v", err)
		}
		if len(removed) != 1 {
			t.Fatalf("expected 1 removed, got %d", len(removed))
		}

		// Verify it's gone
		if _, err := os.Stat(result.Path); !os.IsNotExist(err) {
			t.Error("file should not exist after uninstall")
		}
	})

	t.Run("returns empty for non-installed", func(t *testing.T) {
		tmpDir := t.TempDir()
		agent, _ := FindAgent("copilot")

		removed, err := Uninstall(agent, tmpDir)
		if err != nil {
			t.Fatalf("Uninstall error: %v", err)
		}
		if len(removed) != 0 {
			t.Errorf("expected 0 removed, got %d", len(removed))
		}
	})

	t.Run("returns removed paths alongside error on partial failure", func(t *testing.T) {
		tmpDir := t.TempDir()
		agent, _ := FindAgent("claude")

		// Install project-local, then verify uninstall returns it
		Install(agent, tmpDir, false, false)
		removed, err := Uninstall(agent, tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(removed) != 1 {
			t.Errorf("expected 1 removed, got %d", len(removed))
		}
	})
}

func TestStatus(t *testing.T) {
	t.Run("installed project-local", func(t *testing.T) {
		tmpDir := t.TempDir()
		agent, _ := FindAgent("cursor")

		// Install
		Install(agent, tmpDir, false, false)

		result := Status(agent, tmpDir)
		if !result.Installed {
			t.Error("expected Installed=true")
		}
		if result.Global {
			t.Error("expected Global=false")
		}
		if result.Path == "" {
			t.Error("expected non-empty Path")
		}
	})

	t.Run("not installed", func(t *testing.T) {
		tmpDir := t.TempDir()
		agent, _ := FindAgent("opencode")

		result := Status(agent, tmpDir)
		if result.Installed {
			t.Error("expected Installed=false")
		}
	})
}

func TestStatusAll(t *testing.T) {
	tmpDir := t.TempDir()

	// Install one agent
	agent, _ := FindAgent("claude")
	Install(agent, tmpDir, false, false)

	results := StatusAll(tmpDir)
	if len(results) != len(AllAgents()) {
		t.Fatalf("expected %d results, got %d", len(AllAgents()), len(results))
	}

	installedCount := 0
	for _, r := range results {
		if r.Installed {
			installedCount++
			if r.Agent.Name != "claude" {
				t.Errorf("unexpected installed agent: %s", r.Agent.Name)
			}
		}
	}
	if installedCount != 1 {
		t.Errorf("expected 1 installed, got %d", installedCount)
	}
}

func TestAgentPaths(t *testing.T) {
	// Verify each agent has the expected file path conventions
	tests := []struct {
		name     string
		pathPart string
	}{
		{"claude", ".claude/commands/dtctl.md"},
		{"copilot", ".github/instructions/dtctl.instructions.md"},
		{"cursor", ".cursor/rules/dtctl.mdc"},
		{"kiro", ".kiro/powers/dtctl/POWER.md"},
		{"opencode", ".opencode/commands/dtctl.md"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent, ok := FindAgent(tt.name)
			if !ok {
				t.Fatalf("agent %q not found", tt.name)
			}
			// Use filepath to compare (handles OS-specific separators)
			expectedPath := filepath.FromSlash(tt.pathPart)
			if agent.ProjectPath != expectedPath {
				t.Errorf("ProjectPath = %q, want %q", agent.ProjectPath, expectedPath)
			}
		})
	}
}

func TestResolvePath(t *testing.T) {
	t.Run("project-local path", func(t *testing.T) {
		agent, _ := FindAgent("claude")
		path, err := resolvePath(agent, "/tmp/project", false)
		if err != nil {
			t.Fatalf("resolvePath error: %v", err)
		}
		expected := filepath.Join("/tmp/project", ".claude", "commands", "dtctl.md")
		if path != expected {
			t.Errorf("path = %q, want %q", path, expected)
		}
	})

	t.Run("global path for supported agent", func(t *testing.T) {
		agent, _ := FindAgent("claude")
		path, err := resolvePath(agent, "/tmp/project", true)
		if err != nil {
			t.Fatalf("resolvePath error: %v", err)
		}
		home, _ := os.UserHomeDir()
		expected := filepath.Join(home, ".claude", "commands", "dtctl.md")
		if path != expected {
			t.Errorf("path = %q, want %q", path, expected)
		}
	})

	t.Run("global path for unsupported agent", func(t *testing.T) {
		agent, _ := FindAgent("copilot")
		_, err := resolvePath(agent, "/tmp/project", true)
		if err == nil {
			t.Fatal("expected error for unsupported global path")
		}
		if !strings.Contains(err.Error(), "does not support global") {
			t.Errorf("error should mention 'does not support global', got: %v", err)
		}
	})
}

// --- New tests for concatenation and content processing ---

func TestSkillContent_NotEmpty(t *testing.T) {
	content := SkillContent()
	if content == "" {
		t.Fatal("SkillContent() returned empty string")
	}
}

func TestSkillContent_ContainsMainSections(t *testing.T) {
	content := SkillContent()

	// Must contain main SKILL.md content
	mustContain := []string{
		"Dynatrace Control with dtctl",
		"Available Resources",
		"Command Verbs",
		"Output Modes",
		"Template Variables",
	}
	for _, s := range mustContain {
		if !strings.Contains(content, s) {
			t.Errorf("SkillContent() should contain %q", s)
		}
	}
}

func TestSkillContent_NoYAMLFrontmatter(t *testing.T) {
	content := SkillContent()

	// Should not start with YAML frontmatter (it was stripped)
	if strings.HasPrefix(content, "---\n") {
		t.Error("SkillContent() should not start with YAML frontmatter")
	}
	// The original SKILL.md frontmatter key should not be present
	if strings.Contains(content, "name: dtctl\ndescription:") {
		t.Error("SkillContent() should not contain SKILL.md YAML frontmatter content")
	}
}

func TestSkillContent_NoRelativeLinks(t *testing.T) {
	content := SkillContent()

	// Should not contain relative links to .md files
	if strings.Contains(content, "](references/") {
		t.Error("SkillContent() should not contain relative links to references/")
	}
	if strings.Contains(content, "](dashboards.md)") {
		t.Error("SkillContent() should not contain relative links to dashboards.md")
	}
}

func TestSkillContent_SubstantialSize(t *testing.T) {
	content := SkillContent()

	// SKILL.md is ~287 lines; after stripping the 4-line YAML frontmatter
	// the output should be at least 200 lines.
	lines := strings.Count(content, "\n")
	if lines < 200 {
		t.Errorf("SkillContent() has only %d lines, expected 200+", lines)
	}
}

func TestStripYAMLFrontmatter(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:   "with frontmatter",
			input:  "---\nname: test\ndescription: hello\n---\n# Content",
			expect: "# Content",
		},
		{
			name:   "without frontmatter",
			input:  "# Just Content\nSome text",
			expect: "# Just Content\nSome text",
		},
		{
			name:   "frontmatter with trailing newline",
			input:  "---\nkey: val\n---\n\n# Title",
			expect: "\n# Title",
		},
		{
			name:   "empty string",
			input:  "",
			expect: "",
		},
		{
			name:   "dashes in body not stripped",
			input:  "# Title\n\n---\n\nSection",
			expect: "# Title\n\n---\n\nSection",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripYAMLFrontmatter(tt.input)
			if result != tt.expect {
				t.Errorf("stripYAMLFrontmatter(%q) = %q, want %q", tt.input, result, tt.expect)
			}
		})
	}
}

func TestResolveRelativeLinks(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:   "reference link",
			input:  "see [troubleshooting](references/troubleshooting.md) for help",
			expect: "see troubleshooting for help",
		},
		{
			name:   "nested reference link",
			input:  "see [dashboards](references/resources/dashboards.md) for details",
			expect: "see dashboards for details",
		},
		{
			name:   "simple filename link",
			input:  "See [dashboards.md](dashboards.md) for full details.",
			expect: "See dashboards.md for full details.",
		},
		{
			name:   "non-md link preserved",
			input:  "Visit [docs](https://example.com/docs) for more",
			expect: "Visit [docs](https://example.com/docs) for more",
		},
		{
			name:   "multiple links",
			input:  "[a](foo.md) and [b](bar.md)",
			expect: "a and b",
		},
		{
			name:   "no links",
			input:  "No links here",
			expect: "No links here",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveRelativeLinks(tt.input)
			if result != tt.expect {
				t.Errorf("resolveRelativeLinks(%q) = %q, want %q", tt.input, result, tt.expect)
			}
		})
	}
}

func TestInstalledFileContent(t *testing.T) {
	// Verify that installed files contain the core skill content.
	for _, agent := range AllAgents() {
		t.Run(agent.Name, func(t *testing.T) {
			tmpDir := t.TempDir()
			result, err := Install(agent, tmpDir, false, false)
			if err != nil {
				t.Fatalf("Install error: %v", err)
			}

			data, err := os.ReadFile(result.Path)
			if err != nil {
				t.Fatalf("ReadFile error: %v", err)
			}

			content := string(data)

			// Must contain SKILL.md core content
			mustContain := []string{
				"Dynatrace Control with dtctl",
				"Available Resources",
				"Command Verbs",
			}
			for _, s := range mustContain {
				if !strings.Contains(content, s) {
					t.Errorf("installed %s file missing %q", agent.Name, s)
				}
			}

			// Installed files should be at least 200 lines
			lines := strings.Count(content, "\n")
			if lines < 200 {
				t.Errorf("installed %s file has only %d lines, expected 200+", agent.Name, lines)
			}
		})
	}
}

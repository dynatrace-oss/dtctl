// Package skills provides installation and management of AI coding assistant
// skill files for dtctl. It embeds SKILL.md from skills/dtctl/ and writes it
// to the appropriate agent-specific location with agent-specific wrappers.
package skills

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/aidetect"
	"github.com/dynatrace-oss/dtctl/pkg/version"
	dtctlskill "github.com/dynatrace-oss/dtctl/skills/dtctl"
)

// skillContent holds the skill document, built once at init time.
var skillContent string

func init() {
	content, err := buildSkillContent()
	if err != nil {
		panic(fmt.Sprintf("skills: failed to build skill content: %v", err))
	}
	skillContent = content
}

// buildSkillContent reads the embedded SKILL.md, strips YAML frontmatter,
// and resolves relative markdown links.
func buildSkillContent() (string, error) {
	data, err := fs.ReadFile(dtctlskill.Content, "SKILL.md")
	if err != nil {
		return "", fmt.Errorf("reading SKILL.md: %w", err)
	}
	content := stripYAMLFrontmatter(string(data))
	content = resolveRelativeLinks(content)
	return content, nil
}

// yamlFrontmatterRE matches YAML frontmatter at the start of a document:
// a line with exactly "---", followed by content, followed by a line with
// exactly "---", with an optional trailing newline.
var yamlFrontmatterRE = regexp.MustCompile(`(?s)\A---\n.*?\n---\n?`)

// stripYAMLFrontmatter removes YAML frontmatter (delimited by ---) from the
// beginning of a markdown document.
func stripYAMLFrontmatter(content string) string {
	return yamlFrontmatterRE.ReplaceAllString(content, "")
}

// relativeLinkRE matches markdown links with relative paths to .md files.
// Captures: [link text](relative/path.md)
var relativeLinkRE = regexp.MustCompile(`\[([^\]]+)\]\([^)]*\.md\)`)

// resolveRelativeLinks replaces markdown links to relative .md files with
// just the link text, since the referenced content is inlined.
func resolveRelativeLinks(content string) string {
	return relativeLinkRE.ReplaceAllString(content, "$1")
}

// Agent represents a supported AI coding assistant.
type Agent struct {
	// Name is the canonical identifier (e.g. "claude", "copilot").
	Name string
	// DisplayName is the human-readable name (e.g. "Claude Code").
	DisplayName string
	// ProjectPath is the relative path from the project root for project-local install.
	ProjectPath string
	// GlobalPath is the relative path from the user's home directory for global install.
	// Empty means global install is not supported.
	GlobalPath string
	// EnvVar is the environment variable used to detect this agent.
	EnvVar string
	// DetectName is the name returned by aidetect.Detect() for this agent.
	DetectName string
}

// agents is the registry of all supported AI coding assistants.
var agents = []Agent{
	{
		Name:        "claude",
		DisplayName: "Claude Code",
		ProjectPath: filepath.Join(".claude", "commands", "dtctl.md"),
		GlobalPath:  filepath.Join(".claude", "commands", "dtctl.md"),
		EnvVar:      "CLAUDECODE",
		DetectName:  "claude-code",
	},
	{
		Name:        "copilot",
		DisplayName: "GitHub Copilot",
		ProjectPath: filepath.Join(".github", "instructions", "dtctl.instructions.md"),
		GlobalPath:  "",
		EnvVar:      "GITHUB_COPILOT",
		DetectName:  "github-copilot",
	},
	{
		Name:        "cursor",
		DisplayName: "Cursor",
		ProjectPath: filepath.Join(".cursor", "rules", "dtctl.mdc"),
		GlobalPath:  "",
		EnvVar:      "CURSOR_AGENT",
		DetectName:  "cursor",
	},
	{
		Name:        "kiro",
		DisplayName: "Kiro",
		ProjectPath: filepath.Join(".kiro", "powers", "dtctl", "POWER.md"),
		GlobalPath:  "",
		EnvVar:      "KIRO",
		DetectName:  "kiro",
	},
	{
		Name:        "opencode",
		DisplayName: "OpenCode",
		ProjectPath: filepath.Join(".opencode", "commands", "dtctl.md"),
		GlobalPath:  "",
		EnvVar:      "OPENCODE",
		DetectName:  "opencode",
	},
}

// TemplateData contains variables available for content rendering.
type TemplateData struct {
	Version string
}

// InstallResult describes the outcome of an install operation.
type InstallResult struct {
	Agent    Agent
	Path     string
	Global   bool
	Replaced bool
}

// StatusResult describes the current installation state for an agent.
type StatusResult struct {
	Agent     Agent
	Installed bool
	Path      string
	Global    bool
}

// SupportedAgents returns the list of all supported agent names.
func SupportedAgents() []string {
	names := make([]string, len(agents))
	for i, a := range agents {
		names[i] = a.Name
	}
	return names
}

// AllAgents returns the full agent registry.
func AllAgents() []Agent {
	return agents
}

// FindAgent looks up an agent by canonical name.
func FindAgent(name string) (Agent, bool) {
	for _, a := range agents {
		if a.Name == name {
			return a, true
		}
	}
	return Agent{}, false
}

// DetectAgent uses aidetect to find the current agent and maps it to our registry.
func DetectAgent() (Agent, bool) {
	info := aidetect.Detect()
	if !info.Detected {
		return Agent{}, false
	}
	for _, a := range agents {
		if a.DetectName == info.Name {
			return a, true
		}
	}
	return Agent{}, false
}

// Render renders the full skill content for an agent, using the current
// dtctl version.
func Render(agent Agent) (string, error) {
	data := TemplateData{
		Version: version.Version,
	}
	return RenderWithData(agent, data)
}

// RenderWithData renders the full skill content for an agent with custom data.
// The content is the concatenated SKILL.md + all reference files, wrapped
// with agent-specific formatting (e.g. Cursor MDC frontmatter).
func RenderWithData(agent Agent, data TemplateData) (string, error) {
	return wrapForAgent(agent, skillContent, data)
}

// wrapForAgent applies agent-specific formatting to the skill content.
func wrapForAgent(agent Agent, content string, data TemplateData) (string, error) {
	var sb strings.Builder

	switch agent.Name {
	case "cursor":
		// Cursor requires MDC format with YAML frontmatter.
		sb.WriteString("---\n")
		sb.WriteString(fmt.Sprintf("description: dtctl CLI skill (v%s) — kubectl-style CLI for Dynatrace\n", data.Version))
		sb.WriteString("globs: [\"*.yaml\", \"*.yml\", \"*.json\", \"*.dql\"]\n")
		sb.WriteString("---\n\n")
		sb.WriteString(content)
	case "kiro":
		// Kiro uses the Powers format: POWER.md with YAML frontmatter
		// containing name, displayName, description, keywords, and author.
		sb.WriteString("---\n")
		sb.WriteString("name: \"dtctl\"\n")
		sb.WriteString(fmt.Sprintf("displayName: \"dtctl — Dynatrace CLI (v%s)\"\n", data.Version))
		sb.WriteString("description: \"kubectl-style CLI for Dynatrace — manage dashboards, workflows, SLOs, documents, and more using DQL queries and YAML definitions\"\n")
		sb.WriteString("keywords: [\"dynatrace\", \"dtctl\", \"observability\", \"monitoring\", \"dql\", \"dashboards\", \"workflows\", \"slo\", \"grail\", \"davis\"]\n")
		sb.WriteString("author: \"Dynatrace\"\n")
		sb.WriteString("---\n\n")
		sb.WriteString(content)
	default:
		// Claude, Copilot, OpenCode — plain markdown with a version header.
		sb.WriteString(fmt.Sprintf("<!-- dtctl skill v%s -->\n\n", data.Version))
		sb.WriteString(content)
	}

	return sb.String(), nil
}

// Install writes the skill file for the given agent to the appropriate location.
// If global is true, it writes to the user's home directory; otherwise to the
// project root (baseDir). It returns an error if the file already exists and
// overwrite is false.
func Install(agent Agent, baseDir string, global bool, overwrite bool) (*InstallResult, error) {
	path, err := resolvePath(agent, baseDir, global)
	if err != nil {
		return nil, err
	}

	replaced := false
	if _, err := os.Stat(path); err == nil {
		if !overwrite {
			return nil, fmt.Errorf("skill file already exists at %s (use --force to overwrite)", path)
		}
		replaced = true
	}

	content, err := Render(agent)
	if err != nil {
		return nil, err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return nil, fmt.Errorf("failed to write skill file: %w", err)
	}

	return &InstallResult{
		Agent:    agent,
		Path:     path,
		Global:   global,
		Replaced: replaced,
	}, nil
}

// Uninstall removes installed skill files. It checks both project-local and
// global locations and removes any that exist. Returns the paths that were
// successfully removed. If some removals fail, the successfully removed paths
// are still returned alongside the error.
func Uninstall(agent Agent, baseDir string) ([]string, error) {
	var removed []string
	var errs []string

	// Check project-local
	projectPath := filepath.Join(baseDir, agent.ProjectPath)
	if _, err := os.Stat(projectPath); err == nil {
		if err := os.Remove(projectPath); err != nil {
			errs = append(errs, fmt.Sprintf("failed to remove %s: %v", projectPath, err))
		} else {
			removed = append(removed, projectPath)
		}
	}

	// Check global
	if agent.GlobalPath != "" {
		home, err := os.UserHomeDir()
		if err != nil {
			errs = append(errs, fmt.Sprintf("failed to determine home directory: %v", err))
		} else {
			globalPath := filepath.Join(home, agent.GlobalPath)
			if _, err := os.Stat(globalPath); err == nil {
				if err := os.Remove(globalPath); err != nil {
					errs = append(errs, fmt.Sprintf("failed to remove %s: %v", globalPath, err))
				} else {
					removed = append(removed, globalPath)
				}
			}
		}
	}

	if len(errs) > 0 {
		return removed, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return removed, nil
}

// Status checks the installation state for a given agent.
func Status(agent Agent, baseDir string) *StatusResult {
	// Check project-local first
	projectPath := filepath.Join(baseDir, agent.ProjectPath)
	if _, err := os.Stat(projectPath); err == nil {
		return &StatusResult{
			Agent:     agent,
			Installed: true,
			Path:      projectPath,
			Global:    false,
		}
	}

	// Check global (best-effort: if home dir lookup fails, treat as not installed)
	if agent.GlobalPath != "" {
		if home, err := os.UserHomeDir(); err == nil {
			globalPath := filepath.Join(home, agent.GlobalPath)
			if _, err := os.Stat(globalPath); err == nil {
				return &StatusResult{
					Agent:     agent,
					Installed: true,
					Path:      globalPath,
					Global:    true,
				}
			}
		}
	}

	return &StatusResult{
		Agent:     agent,
		Installed: false,
	}
}

// StatusAll checks installation state for all supported agents.
func StatusAll(baseDir string) []*StatusResult {
	results := make([]*StatusResult, len(agents))
	for i, a := range agents {
		results[i] = Status(a, baseDir)
	}
	return results
}

// SkillContent returns the raw concatenated skill content (without
// agent-specific wrapping). Exported for testing.
func SkillContent() string {
	return skillContent
}

// resolvePath determines the absolute path for the skill file.
func resolvePath(agent Agent, baseDir string, global bool) (string, error) {
	if global {
		if agent.GlobalPath == "" {
			return "", fmt.Errorf("%s does not support global installation", agent.DisplayName)
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to determine home directory: %w", err)
		}
		return filepath.Join(home, agent.GlobalPath), nil
	}
	return filepath.Join(baseDir, agent.ProjectPath), nil
}

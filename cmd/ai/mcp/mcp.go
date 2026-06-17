package mcp

import (
	_ "embed"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"

	mcp_golang "github.com/metoro-io/mcp-golang"
	mcp_http_transport "github.com/metoro-io/mcp-golang/transport/http"
	"github.com/spf13/cobra"

	dtctlskill "github.com/dynatrace-oss/dtctl/skills/dtctl"
)

var (
	port       int
	endpoint   string
	listenAddr string
)

//go:embed instructions.md
var embeddedInstructions string

func fullInstructions() string {
	var parts []string
	parts = append(parts, strings.TrimSpace(embeddedInstructions))

	_ = fs.WalkDir(dtctlskill.Content, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, readErr := dtctlskill.Content.ReadFile(path)
		if readErr != nil {
			return nil
		}
		parts = append(parts, strings.TrimSpace(string(data)))
		return nil
	})

	return strings.Join(parts, "\n\n")
}

type runQwctlCommandArgs struct {
	Command string   `json:"command" jsonschema:"required,description=The dtctl command to run without the leading dtctl binary name"`
	Args    []string `json:"args" jsonschema:"description=Additional command arguments and flags"`
}

type getQwctlCommandHelpArgs struct {
	Command string `json:"command" jsonschema:"required,description=Top-level dtctl command to get help for (e.g. configure, bootstrap, ai)"`
}

type dynamicCommandArgs struct {
	Args []string `json:"args" jsonschema:"description=Additional args, subcommands and flags for this command path. Natural language is normalized in the binary. For usage examples and exact flag mappings, call get_mcp_usage_guide."`
}

type emptyToolArgs struct{}

type commandSpec struct {
	Path        []string
	Description string
}

// McpCmd represents the MCP command group under ai.
var McpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the dtctl MCP server",
	Long:  "Start the dtctl MCP server",
	RunE: func(cmd *cobra.Command, args []string) error {
		addr := fmt.Sprintf("%s:%d", listenAddr, port)
		transport := mcp_http_transport.NewHTTPTransport(endpoint).WithAddr(addr)

		executable, err := os.Executable()
		if err != nil {
			return fmt.Errorf("failed to resolve dtctl executable: %w", err)
		}

		dynamicCommands, err := discoverDTCTLCommands(executable)
		if err != nil {
			return fmt.Errorf("failed to discover dtctl commands: %w", err)
		}

		dynamicToolLines := make([]string, 0, len(dynamicCommands))
		for _, spec := range dynamicCommands {
			if len(spec.Path) == 0 {
				continue
			}
			if len(spec.Path) >= 2 && spec.Path[0] == "ai" && spec.Path[1] == "mcp" {
				continue
			}
			toolName := "dtctl_" + sanitizeToolName(strings.Join(spec.Path, "_"))
			desc := strings.TrimSpace(spec.Description)
			if strings.TrimSpace(desc) == "" {
				desc = "(no description)"
			}

			dynamicToolLines = append(dynamicToolLines, fmt.Sprintf("- %s => dtctl %s | %s", toolName, strings.Join(spec.Path, " "), desc))
		}
		sort.Strings(dynamicToolLines)

		instructions := fullInstructions()
		server := mcp_golang.NewServer(
			transport,
			mcp_golang.WithName("dtctl-mcp-server"),
			mcp_golang.WithVersion("0.1.0"),
			mcp_golang.WithInstructions(instructions),
		)

		err = server.RegisterTool(
			"get_mcp_usage_guide",
			"Return the embedded dtctl MCP usage guide in Markdown.",
			func(arguments emptyToolArgs) (*mcp_golang.ToolResponse, error) {
				return mcp_golang.NewToolResponse(mcp_golang.NewTextContent(fullInstructions())), nil
			},
		)
		if err != nil {
			return err
		}

		err = server.RegisterTool(
			"list_dtctl_commands",
			"List top-level dtctl commands by returning `dtctl --help` output. Use this first before calling run_dtctl_command.",
			func(arguments emptyToolArgs) (*mcp_golang.ToolResponse, error) {
				runCmd := exec.Command(executable, "--help")
				output, err := runCmd.CombinedOutput()

				exitCode := 0
				if runCmd.ProcessState != nil {
					exitCode = runCmd.ProcessState.ExitCode()
				}

				result := fmt.Sprintf("command: dtctl --help\nexit_code: %d\noutput:\n%s", exitCode, string(output))
				if err != nil {
					return nil, fmt.Errorf("%s", result)
				}

				return mcp_golang.NewToolResponse(mcp_golang.NewTextContent(result)), nil
			},
		)
		if err != nil {
			return err
		}

		err = server.RegisterTool(
			"get_dtctl_command_help",
			"Get help for a specific top-level dtctl command by returning `dtctl <command> --help` output.",
			func(arguments getQwctlCommandHelpArgs) (*mcp_golang.ToolResponse, error) {
				commandName := strings.TrimSpace(arguments.Command)
				if strings.TrimSpace(commandName) == "" {
					return nil, fmt.Errorf("command is required")
				}

				runCmd := exec.Command(executable, commandName, "--help")
				output, err := runCmd.CombinedOutput()

				exitCode := 0
				if runCmd.ProcessState != nil {
					exitCode = runCmd.ProcessState.ExitCode()
				}

				result := fmt.Sprintf("command: dtctl %s --help\nexit_code: %d\noutput:\n%s", commandName, exitCode, string(output))
				if err != nil {
					return nil, fmt.Errorf("%s", result)
				}

				return mcp_golang.NewToolResponse(mcp_golang.NewTextContent(result)), nil
			},
		)
		if err != nil {
			return err
		}

		err = server.RegisterTool(
			"list_mcp_dynamic_tools",
			"List all dynamically generated MCP tools and their mapped dtctl command paths.",
			func(arguments emptyToolArgs) (*mcp_golang.ToolResponse, error) {
				if len(dynamicToolLines) == 0 {
					return mcp_golang.NewToolResponse(mcp_golang.NewTextContent("No dynamic tools discovered.")), nil
				}
				result := fmt.Sprintf("discovered_dynamic_tools: %d\n%s", len(dynamicToolLines), strings.Join(dynamicToolLines, "\n"))
				return mcp_golang.NewToolResponse(mcp_golang.NewTextContent(result)), nil
			},
		)
		if err != nil {
			return err
		}

		for _, spec := range dynamicCommands {
			if len(spec.Path) == 0 {
				continue
			}

			if len(spec.Path) >= 2 && spec.Path[0] == "ai" && spec.Path[1] == "mcp" {
				continue
			}

			toolName := "dtctl_" + sanitizeToolName(strings.Join(spec.Path, "_"))
			desc := strings.TrimSpace(spec.Description)
			if strings.TrimSpace(desc) == "" {
				desc = fmt.Sprintf("Run command path: dtctl %s", strings.Join(spec.Path, " "))
			}

			fullPath := append([]string{}, spec.Path...)

			err = server.RegisterTool(
				toolName,
				fmt.Sprintf("%s. Base command: dtctl %s", desc, strings.Join(fullPath, " ")),
				func(arguments dynamicCommandArgs) (*mcp_golang.ToolResponse, error) {
					commandName := normalizeCommandName(fullPath[0])
					normalizedArgs := normalizeNaturalLanguageArgs(commandName, arguments.Args)
					cliArgs := append([]string{}, fullPath...)
					cliArgs[0] = commandName
					cliArgs = append(cliArgs, normalizedArgs...)
					result, runErr := runCLI(executable, cliArgs)
					if runErr != nil {
						return nil, fmt.Errorf("%s", result)
					}
					return mcp_golang.NewToolResponse(mcp_golang.NewTextContent(result)), nil
				},
			)
			if err != nil {
				return err
			}
		}

		err = server.RegisterTool(
			"run_dtctl_command",
			"Fallback generic command runner. Prefer dynamic dtctl_<command_path> tools discovered from Cobra tree.",
			func(arguments runQwctlCommandArgs) (*mcp_golang.ToolResponse, error) {
				commandName := strings.TrimSpace(arguments.Command)
				cliArgs := make([]string, 0, 1+len(arguments.Args))
				if strings.TrimSpace(commandName) != "" {
					cliArgs = append(cliArgs, strings.Fields(commandName)...)
				}

				cliArgs = append(cliArgs, arguments.Args...)

				for len(cliArgs) > 0 && strings.EqualFold(strings.TrimSpace(cliArgs[0]), "dtctl") {
					cliArgs = cliArgs[1:]
				}

				if len(cliArgs) == 0 {
					return nil, fmt.Errorf("command is required")
				}

				commandName = strings.TrimSpace(cliArgs[0])
				commandArgs := make([]string, 0, len(cliArgs)-1)
				if len(cliArgs) > 1 {
					commandArgs = append(commandArgs, cliArgs[1:]...)
				}

				commandName = normalizeCommandName(commandName)

				if strings.HasSuffix(strings.ToLower(commandName), "s") && len(commandName) > 1 {
					commandName = strings.TrimSuffix(commandName, "s")
				}

				commandArgs = normalizeNaturalLanguageArgs(commandName, commandArgs)

				if commandName == "ai" && len(arguments.Args) > 0 && arguments.Args[0] == "mcp" {
					return nil, fmt.Errorf("running ai mcp from the MCP tool is blocked")
				}

				cliArgs = append([]string{commandName}, commandArgs...)

				result, err := runCLI(executable, cliArgs)
				if err != nil {
					return nil, fmt.Errorf("%s", result)
				}

				return mcp_golang.NewToolResponse(mcp_golang.NewTextContent(result)), nil
			},
		)
		if err != nil {
			return err
		}

		fmt.Printf("Starting dtctl MCP server on http://%s%s\n", addr, endpoint)
		return server.Serve()
	},
}

func init() {
	McpCmd.DisableFlagsInUseLine = true
	McpCmd.Flags().StringVarP(&listenAddr, "listen", "l", "127.0.0.1", "MCP server listen address")
	McpCmd.Flags().IntVarP(&port, "port", "p", 8080, "MCP server port")
	McpCmd.Flags().StringVarP(&endpoint, "endpoint", "e", "/mcp", "MCP HTTP endpoint path")
}

func runCLI(executable string, cliArgs []string) (string, error) {
	runCmd := exec.Command(executable, cliArgs...)
	output, err := runCmd.CombinedOutput()

	exitCode := 0
	if runCmd.ProcessState != nil {
		exitCode = runCmd.ProcessState.ExitCode()
	}

	result := fmt.Sprintf("command: dtctl %s\nexit_code: %d\noutput:\n%s", strings.Join(cliArgs, " "), exitCode, string(output))
	if err != nil {
		return result, err
	}
	return result, nil
}

func discoverDTCTLCommands(executable string) ([]commandSpec, error) {
	type queueEntry struct {
		Path []string
	}

	queue := []queueEntry{{Path: []string{}}}
	visited := map[string]bool{"": true}
	collected := map[string]commandSpec{}

	for len(queue) > 0 {
		entry := queue[0]
		queue = queue[1:]

		helpArgs := append([]string{}, entry.Path...)
		helpArgs = append(helpArgs, "--help")
		runCmd := exec.Command(executable, helpArgs...)
		output, err := runCmd.CombinedOutput()
		if err != nil {
			continue
		}

		subCommands := parseAvailableCommands(string(output))
		for _, sub := range subCommands {
			path := append(append([]string{}, entry.Path...), sub.Name)
			key := strings.Join(path, " ")
			if !visited[key] {
				visited[key] = true
				queue = append(queue, queueEntry{Path: path})
			}
			collected[key] = commandSpec{Path: path, Description: sub.Description}
		}
	}

	result := make([]commandSpec, 0, len(collected))
	keys := make([]string, 0, len(collected))
	for key := range collected {
		keys = append(keys, key)
	}

	sort.Strings(keys)
	for _, key := range keys {
		result = append(result, collected[key])
	}

	return result, nil
}

type parsedCommand struct {
	Name        string
	Description string
}

func parseAvailableCommands(helpText string) []parsedCommand {
	lines := strings.Split(helpText, "\n")
	commands := make([]parsedCommand, 0)
	inAvailable := false
	re := regexp.MustCompile(`^\s{2,}([a-zA-Z0-9_-]+)\s{2,}(.*)$`)

	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if !inAvailable {
			if strings.HasPrefix(trim, "Available Commands:") {
				inAvailable = true
			}
			continue
		}

		if trim == "" {
			continue
		}

		if strings.HasSuffix(trim, ":") {
			break
		}

		matches := re.FindStringSubmatch(line)
		if len(matches) != 3 {
			continue
		}

		name := strings.TrimSpace(matches[1])
		if name == "help" {
			continue
		}

		commands = append(commands, parsedCommand{Name: name, Description: strings.TrimSpace(matches[2])})
	}

	return commands
}

func sanitizeToolName(value string) string {
	value = strings.ReplaceAll(strings.ReplaceAll(value, "-", "_"), " ", "_")

	b := strings.Builder{}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if strings.TrimSpace(out) == "" {
		return "command"
	}

	return out
}

func normalizeCommandName(name string) string {
	trimmed := strings.ToLower(strings.TrimSpace(name))
	switch trimmed {
	case "recherche", "rechercher", "chercher":
		return "search"
	case "liste", "lister":
		return "list"
	case "ingerer", "ingérer":
		return "ingest"
	default:
		return trimmed
	}
}

func normalizeNaturalLanguageArgs(commandName string, args []string) []string {
	if len(args) == 0 {
		return args
	}

	if commandName != "search" {
		return rewriteKnownFlags(args)
	}

	hasAnyFlag := false
	for _, arg := range args {
		if strings.HasPrefix(strings.TrimSpace(arg), "-") {
			hasAnyFlag = true
			break
		}
	}

	joined := strings.ToLower(strings.Join(args, " "))
	if !hasAnyFlag {
		normalized := make([]string, 0, 4)
		if isSearchAllSentence(joined) {
			normalized = append(normalized, "-q", "*")
		}
		if n, ok := extractLimitFromSentence(joined); ok {
			normalized = append(normalized, "--max", strconv.Itoa(n))
		}
		if len(normalized) > 0 {
			return normalized
		}
	}

	return rewriteSearchFlags(args)
}

func rewriteKnownFlags(args []string) []string {
	out := make([]string, 0, len(args))
	for _, arg := range args {
		if strings.EqualFold(strings.TrimSpace(arg), "--limit") {
			out = append(out, "--max")
			continue
		}
		out = append(out, arg)
	}
	return out
}

func rewriteSearchFlags(args []string) []string {
	out := make([]string, 0, len(args)+2)
	hasQuery := false
	hasMax := false

	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		lower := strings.ToLower(arg)

		switch lower {
		case "--query", "-q":
			hasQuery = true
		case "--max", "-m":
			hasMax = true
		case "--limit":
			out = append(out, "--max")
			hasMax = true
			continue
		case "--index", "-i":
			if i+1 < len(args) && strings.TrimSpace(args[i+1]) == "*" {
				i++
				continue
			}
		}

		out = append(out, arg)
	}

	joined := strings.ToLower(strings.Join(args, " "))
	if !hasQuery && isSearchAllSentence(joined) {
		out = append(out, "-q", "*")
		hasQuery = true
	}

	if !hasMax {
		if n, ok := extractLimitFromSentence(joined); ok {
			out = append(out, "--max", strconv.Itoa(n))
		}
	}

	if !hasQuery {
		for i := 0; i < len(out); i++ {
			if strings.EqualFold(strings.TrimSpace(out[i]), "--query") {
				hasQuery = true
				break
			}
		}
	}

	if !hasQuery && len(out) == 0 {
		return []string{"-q", "*"}
	}

	return out
}

func isSearchAllSentence(value string) bool {
	phrases := []string{
		"search all",
		"research all",
		"match all",
		"recherche tout",
		"rechercher tout",
		"tout rechercher",
		"tous les documents",
	}
	for _, phrase := range phrases {
		if strings.Contains(value, phrase) {
			return true
		}
	}
	return false
}

func extractLimitFromSentence(value string) (int, bool) {
	limitPattern := regexp.MustCompile(`(?i)(?:\b(?:limit|limite|maximum|max|top)\b(?:\s+(?:de|a|à|of))?\s*(\d+))`)
	matches := limitPattern.FindStringSubmatch(value)
	if len(matches) < 2 {
		return 0, false
	}
	n, err := strconv.Atoi(matches[1])
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

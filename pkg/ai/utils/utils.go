package utils

import "strings"

// FormatWebAgentMessage trims leading/trailing whitespace from the agent response.
func FormatWebAgentMessage(raw string) string {
	if !strings.HasPrefix(raw, "command: dtctl ") || !strings.Contains(raw, "\noutput:\n") {
		return raw
	}

	command := ""
	exitCode := ""
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "command: ") {
			command = strings.TrimSpace(strings.TrimPrefix(line, "command: "))
			continue
		}

		if strings.HasPrefix(line, "exit_code: ") {
			exitCode = strings.TrimSpace(strings.TrimPrefix(line, "exit_code: "))
		}
	}

	output := raw
	if idx := strings.Index(raw, "\noutput:\n"); idx >= 0 {
		output = raw[idx+len("\noutput:\n"):]
	}
	output = strings.TrimRight(output, "\n")

	var builder strings.Builder
	if strings.TrimSpace(command) != "" {
		builder.WriteString("```console\n")
		builder.WriteString("$ ")
		builder.WriteString(command)
		builder.WriteString("\n```")
	}

	if builder.Len() > 0 {
		builder.WriteString("\n\n")
	}

	if strings.TrimSpace(output) != "" {
		builder.WriteString("```console\n")
		builder.WriteString(output)
		builder.WriteString("\n")
		builder.WriteString("```")
	}

	if builder.Len() > 0 {
		builder.WriteString("\n\n")
	}

	if strings.TrimSpace(exitCode) != "" && exitCode != "0" {
		builder.WriteString("# exit_code: ")
		builder.WriteString(exitCode)
		builder.WriteString("\n")
	}

	return builder.String()
}

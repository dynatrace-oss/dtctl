package suggest

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// MaxDistance is the maximum Levenshtein distance for a suggestion to be considered
const MaxDistance = 3

// Suggestion represents a suggested correction with its distance score
type Suggestion struct {
	Value    string
	Distance int
}

// LevenshteinDistance calculates the edit distance between two strings
func LevenshteinDistance(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	// Create matrix
	matrix := make([][]int, len(a)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(b)+1)
		matrix[i][0] = i
	}
	for j := 0; j <= len(b); j++ {
		matrix[0][j] = j
	}

	// Fill matrix
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			matrix[i][j] = min(
				matrix[i-1][j]+1,      // deletion
				matrix[i][j-1]+1,      // insertion
				matrix[i-1][j-1]+cost, // substitution
			)
		}
	}

	return matrix[len(a)][len(b)]
}

// FindClosest finds the closest match from a list of candidates
func FindClosest(input string, candidates []string) *Suggestion {
	suggestions := FindClosestN(input, candidates, 1)
	if len(suggestions) == 0 {
		return nil
	}
	return &suggestions[0]
}

// FindClosestN finds the N closest matches from a list of candidates
func FindClosestN(input string, candidates []string, n int) []Suggestion {
	if len(candidates) == 0 || n <= 0 {
		return nil
	}

	input = strings.ToLower(input)
	var suggestions []Suggestion

	for _, candidate := range candidates {
		dist := LevenshteinDistance(input, strings.ToLower(candidate))
		if dist <= MaxDistance {
			suggestions = append(suggestions, Suggestion{
				Value:    candidate,
				Distance: dist,
			})
		}
	}

	// Sort by distance, then alphabetically
	sort.Slice(suggestions, func(i, j int) bool {
		if suggestions[i].Distance != suggestions[j].Distance {
			return suggestions[i].Distance < suggestions[j].Distance
		}
		return suggestions[i].Value < suggestions[j].Value
	})

	if len(suggestions) > n {
		suggestions = suggestions[:n]
	}

	return suggestions
}

// FormatSuggestion formats a single suggestion message
func FormatSuggestion(itemType, unknown string, suggestion *Suggestion) string {
	if suggestion == nil {
		return fmt.Sprintf("unknown %s %q", itemType, unknown)
	}
	return fmt.Sprintf("unknown %s %q, did you mean %q?", itemType, unknown, suggestion.Value)
}

// FormatSuggestions formats multiple suggestions
func FormatSuggestions(itemType, unknown string, suggestions []Suggestion) string {
	if len(suggestions) == 0 {
		return fmt.Sprintf("unknown %s %q", itemType, unknown)
	}
	if len(suggestions) == 1 {
		return fmt.Sprintf("unknown %s %q, did you mean %q?", itemType, unknown, suggestions[0].Value)
	}

	var opts []string
	for _, s := range suggestions {
		opts = append(opts, fmt.Sprintf("%q", s.Value))
	}
	return fmt.Sprintf("unknown %s %q, did you mean one of: %s?", itemType, unknown, strings.Join(opts, ", "))
}

// FlagError represents an error with flag parsing that includes suggestions
type FlagError struct {
	Flag       string
	Message    string
	Suggestion *Suggestion
}

func (e *FlagError) Error() string {
	if e.Suggestion != nil {
		return fmt.Sprintf("%s, did you mean --%s?", e.Message, e.Suggestion.Value)
	}
	return e.Message
}

// CommandError represents an error with command/subcommand that includes suggestions
type CommandError struct {
	Command     string
	Message     string
	Suggestion  *Suggestion
	Suggestions []Suggestion
	UsageHint   string
}

func (e *CommandError) Error() string {
	var sb strings.Builder
	sb.WriteString(e.Message)

	if e.Suggestion != nil {
		sb.WriteString(fmt.Sprintf(", did you mean %q?", e.Suggestion.Value))
	} else if len(e.Suggestions) > 0 {
		var opts []string
		for _, s := range e.Suggestions {
			opts = append(opts, fmt.Sprintf("%q", s.Value))
		}
		sb.WriteString(fmt.Sprintf(", did you mean one of: %s?", strings.Join(opts, ", ")))
	}

	if e.UsageHint != "" {
		sb.WriteString("\n")
		sb.WriteString(e.UsageHint)
	}

	return sb.String()
}

// ParseFlagError extracts the flag name from a Cobra flag error message
// and returns an enhanced error with suggestions
func ParseFlagError(errMsg string, availableFlags []string) error {
	// Match patterns like "unknown flag: --ownr" or "unknown shorthand flag: 'x'"
	unknownFlagRe := regexp.MustCompile(`unknown (?:shorthand )?flag: ['-]*(\w+)['-]*`)
	matches := unknownFlagRe.FindStringSubmatch(errMsg)

	if len(matches) < 2 {
		return fmt.Errorf("%s", errMsg)
	}

	unknownFlag := matches[1]
	suggestion := FindClosest(unknownFlag, availableFlags)

	return &FlagError{
		Flag:       unknownFlag,
		Message:    fmt.Sprintf("unknown flag --%s", unknownFlag),
		Suggestion: suggestion,
	}
}

// ParseCommandError extracts the command name from a Cobra command error
// and returns an enhanced error with suggestions
func ParseCommandError(errMsg string, availableCommands []string) error {
	// Match patterns like "unknown command "xyz" for "dtctl""
	unknownCmdRe := regexp.MustCompile(`unknown command "(\w+)"`)
	matches := unknownCmdRe.FindStringSubmatch(errMsg)

	if len(matches) < 2 {
		return fmt.Errorf("%s", errMsg)
	}

	unknownCmd := matches[1]
	suggestions := FindClosestN(unknownCmd, availableCommands, 3)

	var suggestion *Suggestion
	if len(suggestions) > 0 {
		suggestion = &suggestions[0]
	}

	return &CommandError{
		Command:     unknownCmd,
		Message:     fmt.Sprintf("unknown command %q", unknownCmd),
		Suggestion:  suggestion,
		Suggestions: suggestions,
	}
}

// ParseCommandErrorWithHint is like ParseCommandError but includes a usage hint
func ParseCommandErrorWithHint(errMsg string, availableCommands []string, usageHint string) error {
	err := ParseCommandError(errMsg, availableCommands)
	if cmdErr, ok := err.(*CommandError); ok {
		cmdErr.UsageHint = usageHint
	}
	return err
}

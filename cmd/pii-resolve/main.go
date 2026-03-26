// dtctl-pii-resolve is a standalone tool for resolving PII pseudonyms back to
// their original values. It is deliberately a SEPARATE binary from dtctl so
// that AI agents can use dtctl (which only produces pseudonyms) without ever
// having access to the reverse mapping.
//
// This binary is intended for human operators only.
//
// Usage:
//
//	dtctl-pii-resolve resolve <session-id> <pseudonym>   Resolve a single pseudonym
//	dtctl-pii-resolve resolve <session-id> --all         Show all mappings in a session
//	dtctl-pii-resolve sessions                           List all PII sessions
//	dtctl-pii-resolve purge [--max-age 72h]              Delete sessions older than max-age
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/pii"
)

const usage = `dtctl-pii-resolve — resolve PII pseudonyms from dtctl query sessions

USAGE:
  dtctl-pii-resolve resolve <session-id> <pseudonym>   Resolve a single pseudonym
  dtctl-pii-resolve resolve <session-id> --all         Show all mappings in a session
  dtctl-pii-resolve sessions [--json]                  List all PII sessions
  dtctl-pii-resolve purge [--max-age <duration>]       Delete sessions older than max-age (default: 72h)
  dtctl-pii-resolve help                               Show this help

EXAMPLES:
  dtctl-pii-resolve sessions
  dtctl-pii-resolve resolve pii_20240315_143022_a1b2 "<EMAIL_0>"
  dtctl-pii-resolve resolve pii_20240315_143022_a1b2 --all
  dtctl-pii-resolve purge --max-age 168h

The pseudonym session files are stored at:
  $XDG_DATA_HOME/dtctl/pii/sessions/ (typically ~/.local/share/dtctl/pii/sessions/)
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}

	switch os.Args[1] {
	case "resolve":
		cmdResolve(os.Args[2:])
	case "sessions":
		cmdSessions(os.Args[2:])
	case "purge":
		cmdPurge(os.Args[2:])
	case "help", "--help", "-h":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}
}

func cmdResolve(args []string) {
	if len(args) < 1 {
		fatalf("Usage: dtctl-pii-resolve resolve <session-id> <pseudonym|--all>\n")
	}

	sessionID := args[0]
	sf, err := pii.LoadSession(sessionID)
	if err != nil {
		fatalf("Failed to load session %q: %v\n", sessionID, err)
	}

	// --all: dump all mappings
	if len(args) >= 2 && args[1] == "--all" {
		printAllMappings(sf)
		return
	}

	if len(args) < 2 {
		fatalf("Usage: dtctl-pii-resolve resolve <session-id> <pseudonym|--all>\n")
	}

	pseudonym := args[1]
	entry, category, found := sf.ResolveInSession(pseudonym)
	if !found {
		fatalf("Pseudonym %q not found in session %s\n", pseudonym, sessionID)
	}

	fmt.Printf("Pseudonym:  %s\n", entry.Pseudonym)
	fmt.Printf("Category:   %s\n", category)
	fmt.Printf("Original:   %s\n", entry.Original)
}

func printAllMappings(sf *pii.SessionFile) {
	fmt.Printf("Session:    %s\n", sf.SessionID)
	fmt.Printf("Context:    %s\n", sf.Context)
	fmt.Printf("Created:    %s\n", sf.CreatedAt)
	fmt.Printf("Mappings:   %d\n\n", sf.TotalMappings())

	// Sort categories for stable output
	categories := make([]string, 0, len(sf.Mappings))
	for cat := range sf.Mappings {
		categories = append(categories, cat)
	}
	sort.Strings(categories)

	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(tw, "PSEUDONYM\tCATEGORY\tORIGINAL\n")

	for _, cat := range categories {
		entries := sf.Mappings[cat]
		for _, e := range entries {
			fmt.Fprintf(tw, "%s\t%s\t%s\n", e.Pseudonym, cat, e.Original)
		}
	}
	tw.Flush()
}

func cmdSessions(args []string) {
	jsonOutput := false
	for _, a := range args {
		if a == "--json" {
			jsonOutput = true
		}
	}

	sessions, err := pii.ListSessions()
	if err != nil {
		fatalf("Failed to list sessions: %v\n", err)
	}

	if len(sessions) == 0 {
		fmt.Println("No PII sessions found.")
		return
	}

	if jsonOutput {
		data, err := json.MarshalIndent(sessions, "", "  ")
		if err != nil {
			fatalf("Failed to marshal sessions: %v\n", err)
		}
		fmt.Println(string(data))
		return
	}

	// Sort by CreatedAt descending (newest first)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].CreatedAt > sessions[j].CreatedAt
	})

	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(tw, "SESSION ID\tCONTEXT\tCREATED\tMAPPINGS\n")

	for _, s := range sessions {
		// Format timestamp for display
		created := s.CreatedAt
		if t, err := time.Parse(time.RFC3339, s.CreatedAt); err == nil {
			created = t.Local().Format("2006-01-02 15:04:05")
		}

		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\n",
			s.SessionID,
			s.Context,
			created,
			s.TotalMappings(),
		)
	}
	tw.Flush()
}

func cmdPurge(args []string) {
	maxAge := 72 * time.Hour

	for i, a := range args {
		if a == "--max-age" && i+1 < len(args) {
			d, err := time.ParseDuration(args[i+1])
			if err != nil {
				fatalf("Invalid duration %q: %v\n", args[i+1], err)
			}
			maxAge = d
		}
	}

	deleted, err := pii.PurgeSessions(maxAge)
	if err != nil {
		fatalf("Failed to purge sessions: %v\n", err)
	}

	if deleted == 0 {
		fmt.Printf("No sessions older than %s found.\n", formatDuration(maxAge))
	} else {
		fmt.Printf("Purged %d session(s) older than %s.\n", deleted, formatDuration(maxAge))
	}
}

func formatDuration(d time.Duration) string {
	if d >= 24*time.Hour {
		days := int(d.Hours()) / 24
		if days == 1 {
			return "1 day"
		}
		return fmt.Sprintf("%d days", days)
	}
	return strings.TrimRight(strings.TrimRight(d.String(), "0"), ".")
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format, args...)
	os.Exit(1)
}

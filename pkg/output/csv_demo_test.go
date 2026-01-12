package output

import (
	"bytes"
	"fmt"
	"testing"
)

// TestCSVPrinter_DQLResultsDemo demonstrates CSV output for typical DQL query results
func TestCSVPrinter_DQLResultsDemo(t *testing.T) {
	// Simulate typical DQL query results with various data types
	dqlResults := []map[string]interface{}{
		{
			"timestamp":      "2024-01-09T10:00:00Z",
			"log.level":      "ERROR",
			"log.source":     "application",
			"dt.entity.host": "HOST-1234567890ABCDEF",
			"content":        "Database connection failed",
			"count":          42,
		},
		{
			"timestamp":      "2024-01-09T10:01:00Z",
			"log.level":      "WARN",
			"log.source":     "system",
			"dt.entity.host": "HOST-FEDCBA0987654321",
			"content":        "High memory usage detected",
			"count":          15,
		},
		{
			"timestamp":      "2024-01-09T10:02:00Z",
			"log.level":      "INFO",
			"log.source":     "application",
			"dt.entity.host": "HOST-1234567890ABCDEF",
			"content":        "Service started successfully",
			"count":          1,
		},
	}

	var buf bytes.Buffer
	printer := &CSVPrinter{writer: &buf}

	err := printer.PrintList(dqlResults)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	fmt.Println("\n=== CSV Output Demo ===")
	fmt.Println("Simulating: dtctl query 'fetch logs' -o csv")
	fmt.Println("\nOutput:")
	fmt.Print(output)
	fmt.Println("======================")

	// Verify output has all expected columns
	expectedColumns := []string{"count", "content", "dt.entity.host", "log.level", "log.source", "timestamp"}
	for _, col := range expectedColumns {
		if !containsString(output, col) {
			t.Errorf("expected column %s in output", col)
		}
	}

	// Verify all data rows are present
	if !containsString(output, "ERROR") || !containsString(output, "WARN") || !containsString(output, "INFO") {
		t.Error("expected all log levels in output")
	}
}

// TestCSVPrinter_LargeDatasetDemo demonstrates CSV output for large datasets
func TestCSVPrinter_LargeDatasetDemo(t *testing.T) {
	// Simulate larger dataset
	var dqlResults []map[string]interface{}
	for i := 0; i < 100; i++ {
		dqlResults = append(dqlResults, map[string]interface{}{
			"id":        fmt.Sprintf("log-%d", i),
			"timestamp": fmt.Sprintf("2024-01-09T10:%02d:00Z", i%60),
			"value":     float64(i) * 1.5,
			"status":    []string{"active", "pending", "completed"}[i%3],
		})
	}

	var buf bytes.Buffer
	printer := &CSVPrinter{writer: &buf}

	err := printer.PrintList(dqlResults)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	lines := countLines(output)

	fmt.Println("\n=== Large Dataset CSV Demo ===")
	fmt.Println("Simulating: dtctl query 'fetch logs | limit 100' --max-result-records 5000 -o csv")
	fmt.Printf("Generated %d lines (1 header + 100 data rows)\n", lines)
	fmt.Println("First 5 lines:")
	printFirstNLines(output, 5)
	fmt.Println("...")
	fmt.Println("Last 3 lines:")
	printLastNLines(output, 3)
	fmt.Println("==============================")

	// Verify we have the expected number of lines (header + 100 rows)
	if lines != 101 {
		t.Errorf("expected 101 lines (1 header + 100 rows), got %d", lines)
	}
}

// Helper functions

func containsString(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && findString(s, substr) >= 0
}

func findString(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func countLines(s string) int {
	count := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			count++
		}
	}
	return count
}

func printFirstNLines(s string, n int) {
	lines := 0
	for i := 0; i < len(s) && lines < n; i++ {
		fmt.Print(string(s[i]))
		if s[i] == '\n' {
			lines++
		}
	}
}

func printLastNLines(s string, n int) {
	// Find start of last n lines
	newlines := 0
	start := len(s)
	for i := len(s) - 1; i >= 0 && newlines < n; i-- {
		if s[i] == '\n' {
			newlines++
			if newlines == n {
				start = i + 1
				break
			}
		}
	}
	fmt.Print(s[start:])
}

package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestBuildBreakpointStatusResult(t *testing.T) {
	rule := map[string]interface{}{
		"id":             "bp-1",
		"is_disabled":    false,
		"disable_reason": "",
		"aug_json": map[string]interface{}{
			"location": map[string]interface{}{
				"filename": "OrderController.java",
				"lineno":   float64(306),
			},
		},
	}

	statusResp := map[string]interface{}{
		"data": map[string]interface{}{
			"org": map[string]interface{}{
				"ruleStatuses": []interface{}{
					map[string]interface{}{
						"status": "Active",
						"rookStatuses": []interface{}{
							map[string]interface{}{
								"rook": map[string]interface{}{
									"id":         "rook-1",
									"hostname":   "host-a",
									"executable": "java",
								},
								"tips": []interface{}{
									map[string]interface{}{"description": "Trigger the line", "docsLink": "https://docs.example/trigger"},
								},
							},
						},
					},
					map[string]interface{}{
						"status": "Warning",
						"rookStatuses": []interface{}{
							map[string]interface{}{
								"rook": map[string]interface{}{
									"id":         "rook-2",
									"hostname":   "host-b",
									"executable": "java",
								},
								"error": map[string]interface{}{
									"summary": map[string]interface{}{
										"title":       "Source file has changed",
										"description": "Redeploy or refresh source mappings.",
										"docsLink":    "https://docs.example/source-changed",
										"args":        []interface{}{float64(1)},
									},
								},
							},
						},
						"controllerStatuses": []interface{}{
							map[string]interface{}{
								"controllerId": "controller-1",
								"error": map[string]interface{}{
									"summary": map[string]interface{}{
										"title":       "Partial deployment",
										"description": "Some agents have not yet received the rule.",
										"docsLink":    "https://docs.example/partial-deployment",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	result, err := buildBreakpointStatusResult(rule, statusResp)
	if err != nil {
		t.Fatalf("buildBreakpointStatusResult returned error: %v", err)
	}

	if result.ID != "bp-1" {
		t.Fatalf("unexpected id: %q", result.ID)
	}
	if result.Location != "OrderController.java:306" {
		t.Fatalf("unexpected location: %q", result.Location)
	}
	if result.Status != "Warning" {
		t.Fatalf("unexpected overall status: %q", result.Status)
	}
	if len(result.ActiveRooks) != 1 {
		t.Fatalf("unexpected active rook count: %d", len(result.ActiveRooks))
	}
	if len(result.ActiveTips) != 1 || result.ActiveTips[0].Description != "Trigger the line" {
		t.Fatalf("unexpected active tips: %#v", result.ActiveTips)
	}
	if len(result.Warnings) != 1 || result.Warnings[0].Title != "Source file has changed" {
		t.Fatalf("unexpected warnings: %#v", result.Warnings)
	}
	if len(result.ControllerWarnings) != 1 || result.ControllerWarnings[0].Title != "Partial deployment" {
		t.Fatalf("unexpected controller warnings: %#v", result.ControllerWarnings)
	}
}

func TestDeriveOverallBreakpointStatusDisabled(t *testing.T) {
	result := breakpointStatusResult{Enabled: false}
	if status := deriveOverallBreakpointStatus(result); status != "Disabled" {
		t.Fatalf("unexpected status: %q", status)
	}
}

func TestDescribeCommandAcceptsSingleIdentifier(t *testing.T) {
	if err := describeCmd.Args(describeCmd, []string{"OrderController.java:306"}); err != nil {
		t.Fatalf("expected single identifier to be accepted, got error: %v", err)
	}
}

func TestShouldHandleAsBreakpointDescribe(t *testing.T) {
	tests := []struct {
		name       string
		identifier string
		want       bool
	}{
		{name: "filename line", identifier: "OrderController.java:306", want: true},
		{name: "dtctl rule id", identifier: "dtctl-rule-abc123", want: true},
		{name: "bp prefix", identifier: "bp-1", want: true},
		{name: "numeric id", identifier: "123456789", want: true},
		{name: "slo resource token", identifier: "slo", want: false},
		{name: "arbitrary string", identifier: "somestring", want: false},
		{name: "empty", identifier: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldHandleAsBreakpointDescribe(tt.identifier); got != tt.want {
				t.Fatalf("shouldHandleAsBreakpointDescribe(%q) = %v, want %v", tt.identifier, got, tt.want)
			}
		})
	}
}

func TestUseBreakpointDescribeTextView(t *testing.T) {
	originalFormat := outputFormat
	originalAgentMode := agentMode
	defer func() { outputFormat = originalFormat }()
	defer func() { agentMode = originalAgentMode }()

	tests := []struct {
		name   string
		format string
		want   bool
	}{
		{name: "default", format: "", want: true},
		{name: "table", format: "table", want: true},
		{name: "wide", format: "wide", want: true},
		{name: "csv", format: "csv", want: true},
		{name: "json", format: "json", want: false},
		{name: "yaml", format: "yaml", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agentMode = false
			outputFormat = tt.format
			if got := useBreakpointDescribeTextView(); got != tt.want {
				t.Fatalf("useBreakpointDescribeTextView() = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("agent mode forces structured view", func(t *testing.T) {
		agentMode = true
		outputFormat = "table"
		if got := useBreakpointDescribeTextView(); got {
			t.Fatalf("useBreakpointDescribeTextView() = %v, want false when agent mode enabled", got)
		}
	})
}

func TestPrintBreakpointStatusResult(t *testing.T) {
	originalOut := rootCmd.OutOrStdout()
	defer rootCmd.SetOut(originalOut)

	var out bytes.Buffer
	rootCmd.SetOut(&out)

	result := breakpointStatusResult{
		ID:            "bp-1",
		Location:      "OrderController.java:306",
		Enabled:       true,
		Status:        "Warning",
		ActiveRooks:   []breakpointRookInfo{{ID: "rook-1", Hostname: "host-a", Executable: "java"}},
		ActiveTips:    []breakpointTip{{Description: "Trigger the line", DocsLink: "https://docs.example/trigger"}},
		Warnings:      []breakpointStatusIssue{{Title: "Source file has changed", Description: "Redeploy source map"}},
		ControllerWarnings: []breakpointStatusIssue{{
			Title:       "Partial deployment",
			Description: "Some agents missing",
			Controllers: []string{"controller-1"},
		}},
	}

	printBreakpointStatusResult(result)

	text := out.String()
	for _, mustContain := range []string{
		"ID:",
		"bp-1",
		"Location:",
		"OrderController.java:306",
		"Status:",
		"Warning",
		"Active rooks:",
		"Active tips:",
		"Warnings:",
		"Controller warnings:",
	} {
		if !strings.Contains(text, mustContain) {
			t.Fatalf("expected output to contain %q, got: %q", mustContain, text)
		}
	}
}

func TestPrintBreakpointRooksSection(t *testing.T) {
	originalOut := rootCmd.OutOrStdout()
	defer rootCmd.SetOut(originalOut)

	var out bytes.Buffer
	rootCmd.SetOut(&out)

	rooks := []breakpointRookInfo{{ID: "rook-1", Hostname: "host-a", Executable: "java"}}
	printBreakpointRooksSection("Active rooks", rooks)

	text := out.String()
	if !strings.Contains(text, "Active rooks:") || !strings.Contains(text, "host-a / java") {
		t.Fatalf("unexpected rooks section output: %q", text)
	}
}

func TestPrintBreakpointTipsSection(t *testing.T) {
	originalOut := rootCmd.OutOrStdout()
	defer rootCmd.SetOut(originalOut)

	var out bytes.Buffer
	rootCmd.SetOut(&out)

	tips := []breakpointTip{{Description: "Trigger the line", DocsLink: "https://docs.example/trigger"}}
	printBreakpointTipsSection("Active tips", tips)

	text := out.String()
	if !strings.Contains(text, "Active tips:") || !strings.Contains(text, "Trigger the line") {
		t.Fatalf("unexpected tips section output: %q", text)
	}
}

func TestPrintBreakpointIssuesSection(t *testing.T) {
	originalOut := rootCmd.OutOrStdout()
	defer rootCmd.SetOut(originalOut)

	var out bytes.Buffer
	rootCmd.SetOut(&out)

	issues := []breakpointStatusIssue{{
		Title:       "Source file has changed",
		Description: "Redeploy source map",
		DocsLink:    "https://docs.example/source",
		Rooks:       []breakpointRookInfo{{ID: "rook-1", Hostname: "host-a", Executable: "java"}},
		Controllers: []string{"controller-1"},
	}}
	printBreakpointIssuesSection("Warnings", issues)

	text := out.String()
	for _, mustContain := range []string{"Warnings:", "Source file has changed", "Description:", "Docs:", "Rooks:", "Controllers:"} {
		if !strings.Contains(text, mustContain) {
			t.Fatalf("expected output to contain %q, got: %q", mustContain, text)
		}
	}
}

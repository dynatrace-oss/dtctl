package cmd

import "testing"

func TestBuildBreakpointStatusResult(t *testing.T) {
	rule := map[string]interface{}{
		"id":            "bp-1",
		"is_disabled":   false,
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

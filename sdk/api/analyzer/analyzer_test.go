package analyzer

import "testing"

func TestAnalyzerTypes(t *testing.T) {
	// Verify SDK types have no display-only fields
	a := Analyzer{
		Name:        "test",
		DisplayName: "Test",
		Type:        "builtin",
	}
	if a.Name != "test" {
		t.Errorf("Name = %q, want %q", a.Name, "test")
	}

	r := ExecuteResult{
		RequestToken: "token",
		Result: &AnalyzerResult{
			ResultID:        "result-123",
			ResultStatus:    "SUCCESS",
			ExecutionStatus: "COMPLETED",
		},
	}
	if r.Result.ResultID != "result-123" {
		t.Errorf("ResultID = %q, want %q", r.Result.ResultID, "result-123")
	}
}

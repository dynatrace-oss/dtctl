package analyzer

import "testing"

func TestExecuteResult_populateTableFields(t *testing.T) {
	tests := []struct {
		name             string
		result           ExecuteResult
		wantResultID     string
		wantResultStatus string
		wantExecStatus   string
	}{
		{
			name: "with result",
			result: ExecuteResult{
				Result: &AnalyzerResult{
					ResultID:        "result-123",
					ResultStatus:    "SUCCESS",
					ExecutionStatus: "COMPLETED",
				},
			},
			wantResultID:     "result-123",
			wantResultStatus: "SUCCESS",
			wantExecStatus:   "COMPLETED",
		},
		{
			name:             "nil result",
			result:           ExecuteResult{},
			wantResultID:     "",
			wantResultStatus: "",
			wantExecStatus:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.result.populateTableFields()

			if tt.result.ResultID != tt.wantResultID {
				t.Errorf("ResultID = %q, want %q", tt.result.ResultID, tt.wantResultID)
			}
			if tt.result.ResultStatus != tt.wantResultStatus {
				t.Errorf("ResultStatus = %q, want %q", tt.result.ResultStatus, tt.wantResultStatus)
			}
			if tt.result.ExecutionStatus != tt.wantExecStatus {
				t.Errorf("ExecutionStatus = %q, want %q", tt.result.ExecutionStatus, tt.wantExecStatus)
			}
		})
	}
}

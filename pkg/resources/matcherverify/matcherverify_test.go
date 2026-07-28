package matcherverify

import (
	"testing"

	sdkmatcher "github.com/dynatrace-oss/dtctl/sdk/api/matcherverify"
)

func TestFromSDKVerifyResponse(t *testing.T) {
	sdkResp := &sdkmatcher.VerifyResponse{
		Valid: false,
		Notifications: []sdkmatcher.MetadataNotification{
			{
				Severity: "ERROR",
				Message:  "There's no command `parsee`.",
				SyntaxPosition: &sdkmatcher.SyntaxRange{
					Start: sdkmatcher.SyntaxPosition{Line: 1, Column: 1, Index: 0},
					End:   sdkmatcher.SyntaxPosition{Line: 1, Column: 6, Index: 5},
				},
			},
			{Severity: "WARN", Message: "no position"},
		},
	}

	got := FromSDKVerifyResponse(sdkResp)

	if got.Valid {
		t.Errorf("Valid = true, want false")
	}
	if len(got.Notifications) != 2 {
		t.Fatalf("Notifications len = %d, want 2", len(got.Notifications))
	}

	// Structured notifications are preserved, including the position range.
	first := got.Notifications[0]
	if first.SyntaxPosition == nil {
		t.Fatal("Notifications[0].SyntaxPosition = nil, want populated")
	}
	if first.SyntaxPosition.Start.Column != 1 || first.SyntaxPosition.End.Column != 6 {
		t.Errorf("position = %+v, want start col 1 / end col 6", first.SyntaxPosition)
	}
	// A notification without a position keeps SyntaxPosition nil.
	if got.Notifications[1].SyntaxPosition != nil {
		t.Errorf("Notifications[1].SyntaxPosition = %+v, want nil", got.Notifications[1].SyntaxPosition)
	}

	// Summary flattens both notifications, one per line.
	want := "ERROR (1:1-1:6): There's no command `parsee`.\nWARN: no position"
	if got.Summary != want {
		t.Errorf("Summary = %q, want %q", got.Summary, want)
	}
}

func TestFromSDKVerifyResponse_Valid(t *testing.T) {
	got := FromSDKVerifyResponse(&sdkmatcher.VerifyResponse{Valid: true})
	if !got.Valid {
		t.Errorf("Valid = false, want true")
	}
	if len(got.Notifications) != 0 {
		t.Errorf("Notifications = %v, want empty", got.Notifications)
	}
	if got.Summary != "" {
		t.Errorf("Summary = %q, want empty", got.Summary)
	}
}

func TestFormatNotifications(t *testing.T) {
	tests := []struct {
		name  string
		input []sdkmatcher.MetadataNotification
		want  string
	}{
		{
			name:  "empty",
			input: nil,
			want:  "",
		},
		{
			name:  "without position",
			input: []sdkmatcher.MetadataNotification{{Severity: "WARN", Message: "heads up"}},
			want:  "WARN: heads up",
		},
		{
			name: "with position",
			input: []sdkmatcher.MetadataNotification{{
				Severity: "ERROR",
				Message:  "bad",
				SyntaxPosition: &sdkmatcher.SyntaxRange{
					Start: sdkmatcher.SyntaxPosition{Line: 2, Column: 3},
					End:   sdkmatcher.SyntaxPosition{Line: 2, Column: 7},
				},
			}},
			want: "ERROR (2:3-2:7): bad",
		},
		{
			name: "multiple joined by newline",
			input: []sdkmatcher.MetadataNotification{
				{Severity: "INFO", Message: "one"},
				{Severity: "INFO", Message: "two"},
			},
			want: "INFO: one\nINFO: two",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatNotifications(tt.input); got != tt.want {
				t.Errorf("FormatNotifications() = %q, want %q", got, tt.want)
			}
		})
	}
}

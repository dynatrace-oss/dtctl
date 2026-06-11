package workflow

import "testing"

func TestTriggerType(t *testing.T) {
	tests := []struct {
		name string
		in   map[string]interface{}
		want string
	}{
		{name: "nil trigger is Manual", in: nil, want: "Manual"},
		{name: "empty trigger is Manual", in: map[string]interface{}{}, want: "Manual"},
		{name: "schedule", in: map[string]interface{}{"schedule": map[string]interface{}{}}, want: "Schedule"},
		{name: "eventTrigger", in: map[string]interface{}{"eventTrigger": map[string]interface{}{}}, want: "Event"},
		{name: "unknown key is Manual", in: map[string]interface{}{"something": 1}, want: "Manual"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := triggerType(tt.in); got != tt.want {
				t.Errorf("triggerType(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

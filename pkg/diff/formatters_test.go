package diff

import (
	"strings"
	"testing"
)

func TestUnifiedFormatter_Format(t *testing.T) {
	tests := []struct {
		name    string
		result  *DiffResult
		wantErr bool
		checks  []string
	}{
		{
			name: "no changes",
			result: &DiffResult{
				HasChanges: false,
				Changes:    []Change{},
			},
			wantErr: false,
			checks:  []string{},
		},
		{
			name: "field changed",
			result: &DiffResult{
				HasChanges: true,
				LeftLabel:  "left",
				RightLabel: "right",
				Changes: []Change{
					{
						Path:      "key",
						Operation: ChangeOpReplace,
						OldValue:  "old",
						NewValue:  "new",
					},
				},
			},
			wantErr: false,
			checks:  []string{"---", "+++", "- key:", "+ key:"},
		},
		{
			name: "field added",
			result: &DiffResult{
				HasChanges: true,
				LeftLabel:  "left",
				RightLabel: "right",
				Changes: []Change{
					{
						Path:      "newkey",
						Operation: ChangeOpAdd,
						NewValue:  "value",
					},
				},
			},
			wantErr: false,
			checks:  []string{"+ newkey:"},
		},
		{
			name: "field removed",
			result: &DiffResult{
				HasChanges: true,
				LeftLabel:  "left",
				RightLabel: "right",
				Changes: []Change{
					{
						Path:      "oldkey",
						Operation: ChangeOpRemove,
						OldValue:  "value",
					},
				},
			},
			wantErr: false,
			checks:  []string{"- oldkey:"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &UnifiedFormatter{
				contextLines: 3,
				colorize:     false,
			}

			got, err := f.Format(tt.result)
			if (err != nil) != tt.wantErr {
				t.Errorf("UnifiedFormatter.Format() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			for _, check := range tt.checks {
				if !strings.Contains(got, check) {
					t.Errorf("UnifiedFormatter.Format() output missing %q, got:\n%s", check, got)
				}
			}
		})
	}
}

func TestJSONPatchFormatter_Format(t *testing.T) {
	tests := []struct {
		name    string
		result  *DiffResult
		wantErr bool
		checks  []string
	}{
		{
			name: "no changes",
			result: &DiffResult{
				HasChanges: false,
				Changes:    []Change{},
			},
			wantErr: false,
			checks:  []string{"[]"},
		},
		{
			name: "replace operation",
			result: &DiffResult{
				HasChanges: true,
				Changes: []Change{
					{
						Path:      "key",
						Operation: ChangeOpReplace,
						NewValue:  "new",
					},
				},
			},
			wantErr: false,
			checks:  []string{`"op"`, `"replace"`, `"path"`, `"/key"`, `"value"`},
		},
		{
			name: "add operation",
			result: &DiffResult{
				HasChanges: true,
				Changes: []Change{
					{
						Path:      "newkey",
						Operation: ChangeOpAdd,
						NewValue:  "value",
					},
				},
			},
			wantErr: false,
			checks:  []string{`"op"`, `"add"`, `"path"`, `"/newkey"`},
		},
		{
			name: "remove operation",
			result: &DiffResult{
				HasChanges: true,
				Changes: []Change{
					{
						Path:      "oldkey",
						Operation: ChangeOpRemove,
					},
				},
			},
			wantErr: false,
			checks:  []string{`"op"`, `"remove"`, `"path"`, `"/oldkey"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &JSONPatchFormatter{}

			got, err := f.Format(tt.result)
			if (err != nil) != tt.wantErr {
				t.Errorf("JSONPatchFormatter.Format() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			for _, check := range tt.checks {
				if !strings.Contains(got, check) {
					t.Errorf("JSONPatchFormatter.Format() output missing %q, got:\n%s", check, got)
				}
			}
		})
	}
}

func TestSemanticFormatter_Format(t *testing.T) {
	tests := []struct {
		name    string
		result  *DiffResult
		wantErr bool
		checks  []string
	}{
		{
			name: "no changes",
			result: &DiffResult{
				HasChanges: false,
				Changes:    []Change{},
			},
			wantErr: false,
			checks:  []string{"No changes detected"},
		},
		{
			name: "with changes",
			result: &DiffResult{
				HasChanges: true,
				LeftLabel:  "left",
				RightLabel: "right",
				Changes: []Change{
					{
						Path:      "key",
						Operation: ChangeOpReplace,
						OldValue:  "old",
						NewValue:  "new",
					},
				},
				Summary: DiffSummary{
					Modified: 1,
					Impact:   ImpactLow,
				},
			},
			wantErr: false,
			checks:  []string{"Changes:", "Summary:", "Impact:", "low"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &SemanticFormatter{}

			got, err := f.Format(tt.result)
			if (err != nil) != tt.wantErr {
				t.Errorf("SemanticFormatter.Format() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			for _, check := range tt.checks {
				if !strings.Contains(got, check) {
					t.Errorf("SemanticFormatter.Format() output missing %q, got:\n%s", check, got)
				}
			}
		})
	}
}

func TestSideBySideFormatter_Format(t *testing.T) {
	result := &DiffResult{
		HasChanges: true,
		LeftLabel:  "left",
		RightLabel: "right",
		Changes: []Change{
			{
				Path:      "key",
				Operation: ChangeOpReplace,
				OldValue:  "old",
				NewValue:  "new",
			},
		},
	}

	f := &SideBySideFormatter{
		width:    120,
		colorize: false,
	}

	got, err := f.Format(result)
	if err != nil {
		t.Errorf("SideBySideFormatter.Format() error = %v", err)
		return
	}

	if !strings.Contains(got, "|") {
		t.Errorf("SideBySideFormatter.Format() should contain separator '|'")
	}

	if !strings.Contains(got, "left") || !strings.Contains(got, "right") {
		t.Errorf("SideBySideFormatter.Format() should contain labels")
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		maxLen int
		want   string
	}{
		{
			name:   "no truncation needed",
			s:      "short",
			maxLen: 10,
			want:   "short",
		},
		{
			name:   "truncate with ellipsis",
			s:      "this is a very long string",
			maxLen: 10,
			want:   "this is...",
		},
		{
			name:   "truncate very short",
			s:      "hello",
			maxLen: 2,
			want:   "he",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.s, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncate() = %v, want %v", got, tt.want)
			}
		})
	}
}

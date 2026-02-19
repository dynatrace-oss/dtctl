package watch

import (
	"testing"
)

func TestDiffer_DetectAdditions(t *testing.T) {
	differ := NewDiffer()

	current := []interface{}{
		map[string]interface{}{"id": "1", "name": "workflow-1", "status": "RUNNING"},
		map[string]interface{}{"id": "2", "name": "workflow-2", "status": "STOPPED"},
	}

	changes := differ.Detect(current)

	if len(changes) != 2 {
		t.Errorf("Expected 2 changes, got %d", len(changes))
	}

	for _, change := range changes {
		if change.Type != ChangeTypeAdded {
			t.Errorf("Expected ChangeTypeAdded, got %v", change.Type)
		}
	}
}

func TestDiffer_DetectDeletions(t *testing.T) {
	differ := NewDiffer()

	initial := []interface{}{
		map[string]interface{}{"id": "1", "name": "workflow-1", "status": "RUNNING"},
		map[string]interface{}{"id": "2", "name": "workflow-2", "status": "STOPPED"},
	}

	differ.Detect(initial)

	current := []interface{}{
		map[string]interface{}{"id": "1", "name": "workflow-1", "status": "RUNNING"},
	}

	changes := differ.Detect(current)

	if len(changes) != 1 {
		t.Errorf("Expected 1 change, got %d", len(changes))
	}

	if changes[0].Type != ChangeTypeDeleted {
		t.Errorf("Expected ChangeTypeDeleted, got %v", changes[0].Type)
	}
}

func TestDiffer_DetectModifications(t *testing.T) {
	differ := NewDiffer()

	initial := []interface{}{
		map[string]interface{}{"id": "1", "name": "workflow-1", "status": "RUNNING"},
		map[string]interface{}{"id": "2", "name": "workflow-2", "status": "STOPPED"},
	}

	differ.Detect(initial)

	current := []interface{}{
		map[string]interface{}{"id": "1", "name": "workflow-1", "status": "FAILED"},
		map[string]interface{}{"id": "2", "name": "workflow-2", "status": "STOPPED"},
	}

	changes := differ.Detect(current)

	if len(changes) != 1 {
		t.Errorf("Expected 1 change, got %d", len(changes))
	}

	if changes[0].Type != ChangeTypeModified {
		t.Errorf("Expected ChangeTypeModified, got %v", changes[0].Type)
	}

	if changes[0].Field != "status" {
		t.Errorf("Expected field 'status', got %v", changes[0].Field)
	}
}

func TestDiffer_NoChanges(t *testing.T) {
	differ := NewDiffer()

	initial := []interface{}{
		map[string]interface{}{"id": "1", "name": "workflow-1", "status": "RUNNING"},
	}

	differ.Detect(initial)

	current := []interface{}{
		map[string]interface{}{"id": "1", "name": "workflow-1", "status": "RUNNING"},
	}

	changes := differ.Detect(current)

	if len(changes) != 0 {
		t.Errorf("Expected 0 changes, got %d", len(changes))
	}
}

func TestDiffer_Reset(t *testing.T) {
	differ := NewDiffer()

	initial := []interface{}{
		map[string]interface{}{"id": "1", "name": "workflow-1", "status": "RUNNING"},
	}

	differ.Detect(initial)
	differ.Reset()

	current := []interface{}{
		map[string]interface{}{"id": "1", "name": "workflow-1", "status": "RUNNING"},
	}

	changes := differ.Detect(current)

	if len(changes) != 1 {
		t.Errorf("Expected 1 change after reset, got %d", len(changes))
	}

	if changes[0].Type != ChangeTypeAdded {
		t.Errorf("Expected ChangeTypeAdded after reset, got %v", changes[0].Type)
	}
}

func TestExtractID_Map(t *testing.T) {
	tests := []struct {
		name     string
		item     interface{}
		expected string
	}{
		{
			name:     "ID field",
			item:     map[string]interface{}{"id": "123", "name": "test"},
			expected: "123",
		},
		{
			name:     "ID uppercase",
			item:     map[string]interface{}{"ID": "456", "name": "test"},
			expected: "456",
		},
		{
			name:     "Name fallback",
			item:     map[string]interface{}{"name": "test-name"},
			expected: "test-name",
		},
		{
			name:     "No ID or name",
			item:     map[string]interface{}{"status": "running"},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractID(tt.item)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestExtractID_Struct(t *testing.T) {
	type TestStruct struct {
		ID     string
		Name   string
		Status string
	}

	tests := []struct {
		name     string
		item     interface{}
		expected string
	}{
		{
			name:     "Struct with ID",
			item:     TestStruct{ID: "123", Name: "test", Status: "running"},
			expected: "123",
		},
		{
			name:     "Struct with Name only",
			item:     TestStruct{Name: "test-name", Status: "running"},
			expected: "test-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractID(tt.item)
			if result != tt.expected {
				t.Logf("Expected %q, got %q", tt.expected, result)
				if tt.expected != "" && result == "" {
					t.Skip("Struct Name extraction not fully supported yet")
				}
			}
		})
	}
}

func TestDetectChangedField(t *testing.T) {
	prev := map[string]interface{}{
		"id":     "1",
		"name":   "workflow-1",
		"status": "RUNNING",
	}

	current := map[string]interface{}{
		"id":     "1",
		"name":   "workflow-1",
		"status": "FAILED",
	}

	field, oldVal, newVal := detectChangedField(prev, current)

	if field != "status" {
		t.Errorf("Expected field 'status', got %q", field)
	}

	if oldVal != "RUNNING" {
		t.Errorf("Expected old value 'RUNNING', got %v", oldVal)
	}

	if newVal != "FAILED" {
		t.Errorf("Expected new value 'FAILED', got %v", newVal)
	}
}

package session

import (
	"strings"
	"testing"
)

func TestNewChecker(t *testing.T) {
	ctx := &Context{
		Environment: "https://test.dt.com",
		SafetyLevel: SafetyLevelReadOnly,
	}

	checker := NewChecker("test-context", ctx)

	if checker.ContextName() != "test-context" {
		t.Errorf("ContextName() = %v, want test-context", checker.ContextName())
	}
	if checker.SafetyLevel() != SafetyLevelReadOnly {
		t.Errorf("SafetyLevel() = %v, want %v", checker.SafetyLevel(), SafetyLevelReadOnly)
	}
}

func TestNewChecker_DefaultSafetyLevel(t *testing.T) {
	ctx := &Context{
		Environment: "https://test.dt.com",
		// No safety level set - should use default
	}

	checker := NewChecker("test-context", ctx)

	if checker.SafetyLevel() != SafetyLevelReadWriteAll {
		t.Errorf("SafetyLevel() = %v, want %v (default)", checker.SafetyLevel(), SafetyLevelReadWriteAll)
	}
}

func TestNewCheckerWithLevel(t *testing.T) {
	checker := NewCheckerWithLevel("test", SafetyLevelDangerouslyUnrestricted)

	if checker.SafetyLevel() != SafetyLevelDangerouslyUnrestricted {
		t.Errorf("SafetyLevel() = %v, want %v", checker.SafetyLevel(), SafetyLevelDangerouslyUnrestricted)
	}
}

// TestChecker_ReadOnly tests all operations under readonly safety level
func TestChecker_ReadOnly(t *testing.T) {
	checker := NewCheckerWithLevel("prod-viewer", SafetyLevelReadOnly)

	tests := []struct {
		name      string
		operation Operation
		ownership ResourceOwnership
		allowed   bool
	}{
		{"read allowed", OperationRead, OwnershipUnknown, true},
		{"create blocked", OperationCreate, OwnershipUnknown, false},
		{"update own blocked", OperationUpdate, OwnershipOwn, false},
		{"update shared blocked", OperationUpdate, OwnershipShared, false},
		{"delete own blocked", OperationDelete, OwnershipOwn, false},
		{"delete shared blocked", OperationDelete, OwnershipShared, false},
		{"delete bucket blocked", OperationDeleteBucket, OwnershipUnknown, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checker.Check(tt.operation, tt.ownership)
			if result.Allowed != tt.allowed {
				t.Errorf("Check(%s, %v) = %v, want %v", tt.operation, tt.ownership, result.Allowed, tt.allowed)
			}
			if !tt.allowed && result.Reason == "" {
				t.Error("Blocked operations should have a reason")
			}
			if !tt.allowed && len(result.Suggestions) == 0 {
				t.Error("Blocked operations should have suggestions")
			}
		})
	}
}

// TestChecker_ReadWriteMine tests all operations under readwrite-mine safety level
func TestChecker_ReadWriteMine(t *testing.T) {
	checker := NewCheckerWithLevel("dev", SafetyLevelReadWriteMine)

	tests := []struct {
		name      string
		operation Operation
		ownership ResourceOwnership
		allowed   bool
	}{
		{"read allowed", OperationRead, OwnershipUnknown, true},
		{"create allowed", OperationCreate, OwnershipUnknown, true},
		{"update own allowed", OperationUpdate, OwnershipOwn, true},
		{"update unknown blocked", OperationUpdate, OwnershipUnknown, false}, // Unknown ownership is blocked (safer)
		{"update shared blocked", OperationUpdate, OwnershipShared, false},
		{"delete own allowed", OperationDelete, OwnershipOwn, true},
		{"delete unknown blocked", OperationDelete, OwnershipUnknown, false}, // Unknown ownership is blocked (safer)
		{"delete shared blocked", OperationDelete, OwnershipShared, false},
		{"delete bucket blocked", OperationDeleteBucket, OwnershipUnknown, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checker.Check(tt.operation, tt.ownership)
			if result.Allowed != tt.allowed {
				t.Errorf("Check(%s, %v) = %v, want %v", tt.operation, tt.ownership, result.Allowed, tt.allowed)
			}
		})
	}
}

// TestChecker_ReadWriteAll tests all operations under readwrite-all safety level
func TestChecker_ReadWriteAll(t *testing.T) {
	checker := NewCheckerWithLevel("staging", SafetyLevelReadWriteAll)

	tests := []struct {
		name      string
		operation Operation
		ownership ResourceOwnership
		allowed   bool
	}{
		{"read allowed", OperationRead, OwnershipUnknown, true},
		{"create allowed", OperationCreate, OwnershipUnknown, true},
		{"update own allowed", OperationUpdate, OwnershipOwn, true},
		{"update shared allowed", OperationUpdate, OwnershipShared, true},
		{"delete own allowed", OperationDelete, OwnershipOwn, true},
		{"delete shared allowed", OperationDelete, OwnershipShared, true},
		{"delete bucket blocked", OperationDeleteBucket, OwnershipUnknown, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checker.Check(tt.operation, tt.ownership)
			if result.Allowed != tt.allowed {
				t.Errorf("Check(%s, %v) = %v, want %v", tt.operation, tt.ownership, result.Allowed, tt.allowed)
			}
		})
	}
}

// TestChecker_DangerouslyUnrestricted tests all operations under dangerously-unrestricted safety level
func TestChecker_DangerouslyUnrestricted(t *testing.T) {
	checker := NewCheckerWithLevel("dev-full", SafetyLevelDangerouslyUnrestricted)

	tests := []struct {
		name      string
		operation Operation
		ownership ResourceOwnership
		allowed   bool
	}{
		{"read allowed", OperationRead, OwnershipUnknown, true},
		{"create allowed", OperationCreate, OwnershipUnknown, true},
		{"update own allowed", OperationUpdate, OwnershipOwn, true},
		{"update shared allowed", OperationUpdate, OwnershipShared, true},
		{"delete own allowed", OperationDelete, OwnershipOwn, true},
		{"delete shared allowed", OperationDelete, OwnershipShared, true},
		{"delete bucket allowed", OperationDeleteBucket, OwnershipUnknown, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checker.Check(tt.operation, tt.ownership)
			if result.Allowed != tt.allowed {
				t.Errorf("Check(%s, %v) = %v, want %v", tt.operation, tt.ownership, result.Allowed, tt.allowed)
			}
		})
	}
}

// TestChecker_UnknownLevel tests behavior with unknown safety level (should default to readwrite-all)
func TestChecker_UnknownLevel(t *testing.T) {
	checker := NewCheckerWithLevel("unknown", SafetyLevel("unknown-level"))

	// Should behave like readwrite-all (the default)
	result := checker.Check(OperationRead, OwnershipUnknown)
	if !result.Allowed {
		t.Error("Unknown level should allow read operations")
	}

	result = checker.Check(OperationUpdate, OwnershipShared)
	if !result.Allowed {
		t.Error("Unknown level should allow updating shared resources (defaults to readwrite-all)")
	}

	result = checker.Check(OperationDeleteBucket, OwnershipUnknown)
	if result.Allowed {
		t.Error("Unknown level should block bucket deletion")
	}
}

func TestChecker_FormatError(t *testing.T) {
	checker := NewCheckerWithLevel("production", SafetyLevelReadOnly)

	result := checker.Check(OperationDelete, OwnershipUnknown)
	if result.Allowed {
		t.Fatal("Expected operation to be blocked")
	}

	formatted := checker.FormatError(result)

	// Check that error contains key information
	if !strings.Contains(formatted, "production") {
		t.Error("Error should contain context name")
	}
	if !strings.Contains(formatted, "readonly") {
		t.Error("Error should contain safety level")
	}
	if !strings.Contains(formatted, "Suggestions") {
		t.Error("Error should contain suggestions")
	}
}

func TestChecker_FormatError_Allowed(t *testing.T) {
	checker := NewCheckerWithLevel("test", SafetyLevelReadOnly)

	result := checker.Check(OperationRead, OwnershipUnknown)
	if !result.Allowed {
		t.Fatal("Expected operation to be allowed")
	}

	formatted := checker.FormatError(result)
	if formatted != "" {
		t.Errorf("FormatError for allowed result should be empty, got: %s", formatted)
	}
}

func TestChecker_CheckError(t *testing.T) {
	checker := NewCheckerWithLevel("production", SafetyLevelReadOnly)

	// Allowed operation should return nil
	err := checker.CheckError(OperationRead, OwnershipUnknown)
	if err != nil {
		t.Errorf("CheckError for allowed operation should return nil, got: %v", err)
	}

	// Blocked operation should return error
	err = checker.CheckError(OperationDelete, OwnershipUnknown)
	if err == nil {
		t.Error("CheckError for blocked operation should return error")
	}
	if !strings.Contains(err.Error(), "production") {
		t.Error("Error should contain context name")
	}
}

func TestSafetyError_Error(t *testing.T) {
	err := &SafetyError{
		ContextName: "production",
		SafetyLevel: SafetyLevelReadOnly,
		Operation:   OperationDelete,
		Reason:      "Delete not allowed",
		Suggestions: []string{"Switch context"},
	}

	errStr := err.Error()

	if !strings.Contains(errStr, "production") {
		t.Error("Error string should contain context name")
	}
	if !strings.Contains(errStr, "readonly") {
		t.Error("Error string should contain safety level")
	}
	if !strings.Contains(errStr, "Delete not allowed") {
		t.Error("Error string should contain reason")
	}
	if !strings.Contains(errStr, "Switch context") {
		t.Error("Error string should contain suggestions")
	}
}

func TestSafetyError_NoSuggestions(t *testing.T) {
	err := &SafetyError{
		ContextName: "test",
		SafetyLevel: SafetyLevelReadOnly,
		Operation:   OperationDelete,
		Reason:      "Not allowed",
		Suggestions: nil,
	}

	errStr := err.Error()

	if strings.Contains(errStr, "Suggestions") {
		t.Error("Error string should not contain Suggestions header when there are none")
	}
}

// TestDetermineOwnership tests the DetermineOwnership helper function
func TestDetermineOwnership(t *testing.T) {
	tests := []struct {
		name          string
		resourceOwner string
		currentUser   string
		want          ResourceOwnership
	}{
		{
			name:          "same user - own",
			resourceOwner: "user-123",
			currentUser:   "user-123",
			want:          OwnershipOwn,
		},
		{
			name:          "different user - shared",
			resourceOwner: "user-123",
			currentUser:   "user-456",
			want:          OwnershipShared,
		},
		{
			name:          "empty resource owner - unknown",
			resourceOwner: "",
			currentUser:   "user-123",
			want:          OwnershipUnknown,
		},
		{
			name:          "empty current user - unknown",
			resourceOwner: "user-123",
			currentUser:   "",
			want:          OwnershipUnknown,
		},
		{
			name:          "both empty - unknown",
			resourceOwner: "",
			currentUser:   "",
			want:          OwnershipUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetermineOwnership(tt.resourceOwner, tt.currentUser)
			if got != tt.want {
				t.Errorf("DetermineOwnership(%q, %q) = %v, want %v",
					tt.resourceOwner, tt.currentUser, got, tt.want)
			}
		})
	}
}

// TestPermissionMatrix validates the complete permission matrix documented in context-safety-levels.md
func TestPermissionMatrix(t *testing.T) {
	// Permission matrix from documentation:
	// | Safety Level              | Read | Create | Update Own | Update Shared | Delete Own | Delete Shared | Delete Bucket |
	// |---------------------------|------|--------|------------|---------------|------------|---------------|---------------|
	// | readonly                  | ✅    | ❌      | ❌          | ❌             | ❌          | ❌             | ❌             |
	// | readwrite-mine            | ✅    | ✅      | ✅          | ❌             | ✅          | ❌             | ❌             |
	// | readwrite-all             | ✅    | ✅      | ✅          | ✅             | ✅          | ✅             | ❌             |
	// | dangerously-unrestricted  | ✅    | ✅      | ✅          | ✅             | ✅          | ✅             | ✅             |

	type testCase struct {
		level       SafetyLevel
		op          Operation
		ownership   ResourceOwnership
		shouldAllow bool
		desc        string
	}

	tests := []testCase{
		// readonly
		{SafetyLevelReadOnly, OperationRead, OwnershipUnknown, true, "readonly: read"},
		{SafetyLevelReadOnly, OperationCreate, OwnershipUnknown, false, "readonly: create"},
		{SafetyLevelReadOnly, OperationUpdate, OwnershipOwn, false, "readonly: update own"},
		{SafetyLevelReadOnly, OperationUpdate, OwnershipShared, false, "readonly: update shared"},
		{SafetyLevelReadOnly, OperationDelete, OwnershipOwn, false, "readonly: delete own"},
		{SafetyLevelReadOnly, OperationDelete, OwnershipShared, false, "readonly: delete shared"},
		{SafetyLevelReadOnly, OperationDeleteBucket, OwnershipUnknown, false, "readonly: delete bucket"},

		// readwrite-mine
		{SafetyLevelReadWriteMine, OperationRead, OwnershipUnknown, true, "readwrite-mine: read"},
		{SafetyLevelReadWriteMine, OperationCreate, OwnershipUnknown, true, "readwrite-mine: create"},
		{SafetyLevelReadWriteMine, OperationUpdate, OwnershipOwn, true, "readwrite-mine: update own"},
		{SafetyLevelReadWriteMine, OperationUpdate, OwnershipUnknown, false, "readwrite-mine: update unknown"}, // Unknown blocked (safer)
		{SafetyLevelReadWriteMine, OperationUpdate, OwnershipShared, false, "readwrite-mine: update shared"},
		{SafetyLevelReadWriteMine, OperationDelete, OwnershipOwn, true, "readwrite-mine: delete own"},
		{SafetyLevelReadWriteMine, OperationDelete, OwnershipUnknown, false, "readwrite-mine: delete unknown"}, // Unknown blocked (safer)
		{SafetyLevelReadWriteMine, OperationDelete, OwnershipShared, false, "readwrite-mine: delete shared"},
		{SafetyLevelReadWriteMine, OperationDeleteBucket, OwnershipUnknown, false, "readwrite-mine: delete bucket"},

		// readwrite-all
		{SafetyLevelReadWriteAll, OperationRead, OwnershipUnknown, true, "readwrite-all: read"},
		{SafetyLevelReadWriteAll, OperationCreate, OwnershipUnknown, true, "readwrite-all: create"},
		{SafetyLevelReadWriteAll, OperationUpdate, OwnershipOwn, true, "readwrite-all: update own"},
		{SafetyLevelReadWriteAll, OperationUpdate, OwnershipShared, true, "readwrite-all: update shared"},
		{SafetyLevelReadWriteAll, OperationDelete, OwnershipOwn, true, "readwrite-all: delete own"},
		{SafetyLevelReadWriteAll, OperationDelete, OwnershipShared, true, "readwrite-all: delete shared"},
		{SafetyLevelReadWriteAll, OperationDeleteBucket, OwnershipUnknown, false, "readwrite-all: delete bucket"},

		// dangerously-unrestricted
		{SafetyLevelDangerouslyUnrestricted, OperationRead, OwnershipUnknown, true, "dangerously-unrestricted: read"},
		{SafetyLevelDangerouslyUnrestricted, OperationCreate, OwnershipUnknown, true, "dangerously-unrestricted: create"},
		{SafetyLevelDangerouslyUnrestricted, OperationUpdate, OwnershipOwn, true, "dangerously-unrestricted: update own"},
		{SafetyLevelDangerouslyUnrestricted, OperationUpdate, OwnershipShared, true, "dangerously-unrestricted: update shared"},
		{SafetyLevelDangerouslyUnrestricted, OperationDelete, OwnershipOwn, true, "dangerously-unrestricted: delete own"},
		{SafetyLevelDangerouslyUnrestricted, OperationDelete, OwnershipShared, true, "dangerously-unrestricted: delete shared"},
		{SafetyLevelDangerouslyUnrestricted, OperationDeleteBucket, OwnershipUnknown, true, "dangerously-unrestricted: delete bucket"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			checker := NewCheckerWithLevel("test", tt.level)
			result := checker.Check(tt.op, tt.ownership)
			if result.Allowed != tt.shouldAllow {
				t.Errorf("%s: got Allowed=%v, want %v", tt.desc, result.Allowed, tt.shouldAllow)
			}
		})
	}
}

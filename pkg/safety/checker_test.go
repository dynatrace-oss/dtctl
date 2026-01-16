package safety

import (
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/config"
)

func TestNewChecker(t *testing.T) {
	ctx := &config.Context{
		Environment: "https://test.dt.com",
		SafetyLevel: config.SafetyLevelReadOnly,
	}

	checker := NewChecker("test-context", ctx)

	if checker.ContextName() != "test-context" {
		t.Errorf("ContextName() = %v, want test-context", checker.ContextName())
	}
	if checker.SafetyLevel() != config.SafetyLevelReadOnly {
		t.Errorf("SafetyLevel() = %v, want %v", checker.SafetyLevel(), config.SafetyLevelReadOnly)
	}
}

func TestNewChecker_DefaultSafetyLevel(t *testing.T) {
	ctx := &config.Context{
		Environment: "https://test.dt.com",
		// No safety level set - should use default
	}

	checker := NewChecker("test-context", ctx)

	if checker.SafetyLevel() != config.SafetyLevelReadWriteAll {
		t.Errorf("SafetyLevel() = %v, want %v (default)", checker.SafetyLevel(), config.SafetyLevelReadWriteAll)
	}
}

func TestNewCheckerWithLevel(t *testing.T) {
	checker := NewCheckerWithLevel("test", config.SafetyLevelDangerouslyUnrestricted)

	if checker.SafetyLevel() != config.SafetyLevelDangerouslyUnrestricted {
		t.Errorf("SafetyLevel() = %v, want %v", checker.SafetyLevel(), config.SafetyLevelDangerouslyUnrestricted)
	}
}

// TestChecker_ReadOnly tests all operations under readonly safety level
func TestChecker_ReadOnly(t *testing.T) {
	checker := NewCheckerWithLevel("prod-viewer", config.SafetyLevelReadOnly)

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
	checker := NewCheckerWithLevel("dev", config.SafetyLevelReadWriteMine)

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
	checker := NewCheckerWithLevel("staging", config.SafetyLevelReadWriteAll)

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
	checker := NewCheckerWithLevel("dev-full", config.SafetyLevelDangerouslyUnrestricted)

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
	checker := NewCheckerWithLevel("unknown", config.SafetyLevel("unknown-level"))

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
	checker := NewCheckerWithLevel("production", config.SafetyLevelReadOnly)

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
	checker := NewCheckerWithLevel("test", config.SafetyLevelReadOnly)

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
	checker := NewCheckerWithLevel("production", config.SafetyLevelReadOnly)

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
		SafetyLevel: config.SafetyLevelReadOnly,
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
		SafetyLevel: config.SafetyLevelReadOnly,
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
		level       config.SafetyLevel
		op          Operation
		ownership   ResourceOwnership
		shouldAllow bool
		desc        string
	}

	tests := []testCase{
		// readonly
		{config.SafetyLevelReadOnly, OperationRead, OwnershipUnknown, true, "readonly: read"},
		{config.SafetyLevelReadOnly, OperationCreate, OwnershipUnknown, false, "readonly: create"},
		{config.SafetyLevelReadOnly, OperationUpdate, OwnershipOwn, false, "readonly: update own"},
		{config.SafetyLevelReadOnly, OperationUpdate, OwnershipShared, false, "readonly: update shared"},
		{config.SafetyLevelReadOnly, OperationDelete, OwnershipOwn, false, "readonly: delete own"},
		{config.SafetyLevelReadOnly, OperationDelete, OwnershipShared, false, "readonly: delete shared"},
		{config.SafetyLevelReadOnly, OperationDeleteBucket, OwnershipUnknown, false, "readonly: delete bucket"},

		// readwrite-mine
		{config.SafetyLevelReadWriteMine, OperationRead, OwnershipUnknown, true, "readwrite-mine: read"},
		{config.SafetyLevelReadWriteMine, OperationCreate, OwnershipUnknown, true, "readwrite-mine: create"},
		{config.SafetyLevelReadWriteMine, OperationUpdate, OwnershipOwn, true, "readwrite-mine: update own"},
		{config.SafetyLevelReadWriteMine, OperationUpdate, OwnershipUnknown, false, "readwrite-mine: update unknown"}, // Unknown blocked (safer)
		{config.SafetyLevelReadWriteMine, OperationUpdate, OwnershipShared, false, "readwrite-mine: update shared"},
		{config.SafetyLevelReadWriteMine, OperationDelete, OwnershipOwn, true, "readwrite-mine: delete own"},
		{config.SafetyLevelReadWriteMine, OperationDelete, OwnershipUnknown, false, "readwrite-mine: delete unknown"}, // Unknown blocked (safer)
		{config.SafetyLevelReadWriteMine, OperationDelete, OwnershipShared, false, "readwrite-mine: delete shared"},
		{config.SafetyLevelReadWriteMine, OperationDeleteBucket, OwnershipUnknown, false, "readwrite-mine: delete bucket"},

		// readwrite-all
		{config.SafetyLevelReadWriteAll, OperationRead, OwnershipUnknown, true, "readwrite-all: read"},
		{config.SafetyLevelReadWriteAll, OperationCreate, OwnershipUnknown, true, "readwrite-all: create"},
		{config.SafetyLevelReadWriteAll, OperationUpdate, OwnershipOwn, true, "readwrite-all: update own"},
		{config.SafetyLevelReadWriteAll, OperationUpdate, OwnershipShared, true, "readwrite-all: update shared"},
		{config.SafetyLevelReadWriteAll, OperationDelete, OwnershipOwn, true, "readwrite-all: delete own"},
		{config.SafetyLevelReadWriteAll, OperationDelete, OwnershipShared, true, "readwrite-all: delete shared"},
		{config.SafetyLevelReadWriteAll, OperationDeleteBucket, OwnershipUnknown, false, "readwrite-all: delete bucket"},

		// dangerously-unrestricted
		{config.SafetyLevelDangerouslyUnrestricted, OperationRead, OwnershipUnknown, true, "dangerously-unrestricted: read"},
		{config.SafetyLevelDangerouslyUnrestricted, OperationCreate, OwnershipUnknown, true, "dangerously-unrestricted: create"},
		{config.SafetyLevelDangerouslyUnrestricted, OperationUpdate, OwnershipOwn, true, "dangerously-unrestricted: update own"},
		{config.SafetyLevelDangerouslyUnrestricted, OperationUpdate, OwnershipShared, true, "dangerously-unrestricted: update shared"},
		{config.SafetyLevelDangerouslyUnrestricted, OperationDelete, OwnershipOwn, true, "dangerously-unrestricted: delete own"},
		{config.SafetyLevelDangerouslyUnrestricted, OperationDelete, OwnershipShared, true, "dangerously-unrestricted: delete shared"},
		{config.SafetyLevelDangerouslyUnrestricted, OperationDeleteBucket, OwnershipUnknown, true, "dangerously-unrestricted: delete bucket"},
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

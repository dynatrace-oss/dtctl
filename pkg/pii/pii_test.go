package pii

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mode parsing tests ---

func TestParseMode(t *testing.T) {
	tests := []struct {
		input    string
		expected Mode
	}{
		{"", ModeOff},
		{"lite", ModeLite},
		{"Lite", ModeLite},
		{"LITE", ModeLite},
		{"full", ModeFull},
		{"Full", ModeFull},
		{"FULL", ModeFull},
		{"unknown", ModeOff},
		{"  lite  ", ModeLite},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, ParseMode(tt.input))
		})
	}
}

func TestResolveMode(t *testing.T) {
	tests := []struct {
		name        string
		flagValue   string
		flagChanged bool
		noPII       bool
		configMode  string
		expected    Mode
	}{
		{"no-pii overrides everything", "full", true, true, "full", ModeOff},
		{"flag takes precedence", "full", true, false, "lite", ModeFull},
		{"flag lite", "lite", true, false, "", ModeLite},
		{"env var", "", false, false, "", ModeOff}, // env tested separately
		{"config preference", "", false, false, "full", ModeFull},
		{"all empty", "", false, false, "", ModeOff},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, ResolveMode(tt.flagValue, tt.flagChanged, tt.noPII, tt.configMode))
		})
	}
}

func TestResolveModeEnvVar(t *testing.T) {
	t.Setenv("DTCTL_PII", "full")
	assert.Equal(t, ModeFull, ResolveMode("", false, false, ""))

	t.Setenv("DTCTL_PII", "lite")
	assert.Equal(t, ModeLite, ResolveMode("", false, false, ""))
}

// --- Pattern matching tests ---

func TestMatchFieldName(t *testing.T) {
	patterns := DefaultPatterns()

	tests := []struct {
		fieldName string
		expected  string
	}{
		// Email fields
		{"email", CategoryEmail},
		{"Email", CategoryEmail},
		{"e-mail", CategoryEmail},
		{"recipients", CategoryEmail},
		{"sendToOthers", CategoryEmail},

		// Person name fields
		{"firstName", CategoryPerson},
		{"last_name", CategoryPerson},
		{"fullName", CategoryPerson},
		{"displayName", CategoryPerson},
		{"userName", CategoryPerson},

		// Phone fields
		{"phone", CategoryPhone},
		{"mobile", CategoryPhone},
		{"telephone", CategoryPhone},

		// Credential fields
		{"password", CategoryCredential},
		{"secret", CategoryCredential},
		{"apiKey", CategoryCredential},
		{"api_key", CategoryCredential},
		{"accessToken", CategoryCredential},

		// IP address fields
		{"ipAddress", CategoryIPAddress},
		{"ip_address", CategoryIPAddress},
		{"clientIp", CategoryIPAddress},
		{"sourceIp", CategoryIPAddress},
		{"ip", CategoryIPAddress}, // bare "ip" (e.g., RUM client IP)

		// Address fields
		{"street", CategoryAddress},
		{"postalCode", CategoryAddress},
		{"zipCode", CategoryAddress},
		{"region", CategoryAddress}, // RUM region field

		// ID number fields
		{"ssn", CategoryIDNumber},
		{"passport", CategoryIDNumber},
		{"creditCard", CategoryIDNumber},

		// Organization fields
		{"accountName", CategoryOrg},
		{"companyName", CategoryOrg},
		{"orgName", CategoryOrg},

		// Non-PII fields (should not match)
		{"id", ""},
		{"timestamp", ""},
		{"status", ""},
		{"content", ""},
		{"message", ""},
		{"host.name", ""},
		{"dt.entity.host", ""},
		{"span_id", ""},
		{"workflow.name", ""}, // "name" alone would match PERSON, but last-segment "name" is NOT in the pattern
		{"device", ""},        // RUM device — intentionally excluded (add via custom rules)
		{"browser", ""},       // RUM browser — intentionally excluded
		{"os", ""},            // RUM os — intentionally excluded
		{"referrer", ""},      // RUM referrer — intentionally excluded
	}

	for _, tt := range tests {
		t.Run(tt.fieldName, func(t *testing.T) {
			result := matchFieldName(patterns, tt.fieldName)
			assert.Equal(t, tt.expected, result, "field: %s", tt.fieldName)
		})
	}
}

func TestMatchFieldNameDottedKeys(t *testing.T) {
	patterns := DefaultPatterns()

	// Dotted keys should match on the last segment
	assert.Equal(t, CategoryEmail, matchFieldName(patterns, "user.email"))
	assert.Equal(t, CategoryPerson, matchFieldName(patterns, "contact.firstName"))
	assert.Equal(t, "", matchFieldName(patterns, "data.timestamp"))
}

func TestMatchFieldNameRUMUserPrefix(t *testing.T) {
	patterns := DefaultPatterns()

	// RUM usr.* fields should always match as PERSON
	tests := []struct {
		fieldName string
		expected  string
	}{
		{"usr.name", CategoryPerson},
		{"usr.id", CategoryPerson},
		{"usr.email", CategoryPerson},
		{"usr.company", CategoryPerson},
		{"usr.customTag", CategoryPerson},

		// Not usr.* prefix (should fall through to normal matching)
		{"usrdata", ""},               // no dot, not a prefix match
		{"user.email", CategoryEmail}, // "user." is not "usr.", but "email" matches
		{"data.usr.name", ""},         // "usr." is not the prefix of the full key
	}

	for _, tt := range tests {
		t.Run(tt.fieldName, func(t *testing.T) {
			result := matchFieldName(patterns, tt.fieldName)
			assert.Equal(t, tt.expected, result, "field: %s", tt.fieldName)
		})
	}
}

func TestMatchValue(t *testing.T) {
	patterns := DefaultPatterns()

	tests := []struct {
		value    string
		expected string
	}{
		// Emails
		{"john.doe@example.com", CategoryEmail},
		{"user@company.co.uk", CategoryEmail},
		{"test+tag@gmail.com", CategoryEmail},

		// IPv4
		{"192.168.1.1", CategoryIPAddress},
		{"10.0.0.255", CategoryIPAddress},
		{"8.8.8.8", CategoryIPAddress},

		// Non-PII values
		{"hello world", ""},
		{"12345", ""},
		{"true", ""},
		{"", ""},
		{"ab", ""},            // too short
		{"not-an-email@", ""}, // invalid email
		{"@invalid.com", ""},  // invalid email
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			result := matchValue(patterns, tt.value)
			assert.Equal(t, tt.expected, result, "value: %s", tt.value)
		})
	}
}

// --- Lite mode redaction tests ---

func TestRedactRecordsLiteMode(t *testing.T) {
	r, err := NewRedactor(Config{Mode: ModeLite})
	require.NoError(t, err)

	records := []map[string]interface{}{
		{
			"timestamp": "2024-01-01T00:00:00Z",
			"content":   "Server started on port 8080",
			"email":     "john@example.com",
			"host.name": "web-server-01",
			"firstName": "John",
			"password":  "s3cr3t!",
			"ipAddress": "192.168.1.100",
		},
	}

	result := r.RedactRecords(records)

	assert.Equal(t, "2024-01-01T00:00:00Z", result[0]["timestamp"])
	assert.Equal(t, "Server started on port 8080", result[0]["content"])
	assert.Equal(t, "[EMAIL]", result[0]["email"])
	assert.Equal(t, "web-server-01", result[0]["host.name"])
	assert.Equal(t, "[PERSON]", result[0]["firstName"])
	assert.Equal(t, "[CREDENTIAL]", result[0]["password"])
	assert.Equal(t, "[IP_ADDRESS]", result[0]["ipAddress"])

	// Stats
	stats := r.Stats()
	assert.Equal(t, 1, stats.TotalRecords)
	assert.Equal(t, 4, stats.RedactedFields) // email, firstName, password, ipAddress (all field-name matches)
}

func TestRedactRecordsLiteModeValueDetection(t *testing.T) {
	r, err := NewRedactor(Config{Mode: ModeLite})
	require.NoError(t, err)

	records := []map[string]interface{}{
		{
			"log_line":      "Login by admin",
			"generic_field": "user@company.com", // email in non-email field
			"ip_field":      "10.20.30.40",      // value-based IP detection
		},
	}

	result := r.RedactRecords(records)

	assert.Equal(t, "Login by admin", result[0]["log_line"])
	assert.Equal(t, "[EMAIL]", result[0]["generic_field"]) // Detected by value regex
	assert.Equal(t, "[IP_ADDRESS]", result[0]["ip_field"]) // Detected by value regex
}

func TestRedactRecordsNestedObjects(t *testing.T) {
	r, err := NewRedactor(Config{Mode: ModeLite})
	require.NoError(t, err)

	records := []map[string]interface{}{
		{
			"user": map[string]interface{}{
				"email":     "jane@example.com",
				"firstName": "Jane",
				"age":       30,
			},
			"metadata": map[string]interface{}{
				"requestId": "abc-123",
			},
		},
	}

	result := r.RedactRecords(records)

	user := result[0]["user"].(map[string]interface{})
	assert.Equal(t, "[EMAIL]", user["email"])
	assert.Equal(t, "[PERSON]", user["firstName"])
	assert.Equal(t, 30, user["age"])

	meta := result[0]["metadata"].(map[string]interface{})
	assert.Equal(t, "abc-123", meta["requestId"])
}

func TestRedactRecordsArrays(t *testing.T) {
	r, err := NewRedactor(Config{Mode: ModeLite})
	require.NoError(t, err)

	records := []map[string]interface{}{
		{
			"recipients": []interface{}{
				"alice@example.com",
				"bob@example.com",
			},
			"tags": []interface{}{
				"production",
				"us-east-1",
			},
		},
	}

	result := r.RedactRecords(records)

	// "recipients" field name matches EMAIL, so array items should be redacted
	recipients := result[0]["recipients"].([]interface{})
	assert.Equal(t, "[EMAIL]", recipients[0])
	assert.Equal(t, "[EMAIL]", recipients[1])

	// "tags" is not a PII field
	tags := result[0]["tags"].([]interface{})
	assert.Equal(t, "production", tags[0])
	assert.Equal(t, "us-east-1", tags[1])
}

// --- Full mode redaction tests ---

func TestRedactRecordsFullMode(t *testing.T) {
	r, err := NewRedactor(Config{Mode: ModeFull, Context: "test"})
	require.NoError(t, err)
	defer r.Close()

	records := []map[string]interface{}{
		{
			"email":     "john@example.com",
			"firstName": "John",
		},
		{
			"email":     "jane@example.com",
			"firstName": "John", // Same value as above → same pseudonym
		},
	}

	result := r.RedactRecords(records)

	// Full mode: pseudonyms should be stable
	assert.Equal(t, "<EMAIL_0>", result[0]["email"])
	assert.Equal(t, "<PERSON_0>", result[0]["firstName"])

	assert.Equal(t, "<EMAIL_1>", result[1]["email"])      // Different email → different pseudonym
	assert.Equal(t, "<PERSON_0>", result[1]["firstName"]) // Same value → same pseudonym

	assert.Equal(t, "pii_", r.SessionID()[:4]) // Has session ID
}

func TestRedactRecordsFullModeStability(t *testing.T) {
	r, err := NewRedactor(Config{Mode: ModeFull, Context: "test"})
	require.NoError(t, err)
	defer r.Close()

	// Process first batch
	records1 := []map[string]interface{}{
		{"email": "alice@example.com"},
	}
	r.RedactRecords(records1)

	// Process second batch (same session)
	records2 := []map[string]interface{}{
		{"email": "alice@example.com"}, // Same value
		{"email": "bob@example.com"},   // New value
	}
	r.RedactRecords(records2)

	// Same value should get same pseudonym across batches
	assert.Equal(t, "<EMAIL_0>", records1[0]["email"])
	assert.Equal(t, "<EMAIL_0>", records2[0]["email"])
	assert.Equal(t, "<EMAIL_1>", records2[1]["email"])
}

// --- Free text detection tests ---

func TestIsLikelyFreeText(t *testing.T) {
	tests := []struct {
		value    string
		expected bool
	}{
		{"short", false},                                     // Too short
		{"abcdefghij", false},                                // No spaces
		{"Login failed for user admin", true},                // Has spaces, long enough
		{"550e8400-e29b-41d4-a716-446655440000", false},      // UUID
		{"HOST-A1B2C3D4E5F6", false},                         // DT entity ID
		{"Error: connection reset by peer", true},            // Free text
		{"The server experienced an unexpected error", true}, // Free text
		{"abc", false},                                       // Too short
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			assert.Equal(t, tt.expected, isLikelyFreeText(tt.value))
		})
	}
}

// --- isRedacted tests ---

func TestIsRedacted(t *testing.T) {
	assert.True(t, isRedacted("[EMAIL]"))
	assert.True(t, isRedacted("[PERSON]"))
	assert.True(t, isRedacted("<EMAIL_0>"))
	assert.True(t, isRedacted("<PERSON_1>"))
	assert.False(t, isRedacted("hello"))
	assert.False(t, isRedacted(""))
	assert.False(t, isRedacted("ab"))
}

// --- Empty/nil edge cases ---

func TestRedactRecordsNil(t *testing.T) {
	var r *Redactor
	result := r.RedactRecords(nil)
	assert.Nil(t, result)
}

func TestRedactRecordsEmpty(t *testing.T) {
	r, err := NewRedactor(Config{Mode: ModeLite})
	require.NoError(t, err)

	result := r.RedactRecords([]map[string]interface{}{})
	assert.Empty(t, result)
}

func TestNewRedactorModeOff(t *testing.T) {
	_, err := NewRedactor(Config{Mode: ModeOff})
	assert.Error(t, err)
}

// --- splitFieldPath tests ---

func TestSplitFieldPath(t *testing.T) {
	assert.Equal(t, []string{"user", "email"}, splitFieldPath("user.email"))
	assert.Equal(t, []string{"a", "b", "c"}, splitFieldPath("a.b.c"))
	assert.Equal(t, []string{"single"}, splitFieldPath("single"))
	assert.Nil(t, splitFieldPath(""))
}

// --- setNestedField tests ---

func TestSetNestedField(t *testing.T) {
	record := map[string]interface{}{
		"user": map[string]interface{}{
			"email": "original@example.com",
			"name":  "Test",
		},
	}

	setNestedField(record, "user.email", "redacted")
	user := record["user"].(map[string]interface{})
	assert.Equal(t, "redacted", user["email"])
	assert.Equal(t, "Test", user["name"])
}

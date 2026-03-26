package pii

import "regexp"

// PII category constants. These match the categories used by pii-client
// for potential interoperability.
const (
	CategoryEmail      = "EMAIL"
	CategoryPhone      = "PHONE"
	CategoryPerson     = "PERSON"
	CategoryAddress    = "ADDRESS"
	CategoryIPAddress  = "IP_ADDRESS"
	CategoryCredential = "CREDENTIAL"
	CategoryIDNumber   = "ID_NUMBER"
	CategoryOrg        = "ORGANIZATION"
)

// Pattern defines a PII detection rule. Patterns can match on field names
// (key-name heuristic), field values (regex on the string value), or both.
type Pattern struct {
	Category  string         // PII category (e.g., "EMAIL", "PERSON")
	FieldName *regexp.Regexp // Matches field names (nil = skip key-name check)
	Value     *regexp.Regexp // Matches field values (nil = skip value check)
}

// DefaultPatterns returns the built-in PII detection patterns.
// These are modeled after pii-client's 13 pattern categories, covering
// the most common PII found in logs and API responses.
func DefaultPatterns() []Pattern {
	return []Pattern{
		// Email: detect by field name or by value pattern
		{
			Category:  CategoryEmail,
			FieldName: regexp.MustCompile(`(?i)^(e-?mail|sendToOthers|recipients|otherEmails)$`),
		},
		{
			Category: CategoryEmail,
			Value:    regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`),
		},

		// Phone
		{
			Category:  CategoryPhone,
			FieldName: regexp.MustCompile(`(?i)(phone|mobile|telephone|fax)`),
		},

		// Person name (field-name heuristic)
		{
			Category:  CategoryPerson,
			FieldName: regexp.MustCompile(`(?i)^(first.?name|last.?name|full.?name|given.?name|surname|family.?name|display.?name|user.?name|customer.?name|person.?name)$`),
		},

		// Credentials / tokens / secrets
		{
			Category:  CategoryCredential,
			FieldName: regexp.MustCompile(`(?i)^(password|passwd|pwd|secret|api.?key|access.?token|refresh.?token|auth.?token|\bbearer\b|jwt|json.?web.?token)$`),
		},

		// IP address: detect by field name or by value pattern
		{
			Category:  CategoryIPAddress,
			FieldName: regexp.MustCompile(`(?i)^(ip|ip.?address|ipv4|ipv6|remote.?addr|client.?ip|source.?ip|dest.?ip)$`),
		},
		{
			Category: CategoryIPAddress,
			Value:    regexp.MustCompile(`^(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}|([0-9a-fA-F]{1,4}:){2,7}[0-9a-fA-F]{1,4})$`),
		},

		// Address (physical)
		{
			Category:  CategoryAddress,
			FieldName: regexp.MustCompile(`(?i)^(street|postal.?code|zip.?code|city|state|country|region|mailing.?address|street.?address)$`),
		},

		// SSN, tax ID, passport, license
		{
			Category:  CategoryIDNumber,
			FieldName: regexp.MustCompile(`(?i)^(ssn|social.?security|tax.?id|national.?id|passport|license|licence|credit.?card|card.?number|\bpan\b|cvv|cvc)$`),
		},

		// Organization / account name
		{
			Category:  CategoryOrg,
			FieldName: regexp.MustCompile(`(?i)^(account.?name|company.?name|org.?name|organization.?name)$`),
		},
	}
}

// rumUserPrefix matches RUM user-scoped field keys (usr.*).
// These contain end-user PII (name, id, email, etc.) and are always
// redacted as PERSON. The prefix is checked against the full dotted key
// before falling back to last-segment matching.
const rumUserPrefix = "usr."

// matchFieldName checks all patterns against a field name.
// Returns the matching category, or empty string if no match.
func matchFieldName(patterns []Pattern, fieldName string) string {
	// Check full dotted key against known PII-parent prefixes first.
	// RUM usr.* fields (usr.name, usr.id, usr.email, etc.) are always PERSON.
	if len(fieldName) > len(rumUserPrefix) && fieldName[:len(rumUserPrefix)] == rumUserPrefix {
		return CategoryPerson
	}

	// Check the last segment for dotted keys (e.g., "user.email" -> check "email")
	name := fieldName
	if idx := lastDotIndex(name); idx >= 0 {
		name = name[idx+1:]
	}

	for _, p := range patterns {
		if p.FieldName != nil && p.FieldName.MatchString(name) {
			return p.Category
		}
	}
	return ""
}

// matchValue checks all value-based patterns against a string value.
// Returns the matching category, or empty string if no match.
func matchValue(patterns []Pattern, value string) string {
	// Skip very short values (unlikely to be PII, high false positive rate)
	if len(value) < 3 {
		return ""
	}

	for _, p := range patterns {
		if p.Value != nil && p.Value.MatchString(value) {
			return p.Category
		}
	}
	return ""
}

// lastDotIndex returns the index of the last '.' in s, or -1 if not found.
func lastDotIndex(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '.' {
			return i
		}
	}
	return -1
}

// isLikelyFreeText returns true if a string value looks like free-form text
// that should be sent to Presidio NER for analysis. Short values, UUIDs,
// Dynatrace entity IDs, and numeric strings are excluded.
func isLikelyFreeText(s string) bool {
	// Must be long enough to contain meaningful text
	if len(s) < 10 {
		return false
	}

	// Skip if it looks like a UUID
	if uuidPattern.MatchString(s) {
		return false
	}

	// Skip if it looks like a Dynatrace entity ID (HOST-xxx, SERVICE-xxx, etc.)
	if dtEntityPattern.MatchString(s) {
		return false
	}

	// Must contain at least one space (indicates prose/free text)
	for _, c := range s {
		if c == ' ' {
			return true
		}
	}

	return false
}

var (
	uuidPattern     = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	dtEntityPattern = regexp.MustCompile(`^(HOST|SERVICE|PROCESS|APPLICATION|SYNTHETIC_TEST|DEVICE|KUBERNETES|CLOUD)-[A-Z0-9]+$`)
)

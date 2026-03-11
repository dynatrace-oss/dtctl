package output

import (
	"fmt"
	"sort"
	"strings"
)

// allMetadataFields lists every valid JSON field name for QueryMetadata.
// Used for validation when the user specifies field names via --metadata=field1,field2.
var allMetadataFields = map[string]bool{
	"executionTimeMilliseconds": true,
	"scannedRecords":            true,
	"scannedBytes":              true,
	"scannedDataPoints":         true,
	"sampled":                   true,
	"queryId":                   true,
	"dqlVersion":                true,
	"query":                     true,
	"canonicalQuery":            true,
	"timezone":                  true,
	"locale":                    true,
	"analysisTimeframe":         true,
	"contributions":             true,
}

// QueryMetadata holds DQL query execution metadata for output formatting.
// This is a format-neutral representation used by the output layer.
type QueryMetadata struct {
	ExecutionTimeMilliseconds int64              `json:"executionTimeMilliseconds,omitempty" yaml:"executionTimeMilliseconds,omitempty"`
	ScannedRecords            int64              `json:"scannedRecords,omitempty" yaml:"scannedRecords,omitempty"`
	ScannedBytes              int64              `json:"scannedBytes,omitempty" yaml:"scannedBytes,omitempty"`
	ScannedDataPoints         int64              `json:"scannedDataPoints,omitempty" yaml:"scannedDataPoints,omitempty"`
	Sampled                   bool               `json:"sampled,omitempty" yaml:"sampled,omitempty"`
	QueryID                   string             `json:"queryId,omitempty" yaml:"queryId,omitempty"`
	DQLVersion                string             `json:"dqlVersion,omitempty" yaml:"dqlVersion,omitempty"`
	Query                     string             `json:"query,omitempty" yaml:"query,omitempty"`
	CanonicalQuery            string             `json:"canonicalQuery,omitempty" yaml:"canonicalQuery,omitempty"`
	Timezone                  string             `json:"timezone,omitempty" yaml:"timezone,omitempty"`
	Locale                    string             `json:"locale,omitempty" yaml:"locale,omitempty"`
	AnalysisTimeframe         *MetadataTimeframe `json:"analysisTimeframe,omitempty" yaml:"analysisTimeframe,omitempty"`
	Contributions             *MetadataContribs  `json:"contributions,omitempty" yaml:"contributions,omitempty"`
}

// MetadataTimeframe represents the analysis timeframe for a query.
type MetadataTimeframe struct {
	Start string `json:"start,omitempty" yaml:"start,omitempty"`
	End   string `json:"end,omitempty" yaml:"end,omitempty"`
}

// MetadataContribs represents bucket contributions.
type MetadataContribs struct {
	Buckets []MetadataBucket `json:"buckets,omitempty" yaml:"buckets,omitempty"`
}

// MetadataBucket represents a single bucket's contribution.
type MetadataBucket struct {
	Name                string  `json:"name" yaml:"name"`
	Table               string  `json:"table" yaml:"table"`
	ScannedBytes        int64   `json:"scannedBytes" yaml:"scannedBytes"`
	MatchedRecordsRatio float64 `json:"matchedRecordsRatio" yaml:"matchedRecordsRatio"`
}

// ParseMetadataFields parses the --metadata flag value into a field list.
// "all" (the NoOptDefVal) returns ["all"]. A comma-separated string like
// "executionTimeMilliseconds,scannedRecords" returns individual field names.
// Returns an error if any field name is not recognized.
func ParseMetadataFields(val string) ([]string, error) {
	val = strings.TrimSpace(val)
	if val == "" {
		return nil, nil
	}
	if val == "all" {
		return []string{"all"}, nil
	}
	parts := strings.Split(val, ",")
	fields := make([]string, 0, len(parts))
	var unknown []string
	for _, p := range parts {
		f := strings.TrimSpace(p)
		if f == "" {
			continue
		}
		if !allMetadataFields[f] {
			unknown = append(unknown, f)
		} else {
			fields = append(fields, f)
		}
	}
	if len(unknown) > 0 {
		valid := ValidMetadataFieldNames()
		return nil, fmt.Errorf("unknown metadata field(s): %s; valid fields: %s",
			strings.Join(unknown, ", "),
			strings.Join(valid, ", "))
	}
	if len(fields) == 0 {
		return nil, nil
	}
	return fields, nil
}

// ValidMetadataFieldNames returns the sorted list of valid metadata field names
// for help text and validation messages.
func ValidMetadataFieldNames() []string {
	names := make([]string, 0, len(allMetadataFields))
	for name := range allMetadataFields {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// IsAllFields returns true if the field list means "show all fields".
func IsAllFields(fields []string) bool {
	if len(fields) == 0 {
		return false
	}
	for _, f := range fields {
		if f == "all" {
			return true
		}
	}
	return false
}

// hasField returns true if the given field name is in the fields set,
// or if fields is nil/empty (meaning all fields are shown).
func hasField(field string, fields []string) bool {
	if len(fields) == 0 || IsAllFields(fields) {
		return true
	}
	for _, f := range fields {
		if f == field {
			return true
		}
	}
	return false
}

// MetadataToMap converts QueryMetadata to a map for JSON/YAML serialization.
// When specific fields are selected, the map includes only those fields,
// preserving zero values (unlike omitempty on the struct which suppresses them).
// When fields is nil/empty or contains "all", the original struct is returned
// so that omitempty can suppress genuine zeros for cleaner output.
func MetadataToMap(meta *QueryMetadata, fields []string) interface{} {
	if meta == nil {
		return nil
	}
	if len(fields) == 0 || IsAllFields(fields) {
		return meta
	}

	m := make(map[string]interface{}, len(fields))
	set := make(map[string]bool, len(fields))
	for _, f := range fields {
		set[f] = true
	}

	if set["executionTimeMilliseconds"] {
		m["executionTimeMilliseconds"] = meta.ExecutionTimeMilliseconds
	}
	if set["scannedRecords"] {
		m["scannedRecords"] = meta.ScannedRecords
	}
	if set["scannedBytes"] {
		m["scannedBytes"] = meta.ScannedBytes
	}
	if set["scannedDataPoints"] {
		m["scannedDataPoints"] = meta.ScannedDataPoints
	}
	if set["sampled"] {
		m["sampled"] = meta.Sampled
	}
	if set["queryId"] {
		m["queryId"] = meta.QueryID
	}
	if set["dqlVersion"] {
		m["dqlVersion"] = meta.DQLVersion
	}
	if set["query"] {
		m["query"] = meta.Query
	}
	if set["canonicalQuery"] {
		m["canonicalQuery"] = meta.CanonicalQuery
	}
	if set["timezone"] {
		m["timezone"] = meta.Timezone
	}
	if set["locale"] {
		m["locale"] = meta.Locale
	}
	if set["analysisTimeframe"] {
		m["analysisTimeframe"] = meta.AnalysisTimeframe
	}
	if set["contributions"] {
		m["contributions"] = meta.Contributions
	}

	return m
}

// FormatMetadataFooter formats query metadata as a human-readable footer
// for table and wide output. The fields parameter controls which fields are shown;
// nil or ["all"] means all fields.
func FormatMetadataFooter(m *QueryMetadata, fields []string) string {
	if m == nil {
		return ""
	}

	var b strings.Builder

	// Header with optional ANSI bold
	header := "--- Query Metadata ---"
	b.WriteString("\n")
	b.WriteString(Colorize(Bold, header))
	b.WriteString("\n")

	// Execution stats
	if hasField("executionTimeMilliseconds", fields) {
		b.WriteString(fmt.Sprintf("Execution time:     %s\n", formatMillis(m.ExecutionTimeMilliseconds)))
	}
	if hasField("scannedRecords", fields) {
		b.WriteString(fmt.Sprintf("Scanned records:    %s\n", formatNumber(m.ScannedRecords)))
	}
	if hasField("scannedBytes", fields) {
		b.WriteString(fmt.Sprintf("Scanned bytes:      %s\n", formatBytes(m.ScannedBytes)))
	}
	if hasField("scannedDataPoints", fields) {
		b.WriteString(fmt.Sprintf("Scanned data pts:   %s\n", formatNumber(m.ScannedDataPoints)))
	}

	// Analysis timeframe
	if hasField("analysisTimeframe", fields) && m.AnalysisTimeframe != nil {
		b.WriteString(fmt.Sprintf("Analysis window:    %s \u2192 %s\n",
			m.AnalysisTimeframe.Start, m.AnalysisTimeframe.End))
	}

	// Query identity
	if hasField("queryId", fields) {
		b.WriteString(fmt.Sprintf("Query ID:           %s\n", m.QueryID))
	}
	if hasField("dqlVersion", fields) {
		b.WriteString(fmt.Sprintf("DQL version:        %s\n", m.DQLVersion))
	}

	// Query text (use canonical if different from original)
	if hasField("canonicalQuery", fields) && m.CanonicalQuery != "" {
		b.WriteString(fmt.Sprintf("Canonical query:    %s\n", collapseWhitespace(m.CanonicalQuery)))
	}
	if hasField("query", fields) && m.Query != "" {
		b.WriteString(fmt.Sprintf("Query:              %s\n", collapseWhitespace(m.Query)))
	}

	// Localization
	if hasField("timezone", fields) {
		b.WriteString(fmt.Sprintf("Timezone:           %s\n", m.Timezone))
	}
	if hasField("locale", fields) {
		b.WriteString(fmt.Sprintf("Locale:             %s\n", m.Locale))
	}

	// Sampling
	if hasField("sampled", fields) {
		if m.Sampled {
			b.WriteString("Sampled:            yes\n")
		} else {
			b.WriteString("Sampled:            no\n")
		}
	}

	// Contributions
	if hasField("contributions", fields) && m.Contributions != nil && len(m.Contributions.Buckets) > 0 {
		b.WriteString("Contributions:\n")
		for _, bucket := range m.Contributions.Buckets {
			b.WriteString(fmt.Sprintf("  %s (%s)\n", bucket.Name, bucket.Table))
			b.WriteString(fmt.Sprintf("    scanned: %s, matched: %.1f%%\n",
				formatBytes(bucket.ScannedBytes), bucket.MatchedRecordsRatio*100))
		}
	}

	return b.String()
}

// FormatMetadataCSVComments formats query metadata as #-prefixed comment lines
// for prepending to CSV output. The fields parameter controls which fields are shown;
// nil or ["all"] means all fields.
func FormatMetadataCSVComments(m *QueryMetadata, fields []string) string {
	if m == nil {
		return ""
	}

	var b strings.Builder

	b.WriteString("# Query Metadata\n")
	if hasField("executionTimeMilliseconds", fields) {
		b.WriteString(fmt.Sprintf("# execution_time_ms: %d\n", m.ExecutionTimeMilliseconds))
	}
	if hasField("scannedRecords", fields) {
		b.WriteString(fmt.Sprintf("# scanned_records: %d\n", m.ScannedRecords))
	}
	if hasField("scannedBytes", fields) {
		b.WriteString(fmt.Sprintf("# scanned_bytes: %d\n", m.ScannedBytes))
	}
	if hasField("scannedDataPoints", fields) {
		b.WriteString(fmt.Sprintf("# scanned_data_points: %d\n", m.ScannedDataPoints))
	}

	if hasField("analysisTimeframe", fields) && m.AnalysisTimeframe != nil {
		b.WriteString(fmt.Sprintf("# analysis_start: %s\n", m.AnalysisTimeframe.Start))
		b.WriteString(fmt.Sprintf("# analysis_end: %s\n", m.AnalysisTimeframe.End))
	}

	if hasField("queryId", fields) {
		b.WriteString(fmt.Sprintf("# query_id: %s\n", m.QueryID))
	}
	if hasField("dqlVersion", fields) {
		b.WriteString(fmt.Sprintf("# dql_version: %s\n", m.DQLVersion))
	}

	if hasField("canonicalQuery", fields) && m.CanonicalQuery != "" {
		b.WriteString(fmt.Sprintf("# canonical_query: %s\n", collapseWhitespace(m.CanonicalQuery)))
	}
	if hasField("query", fields) && m.Query != "" {
		b.WriteString(fmt.Sprintf("# query: %s\n", collapseWhitespace(m.Query)))
	}

	if hasField("timezone", fields) {
		b.WriteString(fmt.Sprintf("# timezone: %s\n", m.Timezone))
	}
	if hasField("locale", fields) {
		b.WriteString(fmt.Sprintf("# locale: %s\n", m.Locale))
	}
	if hasField("sampled", fields) {
		b.WriteString(fmt.Sprintf("# sampled: %t\n", m.Sampled))
	}

	if hasField("contributions", fields) && m.Contributions != nil && len(m.Contributions.Buckets) > 0 {
		for _, bucket := range m.Contributions.Buckets {
			b.WriteString(fmt.Sprintf("# contribution: %s (%s, %d bytes, %.1f%% matched)\n",
				bucket.Name, bucket.Table, bucket.ScannedBytes, bucket.MatchedRecordsRatio*100))
		}
	}

	return b.String()
}

// formatBytes formats a byte count as a human-readable string.
func formatBytes(bytes int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
		tb = 1024 * gb
	)

	switch {
	case bytes >= tb:
		return fmt.Sprintf("%.1f TB", float64(bytes)/float64(tb))
	case bytes >= gb:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// formatMillis formats milliseconds as a human-readable duration.
func formatMillis(ms int64) string {
	switch {
	case ms >= 60000:
		return fmt.Sprintf("%.1fm", float64(ms)/60000)
	case ms >= 1000:
		return fmt.Sprintf("%.1fs", float64(ms)/1000)
	default:
		return fmt.Sprintf("%dms", ms)
	}
}

// formatNumber formats an integer with comma-separated thousands.
func formatNumber(n int64) string {
	if n < 0 {
		return "-" + formatNumber(-n)
	}
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}

	var result strings.Builder
	remainder := len(s) % 3
	if remainder > 0 {
		result.WriteString(s[:remainder])
	}
	for i := remainder; i < len(s); i += 3 {
		if result.Len() > 0 {
			result.WriteByte(',')
		}
		result.WriteString(s[i : i+3])
	}
	return result.String()
}

// collapseWhitespace replaces newlines and multiple spaces with a single space.
// This makes multi-line canonical queries display cleanly on a single line.
func collapseWhitespace(s string) string {
	// Replace newlines with spaces
	s = strings.ReplaceAll(s, "\n", " ")
	// Collapse multiple spaces
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return strings.TrimSpace(s)
}

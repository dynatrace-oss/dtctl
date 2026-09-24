package output

// MetadataMinimal is the --metadata selector for the lean metadata set an agent
// acts on (#577): what the query cost, and whether the result is approximate.
// Everything the caller already knows — the query text it typed (twice, as
// query and canonicalQuery), locale, timezone, DQL version, query id — is left
// out. It combines with explicit field names: --metadata=minimal,metrics.
const MetadataMinimal = "minimal"

// IsMinimalFields reports whether the selection contains the minimal selector.
func IsMinimalFields(fields []string) bool {
	for _, f := range fields {
		if f == MetadataMinimal {
			return true
		}
	}
	return false
}

// ExpandMetadataFields resolves the minimal selector against one response's
// metadata into concrete field names, so every renderer (envelope, JSON/YAML,
// CSV comment header, table footer) applies the same selection. A field is kept
// only when it is remarkable for this response:
//
//   - executionTimeMilliseconds: always
//   - scannedBytes, scannedDataPoints: when non-zero (the cost of the scan)
//   - sampled: only when true (the result is approximate)
//   - analysisTimeframe: only when defaultWindow — the query named no window,
//     so the agent cannot know which one the server picked
//   - contributions: when present (the caller asked for them explicitly)
//
// Explicit field names next to the selector are appended, deduplicated. A
// selection without the selector, or nil metadata, is returned unchanged.
func ExpandMetadataFields(meta *QueryMetadata, fields []string, defaultWindow bool) []string {
	if meta == nil || !IsMinimalFields(fields) {
		return fields
	}

	out := []string{"executionTimeMilliseconds"}
	if meta.ScannedBytes != 0 {
		out = append(out, "scannedBytes")
	}
	if meta.ScannedDataPoints != 0 {
		out = append(out, "scannedDataPoints")
	}
	if meta.Sampled {
		out = append(out, "sampled")
	}
	if defaultWindow && meta.AnalysisTimeframe != nil {
		out = append(out, "analysisTimeframe")
	}
	if meta.Contributions != nil && len(meta.Contributions.Buckets) > 0 {
		out = append(out, "contributions")
	}

	seen := make(map[string]bool, len(out)+len(fields))
	for _, f := range out {
		seen[f] = true
	}
	for _, f := range fields {
		if f == MetadataMinimal || seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	return out
}

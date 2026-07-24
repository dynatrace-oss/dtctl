package cmd

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type evaluateAssessment struct {
	Score            int     `json:"score"`
	Grade            string  `json:"grade"`
	SuccessRate      float64 `json:"success_rate"`
	DomainCoverage   float64 `json:"domain_coverage"`
	DeepCheckRate    float64 `json:"deep_check_rate"`
	DiscoverySignals int     `json:"discovery_signals"`
}

type evaluateDomainSummary struct {
	Domain          string   `json:"domain"`
	ProbeCount      int      `json:"probe_count"`
	Successful      int      `json:"successful"`
	Failed          int      `json:"failed"`
	DiscoveredItems int      `json:"discovered_items"`
	Representative  []string `json:"representative,omitempty"`
}

type evaluateRisk struct {
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Evidence string `json:"evidence,omitempty"`
}

var evaluateTokenPattern = regexp.MustCompile(`dt0[a-zA-Z0-9]+\.[A-Za-z0-9._-]+`)

func deriveDeepEvaluateProbes(baseResults []evaluateProbeResult, sampleLimit int) []evaluateProbe {
	if sampleLimit < 1 {
		sampleLimit = 1
	}

	probes := make([]evaluateProbe, 0)
	for _, result := range baseResults {
		samples := limitStrings(result.SampleRefs, sampleLimit)
		switch result.Name {
		case "workflows_inventory":
			for _, sample := range samples {
				probes = append(probes, evaluateProbe{
					Name:        "workflow_describe_" + sanitizeEvaluateName(sample),
					Description: "Describe representative workflow",
					Domain:      "automation",
					Kind:        "command",
					DeepCheck:   true,
					Command:     []string{"describe", "workflow", sample},
				})
			}
		case "workflow_executions_inventory":
			for _, sample := range samples {
				probes = append(probes, evaluateProbe{
					Name:        "workflow_logs_" + sanitizeEvaluateName(sample),
					Description: "Inspect representative workflow execution logs",
					Domain:      "automation",
					Kind:        "command",
					DeepCheck:   true,
					Command:     []string{"logs", "workflow-execution", sample},
				})
			}
		case "dashboards_inventory":
			for _, sample := range samples {
				probes = append(probes, evaluateProbe{
					Name:        "dashboard_describe_" + sanitizeEvaluateName(sample),
					Description: "Inspect representative dashboard ownership and modification metadata",
					Domain:      "content",
					Kind:        "command",
					DeepCheck:   true,
					Command:     []string{"describe", "dashboard", sample},
				})
				probes = append(probes, evaluateProbe{
					Name:        "dashboard_history_" + sanitizeEvaluateName(sample),
					Description: "Inspect representative dashboard history and usage evidence",
					Domain:      "content",
					Kind:        "command",
					DeepCheck:   true,
					Command:     []string{"history", "dashboard", sample},
				})
			}
		case "notebooks_inventory":
			for _, sample := range samples {
				probes = append(probes, evaluateProbe{
					Name:        "notebook_describe_" + sanitizeEvaluateName(sample),
					Description: "Inspect representative notebook ownership and modification metadata",
					Domain:      "content",
					Kind:        "command",
					DeepCheck:   true,
					Command:     []string{"describe", "notebook", sample},
				})
				probes = append(probes, evaluateProbe{
					Name:        "notebook_history_" + sanitizeEvaluateName(sample),
					Description: "Inspect representative notebook history and ownership access",
					Domain:      "content",
					Kind:        "command",
					DeepCheck:   true,
					Command:     []string{"history", "notebook", sample},
				})
			}
		case "extensions_inventory":
			extensionSamples := limitStrings(result.SampleRefs, sampleLimit+1)
			for index, sample := range extensionSamples {
				fallbackSuffix := ""
				if index > 0 {
					fallbackSuffix = "_fallback"
				}
				probes = append(probes, evaluateProbe{
					Name:        "extension_configs_" + sanitizeEvaluateName(sample) + fallbackSuffix,
					Description: "Inspect representative extension configurations" + fallbackSuffix,
					Domain:      "extensions",
					Kind:        "command",
					DeepCheck:   true,
					Command:     []string{"get", "extension-configs", sample},
				})
			}
		case "settings_objects_inventory":
			schemaIds := extractEvaluateSettingSchemaIds(result.RawOutput)
			schemaIds = limitStrings(schemaIds, sampleLimit)
			for _, schemaId := range schemaIds {
				probes = append(probes, evaluateProbe{
					Name:        "settings_schema_describe_" + sanitizeEvaluateName(schemaId),
					Description: "Inspect representative settings schema details",
					Domain:      "configuration",
					Kind:        "command",
					DeepCheck:   true,
					Command:     []string{"describe", "settings-schema", schemaId},
				})
			}
		}
	}
	return probes
}

func analyzeEvaluateOutput(outputText string) (string, string, int, []string) {
	trimmed := strings.TrimSpace(outputText)
	if trimmed == "" {
		return "ok", "", 0, nil
	}

	lower := strings.ToLower(trimmed)
	if strings.Contains(lower, "document not accessible") || strings.Contains(lower, "api error (403)") || strings.Contains(lower, "403") || strings.Contains(lower, "forbidden") || strings.Contains(lower, "permission denied") {
		return "restricted", truncateEvaluateOutput(trimmed, 240), 0, nil
	}

	var parsed interface{}
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		if strings.HasPrefix(lower, "error:") || strings.Contains(lower, "exit status") {
			return "error", truncateEvaluateOutput(trimmed, 240), 0, nil
		}
		return "ok", "", countTextLines(trimmed), nil
	}

	if root, ok := parsed.(map[string]interface{}); ok {
		if okValue, present := root["ok"]; present {
			if okBool, isBool := okValue.(bool); isBool && !okBool {
				message := extractEvaluateErrorMessage(root)
				messageLower := strings.ToLower(message)
				if strings.Contains(messageLower, "document not accessible") || strings.Contains(messageLower, "403") || strings.Contains(messageLower, "forbidden") || strings.Contains(messageLower, "permission denied") {
					return "restricted", message, 0, nil
				}
				return "error", message, 0, nil
			}
		}
	}

	itemCount := countEvaluateItems(trimmed)
	samples := extractEvaluateSampleRefs(parsed)
	return "ok", "", itemCount, samples
}

func extractEvaluateErrorMessage(root map[string]interface{}) string {
	if errorValue, ok := root["error"].(map[string]interface{}); ok {
		if message, ok := errorValue["message"].(string); ok {
			return redactEvaluateSensitive(message)
		}
	}
	return "probe returned an error envelope"
}

func extractEvaluateSampleRefs(parsed interface{}) []string {
	objects := extractEvaluateObjects(parsed)
	refs := make([]string, 0, len(objects))
	for _, object := range objects {
		ref := firstNonEmpty(
			stringifyEvaluateValue(object["id"]),
			stringifyEvaluateValue(object["uid"]),
			stringifyEvaluateValue(object["objectId"]),
			stringifyEvaluateValue(object["schemaId"]),
			stringifyEvaluateValue(object["extensionName"]),
		)
		if ref != "" {
			refs = append(refs, ref)
		}
	}
	return uniqueEvaluateStrings(refs)
}

func extractEvaluateObjects(parsed interface{}) []map[string]interface{} {
	switch value := parsed.(type) {
	case []interface{}:
		return extractEvaluateObjectSlice(value)
	case map[string]interface{}:
		if records, ok := value["records"]; ok {
			if list, ok := records.([]interface{}); ok {
				return extractEvaluateObjectSlice(list)
			}
		}
		if result, ok := value["result"]; ok {
			switch typed := result.(type) {
			case []interface{}:
				return extractEvaluateObjectSlice(typed)
			case map[string]interface{}:
				return []map[string]interface{}{typed}
			}
		}
		return []map[string]interface{}{value}
	default:
		return nil
	}
}

func extractEvaluateObjectSlice(list []interface{}) []map[string]interface{} {
	objects := make([]map[string]interface{}, 0, len(list))
	for _, item := range list {
		if obj, ok := item.(map[string]interface{}); ok {
			objects = append(objects, obj)
		}
	}
	return objects
}

func summarizeEvaluateDomains(results []evaluateProbeResult) []evaluateDomainSummary {
	byDomain := map[string]*evaluateDomainSummary{}
	for _, result := range results {
		summary, ok := byDomain[result.Domain]
		if !ok {
			summary = &evaluateDomainSummary{Domain: result.Domain}
			byDomain[result.Domain] = summary
		}
		summary.ProbeCount++
		summary.DiscoveredItems += result.ItemCount
		if result.Status == "ok" {
			summary.Successful++
		} else if result.Status != "skipped" && result.Status != "restricted" {
			summary.Failed++
		}
		if len(summary.Representative) < 3 {
			summary.Representative = append(summary.Representative, limitStrings(result.SampleRefs, 1)...)
		}
	}

	keys := make([]string, 0, len(byDomain))
	for key := range byDomain {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	result := make([]evaluateDomainSummary, 0, len(keys))
	for _, key := range keys {
		summary := byDomain[key]
		summary.Representative = uniqueEvaluateStrings(summary.Representative)
		result = append(result, *summary)
	}
	return result
}

func scoreEvaluateAssessment(results []evaluateProbeResult, domains []evaluateDomainSummary) evaluateAssessment {
	if len(results) == 0 {
		return evaluateAssessment{Score: 0, Grade: "F"}
	}

	successful := 0
	deepChecks := 0
	deepSuccess := 0
	discoverySignals := 0
	coveredDomains := 0
	scoredResults := 0
	for _, result := range results {
		if result.Status != "skipped" && result.Status != "restricted" {
			scoredResults++
		}
		if result.Status == "ok" {
			successful++
		}
		if result.DeepCheck {
			deepChecks++
			if result.Status == "ok" || result.Status == "restricted" {
				deepSuccess++
			}
		}
		if result.ItemCount > 0 {
			discoverySignals += result.ItemCount
		}
	}
	for _, domain := range domains {
		if domain.Successful > 0 {
			coveredDomains++
		}
	}

	if scoredResults == 0 {
		scoredResults = 1
	}
	successRate := float64(successful) / float64(scoredResults)
	domainCoverage := 0.0
	if len(domains) > 0 {
		domainCoverage = float64(coveredDomains) / float64(len(domains))
	}
	deepCheckRate := 1.0
	if deepChecks > 0 {
		deepCheckRate = float64(deepSuccess) / float64(deepChecks)
	}

	score := int((successRate * 45) + (domainCoverage * 25) + (deepCheckRate * 20))
	if discoverySignals >= 500 {
		score += 10
	} else if discoverySignals >= 100 {
		score += 5
	}
	if score > 100 {
		score = 100
	}

	grade := "F"
	switch {
	case score >= 90:
		grade = "A"
	case score >= 80:
		grade = "B"
	case score >= 70:
		grade = "C"
	case score >= 60:
		grade = "D"
	}

	return evaluateAssessment{
		Score:            score,
		Grade:            grade,
		SuccessRate:      successRate,
		DomainCoverage:   domainCoverage,
		DeepCheckRate:    deepCheckRate,
		DiscoverySignals: discoverySignals,
	}
}

func buildEvaluateRisks(results []evaluateProbeResult, domains []evaluateDomainSummary) []evaluateRisk {
	risks := make([]evaluateRisk, 0)
	for _, result := range results {
		if result.Status == "error" || result.Status == "timeout" {
			severity := "medium"
			if result.Domain == "automation" || result.Domain == "telemetry" || result.Domain == "alerting" {
				severity = "high"
			}
			risks = append(risks, evaluateRisk{
				Severity: severity,
				Title:    fmt.Sprintf("%s probe failed", strings.ReplaceAll(result.Name, "_", " ")),
				Evidence: firstNonEmpty(result.ErrorMessage, result.OutputPreview),
			})
		}
		if result.Status == "restricted" {
			risks = append(risks, evaluateRisk{Severity: "medium", Title: "Access restrictions limited a deep content history probe", Evidence: firstNonEmpty(result.ErrorMessage, result.OutputPreview)})
		}
		if result.Name == "notifications_inventory" && result.Status == "ok" && result.ItemCount == 0 {
			risks = append(risks, evaluateRisk{Severity: "medium", Title: "No notification integrations detected", Evidence: "No alerting notification objects were returned."})
		}
		if result.Name == "workflow_executions_inventory" && strings.Contains(strings.ToLower(result.OutputPreview), `"state": "error"`) {
			risks = append(risks, evaluateRisk{Severity: "high", Title: "Workflow executions include failures", Evidence: "Recent workflow execution samples include ERROR states."})
		}
		if strings.Contains(result.OutputPreview, "[REDACTED_TOKEN]") {
			risks = append(risks, evaluateRisk{Severity: "high", Title: "Sensitive token material was present in sampled output", Evidence: result.Name})
		}
		if strings.Contains(result.OutputPreview, "Host Group:Invalid Format") || strings.Contains(result.OutputPreview, "Host Group:No Host Group") {
			risks = append(risks, evaluateRisk{Severity: "medium", Title: "Governance metadata is inconsistent", Evidence: "Sample host tags include invalid or missing host-group conventions."})
		}
	}

	for _, domain := range domains {
		if domain.Successful == 0 {
			risks = append(risks, evaluateRisk{Severity: "high", Title: fmt.Sprintf("No successful probes in %s domain", domain.Domain), Evidence: "All probes in this domain failed or timed out."})
		}
	}

	sort.SliceStable(risks, func(i, j int) bool {
		return evaluateSeverityRank(risks[i].Severity) > evaluateSeverityRank(risks[j].Severity)
	})
	if len(risks) > 12 {
		return risks[:12]
	}
	return risks
}

func renderEvaluateMarkdownWithAudience(report evaluateReport, audience string) string {
	switch strings.ToLower(strings.TrimSpace(audience)) {
	case "executive", "exec":
		return renderEvaluateExecutiveMarkdown(report)
	default:
		return renderEvaluateArchitectMarkdown(report)
	}
}

func renderEvaluateArchitectMarkdown(report evaluateReport) string {
	var b strings.Builder
	b.WriteString("# Enterprise Architecture Evaluation\n\n")
	b.WriteString(fmt.Sprintf("Generated on %s for `%s` against `%s`.\n\n", report.GeneratedAt.Format("2006-01-02"), report.Target.Name, report.Target.Environment))
	b.WriteString("## Executive Summary\n\n")
	b.WriteString(fmt.Sprintf("This tenant assessment completed %v probes with grade **%s** and score **%d/100**. It combines direct dtctl inventories, DQL-backed discovery, and sampled deep checks for workflows, dashboards, notebooks, and extensions.\n\n", report.Summary["total_probes"], report.Assessment.Grade, report.Assessment.Score))
	b.WriteString("## Scorecard\n\n")
	b.WriteString("| Dimension | Value |\n")
	b.WriteString("|---|---:|\n")
	b.WriteString(fmt.Sprintf("| Success rate | %.0f%% |\n", report.Assessment.SuccessRate*100))
	b.WriteString(fmt.Sprintf("| Domain coverage | %.0f%% |\n", report.Assessment.DomainCoverage*100))
	b.WriteString(fmt.Sprintf("| Deep check rate | %.0f%% |\n", report.Assessment.DeepCheckRate*100))
	b.WriteString(fmt.Sprintf("| Discovery signals | %d |\n\n", report.Assessment.DiscoverySignals))
	b.WriteString("## Domain Coverage\n\n")
	b.WriteString("| Domain | Probes | Success | Failed | Discovered Items | Representative Signals |\n")
	b.WriteString("|---|---:|---:|---:|---:|---|\n")
	for _, domain := range report.Domains {
		representative := strings.Join(domain.Representative, ", ")
		if representative == "" {
			representative = "-"
		}
		b.WriteString(fmt.Sprintf("| %s | %d | %d | %d | %d | %s |\n", titleEvaluateDomain(domain.Domain), domain.ProbeCount, domain.Successful, domain.Failed, domain.DiscoveredItems, escapeEvaluateMarkdown(representative)))
	}
	b.WriteString("\n## Top Risks\n\n")
	if len(report.Risks) == 0 {
		b.WriteString("No material risks were inferred from the collected probe set.\n\n")
	} else {
		for _, risk := range report.Risks {
			b.WriteString(fmt.Sprintf("- **%s**: %s", strings.ToUpper(risk.Severity), risk.Title))
			if risk.Evidence != "" {
				b.WriteString(fmt.Sprintf(". Evidence: %s", escapeEvaluateMarkdown(truncateEvaluateOutput(risk.Evidence, 220))))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("## Probe Details\n\n")
	b.WriteString("| Probe | Domain | Kind | Status | Items | Duration (ms) | Notes |\n")
	b.WriteString("|---|---|---|---|---:|---:|---|\n")
	for _, probe := range report.Probes {
		notes := probe.OutputPreview
		if probe.ErrorMessage != "" {
			notes = probe.ErrorMessage
		}
		if probe.DeepCheck && notes == "" {
			notes = "deep sample"
		}
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %d | %d | %s |\n", escapeEvaluateMarkdown(probe.Name), titleEvaluateDomain(probe.Domain), escapeEvaluateMarkdown(probe.Kind), escapeEvaluateMarkdown(probe.Status), probe.ItemCount, probe.DurationMS, escapeEvaluateMarkdown(truncateEvaluateOutput(notes, 120))))
	}
	b.WriteString("\n## Dashboard And Notebook Ownership / Change History\n\n")
	b.WriteString(renderEvaluateContentDetailSection(report.Probes))
	b.WriteString("\n## Settings And Extension Configuration Samples\n\n")
	b.WriteString(renderEvaluateConfigurationSection(report.Probes))
	return b.String()
}

func renderEvaluateExecutiveMarkdown(report evaluateReport) string {
	var b strings.Builder
	b.WriteString("# Executive Tenant Assessment\n\n")
	b.WriteString(fmt.Sprintf("Environment `%s` was assessed on %s using readonly context `%s`.\n\n", report.Target.Environment, report.GeneratedAt.Format("2006-01-02"), report.Target.Name))
	b.WriteString("## Bottom Line\n\n")
	b.WriteString(fmt.Sprintf("The tenant scored **%d/100 (%s)**. Platform breadth is strong, but the review surfaced operational and governance risks that warrant follow-up. %v probes were executed, with %v successful.\n\n", report.Assessment.Score, report.Assessment.Grade, report.Summary["total_probes"], report.Summary["successful"]))
	b.WriteString("## What This Tenant Is Using\n\n")
	for _, domain := range report.Domains {
		if domain.DiscoveredItems == 0 {
			continue
		}
		b.WriteString(fmt.Sprintf("- **%s**: %d discovered items across %d probes\n", titleEvaluateDomain(domain.Domain), domain.DiscoveredItems, domain.ProbeCount))
	}
	b.WriteString("\n## What Needs Attention\n\n")
	if len(report.Risks) == 0 {
		b.WriteString("No material risks were inferred from the sampled enterprise surfaces.\n")
	} else {
		limit := len(report.Risks)
		if limit > 5 {
			limit = 5
		}
		for index := 0; index < limit; index++ {
			risk := report.Risks[index]
			b.WriteString(fmt.Sprintf("- **%s**: %s\n", strings.ToUpper(risk.Severity), risk.Title))
		}
	}
	b.WriteString("\n## Recommendation\n\n")
	b.WriteString("Use the architect-oriented Markdown view when you need probe-by-probe evidence. Use this executive view for steering, prioritization, and status reporting.\n")
	return b.String()
}

func renderEvaluateContentDetailSection(results []evaluateProbeResult) string {
	var b strings.Builder
	dashboardDescribe := findEvaluateProbesByPrefix(results, "dashboard_describe_")
	dashboardHistory := findEvaluateProbesByPrefix(results, "dashboard_history_")
	notebookDescribe := findEvaluateProbesByPrefix(results, "notebook_describe_")
	notebookHistory := findEvaluateProbesByPrefix(results, "notebook_history_")

	if len(dashboardDescribe) == 0 && len(notebookDescribe) == 0 {
		return "No dashboard or notebook deep samples were collected.\n"
	}

	for _, probe := range dashboardDescribe {
		b.WriteString("### Dashboard Sample\n\n")
		b.WriteString(renderOwnershipSnippet(probe.RawOutput))
		if history := matchingDeepHistory(dashboardHistory, probe.Name, "dashboard_describe_"); history != nil {
			b.WriteString(fmt.Sprintf("- History signal: %s\n", escapeEvaluateMarkdown(firstNonEmpty(history.ErrorMessage, history.OutputPreview))))
		}
		b.WriteString("\n")
	}
	for _, probe := range notebookDescribe {
		b.WriteString("### Notebook Sample\n\n")
		b.WriteString(renderOwnershipSnippet(probe.RawOutput))
		if history := matchingDeepHistory(notebookHistory, probe.Name, "notebook_describe_"); history != nil {
			b.WriteString(fmt.Sprintf("- History signal: %s\n", escapeEvaluateMarkdown(firstNonEmpty(history.ErrorMessage, history.OutputPreview))))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func renderEvaluateConfigurationSection(results []evaluateProbeResult) string {
	var b strings.Builder
	settingsObjects := findEvaluateProbe(results, "settings_objects_inventory")
	settingsSchemas := findEvaluateProbesByPrefix(results, "settings_schema_describe_")
	extensionConfigs := findEvaluateProbesByPrefix(results, "extension_configs_")

	if settingsObjects != nil {
		b.WriteString("### Settings Objects\n\n")
		b.WriteString(renderSettingsInventorySnippet(settingsObjects.RawOutput))
		b.WriteString("\n")
	}
	for _, probe := range settingsSchemas {
		b.WriteString("### Settings Schema Sample\n\n")
		b.WriteString(renderSettingsSchemaSnippet(probe.RawOutput))
		b.WriteString("\n")
	}
	for _, probe := range extensionConfigs {
		b.WriteString("### Extension Config Sample\n\n")
		b.WriteString(renderExtensionConfigSnippet(probe.RawOutput))
		b.WriteString("\n")
	}
	if b.Len() == 0 {
		return "No settings or extension configuration samples were collected.\n"
	}
	return b.String()
}

func sanitizeEvaluateName(value string) string {
	value = strings.ToLower(value)
	replacer := strings.NewReplacer("-", "_", " ", "_", "/", "_", ":", "_", ".", "_")
	return replacer.Replace(value)
}

func redactEvaluateSensitive(input string) string {
	output := strings.ReplaceAll(input, "Api-Token ", "Api-Token [REDACTED] ")
	output = strings.ReplaceAll(output, `Authorization": "Api-Token `, `Authorization": "Api-Token [REDACTED] `)
	output = evaluateTokenPattern.ReplaceAllString(output, "[REDACTED_TOKEN]")
	return output
}

func countTextLines(input string) int {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return 0
	}
	return len(strings.Split(trimmed, "\n"))
}

func uniqueEvaluateStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func limitStrings(values []string, limit int) []string {
	if limit < 1 || len(values) <= limit {
		return values
	}
	return values[:limit]
}

func stringifyEvaluateValue(value interface{}) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprintf("%v", typed)
	}
}

func evaluateSeverityRank(severity string) int {
	switch severity {
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func titleEvaluateDomain(domain string) string {
	parts := strings.Split(domain, "_")
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func escapeEvaluateMarkdown(input string) string {
	input = strings.ReplaceAll(input, "|", "\\|")
	input = strings.ReplaceAll(input, "\n", " ")
	return input
}

func findEvaluateProbe(results []evaluateProbeResult, name string) *evaluateProbeResult {
	for index := range results {
		if results[index].Name == name {
			return &results[index]
		}
	}
	return nil
}

func findEvaluateProbesByPrefix(results []evaluateProbeResult, prefix string) []evaluateProbeResult {
	matches := make([]evaluateProbeResult, 0)
	for _, result := range results {
		if strings.HasPrefix(result.Name, prefix) {
			matches = append(matches, result)
		}
	}
	return matches
}

func matchingDeepHistory(results []evaluateProbeResult, describeName string, describePrefix string) *evaluateProbeResult {
	suffix := strings.TrimPrefix(describeName, describePrefix)
	for index := range results {
		if strings.HasSuffix(results[index].Name, suffix) {
			return &results[index]
		}
	}
	return nil
}

func renderOwnershipSnippet(raw string) string {
	parsed := parseEvaluateJSONObject(raw)
	if parsed == nil {
		return fmt.Sprintf("- Sample output: %s\n", escapeEvaluateMarkdown(truncateEvaluateOutput(raw, 220)))
	}
	name := firstNonEmpty(stringifyEvaluateValue(parsed["name"]), stringifyEvaluateValue(parsed["id"]))
	owner := stringifyEvaluateValue(parsed["owner"])
	privateFlag := stringifyEvaluateValue(parsed["isPrivate"])
	access := renderEvaluateList(parsed["access"])
	created := renderNestedString(parsed, "modificationInfo", "createdTime")
	modified := renderNestedString(parsed, "modificationInfo", "lastModifiedTime")
	modifiedBy := renderNestedString(parsed, "modificationInfo", "lastModifiedBy")

	var b strings.Builder
	b.WriteString(fmt.Sprintf("- Name: %s\n", escapeEvaluateMarkdown(name)))
	b.WriteString(fmt.Sprintf("- Owner: %s\n", escapeEvaluateMarkdown(owner)))
	b.WriteString(fmt.Sprintf("- Private: %s\n", escapeEvaluateMarkdown(privateFlag)))
	if access != "" {
		b.WriteString(fmt.Sprintf("- Access: %s\n", escapeEvaluateMarkdown(access)))
	}
	if created != "" {
		b.WriteString(fmt.Sprintf("- Created: %s\n", escapeEvaluateMarkdown(created)))
	}
	if modified != "" {
		b.WriteString(fmt.Sprintf("- Last modified: %s\n", escapeEvaluateMarkdown(modified)))
	}
	if modifiedBy != "" {
		b.WriteString(fmt.Sprintf("- Last modified by: %s\n", escapeEvaluateMarkdown(modifiedBy)))
	}
	return b.String()
}

func renderSettingsInventorySnippet(raw string) string {
	objects := parseEvaluateJSONArray(raw)
	if len(objects) == 0 {
		return fmt.Sprintf("- Sample output: %s\n", escapeEvaluateMarkdown(truncateEvaluateOutput(raw, 220)))
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("- Returned settings objects: %d\n", len(objects)))
	for index, obj := range objects {
		if index >= 3 {
			break
		}
		summary := stringifyEvaluateValue(obj["summary"])
		schemaID := stringifyEvaluateValue(obj["schemaId"])
		scope := stringifyEvaluateValue(obj["scope"])
		b.WriteString(fmt.Sprintf("- %s | schema `%s` | scope `%s`\n", escapeEvaluateMarkdown(summary), escapeEvaluateMarkdown(schemaID), escapeEvaluateMarkdown(scope)))
	}
	return b.String()
}

func renderSettingsSchemaSnippet(raw string) string {
	parsed := parseEvaluateJSONObject(raw)
	if parsed == nil {
		return fmt.Sprintf("- Sample output: %s\n", escapeEvaluateMarkdown(truncateEvaluateOutput(raw, 220)))
	}
	name := stringifyEvaluateValue(parsed["displayName"])
	description := stringifyEvaluateValue(parsed["description"])
	scopes := renderEvaluateList(parsed["allowedScopes"])
	var b strings.Builder
	b.WriteString(fmt.Sprintf("- Schema: %s\n", escapeEvaluateMarkdown(name)))
	if scopes != "" {
		b.WriteString(fmt.Sprintf("- Allowed scopes: %s\n", escapeEvaluateMarkdown(scopes)))
	}
	if description != "" {
		b.WriteString(fmt.Sprintf("- Description: %s\n", escapeEvaluateMarkdown(truncateEvaluateOutput(description, 200))))
	}
	return b.String()
}

func renderExtensionConfigSnippet(raw string) string {
	objects := parseEvaluateJSONArray(raw)
	if len(objects) == 0 {
		return "- No extension configuration objects were returned for the sampled extension.\n"
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("- Returned extension configs: %d\n", len(objects)))
	for index, obj := range objects {
		if index >= 3 {
			break
		}
		configID := firstNonEmpty(stringifyEvaluateValue(obj["id"]), stringifyEvaluateValue(obj["objectId"]), stringifyEvaluateValue(obj["configId"]))
		version := stringifyEvaluateValue(obj["version"])
		summary := firstNonEmpty(stringifyEvaluateValue(obj["name"]), stringifyEvaluateValue(obj["summary"]), configID)
		b.WriteString(fmt.Sprintf("- %s | version `%s` | id `%s`\n", escapeEvaluateMarkdown(summary), escapeEvaluateMarkdown(version), escapeEvaluateMarkdown(configID)))
	}
	return b.String()
}

func parseEvaluateJSONObject(raw string) map[string]interface{} {
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &parsed); err != nil {
		return nil
	}
	return parsed
}

func parseEvaluateJSONArray(raw string) []map[string]interface{} {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	var arrayParsed []map[string]interface{}
	if err := json.Unmarshal([]byte(trimmed), &arrayParsed); err == nil {
		return arrayParsed
	}
	var generic []interface{}
	if err := json.Unmarshal([]byte(trimmed), &generic); err == nil {
		return extractEvaluateObjectSlice(generic)
	}
	return nil
}

func renderNestedString(root map[string]interface{}, parent string, child string) string {
	parentValue, ok := root[parent].(map[string]interface{})
	if !ok {
		return ""
	}
	return stringifyEvaluateValue(parentValue[child])
}

func renderEvaluateList(value interface{}) string {
	list, ok := value.([]interface{})
	if !ok {
		return ""
	}
	parts := make([]string, 0, len(list))
	for _, item := range list {
		parts = append(parts, stringifyEvaluateValue(item))
	}
	return strings.Join(parts, ", ")
}
	func extractEvaluateSettingSchemaIds(raw string) []string {
		objects := parseEvaluateJSONArray(raw)
		if len(objects) == 0 {
			return nil
		}

		seen := map[string]struct{}{}
		result := make([]string, 0)

		for _, obj := range objects {
			schemaId := stringifyEvaluateValue(obj["schemaId"])
			if schemaId != "" && schemaId != "nil" {
				if _, exists := seen[schemaId]; !exists {
					seen[schemaId] = struct{}{}
					result = append(result, schemaId)
				}
			}
		}
		return result
	}
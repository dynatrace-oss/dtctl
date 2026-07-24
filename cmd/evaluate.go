package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/commands"
	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/dynatrace-oss/dtctl/pkg/output"
)

var (
	evaluateOutPath      string
	evaluateFormat       string
	evaluateToken        string
	evaluateTokenEnv     string
	evaluateTokenStdin   bool
	evaluateTokenRef     string
	evaluateDescription  string
	evaluateEnvironment  string
	evaluateProbeTimeout time.Duration
	evaluateSampleLimit  int
	evaluateAudience     string
	evaluateIncludeCatalog bool
)

type evaluateTool struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type evaluateContext struct {
	Name        string `json:"name"`
	Environment string `json:"environment,omitempty"`
	SafetyLevel string `json:"safety_level,omitempty"`
	Description string `json:"description,omitempty"`
}

type evaluateReport struct {
	SchemaVersion int                    `json:"schema_version"`
	GeneratedAt   time.Time              `json:"generated_at"`
	Tool          evaluateTool           `json:"tool"`
	Target        evaluateContext        `json:"target"`
	Contexts      []evaluateContext      `json:"contexts,omitempty"`
	Catalog       interface{}            `json:"command_catalog,omitempty"`
	Assessment    evaluateAssessment     `json:"assessment"`
	Domains       []evaluateDomainSummary `json:"domains,omitempty"`
	Risks         []evaluateRisk         `json:"risks,omitempty"`
	Probes        []evaluateProbeResult  `json:"probes,omitempty"`
	Summary       map[string]any         `json:"summary,omitempty"`
}

type evaluateProbe struct {
	Name        string
	Description string
	Domain      string
	Kind        string
	Command     []string
	Query       string
	DeepCheck   bool
}

type evaluateProbeResult struct {
	Name          string   `json:"name"`
	Description   string   `json:"description,omitempty"`
	Domain        string   `json:"domain,omitempty"`
	Kind          string   `json:"kind,omitempty"`
	Command       []string `json:"command"`
	Status        string   `json:"status"`
	DurationMS    int64    `json:"duration_ms"`
	ItemCount     int      `json:"item_count,omitempty"`
	DeepCheck     bool     `json:"deep_check,omitempty"`
	ErrorMessage  string   `json:"error_message,omitempty"`
	OutputPreview string   `json:"output_preview,omitempty"`
	SampleRefs    []string `json:"sample_refs,omitempty"`
	RawOutput     string   `json:"-"`
}

var evaluateCmd = &cobra.Command{
	Use:   "evaluate",
	Short: "Generate tenant evaluation snapshots",
	Long: `Generate a snapshot of the current tenant's command coverage and configuration.

Use 'dtctl evaluate init' to create a readonly evaluation context, then
use 'dtctl evaluate tenant' to capture the snapshot output.`,
	RunE:  requireSubcommand,
}

var evaluateInitCmd = &cobra.Command{
	Use:   "init <context-name>",
	Short: "Create a readonly evaluation context and store its token",
	Args:  cobra.ExactArgs(1),
	RunE:  runEvaluateInit,
}

var evaluateTenantCmd = &cobra.Command{
	Use:   "tenant",
	Short: "Generate a tenant evaluation snapshot",
	RunE:  runEvaluateTenant,
}

func init() {
	evaluateInitCmd.Flags().StringVar(&evaluateEnvironment, "environment", "", "Dynatrace environment URL")
	evaluateInitCmd.Flags().StringVar(&evaluateDescription, "description", "Readonly tenant evaluation context", "context description")
	evaluateInitCmd.Flags().StringVar(&evaluateTokenRef, "token-ref", "", "token reference name")
	evaluateInitCmd.Flags().StringVar(&evaluateToken, "token", "", "API token value")
	evaluateInitCmd.Flags().StringVar(&evaluateTokenEnv, "token-env", "", "environment variable containing the API token")
	evaluateInitCmd.Flags().BoolVar(&evaluateTokenStdin, "token-stdin", false, "read the API token from stdin")

	evaluateTenantCmd.Flags().StringVar(&evaluateOutPath, "out", "", "write output to a file")
	evaluateTenantCmd.Flags().StringVar(&evaluateFormat, "format", "json", "output format: json or markdown")
	evaluateTenantCmd.Flags().DurationVar(&evaluateProbeTimeout, "probe-timeout", 20*time.Second, "timeout per probe command")
	evaluateTenantCmd.Flags().IntVar(&evaluateSampleLimit, "sample-limit", 1, "number of representative items to inspect with deeper follow-up probes")
	evaluateTenantCmd.Flags().StringVar(&evaluateAudience, "audience", "architect", "markdown audience: architect or executive")
	evaluateTenantCmd.Flags().BoolVar(&evaluateIncludeCatalog, "include-command-catalog", true, "include the dtctl command catalog in the report")

	evaluateCmd.AddCommand(evaluateInitCmd)
	evaluateCmd.AddCommand(evaluateTenantCmd)
	rootCmd.AddCommand(evaluateCmd)
}

func runEvaluateInit(cmd *cobra.Command, args []string) error {
	contextName := args[0]
	if strings.TrimSpace(evaluateEnvironment) == "" {
		return fmt.Errorf("--environment is required")
	}

	token, err := resolveEvaluateToken()
	if err != nil {
		return err
	}

	cfg, err := loadConfigRaw()
	if err != nil {
		cfg = config.NewConfig()
	}

	if evaluateTokenRef == "" {
		evaluateTokenRef = contextName + "-token"
	}

	upsertToken(cfg, evaluateTokenRef, token)
	upsertContext(cfg, contextName, evaluateEnvironment, evaluateTokenRef, evaluateDescription)
	cfg.CurrentContext = contextName

	if err := saveConfig(cfg); err != nil {
		return err
	}

	output.PrintSuccess("Evaluation context %q configured", contextName)
	return nil
}

func runEvaluateTenant(cmd *cobra.Command, args []string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}

	ctx, err := cfg.CurrentContextObj()
	if err != nil {
		return err
	}

	report := evaluateReport{
		SchemaVersion: 1,
		GeneratedAt:   time.Now().UTC(),
		Tool:          evaluateTool{Name: "dtctl", Version: "local"},
		Target: evaluateContext{
			Name:        cfg.CurrentContext,
			Environment: ctx.Environment,
			SafetyLevel: ctx.SafetyLevel.String(),
			Description: ctx.Description,
		},
		Contexts: collectEvaluateContexts(cfg),
	}
	if evaluateIncludeCatalog {
		report.Catalog = commands.Build(rootCmd)
	}

	baseResults := runEvaluateProbes(cfg.CurrentContext, defaultEvaluateProbes(), evaluateProbeTimeout)
	deepResults := runEvaluateProbes(cfg.CurrentContext, deriveDeepEvaluateProbes(baseResults, evaluateSampleLimit), evaluateProbeTimeout)
	report.Probes = append(baseResults, deepResults...)
	report.Domains = summarizeEvaluateDomains(report.Probes)
	report.Assessment = scoreEvaluateAssessment(report.Probes, report.Domains)
	report.Risks = buildEvaluateRisks(report.Probes, report.Domains)
	report.Summary = buildEvaluateSummary(len(cfg.Contexts), report.Probes, report.Assessment)

	format := strings.ToLower(strings.TrimSpace(evaluateFormat))
	if format == "" {
		format = "json"
	}

	var data []byte
	switch format {
	case "json":
		data, err = json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		data = append(data, '\n')
	case "markdown", "md":
		data = []byte(renderEvaluateMarkdown(report))
	default:
		return fmt.Errorf("unsupported format %q", evaluateFormat)
	}

	if evaluateOutPath != "" {
		return os.WriteFile(evaluateOutPath, data, 0600)
	}
	_, err = os.Stdout.Write(data)
	return err
}

func resolveEvaluateToken() (string, error) {
	sources := 0
	if strings.TrimSpace(evaluateToken) != "" {
		sources++
	}
	if strings.TrimSpace(evaluateTokenEnv) != "" {
		sources++
	}
	if evaluateTokenStdin {
		sources++
	}
	if sources == 0 {
		return "", nil
	}
	if sources > 1 {
		return "", fmt.Errorf("use only one of --token, --token-env, or --token-stdin")
	}
	if evaluateToken != "" {
		return evaluateToken, nil
	}
	if evaluateTokenEnv != "" {
		value := strings.TrimSpace(os.Getenv(evaluateTokenEnv))
		if value == "" {
			return "", fmt.Errorf("environment variable %q is empty or unset", evaluateTokenEnv)
		}
		return value, nil
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return "", fmt.Errorf("token read from stdin was empty")
	}
	return value, nil
}

func upsertToken(cfg *config.Config, name, token string) {
	for i := range cfg.Tokens {
		if cfg.Tokens[i].Name == name {
			cfg.Tokens[i].Token = token
			return
		}
	}
	cfg.Tokens = append(cfg.Tokens, config.NamedToken{Name: name, Token: token})
}

func upsertContext(cfg *config.Config, name, env, tokenRef, description string) {
	for i := range cfg.Contexts {
		if cfg.Contexts[i].Name == name {
			cfg.Contexts[i].Context.Environment = env
			cfg.Contexts[i].Context.TokenRef = tokenRef
			cfg.Contexts[i].Context.SafetyLevel = config.SafetyLevelReadOnly
			cfg.Contexts[i].Context.Description = description
			return
		}
	}
	cfg.Contexts = append(cfg.Contexts, config.NamedContext{
		Name: name,
		Context: config.Context{
			Environment: env,
			TokenRef:    tokenRef,
			SafetyLevel: config.SafetyLevelReadOnly,
			Description: description,
		},
	})
}

func collectEvaluateContexts(cfg *config.Config) []evaluateContext {
	contexts := make([]evaluateContext, 0, len(cfg.Contexts))
	for _, namedCtx := range cfg.Contexts {
		contexts = append(contexts, evaluateContext{
			Name:        namedCtx.Name,
			Environment: namedCtx.Context.Environment,
			SafetyLevel: namedCtx.Context.SafetyLevel.String(),
			Description: namedCtx.Context.Description,
		})
	}
	sort.Slice(contexts, func(i, j int) bool { return contexts[i].Name < contexts[j].Name })
	return contexts
}

func renderEvaluateMarkdown(report evaluateReport) string {
	return renderEvaluateMarkdownWithAudience(report, evaluateAudience)
}

func defaultEvaluateProbes() []evaluateProbe {
	return []evaluateProbe{
		{Name: "workflows_inventory", Description: "List workflows", Domain: "automation", Kind: "command", Command: []string{"get", "workflows"}},
		{Name: "workflow_executions_inventory", Description: "List workflow executions", Domain: "automation", Kind: "command", Command: []string{"get", "workflow-executions"}},
		{Name: "dashboards_inventory", Description: "List dashboards", Domain: "content", Kind: "command", Command: []string{"get", "dashboards"}},
		{Name: "notebooks_inventory", Description: "List notebooks", Domain: "content", Kind: "command", Command: []string{"get", "notebooks"}},
		{Name: "documents_inventory", Description: "List documents", Domain: "content", Kind: "command", Command: []string{"get", "documents"}},
		{Name: "buckets_inventory", Description: "List buckets", Domain: "platform", Kind: "command", Command: []string{"get", "buckets"}},
		{Name: "segments_inventory", Description: "List segments", Domain: "platform", Kind: "command", Command: []string{"get", "segments"}},
		{Name: "extensions_inventory", Description: "List extensions", Domain: "extensions", Kind: "command", Command: []string{"get", "extensions"}},
		{Name: "settings_objects_inventory", Description: "List log pipeline settings objects", Domain: "configuration", Kind: "command", Command: []string{"get", "settings", "--schema", "builtin:openpipeline.logs.pipelines"}},
		{Name: "lookups_inventory", Description: "List lookup tables", Domain: "configuration", Kind: "command", Command: []string{"get", "lookups"}},
		{Name: "slos_inventory", Description: "List SLOs", Domain: "governance", Kind: "command", Command: []string{"get", "slos"}},
		{Name: "notifications_inventory", Description: "List notifications", Domain: "alerting", Kind: "command", Command: []string{"get", "notifications"}},
		{Name: "analyzers_inventory", Description: "List analyzers", Domain: "alerting", Kind: "command", Command: []string{"get", "analyzers"}},
		{Name: "anomaly_detectors_inventory", Description: "List anomaly detectors", Domain: "alerting", Kind: "command", Command: []string{"get", "anomaly-detectors"}},
		{Name: "aws_connections_inventory", Description: "List AWS connections", Domain: "cloud", Kind: "command", Command: []string{"get", "aws", "connections"}},
		{Name: "aws_monitoring_inventory", Description: "List AWS monitoring configs", Domain: "cloud", Kind: "command", Command: []string{"get", "aws", "monitoring"}},
		{Name: "azure_connections_inventory", Description: "List Azure connections", Domain: "cloud", Kind: "command", Command: []string{"get", "azure", "connections"}},
		{Name: "azure_monitoring_inventory", Description: "List Azure monitoring configs", Domain: "cloud", Kind: "command", Command: []string{"get", "azure", "monitoring"}},
		{Name: "gcp_connections_inventory", Description: "List GCP connections", Domain: "cloud", Kind: "command", Command: []string{"get", "gcp", "connections"}},
		{Name: "gcp_monitoring_inventory", Description: "List GCP monitoring configs", Domain: "cloud", Kind: "command", Command: []string{"get", "gcp", "monitoring"}},
		{Name: "host_groups_query", Description: "Discover host groups", Domain: "topology", Kind: "query", Query: "fetch dt.entity.host_group | limit 200"},
		{Name: "kubernetes_clusters_query", Description: "Discover Kubernetes clusters", Domain: "kubernetes", Kind: "query", Query: "fetch dt.entity.kubernetes_cluster | limit 200"},
		{Name: "cloud_namespaces_query", Description: "Discover cloud application namespaces", Domain: "kubernetes", Kind: "query", Query: "fetch dt.entity.cloud_application_namespace | limit 200"},
		{Name: "host_tags_query", Description: "Sample host tags", Domain: "governance", Kind: "query", Query: "fetch dt.entity.host | fields id, entity.name, tags | limit 50"},
		{Name: "management_zones_query", Description: "Sample management zones", Domain: "governance", Kind: "query", Query: "fetch dt.entity.host | fields id, entity.name, managementZones | limit 50"},
		{Name: "events_query", Description: "Sample active events", Domain: "alerting", Kind: "query", Query: "fetch events | limit 100"},
		{Name: "logs_query", Description: "Sample recent logs", Domain: "telemetry", Kind: "query", Query: "fetch logs | limit 100"},
		{Name: "spans_query", Description: "Sample recent spans", Domain: "telemetry", Kind: "query", Query: "fetch spans | limit 100"},
		{Name: "activegate_otel_query", Description: "Discover ActiveGate and OTel-related process groups", Domain: "deployment", Kind: "query", Query: "fetch dt.entity.process_group_instance | filter contains(lower(entity.name), \"activegate\") or contains(lower(entity.name), \"otel\") | limit 50"},
	}
}

func runEvaluateProbes(contextName string, probes []evaluateProbe, timeout time.Duration) []evaluateProbeResult {
	results := make([]evaluateProbeResult, 0, len(probes))
	if timeout <= 0 {
		timeout = 20 * time.Second
	}

	executable, err := os.Executable()
	if err != nil {
		for _, probe := range probes {
			results = append(results, evaluateProbeResult{
				Name:         probe.Name,
				Description:  probe.Description,
				Command:      probe.Command,
				Status:       "error",
				ErrorMessage: fmt.Sprintf("failed to resolve executable: %v", err),
			})
		}
		return results
	}

	for _, probe := range probes {
		startedAt := time.Now()
		result := evaluateProbeResult{
			Name:        probe.Name,
			Description: probe.Description,
			Domain:      probe.Domain,
			Kind:        probe.Kind,
			DeepCheck:   probe.DeepCheck,
			Command:     evaluateCommandArgs(probe),
		}

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		cmdArgs := append([]string{"--context", contextName, "--plain", "-o", "json"}, result.Command...)
		probeCmd := exec.CommandContext(ctx, executable, cmdArgs...)
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		probeCmd.Stdout = &stdout
		probeCmd.Stderr = &stderr
		runErr := probeCmd.Run()
		cancel()

		result.DurationMS = time.Since(startedAt).Milliseconds()
		stdoutText := strings.TrimSpace(stdout.String())
		stderrText := strings.TrimSpace(stderr.String())
		result.RawOutput = redactEvaluateSensitive(firstNonEmpty(stdoutText, stderrText))
		result.OutputPreview = truncateEvaluateOutput(result.RawOutput, 240)

		if runErr != nil {
			if strings.Contains(result.RawOutput, "[Preview]") {
				result.Status = "skipped"
				result.ErrorMessage = "preview command excluded from scoring"
				results = append(results, result)
				continue
			}
			if strings.Contains(runErr.Error(), "killed") {
				result.Status = "timeout"
				result.ErrorMessage = fmt.Sprintf("probe timed out after %s", timeout)
			} else {
				result.Status = "error"
				result.ErrorMessage = strings.TrimSpace(firstNonEmpty(stderrText, runErr.Error()))
			}
			results = append(results, result)
			continue
		}

		result.Status, result.ErrorMessage, result.ItemCount, result.SampleRefs = analyzeEvaluateOutput(result.RawOutput)
		results = append(results, result)
	}

	return results
}

func evaluateCommandArgs(probe evaluateProbe) []string {
	if probe.Kind == "query" {
		return []string{"query", probe.Query}
	}
	return append([]string{}, probe.Command...)
}

func countEvaluateItems(jsonOutput string) int {
	jsonOutput = strings.TrimSpace(jsonOutput)
	if jsonOutput == "" {
		return 0
	}

	var parsed interface{}
	if err := json.Unmarshal([]byte(jsonOutput), &parsed); err != nil {
		return 0
	}

	switch value := parsed.(type) {
	case []interface{}:
		return len(value)
	case map[string]interface{}:
		if records, ok := value["records"]; ok {
			if list, ok := records.([]interface{}); ok {
				return len(list)
			}
		}
		if result, ok := value["result"]; ok {
			switch resultTyped := result.(type) {
			case []interface{}:
				return len(resultTyped)
			case nil:
				return 0
			default:
				return 1
			}
		}
		return 1
	default:
		return 1
	}
}

func buildEvaluateSummary(contextCount int, probes []evaluateProbeResult, assessment evaluateAssessment) map[string]any {
	okCount := 0
	errorCount := 0
	timeoutCount := 0
	totalItems := 0
	deepChecks := 0

	for _, probe := range probes {
		totalItems += probe.ItemCount
		if probe.DeepCheck {
			deepChecks++
		}
		switch probe.Status {
		case "ok":
			okCount++
		case "timeout":
			timeoutCount++
		case "skipped", "restricted":
			// Informational probe outcomes are excluded from score penalties.
		default:
			errorCount++
		}
	}

	message := "tenant evaluation probes completed"
	if errorCount > 0 || timeoutCount > 0 {
		message = "tenant evaluation completed with probe issues"
	}

	return map[string]any{
		"message":          message,
		"contexts":         contextCount,
		"total_probes":     len(probes),
		"successful":       okCount,
		"failed":           errorCount,
		"timeouts":         timeoutCount,
		"deep_checks":      deepChecks,
		"discovered_items": totalItems,
		"score":            assessment.Score,
		"grade":            assessment.Grade,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func truncateEvaluateOutput(input string, maxLen int) string {
	input = strings.TrimSpace(input)
	if input == "" || maxLen < 1 {
		return ""
	}
	if len(input) <= maxLen {
		return input
	}
	return input[:maxLen-3] + "..."
}

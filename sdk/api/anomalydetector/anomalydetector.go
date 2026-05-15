// Package anomalydetector provides a handler for custom anomaly detectors
// (Settings schema: builtin:davis.anomaly-detectors).
package anomalydetector

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

const (
	SchemaID    = "builtin:davis.anomaly-detectors"
	Scope       = "environment"
	SettingsAPI = "/platform/classic/environment-api/v2/settings/objects"
)

// Handler handles anomaly detector resources.
type Handler struct {
	client *httpclient.Client
}

// NewHandler creates a new anomaly detector handler.
func NewHandler(c *httpclient.Client) *Handler {
	return &Handler{client: c}
}

// AnomalyDetector represents a custom anomaly detector (builtin:davis.anomaly-detectors).
type AnomalyDetector struct {
	ObjectID string `json:"-" table:"OBJECT ID,wide"`

	Title         string `json:"-" table:"TITLE"`
	Enabled       bool   `json:"-" table:"ENABLED"`
	AnalyzerShort string `json:"-" table:"ANALYZER"`
	EventType     string `json:"-" table:"EVENT TYPE"`
	Source        string `json:"-" table:"SOURCE"`
	Description   string `json:"-" table:"DESCRIPTION,wide"`

	Value map[string]any `json:"-" table:"-"`

	SchemaVersion string `json:"-" table:"-"`
}

type rawSettingsEnvelope struct {
	SchemaID      string         `json:"schemaId" yaml:"schemaId"`
	SchemaVersion string         `json:"schemaVersion,omitempty" yaml:"schemaVersion,omitempty"`
	Scope         string         `json:"scope" yaml:"scope"`
	ObjectID      string         `json:"objectId,omitempty" yaml:"objectId,omitempty"`
	Value         map[string]any `json:"value" yaml:"value"`
}

func (a AnomalyDetector) envelope() rawSettingsEnvelope {
	return rawSettingsEnvelope{
		SchemaID:      SchemaID,
		SchemaVersion: a.SchemaVersion,
		Scope:         Scope,
		ObjectID:      a.ObjectID,
		Value:         a.Value,
	}
}

func (a AnomalyDetector) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.envelope())
}

func (a AnomalyDetector) MarshalYAML() (interface{}, error) {
	return a.envelope(), nil
}

// ListOptions configures listing behavior.
type ListOptions struct {
	Enabled *bool
}

type listResponse struct {
	Items       []settingsItem `json:"items"`
	TotalCount  int            `json:"totalCount"`
	NextPageKey string         `json:"nextPageKey,omitempty"`
}

type settingsItem struct {
	ObjectID      string         `json:"objectId"`
	SchemaID      string         `json:"schemaId"`
	SchemaVersion string         `json:"schemaVersion"`
	Scope         string         `json:"scope"`
	Value         map[string]any `json:"value"`
}

type createResponse struct {
	ObjectID string `json:"objectId"`
	Code     int    `json:"code,omitempty"`
	Error    *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func flatten(item settingsItem) AnomalyDetector {
	ad := AnomalyDetector{
		ObjectID:      item.ObjectID,
		Value:         item.Value,
		SchemaVersion: item.SchemaVersion,
	}

	if v, ok := item.Value["title"].(string); ok {
		ad.Title = v
	}
	if v, ok := item.Value["enabled"].(bool); ok {
		ad.Enabled = v
	}
	if v, ok := item.Value["description"].(string); ok {
		ad.Description = v
	}
	if v, ok := item.Value["source"].(string); ok {
		ad.Source = v
	}

	ad.AnalyzerShort = deriveAnalyzerShort(item.Value)
	ad.EventType = deriveEventType(item.Value)

	return ad
}

func deriveAnalyzerShort(value map[string]any) string {
	analyzer, ok := value["analyzer"].(map[string]any)
	if !ok {
		return ""
	}

	name, _ := analyzer["name"].(string)
	input := ExtractKVMap(analyzer, "input")

	switch {
	case strings.Contains(name, "StaticThreshold"):
		condition := input["alertCondition"]
		threshold := input["threshold"]
		op := ">"
		if condition == "BELOW" {
			op = "<"
		}
		if threshold != "" {
			return fmt.Sprintf("static (%s%s)", op, threshold)
		}
		return "static"
	case strings.Contains(name, "AutoAdaptive"):
		return "auto-adaptive"
	default:
		parts := strings.Split(name, ".")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
		return name
	}
}

func deriveEventType(value map[string]any) string {
	et, ok := value["eventTemplate"].(map[string]any)
	if !ok {
		return ""
	}

	props := extractKVSlice(et, "properties")
	for _, prop := range props {
		if prop["key"] == "event.type" {
			if v, ok := prop["value"].(string); ok {
				return v
			}
		}
	}
	return ""
}

// ExtractEventName extracts the event.name from eventTemplate properties.
func ExtractEventName(value map[string]any) string {
	et, ok := value["eventTemplate"].(map[string]any)
	if !ok {
		return ""
	}

	props := extractKVSlice(et, "properties")
	for _, prop := range props {
		if prop["key"] == "event.name" {
			if v, ok := prop["value"].(string); ok {
				return v
			}
		}
	}
	return ""
}

// ExtractKVMap reads a field stored as [{key, value}] and returns it as a simple map.
func ExtractKVMap(parent map[string]any, field string) map[string]string {
	result := make(map[string]string)

	raw, ok := parent[field]
	if !ok {
		return result
	}

	if m, ok := raw.(map[string]any); ok {
		for k, v := range m {
			result[k] = fmt.Sprintf("%v", v)
		}
		return result
	}

	if arr, ok := raw.([]any); ok {
		for _, item := range arr {
			if obj, ok := item.(map[string]any); ok {
				k, _ := obj["key"].(string)
				v := fmt.Sprintf("%v", obj["value"])
				if k != "" {
					result[k] = v
				}
			}
		}
	}

	return result
}

func extractKVSlice(parent map[string]any, field string) []map[string]any {
	raw, ok := parent[field]
	if !ok {
		return nil
	}

	arr, ok := raw.([]any)
	if !ok {
		return nil
	}

	var result []map[string]any
	for _, item := range arr {
		if obj, ok := item.(map[string]any); ok {
			result = append(result, obj)
		}
	}
	return result
}

// List returns all custom anomaly detectors, optionally filtered.
func (h *Handler) List(opts ListOptions) ([]AnomalyDetector, error) {
	var allItems []settingsItem
	nextPageKey := ""

	for {
		req := h.client.HTTP().R()

		params := httpclient.PaginationParams{
			Style:         httpclient.PaginationSettingsAPI,
			PageKeyParam:  "nextPageKey",
			PageSizeParam: "pageSize",
			NextPageKey:   nextPageKey,
			PageSize:      500,
			Filters:       map[string]string{"schemaIds": SchemaID, "scopes": Scope},
		}.QueryParams()

		req.SetQueryParamsFromValues(params)

		var result listResponse
		req.SetResult(&result)

		resp, err := req.Get(SettingsAPI)
		if err != nil {
			return nil, fmt.Errorf("list anomaly detectors: %w", err)
		}
		if err := httpclient.CheckResponse(resp); err != nil {
			return nil, fmt.Errorf("list anomaly detectors: %w", err)
		}

		allItems = append(allItems, result.Items...)

		if result.NextPageKey == "" {
			break
		}
		nextPageKey = result.NextPageKey
	}

	var detectors []AnomalyDetector
	for _, item := range allItems {
		ad := flatten(item)

		if opts.Enabled != nil && ad.Enabled != *opts.Enabled {
			continue
		}

		detectors = append(detectors, ad)
	}

	sort.Slice(detectors, func(i, j int) bool {
		return detectors[i].Title < detectors[j].Title
	})

	return detectors, nil
}

// Get retrieves a single anomaly detector by object ID.
func (h *Handler) Get(objectID string) (*AnomalyDetector, error) {
	var raw settingsItem
	req := h.client.HTTP().R().SetResult(&raw)

	resp, err := req.Get(fmt.Sprintf("%s/%s", SettingsAPI, objectID))
	if err != nil {
		return nil, fmt.Errorf("get anomaly detector %q: %w", objectID, err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("get anomaly detector %q: %w", objectID, err)
	}

	ad := flatten(raw)
	return &ad, nil
}

// GetRaw returns the raw settings value as JSON bytes, suitable for editing.
func (h *Handler) GetRaw(objectID string) ([]byte, error) {
	ad, err := h.Get(objectID)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(ad.Value, "", "  ")
}

// FindByName searches for an anomaly detector by title (case-insensitive prefix match).
func (h *Handler) FindByName(name string) (*AnomalyDetector, error) {
	detectors, err := h.List(ListOptions{})
	if err != nil {
		return nil, err
	}

	nameLower := strings.ToLower(name)

	for i := range detectors {
		if strings.ToLower(detectors[i].Title) == nameLower {
			return &detectors[i], nil
		}
	}

	for i := range detectors {
		if strings.HasPrefix(strings.ToLower(detectors[i].Title), nameLower) {
			return &detectors[i], nil
		}
	}

	return nil, fmt.Errorf("anomaly detector with title %q not found", name)
}

// FindByExactTitle searches for an anomaly detector by exact title (case-insensitive).
func (h *Handler) FindByExactTitle(title string) (*AnomalyDetector, error) {
	detectors, err := h.List(ListOptions{})
	if err != nil {
		return nil, err
	}

	titleLower := strings.ToLower(title)
	for i := range detectors {
		if strings.ToLower(detectors[i].Title) == titleLower {
			return &detectors[i], nil
		}
	}

	return nil, nil
}

// ExtractTitle extracts the title from JSON data in either flattened or raw Settings format.
func ExtractTitle(data []byte) string {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return ""
	}

	if t, ok := raw["title"].(string); ok {
		return t
	}

	if v, ok := raw["value"].(map[string]any); ok {
		if t, ok := v["title"].(string); ok {
			return t
		}
	}

	return ""
}

// Create creates a new anomaly detector from JSON data.
func (h *Handler) Create(data []byte) (*AnomalyDetector, error) {
	apiBody, err := toAPIFormat(data)
	if err != nil {
		return nil, fmt.Errorf("invalid anomaly detector definition: %w", err)
	}

	body := []map[string]any{apiBody}

	resp, err := h.client.HTTP().R().SetBody(body).Post(SettingsAPI)
	if err != nil {
		return nil, fmt.Errorf("create anomaly detector: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("create anomaly detector: %w", err)
	}

	var createResp []createResponse
	if err := json.Unmarshal(resp.Body(), &createResp); err != nil {
		return nil, fmt.Errorf("parse create response: %w", err)
	}
	if len(createResp) == 0 {
		return nil, fmt.Errorf("no items returned in create response")
	}
	if createResp[0].Error != nil {
		return nil, fmt.Errorf("create failed: %s", createResp[0].Error.Message)
	}

	return h.Get(createResp[0].ObjectID)
}

// Update updates an existing anomaly detector.
func (h *Handler) Update(objectID string, data []byte) (*AnomalyDetector, error) {
	existing, err := h.Get(objectID)
	if err != nil {
		return nil, err
	}

	value, err := toAPIValue(data)
	if err != nil {
		return nil, fmt.Errorf("invalid anomaly detector definition: %w", err)
	}

	body := map[string]any{"value": value}

	resp, err := h.client.HTTP().R().
		SetBody(body).
		SetHeader("If-Match", existing.SchemaVersion).
		Put(fmt.Sprintf("%s/%s", SettingsAPI, objectID))
	if err != nil {
		return nil, fmt.Errorf("update anomaly detector %q: %w", objectID, err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("update anomaly detector %q: %w", objectID, err)
	}

	return h.Get(objectID)
}

// Delete deletes an anomaly detector by object ID.
func (h *Handler) Delete(objectID string) error {
	resp, err := h.client.HTTP().R().Delete(fmt.Sprintf("%s/%s", SettingsAPI, objectID))
	if err != nil {
		return fmt.Errorf("delete anomaly detector %q: %w", objectID, err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return fmt.Errorf("delete anomaly detector %q: %w", objectID, err)
	}
	return nil
}

func toAPIFormat(data []byte) (map[string]any, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	if schema, ok := raw["schemaId"].(string); ok && schema == SchemaID {
		if _, ok := raw["scope"]; !ok {
			raw["scope"] = Scope
		}
		return raw, nil
	}

	value, err := flattenedToAPIValue(raw)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"schemaId": SchemaID,
		"scope":    Scope,
		"value":    value,
	}, nil
}

func toAPIValue(data []byte) (map[string]any, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	if _, ok := raw["schemaId"]; ok {
		if v, ok := raw["value"].(map[string]any); ok {
			return v, nil
		}
		return nil, fmt.Errorf("raw Settings format missing 'value' field")
	}

	return flattenedToAPIValue(raw)
}

func flattenedToAPIValue(raw map[string]any) (map[string]any, error) {
	value := make(map[string]any)

	title, _ := raw["title"].(string)
	if title == "" {
		return nil, fmt.Errorf("'title' is required")
	}
	value["title"] = title

	if v, ok := raw["enabled"]; ok {
		value["enabled"] = v
	} else {
		value["enabled"] = true
	}

	if v, ok := raw["description"].(string); ok {
		value["description"] = v
	}

	source, _ := raw["source"].(string)
	if source == "" {
		source = "dtctl"
	}
	value["source"] = source

	analyzer, ok := raw["analyzer"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("'analyzer' is required and must be an object")
	}

	apiAnalyzer := make(map[string]any)
	if name, ok := analyzer["name"].(string); ok {
		apiAnalyzer["name"] = name
	} else {
		return nil, fmt.Errorf("'analyzer.name' is required")
	}

	switch input := analyzer["input"].(type) {
	case map[string]any:
		apiAnalyzer["input"] = mapToKVArray(input)
	case map[string]string:
		m := make(map[string]any, len(input))
		for k, v := range input {
			m[k] = v
		}
		apiAnalyzer["input"] = mapToKVArray(m)
	case []any:
		apiAnalyzer["input"] = input
	default:
		apiAnalyzer["input"] = []map[string]any{}
	}

	value["analyzer"] = apiAnalyzer

	switch et := raw["eventTemplate"].(type) {
	case map[string]any:
		value["eventTemplate"] = map[string]any{
			"properties": mapToKVArray(et),
		}
	case map[string]string:
		m := make(map[string]any, len(et))
		for k, v := range et {
			m[k] = v
		}
		value["eventTemplate"] = map[string]any{
			"properties": mapToKVArray(m),
		}
	}

	if es, ok := raw["executionSettings"]; ok {
		value["executionSettings"] = es
	}

	return value, nil
}

func mapToKVArray(m map[string]any) []map[string]any {
	var result []map[string]any
	for k, v := range m {
		result = append(result, map[string]any{
			"key":   k,
			"value": fmt.Sprintf("%v", v),
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i]["key"].(string) < result[j]["key"].(string)
	})
	return result
}

// ToFlattenedYAML converts an AnomalyDetector's Value into the human-friendly flattened format.
func ToFlattenedYAML(value map[string]any) map[string]any {
	flat := make(map[string]any)

	if v, ok := value["title"]; ok {
		flat["title"] = v
	}
	if v, ok := value["description"]; ok {
		flat["description"] = v
	}
	if v, ok := value["enabled"]; ok {
		flat["enabled"] = v
	}
	if v, ok := value["source"]; ok {
		flat["source"] = v
	}

	if analyzer, ok := value["analyzer"].(map[string]any); ok {
		flatAnalyzer := make(map[string]any)
		if name, ok := analyzer["name"]; ok {
			flatAnalyzer["name"] = name
		}
		flatAnalyzer["input"] = ExtractKVMap(analyzer, "input")
		flat["analyzer"] = flatAnalyzer
	}

	if et, ok := value["eventTemplate"].(map[string]any); ok {
		flat["eventTemplate"] = ExtractKVMap(et, "properties")
	}

	if es, ok := value["executionSettings"]; ok {
		flat["executionSettings"] = es
	}

	return flat
}

// IsRawSettingsFormat checks if JSON data is in raw Settings API format.
func IsRawSettingsFormat(data []byte) bool {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return false
	}
	schema, ok := raw["schemaId"].(string)
	return ok && schema == SchemaID
}

// IsFlattenedFormat checks if JSON data is in the flattened anomaly detector format.
func IsFlattenedFormat(data []byte) bool {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return false
	}
	_, hasAnalyzer := raw["analyzer"]
	_, hasEventTemplate := raw["eventTemplate"]
	return hasAnalyzer && hasEventTemplate
}

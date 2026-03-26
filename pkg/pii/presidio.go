package pii

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// PresidioClient is an HTTP client for the Microsoft Presidio Analyzer API.
// It sends text to Presidio for Named Entity Recognition (NER) and returns
// detected PII entities with their positions and types.
//
// Presidio can be run standalone via Docker:
//
//	docker run -p 5002:5001 mcr.microsoft.com/presidio-analyzer:latest
//
// Or via the pii-client's Python sidecar (presidio_sidecar/).
type PresidioClient struct {
	baseURL        string
	httpClient     *http.Client
	scoreThreshold float64
}

// PresidioEntity represents a PII entity detected by Presidio.
type PresidioEntity struct {
	EntityType string  `json:"entity_type"`
	Start      int     `json:"start"`
	End        int     `json:"end"`
	Score      float64 `json:"score"`
}

// presidioRequest is the request body for the Presidio /analyze endpoint.
type presidioRequest struct {
	Text           string   `json:"text"`
	Language       string   `json:"language"`
	Entities       []string `json:"entities,omitempty"`
	ScoreThreshold float64  `json:"score_threshold,omitempty"`
}

// presidioResponse is the response from the /analyze endpoint (array of entities).
// The response is directly []PresidioEntity.

// presidioHealthResponse is the response from the /health endpoint.
type presidioHealthResponse struct {
	Status string `json:"status"`
}

// NewPresidioClient creates a new client for the given Presidio API URL.
// The URL should be the base URL without a trailing slash (e.g., "http://localhost:5002").
func NewPresidioClient(baseURL string) *PresidioClient {
	return &PresidioClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		scoreThreshold: 0.85,
	}
}

// SetScoreThreshold sets the minimum confidence score for entity detection.
// Default is 0.85. Range: 0.0 to 1.0.
func (c *PresidioClient) SetScoreThreshold(threshold float64) {
	c.scoreThreshold = threshold
}

// activeEntityTypes are the Presidio entity types we care about for PII detection.
// We use a focused set to reduce false positives.
var activeEntityTypes = []string{
	"PERSON",
	"EMAIL_ADDRESS",
	"PHONE_NUMBER",
	"ORGANIZATION",
	"IP_ADDRESS",
}

// presidioToCategory maps Presidio entity types to our PII categories.
var presidioToCategory = map[string]string{
	"PERSON":        CategoryPerson,
	"EMAIL_ADDRESS": CategoryEmail,
	"PHONE_NUMBER":  CategoryPhone,
	"ORGANIZATION":  CategoryOrg,
	"IP_ADDRESS":    CategoryIPAddress,
}

// Analyze sends a single text to Presidio for NER analysis.
func (c *PresidioClient) Analyze(text string) ([]PresidioEntity, error) {
	req := presidioRequest{
		Text:           text,
		Language:       "en",
		Entities:       activeEntityTypes,
		ScoreThreshold: c.scoreThreshold,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Presidio request: %w", err)
	}

	resp, err := c.httpClient.Post(c.baseURL+"/analyze", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("Presidio request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Presidio returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var entities []PresidioEntity
	if err := json.NewDecoder(resp.Body).Decode(&entities); err != nil {
		return nil, fmt.Errorf("failed to decode Presidio response: %w", err)
	}

	// Map Presidio entity types to our categories
	for i := range entities {
		if cat, ok := presidioToCategory[entities[i].EntityType]; ok {
			entities[i].EntityType = cat
		}
	}

	return entities, nil
}

// AnalyzeBatch sends multiple texts to Presidio for NER analysis.
// Returns a slice of entity slices, one per input text.
// If Presidio doesn't support batch analysis, falls back to sequential calls.
func (c *PresidioClient) AnalyzeBatch(texts []string) ([][]PresidioEntity, error) {
	// Presidio's standard API doesn't have a batch endpoint.
	// The pii-client's sidecar adds /analyze-batch, but the standard
	// Presidio image only has /analyze. We fall back to sequential calls.
	results := make([][]PresidioEntity, len(texts))

	for i, text := range texts {
		entities, err := c.Analyze(text)
		if err != nil {
			// Non-fatal: skip this text and continue with others
			continue
		}
		results[i] = entities
	}

	return results, nil
}

// IsHealthy checks if the Presidio service is reachable and healthy.
func (c *PresidioClient) IsHealthy() bool {
	resp, err := c.httpClient.Get(c.baseURL + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

package appengine

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/client"
)

// IntentHandler handles App Engine intent operations
type IntentHandler struct {
	client *client.Client
}

// NewIntentHandler creates a new intent handler
func NewIntentHandler(c *client.Client) *IntentHandler {
	return &IntentHandler{client: c}
}

// IntentProperty represents a property definition in an intent
type IntentProperty struct {
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Format      string `json:"format,omitempty"`
	Description string `json:"description,omitempty"`
}

// Intent represents an app intent from the manifest
type Intent struct {
	AppID         string                    `json:"appId" table:"APP_ID,wide"`
	AppName       string                    `json:"appName" table:"APP"`
	IntentID      string                    `json:"intentId" table:"INTENT_ID"`
	Description   string                    `json:"description" table:"DESCRIPTION"`
	Properties    map[string]IntentProperty `json:"properties,omitempty" table:"-"`
	FullName      string                    `json:"fullName" table:"FULL_NAME"`
	RequiredProps []string                  `json:"requiredProps,omitempty" table:"REQUIRED"`
}

// IntentMatch represents a matched intent with quality score
type IntentMatch struct {
	Intent
	MatchQuality float64  `json:"matchQuality" table:"MATCH%"`
	MatchedProps []string `json:"matchedProps,omitempty" table:"-"`
	MissingProps []string `json:"missingProps,omitempty" table:"-"`
}

// ListIntents lists all intents across apps (or filtered by app ID)
func (h *IntentHandler) ListIntents(appIDFilter string) ([]Intent, error) {
	// Get all apps with manifest
	appHandler := NewHandler(h.client)
	appList, err := appHandler.ListApps()
	if err != nil {
		return nil, err
	}

	var intents []Intent

	// For each app, extract intents from manifest
	for _, app := range appList.Apps {
		// If filter is set, skip apps that don't match
		if appIDFilter != "" && app.ID != appIDFilter {
			continue
		}

		// Extract intents from manifest
		if app.Manifest != nil {
			appIntents := extractIntentsFromManifest(app)
			intents = append(intents, appIntents...)
		}
	}

	return intents, nil
}

// GetIntent gets details about a specific intent
func (h *IntentHandler) GetIntent(fullName string) (*Intent, error) {
	// Parse app-id/intent-id format
	appID, intentID := parseFullIntentName(fullName)
	if appID == "" || intentID == "" {
		return nil, fmt.Errorf("invalid intent name format, expected 'app-id/intent-id', got %q", fullName)
	}

	// Get app details
	appHandler := NewHandler(h.client)
	app, err := appHandler.GetApp(appID)
	if err != nil {
		return nil, err
	}

	// Find the intent in the manifest
	if app.Manifest != nil {
		intents := extractIntentsFromManifest(*app)
		for _, intent := range intents {
			if intent.IntentID == intentID {
				return &intent, nil
			}
		}
	}

	return nil, fmt.Errorf("intent %q not found in app %q", intentID, appID)
}

// FindIntentsForData finds intents that match the given data
func (h *IntentHandler) FindIntentsForData(data map[string]interface{}) ([]IntentMatch, error) {
	// Get all intents
	intents, err := h.ListIntents("")
	if err != nil {
		return nil, err
	}

	var matches []IntentMatch

	// Match each intent against the data
	for _, intent := range intents {
		match := matchIntentToData(intent, data)
		// Only include intents with non-zero match quality
		if match.MatchQuality > 0 {
			matches = append(matches, match)
		}
	}

	// Sort by match quality (descending)
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].MatchQuality > matches[j].MatchQuality
	})

	return matches, nil
}

// GenerateIntentURL generates an intent URL for the given app, intent, and payload
func (h *IntentHandler) GenerateIntentURL(appID, intentID string, payload map[string]interface{}) (string, error) {
	// Get base URL from client
	baseURL := h.client.BaseURL()

	// Marshal payload to JSON
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Construct URL
	intentURL := fmt.Sprintf("%s/ui/intent/%s/%s#%s",
		baseURL, appID, intentID, url.QueryEscape(string(jsonPayload)))

	return intentURL, nil
}

// extractIntentsFromManifest extracts intents from an app manifest
func extractIntentsFromManifest(app App) []Intent {
	var intents []Intent

	// Navigate manifest structure: manifest.intents (object/map at top level)
	if app.Manifest != nil {
		if intentsMap, ok := app.Manifest["intents"].(map[string]interface{}); ok {
			for intentID, intentData := range intentsMap {
				if intentMap, ok := intentData.(map[string]interface{}); ok {
					intent := parseIntentFromMap(app.ID, app.Name, intentID, intentMap)
					intents = append(intents, intent)
				}
			}
		}
	}

	return intents
}

// parseIntentFromMap parses an intent from a manifest map
func parseIntentFromMap(appID, appName, intentID string, intentMap map[string]interface{}) Intent {
	// Get intent metadata
	name, _ := intentMap["name"].(string)
	description, _ := intentMap["description"].(string)

	// If no explicit display name, use description or intent ID
	if name == "" {
		name = description
	}
	if description == "" {
		description = name
	}

	// Parse properties
	properties := make(map[string]IntentProperty)
	var requiredProps []string

	if propsMap, ok := intentMap["properties"].(map[string]interface{}); ok {
		for propName, propData := range propsMap {
			if propMap, ok := propData.(map[string]interface{}); ok {
				// Get required flag (defaults to false)
				propRequired, _ := propMap["required"].(bool)

				// Extract type and format from schema
				propType := "string" // default
				propFormat := ""
				propDescription := ""

				if schema, ok := propMap["schema"].(map[string]interface{}); ok {
					if schemaType, ok := schema["type"].(string); ok {
						propType = schemaType
					}
					if schemaFormat, ok := schema["format"].(string); ok {
						propFormat = schemaFormat
					}
				}

				// Try to get description at property level
				if desc, ok := propMap["description"].(string); ok {
					propDescription = desc
				}

				properties[propName] = IntentProperty{
					Type:        propType,
					Required:    propRequired,
					Format:      propFormat,
					Description: propDescription,
				}

				if propRequired {
					requiredProps = append(requiredProps, propName)
				}
			}
		}
	}

	// Sort required props for consistent output
	sort.Strings(requiredProps)

	return Intent{
		AppID:         appID,
		AppName:       appName,
		IntentID:      intentID,
		Description:   description,
		Properties:    properties,
		FullName:      fmt.Sprintf("%s/%s", appID, intentID),
		RequiredProps: requiredProps,
	}
}

// matchIntentToData matches an intent against provided data
func matchIntentToData(intent Intent, data map[string]interface{}) IntentMatch {
	var matchedProps []string
	var missingProps []string

	// Check all required properties
	for _, reqProp := range intent.RequiredProps {
		if _, exists := data[reqProp]; exists {
			matchedProps = append(matchedProps, reqProp)
		} else {
			missingProps = append(missingProps, reqProp)
		}
	}

	// If any required property is missing, match quality is 0
	if len(missingProps) > 0 {
		return IntentMatch{
			Intent:       intent,
			MatchQuality: 0,
			MatchedProps: matchedProps,
			MissingProps: missingProps,
		}
	}

	// Calculate match quality based on property coverage
	totalProps := len(intent.Properties)
	if totalProps == 0 {
		// Intent with no properties always matches
		return IntentMatch{
			Intent:       intent,
			MatchQuality: 100,
			MatchedProps: matchedProps,
			MissingProps: missingProps,
		}
	}

	// Count all properties in data that match intent properties
	matchedCount := 0
	for propName := range intent.Properties {
		if _, exists := data[propName]; exists {
			matchedCount++
			// Add to matchedProps if not already there (for optional props)
			if !contains(matchedProps, propName) {
				matchedProps = append(matchedProps, propName)
			}
		}
	}

	// Calculate coverage: (matched_properties / total_properties) * 100
	matchQuality := (float64(matchedCount) / float64(totalProps)) * 100

	return IntentMatch{
		Intent:       intent,
		MatchQuality: matchQuality,
		MatchedProps: matchedProps,
		MissingProps: missingProps,
	}
}

// parseFullIntentName parses a full intent name (app-id/intent-id)
func parseFullIntentName(fullName string) (appID, intentID string) {
	parts := strings.SplitN(fullName, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", ""
}

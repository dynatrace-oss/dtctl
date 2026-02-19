package appengine

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// FunctionSchema represents the discovered schema of a function
type FunctionSchema struct {
	FunctionName string
	AppID        string
	Fields       []SchemaField
	ErrorMessage string
}

// SchemaField represents a field in the function schema
type SchemaField struct {
	Name     string
	Type     string
	Required bool
	Hint     string
}

// DiscoverSchema attempts to discover the schema of a function by calling it with an empty payload
func (h *FunctionHandler) DiscoverSchema(appID, functionName string) (*FunctionSchema, error) {
	// Try with empty payload to trigger validation
	req := &FunctionInvokeRequest{
		Method:       "POST",
		AppID:        appID,
		FunctionName: functionName,
		Payload:      "{}",
		Headers:      make(map[string]string),
	}

	resp, err := h.InvokeFunction(req)
	if err != nil {
		// If we got a non-validation error, return it
		if !strings.Contains(err.Error(), "validation") &&
			!strings.Contains(err.Error(), "Invalid input") &&
			!strings.Contains(err.Error(), "missing") &&
			!strings.Contains(err.Error(), "required") &&
			!strings.Contains(err.Error(), "Required") {
			return nil, err
		}
	}

	schema := &FunctionSchema{
		FunctionName: functionName,
		AppID:        appID,
		Fields:       []SchemaField{},
	}

	// Parse the response body to extract schema information
	if resp != nil && resp.Body != "" {
		var bodyData map[string]interface{}
		if err := json.Unmarshal([]byte(resp.Body), &bodyData); err == nil {
			// Try to parse constraintViolations format (nested structure)
			schema.Fields = parseConstraintViolations(bodyData)

			// If no fields found, try parsing error message string
			if len(schema.Fields) == 0 {
				if errorMsg, ok := bodyData["error"].(string); ok {
					schema.ErrorMessage = errorMsg
					schema.Fields = parseSchemaFromError(errorMsg)

					// If no fields discovered and error suggests empty input, try with minimal object
					if len(schema.Fields) == 0 && strings.Contains(errorMsg, "must not be empty") {
						req.Payload = `{"test":"value"}`
						resp2, _ := h.InvokeFunction(req)
						if resp2 != nil && resp2.Body != "" {
							var bodyData2 map[string]interface{}
							if err := json.Unmarshal([]byte(resp2.Body), &bodyData2); err == nil {
								if errorMsg2, ok := bodyData2["error"].(string); ok {
									schema.ErrorMessage = errorMsg2
									schema.Fields = parseSchemaFromError(errorMsg2)
								}
							}
						}
					}
				}
			}

			// If root-level validation found (empty path), try with sample fields to discover actual schema
			if hasRootLevelValidation(schema.Fields) {
				testPayloads := []string{
					`{"prompt": "test"}`,
					`{"query": "test"}`,
					`{"text": "test"}`,
					`{"message": "test"}`,
					`{"input": "test"}`,
					`{"data": {}}`,
				}

				for _, payload := range testPayloads {
					req.Payload = payload
					resp2, _ := h.InvokeFunction(req)
					if resp2 != nil && resp2.Body != "" {
						var bodyData2 map[string]interface{}
						if err := json.Unmarshal([]byte(resp2.Body), &bodyData2); err == nil {
							newFields := parseConstraintViolations(bodyData2)
							if len(newFields) > len(schema.Fields) && !hasRootLevelValidation(newFields) {
								schema.Fields = newFields
								break
							}
						}
					}
				}
			}
		}
	}

	// If no fields were discovered and no error message, check if function succeeded with empty payload
	if len(schema.Fields) == 0 && schema.ErrorMessage == "" {
		if err == nil && resp != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			schema.ErrorMessage = "Function accepts empty payload (no required fields)"
		}
	}

	return schema, nil
}

// parseConstraintViolations extracts schema from constraintViolations format
// Handles nested structures like: body.error.details.constraintViolations
func parseConstraintViolations(data map[string]interface{}) []SchemaField {
	var fields []SchemaField

	// Try to navigate nested structure: body.error.details.constraintViolations
	var constraintViolations []interface{}

	// Pattern 1: Direct constraintViolations array
	if cv, ok := data["constraintViolations"].([]interface{}); ok {
		constraintViolations = cv
	}

	// Pattern 2: Nested in body.error.details.constraintViolations
	if body, ok := data["body"].(map[string]interface{}); ok {
		if errorObj, ok := body["error"].(map[string]interface{}); ok {
			if details, ok := errorObj["details"].(map[string]interface{}); ok {
				if cv, ok := details["constraintViolations"].([]interface{}); ok {
					constraintViolations = cv
				}
			}
		}
	}

	// Pattern 3: Nested in error.details.constraintViolations
	if errorObj, ok := data["error"].(map[string]interface{}); ok {
		if details, ok := errorObj["details"].(map[string]interface{}); ok {
			if cv, ok := details["constraintViolations"].([]interface{}); ok {
				constraintViolations = cv
			}
		}
	}

	// Parse each constraint violation
	for _, cv := range constraintViolations {
		if violation, ok := cv.(map[string]interface{}); ok {
			path, _ := violation["path"].(string)
			message, _ := violation["message"].(string)

			// Skip root-level validation errors (empty path) for now
			// These will trigger a retry with sample payloads
			if path == "" {
				fields = append(fields, SchemaField{
					Name:     "_root_",
					Type:     "unknown",
					Required: true,
					Hint:     message,
				})
				continue
			}

			// Extract field name from path (handle nested paths like "data.field")
			fieldName := path
			if strings.Contains(path, ".") {
				parts := strings.Split(path, ".")
				fieldName = parts[0] // Use top-level field
			}

			field := SchemaField{
				Name:     fieldName,
				Type:     "unknown",
				Required: true,
				Hint:     message,
			}

			// Try to infer type from message
			messageLower := strings.ToLower(message)
			// Look for "expected <type>" pattern first (more specific)
			if strings.Contains(messageLower, "expected number") || strings.Contains(messageLower, "expected integer") {
				field.Type = "number"
			} else if strings.Contains(messageLower, "expected string") {
				field.Type = "string"
			} else if strings.Contains(messageLower, "expected object") {
				field.Type = "object"
			} else if strings.Contains(messageLower, "expected array") {
				field.Type = "array"
			} else if strings.Contains(messageLower, "expected boolean") {
				field.Type = "boolean"
			} else if strings.Contains(messageLower, "string") {
				field.Type = "string"
			} else if strings.Contains(messageLower, "object") {
				field.Type = "object"
			} else if strings.Contains(messageLower, "array") {
				field.Type = "array"
			} else if strings.Contains(messageLower, "boolean") {
				field.Type = "boolean"
			} else if strings.Contains(messageLower, "number") || strings.Contains(messageLower, "integer") {
				field.Type = "number"
			}

			fields = append(fields, field)
		}
	}

	return fields
}

// hasRootLevelValidation checks if the schema only contains root-level validation errors
func hasRootLevelValidation(fields []SchemaField) bool {
	if len(fields) == 0 {
		return false
	}
	for _, field := range fields {
		if field.Name != "_root_" {
			return false
		}
	}
	return true
}

// parseSchemaFromError extracts schema information from error messagesSchemaFromError extracts schema information from error messages
func parseSchemaFromError(errorMsg string) []SchemaField {
	var fields []SchemaField

	// Pattern 0: "Action input must not be empty" - try with minimal object
	if strings.Contains(errorMsg, "Action input must not be empty") {
		// Return empty - this function needs a non-empty object to reveal fields
		return fields
	}

	// Pattern 0.5: Try to parse as JSON structure first (for Zod-style errors)
	// "Invalid input: { \"field\": [\"error\"] }"
	if strings.Contains(errorMsg, "Invalid input:") && strings.Contains(errorMsg, "{") {
		// Extract the JSON part
		jsonStart := strings.Index(errorMsg, "{")
		if jsonStart != -1 {
			jsonStr := errorMsg[jsonStart:]
			var errorFields map[string]interface{}
			if err := json.Unmarshal([]byte(jsonStr), &errorFields); err == nil {
				for fieldName, fieldError := range errorFields {
					field := SchemaField{
						Name:     fieldName,
						Type:     "unknown",
						Required: true,
					}

					// Try to extract type from error message array
					if errArray, ok := fieldError.([]interface{}); ok && len(errArray) > 0 {
						if errMsg, ok := errArray[0].(string); ok {
							field.Hint = errMsg
							// Extract type from error message
							if strings.Contains(errMsg, "expected object") {
								field.Type = "object"
							} else if strings.Contains(errMsg, "expected string") {
								field.Type = "string"
							} else if strings.Contains(errMsg, "expected array") {
								field.Type = "array"
							} else if strings.Contains(errMsg, "expected boolean") {
								field.Type = "boolean"
							} else if strings.Contains(errMsg, "expected number") {
								field.Type = "number"
							}
						}
					}

					fields = append(fields, field)
				}
				return fields
			}
		}
	}

	// Pattern 1: "Input fields 'query' are missing."
	re1 := regexp.MustCompile(`Input fields? ['"]([^'"]+)['"] (?:are|is) missing`)
	if matches := re1.FindStringSubmatch(errorMsg); len(matches) > 1 {
		fieldNames := strings.Split(matches[1], ",")
		for _, name := range fieldNames {
			fields = append(fields, SchemaField{
				Name:     strings.TrimSpace(name),
				Type:     "unknown",
				Required: true,
			})
		}
		return fields
	}

	// Pattern 2: "connectionId - project - issueType - components - summary - description"
	re2 := regexp.MustCompile(`wrong fields?: ['"]([^'"]+)['"]`)
	if matches := re2.FindStringSubmatch(errorMsg); len(matches) > 1 {
		fieldNames := strings.Split(matches[1], " - ")
		for _, name := range fieldNames {
			fields = append(fields, SchemaField{
				Name:     strings.TrimSpace(name),
				Type:     "unknown",
				Required: true,
			})
		}
		return fields
	}

	// Pattern 3: "connection: Required\n   - channel: Required\n   - message: Must be defined"
	re3 := regexp.MustCompile(`(?m)^\s*-?\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*:\s*(Required|Must be defined|Invalid input)`)
	matches := re3.FindAllStringSubmatch(errorMsg, -1)
	if len(matches) > 0 {
		for _, match := range matches {
			if len(match) > 1 {
				fields = append(fields, SchemaField{
					Name:     match[1],
					Type:     "unknown",
					Required: true,
				})
			}
		}
		return fields
	}

	// Pattern 4: "observable": ["Invalid input: expected object, received undefined"]
	re4 := regexp.MustCompile(`"([a-zA-Z_][a-zA-Z0-9_]*)"\s*:\s*\[?\s*"[^"]*expected\s+(\w+),\s*received`)
	matches = re4.FindAllStringSubmatch(errorMsg, -1)
	if len(matches) > 0 {
		for _, match := range matches {
			if len(match) > 2 {
				fields = append(fields, SchemaField{
					Name:     match[1],
					Type:     match[2],
					Required: true,
				})
			}
		}
		return fields
	}

	// Pattern 5: Line-by-line validation errors
	lines := strings.Split(errorMsg, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "- ")

		// Extract field name and type hint
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			fieldName := strings.TrimSpace(parts[0])
			hint := strings.TrimSpace(parts[1])

			// Skip if this doesn't look like a field name
			if fieldName == "" || !regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`).MatchString(fieldName) {
				continue
			}

			field := SchemaField{
				Name:     fieldName,
				Type:     "unknown",
				Required: true,
				Hint:     hint,
			}

			// Try to extract type information from hint
			if strings.Contains(hint, "string") {
				field.Type = "string"
			} else if strings.Contains(hint, "object") {
				field.Type = "object"
			} else if strings.Contains(hint, "array") {
				field.Type = "array"
			} else if strings.Contains(hint, "boolean") {
				field.Type = "boolean"
			} else if strings.Contains(hint, "number") {
				field.Type = "number"
			}

			fields = append(fields, field)
		}
	}

	return fields
}

// FormatSchema formats a schema for display
func (s *FunctionSchema) FormatSchema() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Function: %s/%s\n\n", s.AppID, s.FunctionName))

	if len(s.Fields) == 0 {
		if s.ErrorMessage == "Function accepts empty payload (no required fields)" {
			sb.WriteString("✓ This function accepts an empty payload (no required fields).\n")
			sb.WriteString("  It can be invoked without any input parameters.\n\n")
			sb.WriteString("Example usage:\n")
			sb.WriteString(fmt.Sprintf("  dtctl exec function %s/%s --method POST --payload '{}'\n", s.AppID, s.FunctionName))
			return sb.String()
		}

		sb.WriteString("Unable to discover schema fields.\n")
		if s.ErrorMessage != "" {
			sb.WriteString("\nRaw error message:\n")
			sb.WriteString(s.ErrorMessage)
			sb.WriteString("\n")
		}
		return sb.String()
	}

	sb.WriteString("Required Fields:\n")
	maxNameLen := 0
	maxTypeLen := 0
	for _, field := range s.Fields {
		if len(field.Name) > maxNameLen {
			maxNameLen = len(field.Name)
		}
		if len(field.Type) > maxTypeLen {
			maxTypeLen = len(field.Type)
		}
	}

	for _, field := range s.Fields {
		sb.WriteString(fmt.Sprintf("  %-*s  %-*s", maxNameLen, field.Name, maxTypeLen, field.Type))
		if field.Hint != "" {
			sb.WriteString(fmt.Sprintf("  %s", field.Hint))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("\nExample payload:\n")
	sb.WriteString(s.GenerateExamplePayload())
	sb.WriteString("\n")

	return sb.String()
}

// GenerateExamplePayload generates an example JSON payload based on discovered schema
func (s *FunctionSchema) GenerateExamplePayload() string {
	if len(s.Fields) == 0 {
		return "{}"
	}

	payload := make(map[string]interface{})
	for _, field := range s.Fields {
		switch field.Type {
		case "string":
			payload[field.Name] = "..."
		case "object":
			payload[field.Name] = map[string]interface{}{}
		case "array":
			payload[field.Name] = []interface{}{}
		case "boolean":
			payload[field.Name] = false
		case "number":
			payload[field.Name] = 0
		default:
			payload[field.Name] = "..."
		}
	}

	jsonBytes, err := json.MarshalIndent(payload, "  ", "  ")
	if err != nil {
		return "{}"
	}

	return "  " + string(jsonBytes)
}

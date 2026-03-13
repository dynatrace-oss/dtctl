package livedebugger

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/client"
)

type Handler struct {
	client     *client.Client
	graphqlURL string
	orgID      string
}

func NewHandler(c *client.Client, environmentURL string) (*Handler, error) {
	graphqlURL, err := buildGraphQLURL(environmentURL)
	if err != nil {
		return nil, err
	}

	orgID, err := extractOrgID(environmentURL)
	if err != nil {
		return nil, err
	}

	return &Handler{client: c, graphqlURL: graphqlURL, orgID: orgID}, nil
}

func (h *Handler) GetOrCreateWorkspace(projectPath string) (map[string]interface{}, string, error) {
	query := `query GetOrCreateWorkspaceV2($orgId: ID!, $workspaceInput: WorkspaceGetOrCreateInput) {
  org(id: $orgId) {
    id
    getOrCreateUserWorkspaceV2(workspaceInput: $workspaceInput) {
      id
      orgId
      name
      filterSets {
        labels {
          field
          values
        }
        filters {
          field
          values
        }
      }
      sources
      creationTime
      modificationTime
      creatorEmail
      includeInactive
    }
  }
}`

	variables := map[string]interface{}{
		"orgId": h.orgID,
		"workspaceInput": map[string]interface{}{
			"clientName":  "dtctl",
			"projectPath": projectPath,
		},
	}

	resp, err := h.executeGraphQL(query, variables)
	if err != nil {
		return nil, "", err
	}

	workspaceID, err := ExtractWorkspaceID(resp)
	if err != nil {
		return resp, "", err
	}

	return resp, workspaceID, nil
}

func (h *Handler) UpdateWorkspaceFilters(workspaceID string, filterSets []map[string]interface{}) (map[string]interface{}, error) {
	mutation := `mutation UpdateWorkspaceV2($orgId: ID!, $workspaceId: ID!, $data: WorkspaceInputV2!) {
  org(orgId: $orgId) {
    updateWorkspaceV2(id: $workspaceId, data: $data) {
      id
      orgId
      name
      filterSets {
        labels {
          field
          values
        }
        filters {
          field
          values
        }
      }
      sources
      creationTime
      modificationTime
      creatorEmail
    }
  }
}`

	variables := map[string]interface{}{
		"orgId":       h.orgID,
		"workspaceId": workspaceID,
		"data": map[string]interface{}{
			"filterSets": filterSets,
			"sources":    []interface{}{},
		},
	}

	return h.executeGraphQL(mutation, variables)
}

func (h *Handler) CreateBreakpoint(workspaceID, fileName string, lineNumber int) (map[string]interface{}, error) {
	mutation := `mutation CreateRule($orgId: ID!, $workspaceId: ID!, $ruleData: CreateRuleV2Input!) {
  org(orgId: $orgId) {
    workspace(id: $workspaceId) {
      createRuleV2(data: $ruleData) {
        id
        immutableId
        workspace
        aug {
          mutable_id
          location {
            filename
            sourcePath
            lineno
            sourceRepo
            sha256
            pdbSha256
            line_crc32_2
            line_unique
          }
        }
      }
    }
  }
}`

	variables := map[string]interface{}{
		"orgId":       h.orgID,
		"workspaceId": workspaceID,
		"ruleData": map[string]interface{}{
			"mutableRuleId":           generateMutableRuleID(),
			"lineNumber":              lineNumber,
			"fileName":                fileName,
			"sourceRepo":              "",
			"sourcePath":              fileName,
			"sha256":                  "",
			"lineCrc32_2":             "",
			"lineUnique":              false,
			"pdbSha256":               "",
			"includeExternals":        nil,
			"isDisabled":              false,
			"disableSourceValidation": true,
		},
	}

	return h.executeGraphQL(mutation, variables)
}

func (h *Handler) GetWorkspaceRules(workspaceID string) (map[string]interface{}, error) {
	query := `query GetWorkspaceRules($orgId: ID!, $workspaceId: ID!) {
	org(id: $orgId) {
		id
		workspace(id: $workspaceId) {
			rules {
				id
				immutableId
				template_id
				template_type
				selector
				workspace
				user_email
				workspace_name
				aug_json {
					id
					mutable_id
					location {
						name
						filename
						sourcePath
						sourceRepo
						lineno
						sha256
						includeExternals
						pdbSha256
						line_crc32_2
						line_unique
						role
					}
					action {
						name
						operations
					}
					rateLimit
					conditional
					originalCondition
					globalHitLimit
					globalDisableAfterTime
				}
				is_disabled
				disable_reason
				revision_count
				processing
				indicator {
					indicatorState
					indicatorWarning
				}
			}
		}
	}
}`

	variables := map[string]interface{}{
		"orgId":       h.orgID,
		"workspaceId": workspaceID,
	}

	return h.executeGraphQL(query, variables)
}

func (h *Handler) DeleteBreakpoint(workspaceID, ruleID string) (map[string]interface{}, error) {
	mutation := `mutation DeleteRule($orgId: ID!, $workspaceId: ID!, $ruleId: ID!) {
  org(orgId: $orgId) {
    workspace(id: $workspaceId) {
      deleteRuleV2(mutableId: $ruleId)
    }
  }
}`

	variables := map[string]interface{}{
		"orgId":       h.orgID,
		"workspaceId": workspaceID,
		"ruleId":      ruleID,
	}

	return h.executeGraphQL(mutation, variables)
}

func (h *Handler) GetRuleStatusBreakdown(ruleID string) (map[string]interface{}, error) {
	query := `query GetRuleStatusBreakdown($orgId: ID!, $ruleId: ID!) {
	org(id: $orgId) {
		id
		ruleStatuses(mutableId: $ruleId) {
			ruleId
			status
			rookStatuses {
				rook {
					id
					executable
					hostname
				}
				error {
					message
					type
					parameters
					summary {
						title
						description
						docsLink
						args
					}
				}
				tips {
					description
					docsLink
				}
			}
			agentStatuses {
				controllerId
				error {
					message
					type
					parameters
					summary {
						title
						description
						docsLink
						args
					}
				}
			}
			controllerStatuses {
				controllerId
				error {
					message
					type
					parameters
					summary {
						title
						description
						docsLink
						args
					}
				}
			}
		}
	}
}`

	variables := map[string]interface{}{
		"orgId":  h.orgID,
		"ruleId": ruleID,
	}

	return h.executeGraphQL(query, variables)
}

func (h *Handler) EditBreakpoint(workspaceID string, ruleSettings map[string]interface{}) (map[string]interface{}, error) {
	mutation := `mutation EditRuleV2($orgId: ID!, $workspaceId: ID!, $ruleSettings: EditRuleV2Input!) {
	org(orgId: $orgId) {
		workspace(id: $workspaceId) {
			editRuleV2(data: $ruleSettings) {
				id
				immutableId
				template_id
				template_type
				selector
				workspace
				user_email
				workspace_name
				aug {
					id
					mutable_id
					location {
						name
						filename
						sourcePath
						sourceRepo
						lineno
						sha256
						includeExternals
					}
					action {
						name
						operations
					}
					conditional
					originalCondition
				}
				is_disabled
				disable_reason
				revision_count
				processing
			}
		}
	}
}`

	variables := map[string]interface{}{
		"orgId":        h.orgID,
		"workspaceId":  workspaceID,
		"ruleSettings": ruleSettings,
	}

	return h.executeGraphQL(mutation, variables)
}

func (h *Handler) EnableOrDisableBreakpoints(workspaceID string, ruleIDs []string, isDisabled bool) (map[string]interface{}, error) {
	mutation := `mutation EnableOrDisableRules($orgId: ID!, $workspaceId: ID!, $rulesIds: [String]!, $isDisabled: Boolean!) {
	org(orgId: $orgId) {
		workspace(id: $workspaceId) {
			enableOrDisableRules(isDisabled: $isDisabled, rulesIds: $rulesIds) {
				id
				immutableId
				is_disabled
			}
		}
	}
}`

	variables := map[string]interface{}{
		"orgId":       h.orgID,
		"workspaceId": workspaceID,
		"rulesIds":    ruleIDs,
		"isDisabled":  isDisabled,
	}

	return h.executeGraphQL(mutation, variables)
}

func (h *Handler) DeleteAllBreakpoints(workspaceID string) (map[string]interface{}, error) {
	mutation := `mutation DeleteAllRulesFromWorkspace($orgId: ID!, $workspaceId: ID!, $data: DeleteWorkspaceRulesInput!) {
  org(orgId: $orgId) {
    workspace(id: $workspaceId) {
      deleteAllRulesFromWorkspaceV2(data: $data)
    }
  }
}`

	variables := map[string]interface{}{
		"orgId":       h.orgID,
		"workspaceId": workspaceID,
		"data": map[string]interface{}{
			"ruleType": "DumpFrame",
		},
	}

	return h.executeGraphQL(mutation, variables)
}

func BuildFilterSets(filters map[string][]string) []map[string]interface{} {
	if len(filters) == 0 {
		return []map[string]interface{}{}
	}

	labelList := make([]map[string]interface{}, 0, len(filters))
	for field, values := range filters {
		labelList = append(labelList, map[string]interface{}{
			"field":  field,
			"values": values,
		})
	}

	return []map[string]interface{}{
		{
			"filters": []interface{}{},
			"labels":  labelList,
		},
	}
}

func (h *Handler) executeGraphQL(query string, variables map[string]interface{}) (map[string]interface{}, error) {
	requestBody := map[string]interface{}{
		"query":     query,
		"variables": variables,
	}

	var response map[string]interface{}
	resp, err := h.client.HTTP().R().
		SetHeader("Content-Type", "application/json").
		SetHeader("dt-external-source", "dtctl").
		SetBody(requestBody).
		SetResult(&response).
		Post(h.graphqlURL)
	if err != nil {
		return nil, fmt.Errorf("failed to call live debugger graphql: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("live debugger graphql request failed: status %d: %s", resp.StatusCode(), resp.String())
	}

	if errorsValue, ok := response["errors"]; ok {
		return response, fmt.Errorf("live debugger graphql returned errors: %v", errorsValue)
	}

	return response, nil
}

func buildGraphQLURL(environmentURL string) (string, error) {
	envURL := strings.TrimSpace(environmentURL)
	if envURL == "" {
		return "", fmt.Errorf("context environment is required")
	}

	parsed, err := url.Parse(envURL)
	if err != nil {
		return "", fmt.Errorf("invalid context environment URL %q: %w", environmentURL, err)
	}

	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("context environment must be a full URL, got %q", environmentURL)
	}

	return strings.TrimRight(parsed.String(), "/") + "/platform/dob/graphql", nil
}

func extractOrgID(environmentURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(environmentURL))
	if err != nil {
		return "", fmt.Errorf("invalid context environment URL %q: %w", environmentURL, err)
	}

	host := parsed.Hostname()
	if host == "" {
		return "", fmt.Errorf("context environment URL has no host: %q", environmentURL)
	}

	parts := strings.Split(host, ".")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return "", fmt.Errorf("failed to extract org id from host %q", host)
	}

	return parts[0], nil
}

func generateMutableRuleID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "dtctl-rule-" + strconv.FormatInt(1, 10)
	}
	return "dtctl-rule-" + hex.EncodeToString(buf)
}

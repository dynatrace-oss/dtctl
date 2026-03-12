package livedebugger

import (
	"encoding/json"
	"fmt"
)

type GraphQLWorkspaceResponse struct {
	Data struct {
		Org struct {
			Workspace            Workspace        `json:"workspace"`
			GetOrCreateWorkspace Workspace        `json:"getOrCreateUserWorkspaceV2"`
			RuleStatuses         []RuleStatusNode `json:"ruleStatuses"`
		} `json:"org"`
	} `json:"data"`
}

type Workspace struct {
	ID    string           `json:"id"`
	Rules []BreakpointRule `json:"rules"`
}

type BreakpointRule struct {
	ID            string                 `json:"id" table:"ID"`
	IsDisabled    bool                   `json:"is_disabled" table:"ACTIVE"`
	DisableReason string                 `json:"disable_reason,omitempty"`
	AugJSON       map[string]interface{} `json:"aug_json"`
	Processing    map[string]interface{} `json:"processing"`
}

type RuleStatusNode struct {
	RuleID             string                   `json:"ruleId"`
	Status             string                   `json:"status"`
	RookStatuses       []map[string]interface{} `json:"rookStatuses"`
	AgentStatuses      []map[string]interface{} `json:"agentStatuses"`
	ControllerStatuses []map[string]interface{} `json:"controllerStatuses"`
}

type DeleteAllRulesResponse struct {
	Data struct {
		Org struct {
			Workspace struct {
				DeleteAllRulesFromWorkspaceV2 []string `json:"deleteAllRulesFromWorkspaceV2"`
			} `json:"workspace"`
		} `json:"org"`
	} `json:"data"`
}

func decodeGraphQLResponse[T any](resp map[string]interface{}, target *T) error {
	raw, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("failed to serialize graphql response: %w", err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("failed to decode graphql response: %w", err)
	}
	return nil
}

func ExtractWorkspaceID(resp map[string]interface{}) (string, error) {
	var decoded GraphQLWorkspaceResponse
	if err := decodeGraphQLResponse(resp, &decoded); err != nil {
		return "", err
	}
	workspaceID := decoded.Data.Org.GetOrCreateWorkspace.ID
	if workspaceID == "" {
		return "", fmt.Errorf("graphql response missing workspace id")
	}
	return workspaceID, nil
}

func ExtractWorkspaceRules(resp map[string]interface{}) ([]BreakpointRule, error) {
	dataObj, ok := resp["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("graphql response missing data object")
	}
	orgObj, ok := dataObj["org"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("graphql response missing org object")
	}
	workspaceObj, ok := orgObj["workspace"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("graphql response missing workspace object")
	}
	if _, ok := workspaceObj["rules"]; !ok {
		return nil, fmt.Errorf("graphql response missing rules list")
	}

	var decoded GraphQLWorkspaceResponse
	if err := decodeGraphQLResponse(resp, &decoded); err != nil {
		return nil, err
	}
	return decoded.Data.Org.Workspace.Rules, nil
}

func ExtractRuleStatuses(resp map[string]interface{}) ([]RuleStatusNode, error) {
	dataObj, ok := resp["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("graphql response missing data object")
	}
	orgObj, ok := dataObj["org"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("graphql response missing org object")
	}
	if _, ok := orgObj["ruleStatuses"]; !ok {
		return []RuleStatusNode{}, nil
	}

	var decoded GraphQLWorkspaceResponse
	if err := decodeGraphQLResponse(resp, &decoded); err != nil {
		return nil, err
	}
	return decoded.Data.Org.RuleStatuses, nil
}

func ExtractDeletedRuleIDs(resp map[string]interface{}) ([]string, error) {
	dataObj, ok := resp["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("graphql response missing data object")
	}
	orgObj, ok := dataObj["org"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("graphql response missing org object")
	}
	workspaceObj, ok := orgObj["workspace"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("graphql response missing workspace object")
	}
	if _, ok := workspaceObj["deleteAllRulesFromWorkspaceV2"]; !ok {
		return nil, fmt.Errorf("graphql response missing deleted ids list")
	}

	var decoded DeleteAllRulesResponse
	if err := decodeGraphQLResponse(resp, &decoded); err != nil {
		return nil, err
	}
	ids := decoded.Data.Org.Workspace.DeleteAllRulesFromWorkspaceV2
	return ids, nil
}

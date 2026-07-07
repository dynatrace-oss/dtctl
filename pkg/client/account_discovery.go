package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type AccessInfoResponse struct {
	Accounts []AccessInfoAccount `json:"accounts"`
}

type AccessInfoAccount struct {
	UUID              string                 `json:"uuid"`
	Name              string                 `json:"name"`
	AccountServiceURL string                 `json:"accountServiceUrl,omitempty"`
	Environments      []AccessInfoEnvironment `json:"environments,omitempty"`
}

type AccessInfoEnvironment struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	ServiceURL string `json:"serviceUrl,omitempty"`
}

// DiscoverAccountUUID calls the IAM access-info endpoint to find the account UUID
// that contains the given environment ID. Uses the environment token (needs `openid` scope).
// Returns (uuid, accountName, error).
func DiscoverAccountUUID(iamBaseURL, envToken, environmentID string) (string, string, error) {
	req, err := http.NewRequest("GET", iamBaseURL+"/iam/v1/access-info", nil)
	if err != nil {
		return "", "", fmt.Errorf("build access-info request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+envToken)
	req.Header.Set("Accept", "application/json")

	c := &http.Client{Timeout: 10 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("access-info request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("access-info returned %d", resp.StatusCode)
	}

	var info AccessInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", "", fmt.Errorf("decode access-info: %w", err)
	}

	for _, acct := range info.Accounts {
		for _, env := range acct.Environments {
			if env.ID == environmentID {
				return acct.UUID, acct.Name, nil
			}
		}
	}
	return "", "", fmt.Errorf("no account found for environment %q", environmentID)
}

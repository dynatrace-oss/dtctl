package auth

import "github.com/dynatrace-oss/dtctl/pkg/config"

// accountScopesForSafetyLevel returns account OAuth scopes for the given safety level.
// platform-token:tokens:manage covers list/revoke; platform-token:tokens:write is required for create.
func accountScopesForSafetyLevel(level config.SafetyLevel) []string {
	switch level {
	case config.SafetyLevelReadOnly:
		return []string{"account-idm-read", "platform-token:tokens:manage"}
	default:
		return []string{"account-idm-read", "account-idm-write", "platform-token:tokens:manage", "platform-token:tokens:write"}
	}
}

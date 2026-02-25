package awsmonitoringconfig

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/resources/awsconnection"
)

var roleARNAccountIDPattern = regexp.MustCompile(`^arn:[^:]*:iam::([0-9]{12}):role/.+$`)

func ResolveCredential(identifier string, handler *awsconnection.Handler) (Credential, error) {
	item, err := handler.FindByName(identifier)
	if err != nil {
		item, err = handler.Get(identifier)
		if err != nil {
			return Credential{}, fmt.Errorf("aws connection %q not found by name or ID", identifier)
		}
	}

	accountID := ""
	if item.Value.AWSRoleBasedAuthentication != nil {
		accountID = ExtractAccountIDFromRoleARN(item.Value.AWSRoleBasedAuthentication.RoleArn)
	}

	return Credential{
		Enabled:                     true,
		Description:                 item.Name,
		ConnectionID:                item.ObjectID,
		AccountID:                   accountID,
		OverrideParentConfiguration: false,
	}, nil
}

func ExtractAccountIDFromRoleARN(roleArn string) string {
	match := roleARNAccountIDPattern.FindStringSubmatch(strings.TrimSpace(roleArn))
	if len(match) != 2 {
		return ""
	}
	return match[1]
}

func ParseOrDefaultRegions(input string, handler *Handler) ([]string, error) {
	if strings.TrimSpace(input) != "" {
		return SplitCSV(input), nil
	}

	available, err := handler.ListAvailableRegions()
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(available))
	for _, region := range available {
		out = append(out, region.Value)
	}
	return out, nil
}

func ParseOrDefaultFeatureSets(input string, handler *Handler) ([]string, error) {
	if strings.TrimSpace(input) != "" {
		return SplitCSV(input), nil
	}

	available, err := handler.ListAvailableFeatureSets()
	if err != nil {
		return nil, err
	}

	out := make([]string, 0, len(available))
	for _, featureSet := range available {
		if strings.HasSuffix(featureSet.Value, "_essential") {
			out = append(out, featureSet.Value)
		}
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("no feature sets with suffix _essential found")
	}

	return out, nil
}

func SplitCSV(input string) []string {
	parts := strings.Split(input, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

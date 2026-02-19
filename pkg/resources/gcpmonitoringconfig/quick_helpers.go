package gcpmonitoringconfig

import (
	"fmt"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/resources/gcpconnection"
)

func ResolveCredential(identifier string, handler *gcpconnection.Handler) (Credential, error) {
	item, err := handler.FindByName(identifier)
	if err != nil {
		item, err = handler.Get(identifier)
		if err != nil {
			return Credential{}, fmt.Errorf("gcp connection %q not found by name or ID", identifier)
		}
	}

	serviceAccount := ""
	if item.Value.ServiceAccountImpersonation != nil {
		serviceAccount = item.Value.ServiceAccountImpersonation.ServiceAccountID
	}

	return Credential{
		Enabled:        true,
		Description:    item.Name,
		ConnectionID:   item.ObjectID,
		ServiceAccount: serviceAccount,
	}, nil
}

func ParseOrDefaultLocations(input string, handler *Handler) ([]string, error) {
	if strings.TrimSpace(input) != "" {
		return SplitCSV(input), nil
	}

	available, err := handler.ListAvailableLocations()
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(available))
	for _, location := range available {
		out = append(out, location.Value)
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

package apply

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

const migrationProcessedField = "ingestEnrichmentMigrationProcessed"

type cloudMonitoringDocument struct {
	cloudField string
	cloud      map[string]json.RawMessage
}

func parseCloudMonitoringDocument(data []byte, cloudField string) (*cloudMonitoringDocument, error) {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, err
	}

	cloud := make(map[string]json.RawMessage)
	if valueJSON, ok := findJSONField(document, "value"); ok {
		var value map[string]json.RawMessage
		if err := json.Unmarshal(valueJSON, &value); err != nil {
			return nil, fmt.Errorf("value must be an object: %w", err)
		}
		if cloudJSON, ok := findJSONField(value, cloudField); ok {
			if err := json.Unmarshal(cloudJSON, &cloud); err != nil {
				return nil, fmt.Errorf("%s must be an object: %w", cloudField, err)
			}
		}
	}

	if _, supplied := findJSONField(cloud, migrationProcessedField); supplied {
		return nil, fmt.Errorf("%s is backend-owned and must not be supplied", migrationProcessedField)
	}

	return &cloudMonitoringDocument{cloudField: cloudField, cloud: cloud}, nil
}

func (d *cloudMonitoringDocument) marshal(config, cloudModel any) ([]byte, error) {
	canonical, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}

	var document map[string]json.RawMessage
	if err := json.Unmarshal(canonical, &document); err != nil {
		return nil, err
	}
	valueJSON, ok := document["value"]
	if !ok {
		return canonical, nil
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(valueJSON, &value); err != nil {
		return nil, err
	}
	cloudJSON, ok := value[d.cloudField]
	if !ok {
		return canonical, nil
	}
	var cloud map[string]json.RawMessage
	if err := json.Unmarshal(cloudJSON, &cloud); err != nil {
		return nil, err
	}

	knownFields := jsonFieldNames(cloudModel)
	for name, raw := range d.cloud {
		if _, known := knownFields[strings.ToLower(name)]; known {
			continue
		}
		cloud[name] = raw
	}

	value[d.cloudField], err = json.Marshal(cloud)
	if err != nil {
		return nil, err
	}
	document["value"], err = json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return json.Marshal(document)
}

func findJSONField(fields map[string]json.RawMessage, name string) (json.RawMessage, bool) {
	for field, value := range fields {
		if strings.EqualFold(field, name) {
			return value, true
		}
	}
	return nil, false
}

func jsonFieldNames(model any) map[string]struct{} {
	typeOf := reflect.TypeOf(model)
	if typeOf.Kind() == reflect.Pointer {
		typeOf = typeOf.Elem()
	}
	fields := make(map[string]struct{}, typeOf.NumField())
	for i := 0; i < typeOf.NumField(); i++ {
		name := strings.Split(typeOf.Field(i).Tag.Get("json"), ",")[0]
		if name != "" && name != "-" {
			fields[strings.ToLower(name)] = struct{}{}
		}
	}
	return fields
}

func validateCloudMonitoringDocument(resourceType ResourceType, data []byte) error {
	var cloudField string
	switch resourceType {
	case ResourceAWSMonitoringConfig:
		cloudField = "aws"
	case ResourceAzureMonitoringConfig:
		cloudField = "azure"
	case ResourceGCPMonitoringConfig:
		cloudField = "googleCloud"
	default:
		return nil
	}
	_, err := parseCloudMonitoringDocument(data, cloudField)
	return err
}

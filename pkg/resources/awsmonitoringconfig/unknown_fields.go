package awsmonitoringconfig

import (
	"github.com/dynatrace-oss/dtctl/pkg/util/format"
	"github.com/dynatrace-oss/dtctl/pkg/util/unknownfields"
)

// The monitoring-configuration document is read, modified and written back as
// a whole, so every struct in the value tree keeps the members it does not
// model (see package unknownfields) and renders YAML through its JSON shape —
// yaml.v3 reflection would otherwise ignore the json tags and drop Extra,
// breaking the `get -o yaml` → `apply -f` round trip.

func (v *Value) UnmarshalJSON(data []byte) error {
	type plain Value
	extra, err := unknownfields.Unmarshal(data, (*plain)(v))
	if err != nil {
		return err
	}
	v.Extra = extra
	return nil
}

func (v Value) MarshalJSON() ([]byte, error) {
	type plain Value
	return unknownfields.Marshal(plain(v), v.Extra)
}

func (v Value) MarshalYAML() (any, error) { return format.YAMLNodeFromJSON(v) }

func (v *AWSConfig) UnmarshalJSON(data []byte) error {
	type plain AWSConfig
	extra, err := unknownfields.Unmarshal(data, (*plain)(v))
	if err != nil {
		return err
	}
	v.Extra = extra
	return nil
}

func (v AWSConfig) MarshalJSON() ([]byte, error) {
	type plain AWSConfig
	return unknownfields.Marshal(plain(v), v.Extra)
}

func (v AWSConfig) MarshalYAML() (any, error) { return format.YAMLNodeFromJSON(v) }

func (v *Credential) UnmarshalJSON(data []byte) error {
	type plain Credential
	extra, err := unknownfields.Unmarshal(data, (*plain)(v))
	if err != nil {
		return err
	}
	v.Extra = extra
	return nil
}

func (v Credential) MarshalJSON() ([]byte, error) {
	type plain Credential
	return unknownfields.Marshal(plain(v), v.Extra)
}

func (v Credential) MarshalYAML() (any, error) { return format.YAMLNodeFromJSON(v) }

func (v *FlagConfig) UnmarshalJSON(data []byte) error {
	type plain FlagConfig
	extra, err := unknownfields.Unmarshal(data, (*plain)(v))
	if err != nil {
		return err
	}
	v.Extra = extra
	return nil
}

func (v FlagConfig) MarshalJSON() ([]byte, error) {
	type plain FlagConfig
	return unknownfields.Marshal(plain(v), v.Extra)
}

func (v FlagConfig) MarshalYAML() (any, error) { return format.YAMLNodeFromJSON(v) }

func (v *RegionalFlagConfig) UnmarshalJSON(data []byte) error {
	type plain RegionalFlagConfig
	extra, err := unknownfields.Unmarshal(data, (*plain)(v))
	if err != nil {
		return err
	}
	v.Extra = extra
	return nil
}

func (v RegionalFlagConfig) MarshalJSON() ([]byte, error) {
	type plain RegionalFlagConfig
	return unknownfields.Marshal(plain(v), v.Extra)
}

func (v RegionalFlagConfig) MarshalYAML() (any, error) { return format.YAMLNodeFromJSON(v) }

func (v *TagFilter) UnmarshalJSON(data []byte) error {
	type plain TagFilter
	extra, err := unknownfields.Unmarshal(data, (*plain)(v))
	if err != nil {
		return err
	}
	v.Extra = extra
	return nil
}

func (v TagFilter) MarshalJSON() ([]byte, error) {
	type plain TagFilter
	return unknownfields.Marshal(plain(v), v.Extra)
}

func (v TagFilter) MarshalYAML() (any, error) { return format.YAMLNodeFromJSON(v) }

func (v *DtLabelMapping) UnmarshalJSON(data []byte) error {
	type plain DtLabelMapping
	extra, err := unknownfields.Unmarshal(data, (*plain)(v))
	if err != nil {
		return err
	}
	v.Extra = extra
	return nil
}

func (v DtLabelMapping) MarshalJSON() ([]byte, error) {
	type plain DtLabelMapping
	return unknownfields.Marshal(plain(v), v.Extra)
}

func (v DtLabelMapping) MarshalYAML() (any, error) { return format.YAMLNodeFromJSON(v) }

func (v *CustomNamespace) UnmarshalJSON(data []byte) error {
	type plain CustomNamespace
	extra, err := unknownfields.Unmarshal(data, (*plain)(v))
	if err != nil {
		return err
	}
	v.Extra = extra
	return nil
}

func (v CustomNamespace) MarshalJSON() ([]byte, error) {
	type plain CustomNamespace
	return unknownfields.Marshal(plain(v), v.Extra)
}

func (v CustomNamespace) MarshalYAML() (any, error) { return format.YAMLNodeFromJSON(v) }

func (v *CustomMetric) UnmarshalJSON(data []byte) error {
	type plain CustomMetric
	extra, err := unknownfields.Unmarshal(data, (*plain)(v))
	if err != nil {
		return err
	}
	v.Extra = extra
	return nil
}

func (v CustomMetric) MarshalJSON() ([]byte, error) {
	type plain CustomMetric
	return unknownfields.Marshal(plain(v), v.Extra)
}

func (v CustomMetric) MarshalYAML() (any, error) { return format.YAMLNodeFromJSON(v) }

// MarshalYAML renders the configuration through its JSON shape, so `-o yaml`
// carries the same camelCase keys and unmodelled members as `-o json`.
func (c AWSMonitoringConfig) MarshalYAML() (any, error) { return format.YAMLNodeFromJSON(c) }

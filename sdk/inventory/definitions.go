package inventory

import (
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

// BuiltinDefinitions returns the capability set dtctl ships with. It is
// deliberately structural-only (data-object / census / metric-catalog shapes):
// cheap to evaluate, deterministic, and strong on the negative side. Probe
// shapes are fully supported for user-supplied definitions, where the owner
// can weigh their cost and their weak negatives.
func BuiltinDefinitions() map[string]*CapabilityDef {
	return map[string]*CapabilityDef{
		// topology
		"hosts": {EntityTypes: []string{"HOST"}},
		"k8s":   {EntityTypes: []string{"K8S_*"}},
		"aws":   {EntityTypes: []string{"AWS_*"}},
		"azure": {EntityTypes: []string{"AZURE_*"}},
		"gcp":   {EntityTypes: []string{"GCP_*"}},
		// signal streams
		"spans":        {DataObject: "spans"},
		"logs":         {DataObject: "logs"},
		"bizevents":    {DataObject: "bizevents"},
		"rum":          {DataObject: "user.events"},
		"davis":        {DataObject: "dt.davis.problems"},
		"davis-events": {DataObject: "dt.davis.events"},
		"security":     {DataObject: "security.events"},
		"synthetic":    {DataObject: "dt.synthetic.events"},
		// metric families
		"host-metrics":    {MetricKey: "dt.host.*"},
		"process-metrics": {MetricKey: "dt.process.*"},
		"k8s-metrics":     {MetricKey: "dt.kubernetes.*"},
		"service-metrics": {MetricKey: "dt.service.*"},
		"aws-cloudwatch":  {MetricKey: "cloud.aws.*"},
	}
}

// ParseDefinitions parses and validates one capability-definitions document
// (the SDK never reads files — the caller supplies the bytes and owns path
// context in errors).
func ParseDefinitions(data []byte) (*Definitions, error) {
	var defs Definitions
	if err := yaml.Unmarshal(data, &defs); err != nil {
		return nil, fmt.Errorf("failed to parse definitions: %w", err)
	}
	if defs.Kind != "" && defs.Kind != KindDefinitions {
		return nil, fmt.Errorf("kind is %q, expected %q", defs.Kind, KindDefinitions)
	}
	for name, def := range defs.Capabilities {
		if def == nil {
			continue // explicit null: removes the capability on merge
		}
		if err := validateDef(def); err != nil {
			return nil, fmt.Errorf("capability %q: %w", name, err)
		}
	}
	return &defs, nil
}

// MergeDefinitions overlays each definition set over the base in order: a
// later definition replaces an earlier one of the same name, and an explicit
// null removes it.
func MergeDefinitions(base map[string]*CapabilityDef, overlays ...*Definitions) map[string]*CapabilityDef {
	merged := make(map[string]*CapabilityDef, len(base))
	for name, def := range base {
		merged[name] = def
	}
	for _, o := range overlays {
		if o == nil {
			continue
		}
		for name, def := range o.Capabilities {
			if def == nil {
				delete(merged, name)
				continue
			}
			merged[name] = def
		}
	}
	return merged
}

// ValidateDefinitions checks a definition set the same way ParseDefinitions
// checks a parsed document: every definition must declare exactly one
// discovery shape, and probe shapes must declare their evidence window. It
// exists for sets constructed in Go rather than parsed — Discover runs it up
// front, so a malformed definition fails fast instead of being silently
// skipped. One asymmetry to ParseDefinitions: a nil definition is an error
// here, because nil-as-removal is meaningful only inside an overlay handed to
// MergeDefinitions, never in a merged set.
func ValidateDefinitions(defs map[string]*CapabilityDef) error {
	names := make([]string, 0, len(defs))
	for name := range defs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		def := defs[name]
		if def == nil {
			return fmt.Errorf("capability %q: definition is nil (null removes a capability only in a MergeDefinitions overlay)", name)
		}
		if err := validateDef(def); err != nil {
			return fmt.Errorf("capability %q: %w", name, err)
		}
	}
	return nil
}

// validateDef enforces the exactly-one-shape contract, and that probe shapes
// declare their evidence window.
func validateDef(def *CapabilityDef) error {
	shapes := 0
	if def.DataObject != "" {
		shapes++
	}
	if len(def.EntityTypes) > 0 {
		shapes++
	}
	if def.MetricKey != "" {
		shapes++
	}
	if def.Probe != "" {
		shapes++
	}
	if shapes != 1 {
		return fmt.Errorf("exactly one discovery shape (dataObject | entityTypes | metricKey | probe) must be set, got %d", shapes)
	}
	if def.Probe != "" && def.Window == "" {
		return fmt.Errorf("probe-shaped definitions must declare window (the evidence window the probe covers)")
	}
	if def.Probe == "" && def.Window != "" {
		return fmt.Errorf("window is only valid with a probe shape")
	}
	return nil
}

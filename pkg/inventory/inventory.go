// Package inventory probes a Dynatrace environment for what data actually
// exists there: which Grail data objects are fetchable, which buckets and
// entity types are present, which capabilities (spans, RUM, k8s, cloud
// integrations, …) are backed by live evidence — and which are absent, with
// the evidence cited. The capability set is defined declaratively and is
// user-extensible (see Definitions).
//
// Everything here is read-only: discovery runs a small, budgeted battery of
// DQL queries and returns a report; nothing is persisted.
package inventory

// APIVersion / KindDefinitions identify a capability-definitions file.
const (
	APIVersion      = "dtctl.dev/v1alpha1"
	KindDefinitions = "InventoryDefinitions"
)

// CapabilityDef defines a capability by how it is discovered — one of four
// fixed shapes, deliberately not an expression language:
//
//   - DataObject: the named object exists in the dt.system.data_objects catalog
//   - EntityTypes: at least one entity of a matching type (glob patterns) is in
//     the live topology census
//   - MetricKey: at least one key matching the glob is in the metric catalog
//   - Probe (+ Window): a DQL probe returns at least one row — weak evidence,
//     since absence of events is not absence of capability
//
// Structural shapes (the first three) are preferred: they are cheap, and their
// negatives are strong. Exactly one shape must be set.
type CapabilityDef struct {
	DataObject  string   `json:"dataObject,omitempty" yaml:"dataObject,omitempty"`
	EntityTypes []string `json:"entityTypes,omitempty" yaml:"entityTypes,omitempty"`
	MetricKey   string   `json:"metricKey,omitempty" yaml:"metricKey,omitempty"`
	Probe       string   `json:"probe,omitempty" yaml:"probe,omitempty"`
	Window      string   `json:"window,omitempty" yaml:"window,omitempty"`
}

// Definitions is the on-disk customization format: a named set of capability
// definitions merged over (or replacing) the built-in set. A capability mapped
// to null removes it from the merged set.
type Definitions struct {
	APIVersion   string                    `yaml:"apiVersion,omitempty"`
	Kind         string                    `yaml:"kind,omitempty"`
	Capabilities map[string]*CapabilityDef `yaml:"capabilities"`
}

// SegmentInfo is a Grail filter segment present on the environment.
type SegmentInfo struct {
	UID         string `json:"uid" yaml:"uid"`
	Name        string `json:"name,omitempty" yaml:"name,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

// CapabilityStatus names a capability together with the evidence for its
// verdict: for an absent capability, what was checked and came up empty; for
// an unknown one, why no check actually produced a verdict.
type CapabilityStatus struct {
	Name     string `json:"name" yaml:"name"`
	Evidence string `json:"evidence" yaml:"evidence"`
}

// Inventory is the discovery result: what data is available on the
// environment, with negative findings carried as structured evidence.
type Inventory struct {
	Context     string `json:"context,omitempty" yaml:"context,omitempty"`
	GeneratedAt string `json:"generatedAt" yaml:"generatedAt"`
	// Capabilities that discovery found present. Absent capabilities cite what
	// was checked ("no user.events in the data-object catalog"), so the
	// negative is usable without re-probing. Unknown capabilities got no
	// verdict — the probe failed, the budget ran out, or the fact source they
	// evaluate against was unavailable — and must not be read as absent.
	Capabilities []string           `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
	Absent       []CapabilityStatus `json:"absent,omitempty" yaml:"absent,omitempty"`
	Unknown      []CapabilityStatus `json:"unknown,omitempty" yaml:"unknown,omitempty"`
	// EntityTypes is the live topology census: entity type → count.
	EntityTypes map[string]int64 `json:"entityTypes,omitempty" yaml:"entityTypes,omitempty"`
	// DataObjects are catalog objects that support fetch; QueryOnly are
	// catalog objects that are queried through other commands instead
	// (metrics → timeseries, smartscape.* → smartscapeNodes/smartscapeEdges).
	DataObjects []string      `json:"dataObjects,omitempty" yaml:"dataObjects,omitempty"`
	QueryOnly   []string      `json:"queryOnly,omitempty" yaml:"queryOnly,omitempty"`
	Buckets     []string      `json:"buckets,omitempty" yaml:"buckets,omitempty"`
	Segments    []SegmentInfo `json:"segments,omitempty" yaml:"segments,omitempty"`
	// Notes carry cross-cutting facts about how this environment's data is
	// queried (canonical streams, catalog caveats).
	Notes []string `json:"notes,omitempty" yaml:"notes,omitempty"`
	// Discovery is the consumption receipt of the run that produced this
	// inventory.
	Discovery *Report `json:"discovery,omitempty" yaml:"discovery,omitempty"`
}

// Report is the consumption receipt of a discovery run.
type Report struct {
	Queries int      `json:"queries" yaml:"queries"`
	Seconds float64  `json:"seconds" yaml:"seconds"`
	Notes   []string `json:"notes,omitempty" yaml:"notes,omitempty"`
}

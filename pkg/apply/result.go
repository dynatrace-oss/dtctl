package apply

// ApplyResult is the interface implemented by all apply result types.
// Each resource type has its own concrete struct embedding ApplyResultBase.
type ApplyResult interface {
	applyResult() // unexported marker method — prevents external implementation
}

// ApplyResultBase contains fields common to all apply results.
// Embed this in every per-resource result type.
type ApplyResultBase struct {
	Action       string   `json:"action"       yaml:"action"       table:"ACTION"`
	ResourceType string   `json:"resourceType" yaml:"resourceType" table:"TYPE"`
	ID           string   `json:"id"           yaml:"id"           table:"ID"`
	Name         string   `json:"name,omitempty" yaml:"name,omitempty" table:"NAME"`
	Warnings     []string `json:"warnings,omitempty" yaml:"warnings,omitempty" table:"-"`
}

func (ApplyResultBase) applyResult() {} // satisfies ApplyResult interface

// Action constants
const (
	ActionCreated   = "created"
	ActionUpdated   = "updated"
	ActionUnchanged = "unchanged"
)

// WorkflowApplyResult is the result of applying a workflow resource.
type WorkflowApplyResult struct {
	ApplyResultBase `yaml:",inline"`
}

// DashboardApplyResult is the result of applying a dashboard resource.
type DashboardApplyResult struct {
	ApplyResultBase `yaml:",inline"`
	URL             string `json:"url,omitempty"       yaml:"url,omitempty"       table:"URL,wide"`
	TileCount       int    `json:"tileCount,omitempty" yaml:"tileCount,omitempty" table:"TILES"`
}

// NotebookApplyResult is the result of applying a notebook resource.
type NotebookApplyResult struct {
	ApplyResultBase `yaml:",inline"`
	URL             string `json:"url,omitempty"          yaml:"url,omitempty"          table:"URL,wide"`
	SectionCount    int    `json:"sectionCount,omitempty" yaml:"sectionCount,omitempty" table:"SECTIONS"`
}

// SLOApplyResult is the result of applying an SLO resource.
type SLOApplyResult struct {
	ApplyResultBase `yaml:",inline"`
}

// BucketApplyResult is the result of applying a bucket resource.
type BucketApplyResult struct {
	ApplyResultBase `yaml:",inline"`
	Status          string `json:"status,omitempty" yaml:"status,omitempty" table:"STATUS"`
}

// SettingsApplyResult is the result of applying a settings object.
type SettingsApplyResult struct {
	ApplyResultBase `yaml:",inline"`
	SchemaID        string `json:"schemaId,omitempty" yaml:"schemaId,omitempty" table:"SCHEMA"`
	Scope           string `json:"scope,omitempty"    yaml:"scope,omitempty"    table:"SCOPE"`
	Summary         string `json:"summary,omitempty"  yaml:"summary,omitempty"  table:"-"`
}

// ConnectionApplyResult is the result of applying a cloud connection (Azure or GCP).
type ConnectionApplyResult struct {
	ApplyResultBase `yaml:",inline"`
	SchemaID        string `json:"schemaId,omitempty" yaml:"schemaId,omitempty" table:"SCHEMA"`
	Scope           string `json:"scope,omitempty"    yaml:"scope,omitempty"    table:"SCOPE"`
}

// MonitoringConfigApplyResult is the result of applying a monitoring config (Azure or GCP).
type MonitoringConfigApplyResult struct {
	ApplyResultBase `yaml:",inline"`
	Scope           string `json:"scope,omitempty" yaml:"scope,omitempty" table:"SCOPE"`
}

# Future Features Implementation Plan

## Overview

This document outlines the implementation plan for adding new API categories to dtctl:
1. Platform Management
2. State Management for Apps
3. Grail Filter Segments
4. Grail Fieldsets
5. Grail Resource Store

## Implementation Order

We'll implement in this order (simple to complex):

1. **Platform Management** - Read-only, simple endpoints
2. **State Management** - Delete operations only, simple
3. **Grail Fieldsets** - Standard CRUD pattern
4. **Grail Filter Segments** - Standard CRUD pattern
5. **Grail Resource Store** - Standard CRUD pattern

## 1. Platform Management

**API Spec:** `platform-management.yaml`

### Endpoints to Implement

- `GET /platform/management/v1/environment` - Get environment info
- `GET /platform/management/v1/environment/license` - Get license info

### Commands

```bash
# Get environment information
dtctl get environment
dtctl describe environment

# Get license information
dtctl get license
dtctl describe license
```

### Files to Create/Modify

- `pkg/resources/platform/platform.go` - Handler implementation
- `cmd/get.go` - Add commands: getEnvironmentCmd, getLicenseCmd
- `cmd/describe.go` - Add describe commands for detailed view

### Data Structures

```go
type EnvironmentInfo struct {
    ID          string `json:"id"`
    Name        string `json:"name"`
    Region      string `json:"region"`
    Trial       bool   `json:"trial"`
    // ... other fields from API spec
}

type License struct {
    Type           string    `json:"type"`
    ExpirationDate time.Time `json:"expirationDate"`
    MaxDemUnits    int       `json:"maxDemUnits"`
    // ... other fields from API spec
}
```

### Scope Required
- `app-engine:apps:run` OR `app-engine:functions:run`

---

## 2. State Management for Apps

**API Spec:** `state-management.yaml`

### Endpoints to Implement

- `DELETE /platform/state-management/v1/{appId}/app-states` - Delete all app states
- `DELETE /platform/state-management/v1/{appId}/user-app-states` - Delete all user app states
- `DELETE /platform/state-management/v1/{appId}/user-app-states/self` - Delete own user app states

### Commands

```bash
# Delete all app states for an app
dtctl delete app-state <app-id>

# Delete all user app states for an app (admin)
dtctl delete user-app-states <app-id>

# Delete own user app states for an app
dtctl delete user-app-states <app-id> --self
```

### Files to Create/Modify

- `pkg/resources/statemanagement/statemanagement.go` - Handler implementation
- `cmd/get.go` - No get commands (delete only)
- `cmd/delete.go` - Add delete commands

### Scopes Required
- `state-management:app-states:delete`
- `state-management:user-app-states:delete-all`
- `state-management:user-app-states:delete`

---

## 3. Grail Fieldsets

**API Spec:** `grail-fieldsets.yaml`

### Endpoints to Implement

- `GET /platform/storage/management/v1/fieldsets` - List fieldsets
- `GET /platform/storage/management/v1/fieldsets/{fieldsetName}` - Get fieldset
- `POST /platform/storage/management/v1/fieldsets` - Create fieldset
- `PUT /platform/storage/management/v1/fieldsets/{fieldsetName}` - Update fieldset
- `DELETE /platform/storage/management/v1/fieldsets/{fieldsetName}` - Delete fieldset

### Commands

```bash
# List all fieldsets
dtctl get fieldsets

# Get a specific fieldset
dtctl get fieldset <fieldset-name>
dtctl describe fieldset <fieldset-name>

# Create a fieldset from YAML
dtctl create fieldset -f fieldset.yaml
dtctl apply -f fieldset.yaml

# Edit a fieldset
dtctl edit fieldset <fieldset-name>

# Delete a fieldset
dtctl delete fieldset <fieldset-name>
```

### Files to Create/Modify

- `pkg/resources/grail/fieldsets.go` - Handler implementation
- `cmd/get.go` - Add getFieldsetsCmd
- `cmd/describe.go` - Add describeFieldsetCmd
- `cmd/create.go` - Add createFieldsetCmd
- `cmd/edit.go` - Add editFieldsetCmd
- `cmd/delete.go` - Add deleteFieldsetCmd
- `cmd/apply.go` - Add fieldset support

### Data Structures

```go
type Fieldset struct {
    Name        string   `json:"name" yaml:"name"`
    DisplayName string   `json:"displayName,omitempty" yaml:"displayName,omitempty"`
    Description string   `json:"description,omitempty" yaml:"description,omitempty"`
    Fields      []string `json:"fields" yaml:"fields"`
    // ... other fields from API spec
}
```

### Scopes Required
- `storage:fieldsets:read`
- `storage:fieldsets:write`
- `storage:fieldsets:delete`

---

## 4. Grail Filter Segments

**API Spec:** `grail-filter-segments.yaml`

### Endpoints to Implement

- `GET /platform/storage/management/v1/filter-segments` - List segments
- `GET /platform/storage/management/v1/filter-segments/{segmentName}` - Get segment
- `POST /platform/storage/management/v1/filter-segments` - Create segment
- `PUT /platform/storage/management/v1/filter-segments/{segmentName}` - Update segment
- `DELETE /platform/storage/management/v1/filter-segments/{segmentName}` - Delete segment

### Commands

```bash
# List all filter segments
dtctl get filter-segments
dtctl get segments

# Get a specific segment
dtctl get segment <segment-name>
dtctl describe segment <segment-name>

# Create a segment from YAML
dtctl create segment -f segment.yaml
dtctl apply -f segment.yaml

# Edit a segment
dtctl edit segment <segment-name>

# Delete a segment
dtctl delete segment <segment-name>
```

### Files to Create/Modify

- `pkg/resources/grail/segments.go` - Handler implementation
- `cmd/get.go` - Add getSegmentsCmd
- `cmd/describe.go` - Add describeSegmentCmd
- `cmd/create.go` - Add createSegmentCmd
- `cmd/edit.go` - Add editSegmentCmd
- `cmd/delete.go` - Add deleteSegmentCmd
- `cmd/apply.go` - Add segment support

### Data Structures

```go
type FilterSegment struct {
    Name        string `json:"name" yaml:"name"`
    DisplayName string `json:"displayName,omitempty" yaml:"displayName,omitempty"`
    Description string `json:"description,omitempty" yaml:"description,omitempty"`
    Filter      string `json:"filter" yaml:"filter"`
    // ... other fields from API spec
}
```

### Scopes Required
- `storage:filter-segments:read`
- `storage:filter-segments:write`
- `storage:filter-segments:delete`

---

## 5. Grail Resource Store

**API Spec:** `grail-resource-store.yaml`

### Endpoints to Implement

- `GET /platform/storage/management/v1/resources` - List resources
- `GET /platform/storage/management/v1/resources/{resourceName}` - Get resource
- `POST /platform/storage/management/v1/resources` - Create resource
- `PUT /platform/storage/management/v1/resources/{resourceName}` - Update resource
- `DELETE /platform/storage/management/v1/resources/{resourceName}` - Delete resource

### Commands

```bash
# List all resources
dtctl get resources
dtctl get resource-store

# Get a specific resource
dtctl get resource <resource-name>
dtctl describe resource <resource-name>

# Create a resource from file
dtctl create resource -f resource.yaml
dtctl apply -f resource.yaml

# Edit a resource
dtctl edit resource <resource-name>

# Delete a resource
dtctl delete resource <resource-name>
```

### Files to Create/Modify

- `pkg/resources/grail/resourcestore.go` - Handler implementation
- `cmd/get.go` - Add getResourcesCmd
- `cmd/describe.go` - Add describeResourceCmd
- `cmd/create.go` - Add createResourceCmd
- `cmd/edit.go` - Add editResourceCmd
- `cmd/delete.go` - Add deleteResourceCmd
- `cmd/apply.go` - Add resource support

### Data Structures

```go
type Resource struct {
    Name        string                 `json:"name" yaml:"name"`
    DisplayName string                 `json:"displayName,omitempty" yaml:"displayName,omitempty"`
    Description string                 `json:"description,omitempty" yaml:"description,omitempty"`
    Content     map[string]interface{} `json:"content" yaml:"content"`
    Version     string                 `json:"version,omitempty" yaml:"version,omitempty"`
    // ... other fields from API spec
}
```

### Scopes Required
- `storage:resources:read`
- `storage:resources:write`
- `storage:resources:delete`

---

## Testing Strategy

For each feature:

1. **Unit Tests** - Test handlers in isolation
2. **Integration Tests** - Test against real Dynatrace environment (optional, in `test/integration/`)
3. **E2E Tests** - Test CLI commands end-to-end (in `test/e2e/`)

### Test Files to Create

- `pkg/resources/platform/platform_test.go`
- `pkg/resources/statemanagement/statemanagement_test.go`
- `pkg/resources/grail/fieldsets_test.go`
- `pkg/resources/grail/segments_test.go`
- `pkg/resources/grail/resourcestore_test.go`

---

## Documentation Updates

### Files to Update

1. **README.md** - Add new resources to "What Can It Do?" table
2. **QUICK_START.md** - Add examples for new resources
3. **API_DESIGN.md** - Document new commands and flags
4. **IMPLEMENTATION_STATUS.md** - Mark features as complete

### New Documentation

Create usage examples for each resource type with common workflows.

---

## Implementation Checklist

### Phase 1: Simple Read-Only Resources (Day 1)
- [ ] Platform Management implementation
- [ ] Platform Management commands
- [ ] Platform Management tests
- [ ] Platform Management documentation

### Phase 2: Simple Delete-Only Resources (Day 1)
- [ ] State Management implementation
- [ ] State Management commands
- [ ] State Management tests
- [ ] State Management documentation

### Phase 3: Grail CRUD Resources (Day 2-3)
- [ ] Grail Fieldsets implementation
- [ ] Grail Fieldsets commands
- [ ] Grail Fieldsets tests
- [ ] Grail Filter Segments implementation
- [ ] Grail Filter Segments commands
- [ ] Grail Filter Segments tests
- [ ] Grail Resource Store implementation
- [ ] Grail Resource Store commands
- [ ] Grail Resource Store tests
- [ ] Grail resources documentation

### Phase 4: Final Polish (Day 4)
- [ ] Update all documentation
- [ ] Integration tests
- [ ] E2E tests
- [ ] Shell completion updates
- [ ] Code review and refactoring

---

## Command Naming Conventions

Following kubectl conventions:

- **Singular for get specific**: `dtctl get environment`, `dtctl get license`
- **Plural for list**: `dtctl get fieldsets`, `dtctl get segments`
- **Short aliases**: `seg` for segments, `fs` for fieldsets
- **Consistent verbs**: get, describe, create, edit, delete, apply

---

## Code Structure Guidelines

### Handler Interface Pattern

All resource handlers should implement common patterns:

```go
type Handler struct {
    client *client.Client
}

func NewHandler(c *client.Client) *Handler {
    return &Handler{client: c}
}

func (h *Handler) List() ([]Resource, error) { ... }
func (h *Handler) Get(id string) (*Resource, error) { ... }
func (h *Handler) Create(resource *Resource) (*Resource, error) { ... }
func (h *Handler) Update(id string, resource *Resource) (*Resource, error) { ... }
func (h *Handler) Delete(id string) error { ... }
```

### Error Handling

Use the existing error patterns from `pkg/client/errors.go`

### Output Formatting

Support all existing output formats:
- Table (default)
- JSON (`-o json`)
- YAML (`-o yaml`)
- Wide (`-o wide`)

### Configuration

Support global flags:
- `--context` - Select environment
- `--output` - Output format
- `--verbose` - Verbose output
- `--dry-run` - Dry run mode
- `--chunk-size` - Pagination

---

## Notes

- All new features require corresponding API tokens with appropriate scopes
- Grail resources share common patterns, implement one as template for others
- State Management is destructive (delete-only), add appropriate confirmations
- Platform Management is read-only, simplest to implement first

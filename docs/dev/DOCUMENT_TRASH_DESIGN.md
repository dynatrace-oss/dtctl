# Document Trash Design Proposal

**Status:** Implemented ✅ (Core functionality only)
**Created:** 2026-02-02
**Implemented:** 2026-02-02
**Author:** dtctl team

> **Implementation Note:** This document was written as a design proposal. The final implementation is based on the actual Dynatrace Platform API, which provides a subset of the proposed functionality. Features like "empty trash", expiration time display, and filtering by document owner are NOT supported by the API and therefore not implemented. See the "Actual Implementation" section below for what was actually built.

## Overview

Document trash functionality allows users to list, restore, and permanently delete dashboards and notebooks that have been soft-deleted. This provides a safety net for accidental deletions and enables recovery workflows.

## Goals

1. **List trashed documents** - View deleted dashboards and notebooks ✅
2. **Restore documents** - Recover accidentally deleted items ✅
3. **Permanent deletion** - Delete specific items ✅ (Note: "Empty trash" is NOT supported by API)
4. **Retention awareness** - Show deletion info ✅ (Note: Expiration time is NOT exposed by API)
5. **Bulk operations** - Restore or delete multiple items ✅

## Non-Goals

- Trash for non-document resources (workflows, SLOs, etc.)
- Version history in trash (use `history` command)
- Trash size limits or quotas
- Cross-environment trash management

---

## Actual Implementation

Based on the Dynatrace Platform API (`/platform/document/v1/trash/documents`), the following features are **implemented**:

### ✅ Supported Features

- **List trashed documents**: `dtctl get trash` with filters:
  - `--type` (dashboard, notebook)
  - `--deleted-by` (filter by user who deleted)
  - `--deleted-after` / `--deleted-before` (filter by deletion date)
- **Get trash details**: `dtctl describe trash <id>`
- **Restore documents**: `dtctl restore trash <id> [<id>...]`
- **Permanent deletion**: `dtctl delete trash <id> [<id>...] --permanent`
- **Watch mode**: `dtctl get trash --watch`
- **All output formats**: table, JSON, YAML, CSV

### ❌ NOT Supported by API (Not Implemented)

- **Empty trash** - No API endpoint exists for bulk trash deletion
- **Expiration time display** - API does not expose when documents expire
- **Filter by owner** (`--mine`, `--owner` flags) - API only supports `deletedBy` filter
- **Trash size/metadata** - API does not expose document sizes or detailed metadata in trash

### API Structure

The actual API provides:
- `GET /platform/document/v1/trash/documents` - List trash (with filter support)
- `GET /platform/document/v1/trash/documents/{id}` - Get trash details
- `POST /platform/document/v1/trash/documents/{id}/restore` - Restore document
- `DELETE /platform/document/v1/trash/documents/{id}` - Permanently delete

**Data fields available:**
- `id`, `name`, `type` (dashboard/notebook)
- `deletionInfo`: `deletedBy` (user email), `deletedTime` (timestamp)
- `version`, `owner` (in detailed view)
- `modificationInfo` (last modified time/user, in detailed view)

---

## User Experience (Proposed Design)

### Basic Usage

```bash
# List all trashed documents
dtctl get trash
dtctl get trash --type dashboard
dtctl get trash --type notebook

# Show detailed trash info
dtctl describe trash <document-id>

# Restore a document
dtctl restore trash <document-id>
dtctl restore dashboard <document-id>  # Alternative syntax

# Permanently delete
dtctl delete trash <document-id> --permanent
dtctl empty trash  # Delete all trashed items

# Restore multiple documents
dtctl restore trash <id1> <id2> <id3>

# Filter trash
dtctl get trash --mine
dtctl get trash --deleted-after 2024-01-01
dtctl get trash --deleted-before 2024-12-31
```

### Output Examples

**List trash (table format):**
```
ID                                    TYPE        TITLE                DELETED BY    DELETED AT           EXPIRES IN
aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee  dashboard   Production Overview  john.doe      2024-01-15 10:30:00  29 days
ffffffff-gggg-hhhh-iiii-jjjjjjjjjjjj  notebook    Debug Session        jane.smith    2024-01-20 14:45:00  24 days
```

**Describe trash item:**
```yaml
id: aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee
type: dashboard
title: Production Overview
owner: john.doe@example.com
deletedBy: john.doe@example.com
deletedAt: 2024-01-15T10:30:00Z
expiresAt: 2024-02-14T10:30:00Z
daysUntilExpiration: 29
size: 45KB
metadata:
  originalPath: /dashboards/production
  tags:
    - production
    - monitoring
  lastModified: 2024-01-10T08:20:00Z
```

**Restore confirmation:**
```
Restoring dashboard: Production Overview (aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee)
✓ Dashboard restored successfully
View at: https://your-env.apps.dynatrace.com/ui/dashboards/aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee
```

---

## Technical Design

### API Endpoints

Based on `document.yaml` API spec:

```
GET    /platform/document/v1/documents:trash           # List trashed documents
GET    /platform/document/v1/documents/{id}:trash      # Get trashed document details
POST   /platform/document/v1/documents/{id}:restore    # Restore document
DELETE /platform/document/v1/documents/{id}:purge      # Permanently delete
POST   /platform/document/v1/documents:emptyTrash      # Empty entire trash
```

### Command Structure

```go
// cmd/trash.go
var trashCmd = &cobra.Command{
    Use:     "trash",
    Aliases: []string{"deleted"},
    Short:   "Manage trashed documents (dashboards and notebooks)",
    Long: `List, restore, or permanently delete trashed dashboards and notebooks.
    
Documents are soft-deleted and kept in trash for 30 days before permanent deletion.`,
}

var getTrashCmd = &cobra.Command{
    Use:   "trash",
    Short: "List trashed documents",
    RunE:  runGetTrash,
}

var describeTrashCmd = &cobra.Command{
    Use:   "trash DOCUMENT_ID",
    Short: "Show details of a trashed document",
    Args:  cobra.ExactArgs(1),
    RunE:  runDescribeTrash,
}

var restoreTrashCmd = &cobra.Command{
    Use:     "trash DOCUMENT_ID [DOCUMENT_ID...]",
    Aliases: []string{"undelete"},
    Short:   "Restore trashed document(s)",
    Args:    cobra.MinimumNArgs(1),
    RunE:    runRestoreTrash,
}

var emptyTrashCmd = &cobra.Command{
    Use:   "trash",
    Short: "Permanently delete all trashed documents",
    RunE:  runEmptyTrash,
}

func init() {
    // Get trash flags
    getTrashCmd.Flags().String("type", "", "Filter by type: dashboard, notebook")
    getTrashCmd.Flags().Bool("mine", false, "Show only my deleted documents")
    getTrashCmd.Flags().String("deleted-after", "", "Show documents deleted after date")
    getTrashCmd.Flags().String("deleted-before", "", "Show documents deleted before date")
    getTrashCmd.Flags().String("owner", "", "Filter by original owner")
    getTrashCmd.Flags().String("deleted-by", "", "Filter by who deleted it")
    
    // Restore flags
    restoreTrashCmd.Flags().Bool("force", false, "Restore even if name conflicts exist")
    restoreTrashCmd.Flags().String("new-name", "", "Restore with a new name")
    
    // Delete flags
    deleteTrashCmd.Flags().Bool("permanent", false, "Permanently delete (required)")
    
    // Empty trash flags
    emptyTrashCmd.Flags().Bool("confirm", false, "Confirm emptying trash")
    emptyTrashCmd.Flags().String("older-than", "", "Only delete items older than duration (e.g., 7d)")
    
    // Add to root commands
    getCmd.AddCommand(getTrashCmd)
    describeCmd.AddCommand(describeTrashCmd)
    restoreCmd.AddCommand(restoreTrashCmd)
    deleteCmd.AddCommand(deleteTrashCmd)
    rootCmd.AddCommand(emptyTrashCmd)
}
```

### Data Structures

```go
// pkg/resources/document/trash.go
package document

type TrashedDocument struct {
    ID                   string                 `json:"id" yaml:"id"`
    Type                 DocumentType           `json:"type" yaml:"type"`
    Name                 string                 `json:"name" yaml:"name"`
    Owner                string                 `json:"owner" yaml:"owner"`
    DeletedBy            string                 `json:"deletedBy" yaml:"deletedBy"`
    DeletedAt            time.Time              `json:"deletedAt" yaml:"deletedAt"`
    ExpiresAt            time.Time              `json:"expiresAt" yaml:"expiresAt"`
    DaysUntilExpiration  int                    `json:"daysUntilExpiration" yaml:"daysUntilExpiration"`
    Size                 int64                  `json:"size,omitempty" yaml:"size,omitempty"`
    Content              map[string]interface{} `json:"content,omitempty" yaml:"content,omitempty"`
    Metadata             TrashMetadata          `json:"metadata" yaml:"metadata"`
}

type TrashMetadata struct {
    OriginalPath string            `json:"originalPath,omitempty" yaml:"originalPath,omitempty"`
    Tags         []string          `json:"tags,omitempty" yaml:"tags,omitempty"`
    LastModified time.Time         `json:"lastModified" yaml:"lastModified"`
    Version      int               `json:"version" yaml:"version"`
    Properties   map[string]string `json:"properties,omitempty" yaml:"properties,omitempty"`
}

type TrashListOptions struct {
    Type        DocumentType
    Owner       string
    DeletedBy   string
    DeletedAfter  time.Time
    DeletedBefore time.Time
    MineOnly    bool
}

type RestoreOptions struct {
    Force   bool
    NewName string
}

type EmptyTrashOptions struct {
    OlderThan time.Duration
    DryRun    bool
}
```

### Handler Implementation

```go
// pkg/resources/document/trash.go

type TrashHandler struct {
    client *client.Client
}

func NewTrashHandler(c *client.Client) *TrashHandler {
    return &TrashHandler{client: c}
}

func (h *TrashHandler) List(opts TrashListOptions) ([]TrashedDocument, error) {
    req := h.client.R().
        SetResult(&struct {
            Documents []TrashedDocument `json:"documents"`
        }{})
    
    // Apply filters
    if opts.Type != "" {
        req.SetQueryParam("type", string(opts.Type))
    }
    if opts.Owner != "" {
        req.SetQueryParam("owner", opts.Owner)
    }
    if opts.DeletedBy != "" {
        req.SetQueryParam("deletedBy", opts.DeletedBy)
    }
    if !opts.DeletedAfter.IsZero() {
        req.SetQueryParam("deletedAfter", opts.DeletedAfter.Format(time.RFC3339))
    }
    if !opts.DeletedBefore.IsZero() {
        req.SetQueryParam("deletedBefore", opts.DeletedBefore.Format(time.RFC3339))
    }
    
    resp, err := req.Get("/platform/document/v1/documents:trash")
    if err != nil {
        return nil, fmt.Errorf("failed to list trash: %w", err)
    }
    
    result := resp.Result().(*struct {
        Documents []TrashedDocument `json:"documents"`
    })
    
    // Filter for --mine if needed
    if opts.MineOnly {
        currentUser, _ := h.getCurrentUser()
        filtered := []TrashedDocument{}
        for _, doc := range result.Documents {
            if doc.Owner == currentUser {
                filtered = append(filtered, doc)
            }
        }
        return filtered, nil
    }
    
    return result.Documents, nil
}

func (h *TrashHandler) Get(id string) (*TrashedDocument, error) {
    var doc TrashedDocument
    
    resp, err := h.client.R().
        SetResult(&doc).
        Get(fmt.Sprintf("/platform/document/v1/documents/%s:trash", id))
    
    if err != nil {
        return nil, fmt.Errorf("failed to get trashed document: %w", err)
    }
    
    if resp.StatusCode() == 404 {
        return nil, fmt.Errorf("document not found in trash: %s", id)
    }
    
    return &doc, nil
}

func (h *TrashHandler) Restore(id string, opts RestoreOptions) error {
    req := h.client.R()
    
    if opts.NewName != "" {
        req.SetBody(map[string]interface{}{
            "name": opts.NewName,
        })
    }
    
    if opts.Force {
        req.SetQueryParam("force", "true")
    }
    
    resp, err := req.Post(fmt.Sprintf("/platform/document/v1/documents/%s:restore", id))
    if err != nil {
        return fmt.Errorf("failed to restore document: %w", err)
    }
    
    if resp.StatusCode() == 409 {
        return fmt.Errorf("name conflict: document with same name exists (use --force or --new-name)")
    }
    
    return nil
}

func (h *TrashHandler) Delete(id string) error {
    resp, err := h.client.R().
        Delete(fmt.Sprintf("/platform/document/v1/documents/%s:purge", id))
    
    if err != nil {
        return fmt.Errorf("failed to permanently delete document: %w", err)
    }
    
    if resp.StatusCode() == 404 {
        return fmt.Errorf("document not found in trash: %s", id)
    }
    
    return nil
}

func (h *TrashHandler) Empty(opts EmptyTrashOptions) (int, error) {
    req := h.client.R()
    
    if !opts.OlderThan.IsZero() {
        cutoffDate := time.Now().Add(-opts.OlderThan)
        req.SetQueryParam("olderThan", cutoffDate.Format(time.RFC3339))
    }
    
    if opts.DryRun {
        req.SetQueryParam("dryRun", "true")
    }
    
    var result struct {
        DeletedCount int `json:"deletedCount"`
    }
    
    resp, err := req.
        SetResult(&result).
        Post("/platform/document/v1/documents:emptyTrash")
    
    if err != nil {
        return 0, fmt.Errorf("failed to empty trash: %w", err)
    }
    
    return result.DeletedCount, nil
}

func (h *TrashHandler) getCurrentUser() (string, error) {
    // Use existing auth whoami logic
    return auth.GetCurrentUser(h.client)
}
```

---

## Safety Features

### Confirmation Prompts

```go
// cmd/trash.go

func runRestoreTrash(cmd *cobra.Command, args []string) error {
    force, _ := cmd.Flags().GetBool("force")
    
    if !force && len(args) > 5 {
        fmt.Printf("About to restore %d documents. Continue? [y/N]: ", len(args))
        if !confirmAction() {
            return fmt.Errorf("restore cancelled")
        }
    }
    
    // Proceed with restore
}

func runEmptyTrash(cmd *cobra.Command, args []string) error {
    confirm, _ := cmd.Flags().GetBool("confirm")
    
    if !confirm {
        fmt.Println("WARNING: This will permanently delete all trashed documents.")
        fmt.Print("Type 'empty trash' to confirm: ")
        
        var input string
        fmt.Scanln(&input)
        
        if input != "empty trash" {
            return fmt.Errorf("operation cancelled")
        }
    }
    
    // Proceed with empty
}

func runDeleteTrash(cmd *cobra.Command, args []string) error {
    permanent, _ := cmd.Flags().GetBool("permanent")
    
    if !permanent {
        return fmt.Errorf("--permanent flag is required to delete from trash")
    }
    
    // Proceed with delete
}
```

### Safety Checks

```go
// pkg/safety/trash.go

func (c *SafetyChecker) CheckTrashOperation(op Operation, docCount int) error {
    switch c.context.SafetyLevel {
    case SafetyLevelReadonly:
        return fmt.Errorf("trash operations not allowed in readonly context")
        
    case SafetyLevelReadwriteMine:
        // Allow restore/delete of own documents only
        if op == OperationRestore || op == OperationDelete {
            return nil
        }
        return fmt.Errorf("empty trash not allowed in readwrite-mine context")
        
    case SafetyLevelReadwriteAll:
        // Allow all operations with confirmation
        if op == OperationEmptyTrash && docCount > 10 {
            return fmt.Errorf("emptying trash with %d items requires dangerously-unrestricted context", docCount)
        }
        return nil
        
    case SafetyLevelDangerouslyUnrestricted:
        return nil
    }
    
    return fmt.Errorf("unknown safety level")
}
```

---

## Use Cases

### 1. Accidental Deletion Recovery

```bash
# User accidentally deletes dashboard
dtctl delete dashboard prod-overview
# Realizes mistake

# List trash to find it
dtctl get trash --mine

# Restore it
dtctl restore trash aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee
```

### 2. Bulk Cleanup

```bash
# List old trashed items
dtctl get trash --deleted-before 2024-01-01

# Empty trash of old items
dtctl empty trash --older-than 30d --confirm

# Or delete specific items
dtctl delete trash <id1> <id2> <id3> --permanent
```

### 3. Audit Trail

```bash
# See who deleted what
dtctl get trash --deleted-by john.doe -o wide

# Get details of specific deletion
dtctl describe trash aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee
```

### 4. Name Conflict Resolution

```bash
# Restore with new name if conflict
dtctl restore trash <id> --new-name "Production Overview (Restored)"

# Or force overwrite
dtctl restore trash <id> --force
```

### 5. Scheduled Cleanup

```bash
# CI/CD job to clean up old trash
#!/bin/bash
# Delete items older than 25 days (before auto-deletion at 30)
dtctl empty trash --older-than 25d --confirm

# Or dry-run first
dtctl empty trash --older-than 25d --dry-run
```

---

## Output Formatting

### Table Format

```go
// pkg/output/trash.go

func (p *TablePrinter) PrintTrash(docs []document.TrashedDocument) error {
    table := tablewriter.NewWriter(p.writer)
    table.SetHeader([]string{"ID", "Type", "Title", "Deleted By", "Deleted At", "Expires In"})
    
    for _, doc := range docs {
        expiresIn := formatDuration(time.Until(doc.ExpiresAt))
        
        row := []string{
            doc.ID,
            string(doc.Type),
            doc.Name,
            doc.DeletedBy,
            doc.DeletedAt.Format("2006-01-02 15:04:05"),
            expiresIn,
        }
        
        table.Append(row)
    }
    
    table.Render()
    return nil
}

func formatDuration(d time.Duration) string {
    days := int(d.Hours() / 24)
    if days > 0 {
        return fmt.Sprintf("%d days", days)
    }
    hours := int(d.Hours())
    if hours > 0 {
        return fmt.Sprintf("%d hours", hours)
    }
    return "< 1 hour"
}
```

### Wide Format

```go
func (p *TablePrinter) PrintTrashWide(docs []document.TrashedDocument) error {
    table := tablewriter.NewWriter(p.writer)
    table.SetHeader([]string{
        "ID", "Type", "Title", "Owner", "Deleted By", 
        "Deleted At", "Expires At", "Size", "Tags",
    })
    
    for _, doc := range docs {
        row := []string{
            doc.ID,
            string(doc.Type),
            doc.Name,
            doc.Owner,
            doc.DeletedBy,
            doc.DeletedAt.Format(time.RFC3339),
            doc.ExpiresAt.Format(time.RFC3339),
            formatSize(doc.Size),
            strings.Join(doc.Metadata.Tags, ", "),
        }
        
        table.Append(row)
    }
    
    table.Render()
    return nil
}
```

---

## Error Handling

### Document Not in Trash

```go
if err := handler.Get(id); err != nil {
    if errors.Is(err, ErrNotFound) {
        // Check if document exists but not in trash
        if doc, err := handler.GetDocument(id); err == nil {
            return fmt.Errorf("document %s exists but is not in trash", id)
        }
        return fmt.Errorf("document %s not found", id)
    }
    return err
}
```

### Expired Document

```go
func (h *TrashHandler) Restore(id string, opts RestoreOptions) error {
    doc, err := h.Get(id)
    if err != nil {
        return err
    }
    
    if time.Now().After(doc.ExpiresAt) {
        return fmt.Errorf("document has expired and been permanently deleted")
    }
    
    // Proceed with restore
}
```

### Name Conflicts

```go
func (h *TrashHandler) Restore(id string, opts RestoreOptions) error {
    // ... restore logic
    
    if resp.StatusCode() == 409 {
        return &NameConflictError{
            DocumentID: id,
            Name:       doc.Name,
            Suggestion: fmt.Sprintf("Use --new-name or --force flag"),
        }
    }
}
```

---

## Testing Strategy

### Unit Tests

```go
// pkg/resources/document/trash_test.go

func TestTrashHandler_List(t *testing.T) {
    tests := []struct {
        name     string
        opts     TrashListOptions
        expected int
    }{
        {
            name: "list all",
            opts: TrashListOptions{},
            expected: 5,
        },
        {
            name: "filter by type",
            opts: TrashListOptions{Type: DocumentTypeDashboard},
            expected: 3,
        },
        {
            name: "filter mine only",
            opts: TrashListOptions{MineOnly: true},
            expected: 2,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            handler := NewTrashHandler(mockClient)
            docs, err := handler.List(tt.opts)
            require.NoError(t, err)
            assert.Len(t, docs, tt.expected)
        })
    }
}

func TestTrashHandler_Restore(t *testing.T) {
    handler := NewTrashHandler(mockClient)
    
    // Test successful restore
    err := handler.Restore("test-id", RestoreOptions{})
    assert.NoError(t, err)
    
    // Test name conflict
    err = handler.Restore("conflict-id", RestoreOptions{})
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "name conflict")
    
    // Test force restore
    err = handler.Restore("conflict-id", RestoreOptions{Force: true})
    assert.NoError(t, err)
}
```

### Integration Tests

```go
// test/integration/trash_test.go

func TestTrash_EndToEnd(t *testing.T) {
    // Create and delete a dashboard
    dashboard := createTestDashboard(t)
    err := deleteDashboard(t, dashboard.ID)
    require.NoError(t, err)
    
    // List trash
    trash, err := listTrash(t)
    require.NoError(t, err)
    assert.Contains(t, trash, dashboard.ID)
    
    // Restore dashboard
    err = restoreTrash(t, dashboard.ID)
    require.NoError(t, err)
    
    // Verify restored
    restored, err := getDashboard(t, dashboard.ID)
    require.NoError(t, err)
    assert.Equal(t, dashboard.Name, restored.Name)
    
    // Cleanup
    deleteDashboard(t, dashboard.ID)
}
```

### E2E Tests

```bash
# test/e2e/trash_test.sh

# Create and delete dashboard
DASHBOARD_ID=$(dtctl create dashboard -f testdata/dashboard.yaml -o json | jq -r '.id')
dtctl delete dashboard $DASHBOARD_ID

# Verify in trash
dtctl get trash | grep -q $DASHBOARD_ID || exit 1

# Restore
dtctl restore trash $DASHBOARD_ID

# Verify restored
dtctl get dashboard $DASHBOARD_ID || exit 1

# Cleanup
dtctl delete dashboard $DASHBOARD_ID
```

---

## Performance Considerations

### Pagination

```go
func (h *TrashHandler) List(opts TrashListOptions) ([]TrashedDocument, error) {
    allDocs := []TrashedDocument{}
    pageSize := 100
    nextPageKey := ""
    
    for {
        req := h.client.R().
            SetQueryParam("pageSize", fmt.Sprintf("%d", pageSize))
        
        if nextPageKey != "" {
            req.SetQueryParam("nextPageKey", nextPageKey)
        }
        
        // ... fetch page
        
        allDocs = append(allDocs, page.Documents...)
        
        if page.NextPageKey == "" {
            break
        }
        nextPageKey = page.NextPageKey
    }
    
    return allDocs, nil
}
```

### Caching

```go
// Cache trash list for short duration
type TrashCache struct {
    documents []TrashedDocument
    timestamp time.Time
    ttl       time.Duration
}

func (c *TrashCache) Get() ([]TrashedDocument, bool) {
    if time.Since(c.timestamp) > c.ttl {
        return nil, false
    }
    return c.documents, true
}
```

---

## Documentation Requirements

### User Documentation

```markdown
# Trash Management

Manage deleted dashboards and notebooks in the trash.

## Retention Policy

Deleted documents are kept in trash for 30 days before permanent deletion.

## Commands

### List Trash
dtctl get trash [flags]

### Restore Document
dtctl restore trash DOCUMENT_ID [flags]

### Permanently Delete
dtctl delete trash DOCUMENT_ID --permanent

### Empty Trash
dtctl empty trash --confirm

## Examples

See QUICK_START.md for detailed examples.
```

---

## Success Metrics

- Trash operations work for dashboards and notebooks
- Restore success rate > 99%
- No data loss from trash operations
- Clear expiration warnings
- User adoption for recovery workflows
- Integration with backup/restore procedures

---

## References

- Dynatrace Document API: https://docs.dynatrace.com/docs/dynatrace-api/environment-api/document
- Trash/Recycle Bin UX patterns
- kubectl resource deletion and recovery

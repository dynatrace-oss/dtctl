//go:build integration
// +build integration

package e2e

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/resources/lookup"
	"github.com/dynatrace-oss/dtctl/test/integration"
)

// waitForLookupQueryable polls until the lookup can be queried or timeout is reached
func waitForLookupQueryable(t *testing.T, handler *lookup.Handler, path string, timeout time.Duration) error {
	t.Helper()
	deadline := time.Now().Add(timeout)
	pollInterval := 2 * time.Second

	for time.Now().Before(deadline) {
		_, err := handler.Get(path)
		if err == nil {
			return nil
		}

		// If error doesn't indicate "not found", it's a different error
		if !strings.Contains(err.Error(), "not found") && !strings.Contains(err.Error(), "doesn't exist") {
			return fmt.Errorf("failed to query lookup: %w", err)
		}

		t.Logf("Lookup %s not yet queryable, retrying in %v...", path, pollInterval)
		time.Sleep(pollInterval)
	}

	return fmt.Errorf("lookup %s did not become queryable within %v", path, timeout)
}

func TestLookupLifecycle(t *testing.T) {
	env := integration.SetupIntegration(t)
	defer env.Cleanup.Cleanup(t)

	handler := lookup.NewHandler(env.Client)

	// Generate unique lookup path
	lookupPath := fmt.Sprintf("/lookups/dtctl_test/%s/test_data", env.TestPrefix)
	t.Logf("Testing lookup lifecycle with path: %s", lookupPath)

	// Step 1: Create CSV data
	t.Log("Step 1: Creating test CSV data...")
	csvData := []byte(`code,message,severity
200,OK,info
400,Bad Request,error
401,Unauthorized,error
403,Forbidden,error
404,Not Found,warning
500,Internal Server Error,critical`)

	// Step 2: Create lookup table
	t.Log("Step 2: Creating lookup table...")
	createReq := lookup.CreateRequest{
		FilePath:    lookupPath,
		DisplayName: fmt.Sprintf("Test Lookup %s", env.TestPrefix),
		Description: "E2E test lookup table",
		LookupField: "code",
		DataContent: csvData,
		Overwrite:   false,
	}

	uploadResp, err := handler.Create(createReq)
	if err != nil {
		t.Fatalf("Failed to create lookup: %v", err)
	}
	if uploadResp.Records == 0 {
		t.Fatal("Created lookup has no records")
	}
	t.Logf("✓ Created lookup: %s", lookupPath)
	t.Logf("  Records: %d", uploadResp.Records)
	t.Logf("  File Size: %d bytes", uploadResp.FileSize)
	t.Logf("  Discarded Duplicates: %d", uploadResp.DiscardedDuplicates)

	// Track for cleanup
	env.Cleanup.Track("lookup", lookupPath, createReq.DisplayName)

	// Wait for lookup to be queryable
	t.Log("Waiting for lookup to become queryable...")
	if err := waitForLookupQueryable(t, handler, lookupPath, 30*time.Second); err != nil {
		t.Fatalf("Lookup did not become queryable: %v", err)
	}
	t.Log("✓ Lookup is queryable")

	// Step 3: Get lookup metadata
	t.Log("Step 3: Getting lookup metadata...")
	lu, err := handler.Get(lookupPath)
	if err != nil {
		t.Fatalf("Failed to get lookup: %v", err)
	}
	if lu.Path != lookupPath {
		t.Errorf("Get() path = %v, want %v", lu.Path, lookupPath)
	}
	if len(lu.Columns) != 3 {
		t.Errorf("Get() columns count = %v, want 3", len(lu.Columns))
	}
	t.Logf("✓ Got lookup metadata")
	t.Logf("  Columns: %v", lu.Columns)
	t.Logf("  Records: %d", lu.Records)

	// Step 4: Get lookup data
	t.Log("Step 4: Getting lookup data...")
	data, err := handler.GetData(lookupPath, 0)
	if err != nil {
		t.Fatalf("Failed to get lookup data: %v", err)
	}
	if len(data) != 6 {
		t.Errorf("GetData() record count = %v, want 6", len(data))
	}
	t.Logf("✓ Got lookup data: %d records", len(data))

	// Verify first record
	if len(data) > 0 {
		firstRecord := data[0]
		if code, ok := firstRecord["code"].(string); ok {
			if code != "200" {
				t.Errorf("First record code = %v, want '200'", code)
			}
		}
	}

	// Step 5: Update lookup (overwrite)
	t.Log("Step 5: Updating lookup table...")
	updatedCSV := []byte(`code,message,severity,category
200,OK,info,success
400,Bad Request,error,client
404,Not Found,warning,client
500,Internal Server Error,critical,server`)

	updateReq := lookup.CreateRequest{
		FilePath:    lookupPath,
		DisplayName: fmt.Sprintf("Updated Test Lookup %s", env.TestPrefix),
		Description: "E2E test lookup table (updated)",
		LookupField: "code",
		DataContent: updatedCSV,
		Overwrite:   true,
	}

	updateResp, err := handler.Update(lookupPath, updateReq)
	if err != nil {
		t.Fatalf("Failed to update lookup: %v", err)
	}
	if updateResp.Records != 4 {
		t.Errorf("Update() records = %v, want 4", updateResp.Records)
	}
	t.Logf("✓ Updated lookup: %d records", updateResp.Records)

	// Wait a bit for update to propagate
	time.Sleep(2 * time.Second)

	// Step 6: Verify update
	t.Log("Step 6: Verifying update...")
	updatedData, err := handler.GetData(lookupPath, 0)
	if err != nil {
		t.Fatalf("Failed to get updated lookup data: %v", err)
	}
	if len(updatedData) != 4 {
		t.Errorf("Updated data record count = %v, want 4", len(updatedData))
	}
	// Check if new column exists
	if len(updatedData) > 0 {
		if _, hasCategory := updatedData[0]["category"]; !hasCategory {
			t.Error("Updated data missing 'category' column")
		}
	}
	t.Log("✓ Update verified")

	// Step 7: Check if exists
	t.Log("Step 7: Checking if lookup exists...")
	exists, err := handler.Exists(lookupPath)
	if err != nil {
		t.Fatalf("Failed to check existence: %v", err)
	}
	if !exists {
		t.Error("Exists() = false, want true")
	}
	t.Log("✓ Lookup exists")

	// Step 8: List lookups
	t.Log("Step 8: Listing lookups...")
	list, err := handler.List()
	if err != nil {
		t.Fatalf("Failed to list lookups: %v", err)
	}

	// Find our lookup in the list
	found := false
	for _, item := range list {
		if item.Path == lookupPath {
			found = true
			t.Logf("✓ Found lookup in list: %s", item.Path)
			break
		}
	}
	if !found {
		t.Errorf("Lookup %s not found in list", lookupPath)
	}

	// Step 9: Delete lookup
	t.Log("Step 9: Deleting lookup...")
	if err := handler.Delete(lookupPath); err != nil {
		t.Fatalf("Failed to delete lookup: %v", err)
	}
	t.Log("✓ Lookup deleted")

	// Step 10: Verify deletion
	t.Log("Step 10: Verifying deletion...")
	time.Sleep(2 * time.Second) // Wait for deletion to propagate
	exists, err = handler.Exists(lookupPath)
	if err != nil {
		// Exists check may fail with "not found" error, which is expected
		if !strings.Contains(err.Error(), "not found") && !strings.Contains(err.Error(), "doesn't exist") {
			t.Fatalf("Unexpected error checking existence after deletion: %v", err)
		}
		exists = false
	}
	if exists {
		t.Error("Exists() = true after deletion, want false")
	}
	t.Log("✓ Deletion verified")

	// Untrack from cleanup since we already deleted it
	env.Cleanup.Untrack("lookup", lookupPath)
}

func TestLookupCreate_InvalidPath(t *testing.T) {
	env := integration.SetupIntegration(t)
	defer env.Cleanup.Cleanup(t)

	handler := lookup.NewHandler(env.Client)

	tests := []struct {
		name    string
		path    string
		wantErr string
	}{
		{
			name:    "missing /lookups/ prefix",
			path:    "/data/test",
			wantErr: "must start with /lookups/",
		},
		{
			name:    "empty path",
			path:    "",
			wantErr: "cannot be empty",
		},
		{
			name:    "ends with slash",
			path:    "/lookups/test/",
			wantErr: "must end with alphanumeric",
		},
		{
			name:    "invalid character",
			path:    "/lookups/test@data",
			wantErr: "invalid character",
		},
	}

	csvData := []byte("id,value\n1,test")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := lookup.CreateRequest{
				FilePath:    tt.path,
				LookupField: "id",
				DataContent: csvData,
			}

			_, err := handler.Create(req)
			if err == nil {
				t.Error("Create() expected error, got nil")
				return
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Create() error = %v, want error containing %q", err, tt.wantErr)
			}
			t.Logf("✓ Got expected error: %v", err)
		})
	}
}

func TestLookupCreate_DuplicateDetection(t *testing.T) {
	env := integration.SetupIntegration(t)
	defer env.Cleanup.Cleanup(t)

	handler := lookup.NewHandler(env.Client)
	lookupPath := fmt.Sprintf("/lookups/dtctl_test/%s/duplicates", env.TestPrefix)

	// CSV with duplicate codes
	csvData := []byte(`code,message
200,OK
400,Bad Request
200,Success
404,Not Found
400,Invalid Request`)

	req := lookup.CreateRequest{
		FilePath:    lookupPath,
		LookupField: "code",
		DataContent: csvData,
		Overwrite:   false,
	}

	uploadResp, err := handler.Create(req)
	if err != nil {
		t.Fatalf("Failed to create lookup: %v", err)
	}

	env.Cleanup.Track("lookup", lookupPath, "Duplicates Test")

	t.Logf("Upload response:")
	t.Logf("  Pattern Matches: %d", uploadResp.PatternMatches)
	t.Logf("  Discarded Duplicates: %d", uploadResp.DiscardedDuplicates)
	t.Logf("  Final Records: %d", uploadResp.Records)

	// Should have discarded 2 duplicates (second 200 and second 400)
	expectedRecords := 3 // 200, 400, 404
	if uploadResp.Records != expectedRecords {
		t.Errorf("Records = %d, want %d (duplicates should be removed)", uploadResp.Records, expectedRecords)
	}

	if uploadResp.DiscardedDuplicates != 2 {
		t.Errorf("DiscardedDuplicates = %d, want 2", uploadResp.DiscardedDuplicates)
	}

	t.Log("✓ Duplicate detection working correctly")
}

func TestLookupCreate_CustomParsePattern(t *testing.T) {
	env := integration.SetupIntegration(t)
	defer env.Cleanup.Cleanup(t)

	handler := lookup.NewHandler(env.Client)
	lookupPath := fmt.Sprintf("/lookups/dtctl_test/%s/pipe_delimited", env.TestPrefix)

	// Pipe-delimited data
	pipeData := []byte(`001|Alpha|100
002|Beta|200
003|Gamma|300`)

	req := lookup.CreateRequest{
		FilePath:     lookupPath,
		LookupField:  "id",
		DataContent:  pipeData,
		ParsePattern: "LD:id '|' LD:name '|' LD:value",
		Overwrite:    false,
	}

	uploadResp, err := handler.Create(req)
	if err != nil {
		t.Fatalf("Failed to create lookup with custom pattern: %v", err)
	}

	env.Cleanup.Track("lookup", lookupPath, "Custom Pattern Test")

	if uploadResp.Records != 3 {
		t.Errorf("Records = %d, want 3", uploadResp.Records)
	}

	t.Logf("✓ Custom parse pattern working: %d records", uploadResp.Records)

	// Verify data
	time.Sleep(2 * time.Second)
	data, err := handler.GetData(lookupPath, 0)
	if err != nil {
		t.Fatalf("Failed to get data: %v", err)
	}

	if len(data) != 3 {
		t.Errorf("Data length = %d, want 3", len(data))
	}

	// Check first record has expected columns
	if len(data) > 0 {
		first := data[0]
		if _, hasID := first["id"]; !hasID {
			t.Error("First record missing 'id' column")
		}
		if _, hasName := first["name"]; !hasName {
			t.Error("First record missing 'name' column")
		}
		if _, hasValue := first["value"]; !hasValue {
			t.Error("First record missing 'value' column")
		}
		t.Logf("✓ Data structure verified: %v", first)
	}
}

func TestLookupCreate_AlreadyExists(t *testing.T) {
	env := integration.SetupIntegration(t)
	defer env.Cleanup.Cleanup(t)

	handler := lookup.NewHandler(env.Client)
	lookupPath := fmt.Sprintf("/lookups/dtctl_test/%s/exists_test", env.TestPrefix)

	csvData := []byte(`id,value
1,first`)

	req := lookup.CreateRequest{
		FilePath:    lookupPath,
		LookupField: "id",
		DataContent: csvData,
		Overwrite:   false,
	}

	// Create first time
	_, err := handler.Create(req)
	if err != nil {
		t.Fatalf("Failed to create lookup: %v", err)
	}
	env.Cleanup.Track("lookup", lookupPath, "Exists Test")
	t.Log("✓ Created lookup")

	// Try to create again without overwrite
	_, err = handler.Create(req)
	if err == nil {
		t.Error("Create() expected error for duplicate, got nil")
	} else {
		if !strings.Contains(err.Error(), "already exists") {
			t.Errorf("Create() error = %v, want error containing 'already exists'", err)
		}
		t.Logf("✓ Got expected error: %v", err)
	}

	// Create with overwrite should succeed
	req.Overwrite = true
	csvData2 := []byte(`id,value
1,updated`)
	req.DataContent = csvData2

	_, err = handler.Create(req)
	if err != nil {
		t.Fatalf("Failed to create with overwrite: %v", err)
	}
	t.Log("✓ Overwrite succeeded")
}

func TestLookupDelete_NotFound(t *testing.T) {
	env := integration.SetupIntegration(t)
	defer env.Cleanup.Cleanup(t)

	handler := lookup.NewHandler(env.Client)
	nonExistentPath := fmt.Sprintf("/lookups/dtctl_test/%s/does_not_exist", env.TestPrefix)

	err := handler.Delete(nonExistentPath)
	if err == nil {
		t.Error("Delete() expected error for non-existent lookup, got nil")
	} else {
		if !strings.Contains(err.Error(), "not found") {
			t.Errorf("Delete() error = %v, want error containing 'not found'", err)
		}
		t.Logf("✓ Got expected error: %v", err)
	}
}

func TestLookupList(t *testing.T) {
	env := integration.SetupIntegration(t)
	defer env.Cleanup.Cleanup(t)

	handler := lookup.NewHandler(env.Client)

	// Create a test lookup to ensure we have at least one
	lookupPath := fmt.Sprintf("/lookups/dtctl_test/%s/list_test", env.TestPrefix)
	csvData := []byte(`id,name
1,test`)

	req := lookup.CreateRequest{
		FilePath:    lookupPath,
		LookupField: "id",
		DataContent: csvData,
		DisplayName: "List Test",
	}

	_, err := handler.Create(req)
	if err != nil {
		t.Fatalf("Failed to create test lookup: %v", err)
	}
	env.Cleanup.Track("lookup", lookupPath, "List Test")
	t.Log("✓ Created test lookup")

	// Wait for it to be queryable
	time.Sleep(2 * time.Second)

	// List all lookups
	list, err := handler.List()
	if err != nil {
		t.Fatalf("Failed to list lookups: %v", err)
	}

	if len(list) == 0 {
		t.Error("List() returned no lookups")
	}

	t.Logf("✓ Listed %d lookup(s)", len(list))
	for i, lu := range list {
		t.Logf("  [%d] %s", i+1, lu.Path)
	}
}

package metrics

import (
	"errors"
	"testing"
	"time"
)

func TestCollectorSnapshot(t *testing.T) {
	c := New(true)
	c.RecordCommand("cost estimate", 2*time.Second, nil)
	c.RecordCommand("cost estimate", time.Second, errors.New("boom"))
	c.RecordAPIError("GET /platform/test", 500, "boom")

	snapshot := c.Snapshot()
	if !snapshot.Enabled {
		t.Fatal("snapshot should be enabled")
	}
	if snapshot.TotalCommands != 2 {
		t.Fatalf("TotalCommands = %d, want 2", snapshot.TotalCommands)
	}
	if snapshot.TotalCommandErrors != 1 {
		t.Fatalf("TotalCommandErrors = %d, want 1", snapshot.TotalCommandErrors)
	}
	if snapshot.TotalAPIErrors != 1 {
		t.Fatalf("TotalAPIErrors = %d, want 1", snapshot.TotalAPIErrors)
	}
	if len(snapshot.Commands) != 1 {
		t.Fatalf("len(Commands) = %d, want 1", len(snapshot.Commands))
	}
	if snapshot.Commands[0].Count != 2 {
		t.Fatalf("command count = %d, want 2", snapshot.Commands[0].Count)
	}
	if snapshot.Commands[0].LastSuccess {
		t.Fatal("last run should be marked unsuccessful")
	}
	if len(snapshot.APIErrors) != 1 {
		t.Fatalf("len(APIErrors) = %d, want 1", len(snapshot.APIErrors))
	}
	if snapshot.APIErrors[0].LastStatus != 500 {
		t.Fatalf("LastStatus = %d, want 500", snapshot.APIErrors[0].LastStatus)
	}
}

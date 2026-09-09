package document

import (
	"context"
	"net/http"
	"testing"
)

// snapshotProbe records how a PATCH expressed (or omitted) the snapshot options.
type snapshotProbe struct {
	createSnapshot string
	description    []string
	hasDescription bool
}

// probeSnapshotUpdate issues an update against a stub API and reports what
// reached the wire.
func probeSnapshotUpdate(t *testing.T, req UpdateRequest) snapshotProbe {
	t.Helper()
	var got snapshotProbe
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/documents/doc-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		got.createSnapshot = r.URL.Query().Get("create-snapshot")
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("ParseMultipartForm: %v", err)
		}
		got.description, got.hasDescription = r.MultipartForm.Value["snapshotDescription"]
		writeMetadata(w, "doc-1", 5, nil)
	})

	h := NewHandler(newTestClient(t, mux))
	if _, err := h.UpdateDocument(context.Background(), "doc-1", 4, req); err != nil {
		t.Fatalf("UpdateDocument: %v", err)
	}
	return got
}

// TestUpdateDocument_CreateSnapshot asserts the opt-in flag becomes the
// create-snapshot query parameter and the description rides in the multipart body.
func TestUpdateDocument_CreateSnapshot(t *testing.T) {
	got := probeSnapshotUpdate(t, UpdateRequest{
		Content:             []byte(`{"k":"v"}`),
		CreateSnapshot:      true,
		SnapshotDescription: "before Q3 rework",
	})

	if got.createSnapshot != "true" {
		t.Errorf("create-snapshot = %q, want true", got.createSnapshot)
	}
	if len(got.description) != 1 || got.description[0] != "before Q3 rework" {
		t.Errorf("snapshotDescription = %v, want [before Q3 rework]", got.description)
	}
}

// TestUpdateDocument_NoSnapshotByDefault pins the opt-in contract: a plain
// update must not ask the API for a snapshot.
func TestUpdateDocument_NoSnapshotByDefault(t *testing.T) {
	got := probeSnapshotUpdate(t, UpdateRequest{Content: []byte(`{"k":"v"}`)})

	if got.createSnapshot != "" {
		t.Errorf("create-snapshot = %q, want the parameter to be absent", got.createSnapshot)
	}
	if got.hasDescription {
		t.Error("snapshotDescription must not be sent without CreateSnapshot")
	}
}

// TestUpdateDocument_SnapshotDescriptionWithoutFlag ensures a stray description
// alone neither triggers a snapshot nor reaches the API, which would ignore it.
func TestUpdateDocument_SnapshotDescriptionWithoutFlag(t *testing.T) {
	got := probeSnapshotUpdate(t, UpdateRequest{
		Content:             []byte(`{"k":"v"}`),
		SnapshotDescription: "orphaned",
	})

	if got.createSnapshot != "" {
		t.Errorf("create-snapshot = %q, want the parameter to be absent", got.createSnapshot)
	}
	if got.hasDescription {
		t.Error("snapshotDescription must not be sent without CreateSnapshot")
	}
}

// TestUpdateDocument_CreateSnapshotWithoutDescription covers the common case:
// the flag alone, no description part.
func TestUpdateDocument_CreateSnapshotWithoutDescription(t *testing.T) {
	got := probeSnapshotUpdate(t, UpdateRequest{
		Content:        []byte(`{"k":"v"}`),
		CreateSnapshot: true,
	})

	if got.createSnapshot != "true" {
		t.Errorf("create-snapshot = %q, want true", got.createSnapshot)
	}
	if got.hasDescription {
		t.Error("expected no snapshotDescription part when none was set")
	}
}

// TestCreate_LabelFollowupDoesNotSnapshot guards the label follow-up PATCH that
// Create issues: a freshly created document has no prior state worth snapshotting,
// and a snapshot there would burn one of the API's 5-per-minute allowance.
func TestCreate_LabelFollowupDoesNotSnapshot(t *testing.T) {
	var createSnapshot string
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/documents", func(w http.ResponseWriter, r *http.Request) {
		writeMetadata(w, "new-1", 1, nil)
	})
	mux.HandleFunc("/platform/document/v1/documents/new-1", func(w http.ResponseWriter, r *http.Request) {
		createSnapshot = r.URL.Query().Get("create-snapshot")
		writeMetadata(w, "new-1", 2, []string{"a"})
	})

	h := NewHandler(newTestClient(t, mux))
	if _, err := h.Create(context.Background(), CreateRequest{
		Name: "doc", Type: "acme:config", Content: []byte(`{"k":"v"}`), Labels: []string{"a"},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if createSnapshot != "" {
		t.Errorf("create-snapshot = %q on the label follow-up, want it absent", createSnapshot)
	}
}

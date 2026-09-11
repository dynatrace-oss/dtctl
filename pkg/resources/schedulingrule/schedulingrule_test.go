package schedulingrule

import (
	"testing"
)

func TestSchedulingRuleFields(t *testing.T) {
	r := SchedulingRule{
		ID:          "sr-1",
		Title:       "Business Hours",
		RuleType:    "rrule",
		Description: "Monday to Friday",
		RRule:       map[string]interface{}{"freq": "WEEKLY", "datestart": "2026-01-05"},
	}
	if r.ID != "sr-1" {
		t.Errorf("ID = %q, want %q", r.ID, "sr-1")
	}
	if r.Title != "Business Hours" {
		t.Errorf("Title = %q, want %q", r.Title, "Business Hours")
	}
	if r.RuleType != "rrule" {
		t.Errorf("RuleType = %q, want %q", r.RuleType, "rrule")
	}
	if r.RRule["freq"] != "WEEKLY" {
		t.Errorf("RRule[freq] = %v, want WEEKLY", r.RRule["freq"])
	}
}

func TestSchedulingRuleList(t *testing.T) {
	list := SchedulingRuleList{
		Count: 1,
		Results: []SchedulingRule{
			{ID: "sr-1", Title: "Business Hours", RuleType: "rrule"},
		},
	}
	if list.Count != 1 {
		t.Errorf("Count = %d, want 1", list.Count)
	}
	if len(list.Results) != 1 {
		t.Errorf("len(Results) = %d, want 1", len(list.Results))
	}
}

func TestOwnerID(t *testing.T) {
	// The API has no owner field, so createdBy is the ownership signal. An
	// absent modificationInfo must yield "" so the checker sees OwnershipUnknown.
	var missing SchedulingRule
	if got := missing.OwnerID(); got != "" {
		t.Errorf("OwnerID() with no modificationInfo = %q, want empty", got)
	}

	r := SchedulingRule{ModificationInfo: &ModificationInfo{
		CreatedBy:      "user-abc",
		LastModifiedBy: "user-xyz",
	}}
	if got := r.OwnerID(); got != "user-abc" {
		t.Errorf("OwnerID() = %q, want %q", got, "user-abc")
	}
}

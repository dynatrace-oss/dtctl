package schedulingrule

import (
	"testing"
)

func TestSchedulingRuleFields(t *testing.T) {
	r := SchedulingRule{
		ID:          "sr-1",
		Title:       "Business Hours",
		Timezone:    "UTC",
		Description: "Monday to Friday 9-17",
		Rule:        "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR",
		Owner:       "user-abc",
		OwnerType:   "USER",
	}
	if r.ID != "sr-1" {
		t.Errorf("ID = %q, want %q", r.ID, "sr-1")
	}
	if r.Title != "Business Hours" {
		t.Errorf("Title = %q, want %q", r.Title, "Business Hours")
	}
	if r.Timezone != "UTC" {
		t.Errorf("Timezone = %q, want %q", r.Timezone, "UTC")
	}
}

func TestSchedulingRuleList(t *testing.T) {
	list := SchedulingRuleList{
		Count: 1,
		Results: []SchedulingRule{
			{ID: "sr-1", Title: "Business Hours", Rule: "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR", Timezone: "UTC"},
		},
	}
	if list.Count != 1 {
		t.Errorf("Count = %d, want 1", list.Count)
	}
	if len(list.Results) != 1 {
		t.Errorf("len(Results) = %d, want 1", len(list.Results))
	}
}

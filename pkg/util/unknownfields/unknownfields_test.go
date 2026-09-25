package unknownfields

import (
	"encoding/json"
	"reflect"
	"testing"
)

type inner struct {
	Enabled bool `json:"enabled"`

	Extra map[string]json.RawMessage `json:"-"`
}

func (v *inner) UnmarshalJSON(data []byte) error {
	type plain inner
	extra, err := Unmarshal(data, (*plain)(v))
	if err != nil {
		return err
	}
	v.Extra = extra
	return nil
}

func (v inner) MarshalJSON() ([]byte, error) {
	type plain inner
	return Marshal(plain(v), v.Extra)
}

type outer struct {
	Name     string   `json:"name"`
	Optional *bool    `json:"optional,omitempty"`
	Inner    inner    `json:"inner"`
	Items    []inner  `json:"items,omitempty"`
	Untagged string   // matched by Go field name, as encoding/json does
	Skipped  string   `json:"-"`
	embedded          // promoted fields are modelled too
	List     []string `json:"list"`

	Extra map[string]json.RawMessage `json:"-"`
}

type embedded struct {
	Promoted string `json:"promoted,omitempty"`
}

func (v *outer) UnmarshalJSON(data []byte) error {
	type plain outer
	extra, err := Unmarshal(data, (*plain)(v))
	if err != nil {
		return err
	}
	v.Extra = extra
	return nil
}

func (v outer) MarshalJSON() ([]byte, error) {
	type plain outer
	return Marshal(plain(v), v.Extra)
}

// roundTrip decodes doc, lets mutate change the typed view, re-encodes it and
// returns the result as a generic map for comparison.
func roundTrip(t *testing.T, doc string, mutate func(*outer)) map[string]any {
	t.Helper()
	var v outer
	if err := json.Unmarshal([]byte(doc), &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if mutate != nil {
		mutate(&v)
	}
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("re-marshalled document is not valid JSON: %v\n%s", err, out)
	}
	return got
}

func TestRoundTripKeepsUnknownMembersAtEveryLevel(t *testing.T) {
	doc := `{
		"name": "n",
		"inner": {"enabled": false, "newBlock": {"enabled": true, "locations": ["a"]}},
		"items": [{"enabled": true, "perItem": 1}],
		"list": [],
		"tenantInstanceId": null,
		"dtAttributes": {"k": "v"},
		"flag": false
	}`

	got := roundTrip(t, doc, func(v *outer) {
		v.Name = "changed"
		v.Inner.Enabled = true
	})

	want := map[string]any{
		"name":             "changed",
		"Untagged":         "",
		"inner":            map[string]any{"enabled": true, "newBlock": map[string]any{"enabled": true, "locations": []any{"a"}}},
		"items":            []any{map[string]any{"enabled": true, "perItem": float64(1)}},
		"list":             []any{},
		"tenantInstanceId": nil,
		"dtAttributes":     map[string]any{"k": "v"},
		"flag":             false,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestExplicitNullSurvivesAsNull(t *testing.T) {
	got := roundTrip(t, `{"tenantInstanceId": null}`, nil)
	v, present := got["tenantInstanceId"]
	if !present || v != nil {
		t.Fatalf("tenantInstanceId = %#v (present=%v), want an explicit null", v, present)
	}
}

func TestModelledKeysAreNotDuplicated(t *testing.T) {
	// encoding/json matches member names case-insensitively, so a lowercased
	// key (as older `-o yaml` output produced) is consumed by the typed field
	// and must not also come back verbatim as an unknown member.
	var v outer
	doc := `{"NAME": "x", "untagged": "u", "Promoted": "p", "OPTIONAL": false, "skipped": "kept"}`
	if err := json.Unmarshal([]byte(doc), &v); err != nil {
		t.Fatal(err)
	}
	if v.Name != "x" || v.Untagged != "u" || v.Promoted != "p" || v.Optional == nil || *v.Optional {
		t.Fatalf("typed fields not populated: %+v", v)
	}
	// "skipped" matches a json:"-" field, which encoding/json never fills, so
	// it is an unknown member like any other.
	if len(v.Extra) != 1 || string(v.Extra["skipped"]) != `"kept"` {
		t.Fatalf("Extra = %v, want only skipped", v.Extra)
	}
}

func TestNoExtraWhenEverythingIsModelled(t *testing.T) {
	var v outer
	if err := json.Unmarshal([]byte(`{"name":"n","inner":{"enabled":true}}`), &v); err != nil {
		t.Fatal(err)
	}
	if v.Extra != nil || v.Inner.Extra != nil {
		t.Fatalf("Extra should stay nil, got %v / %v", v.Extra, v.Inner.Extra)
	}
}

func TestNullDocumentIsANoOp(t *testing.T) {
	v := inner{Enabled: true}
	if err := v.UnmarshalJSON([]byte("null")); err != nil {
		t.Fatal(err)
	}
	if !v.Enabled || v.Extra != nil {
		t.Fatalf("null must leave the value untouched, got %+v", v)
	}
}

func TestExtraCannotShadowAModelledField(t *testing.T) {
	v := inner{Enabled: true, Extra: map[string]json.RawMessage{"Enabled": json.RawMessage("false"), "other": json.RawMessage(`"x"`)}}
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"enabled":true,"other":"x"}` {
		t.Fatalf("got %s", out)
	}
}

func TestExtraOnEmptyObject(t *testing.T) {
	type empty struct{}
	out, err := Marshal(empty{}, map[string]json.RawMessage{"b": json.RawMessage("2"), "a": json.RawMessage("1"), "c": nil})
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"a":1,"b":2,"c":null}` {
		t.Fatalf("got %s", out)
	}
}

func TestMalformedExtraIsAnError(t *testing.T) {
	type empty struct{}
	if _, err := Marshal(empty{}, map[string]json.RawMessage{"a": json.RawMessage("{")}); err == nil {
		t.Fatal("expected an error for an invalid raw member")
	}
}

func TestUnmarshalPropagatesTypeErrors(t *testing.T) {
	var v outer
	if err := json.Unmarshal([]byte(`{"name": 1}`), &v); err == nil {
		t.Fatal("expected a type error")
	}
}

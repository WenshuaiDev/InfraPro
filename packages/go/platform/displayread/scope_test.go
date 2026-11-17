package displayread

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
)

func TestReadStrictWirePaths(t *testing.T) {
	for _, test := range []struct{ raw, path string }{
		{`{"target":{"view":"a","view":"b"}}`, "/target/view"},
		{`{"target":{"view":"a","tenant_id":"hidden"}}`, "/target/tenant_id"},
		{`{"target":{"view":"a"},"filters":{"arbitrary_sql":"x"}}`, "/filters/arbitrary_sql"},
		{`{"target":{}}`, "/target/view"},
		{`{"target":{"view":"a","ids":[]}}`, "/target/ids"},
		{`{"target":{"view":"a"},"cursor":"another-query"}`, "/cursor"},
	} {
		_, e := DecodeRequest("resources.query", []byte(test.raw))
		var problem *Error
		if !errors.As(e, &problem) || len(problem.Issues) != 1 || problem.Issues[0].Path != test.path {
			t.Errorf("%s: %v", test.raw, e)
		}
	}
	var value map[string]any
	if e := Decode([]byte(`{"count":9007199254740993}`), &value); e != nil || value["count"] != json.Number("9007199254740993") {
		t.Fatalf("precision: %v %v", value, e)
	}
}
func TestReadFuturePredicateAndFieldCeilings(t *testing.T) {
	parent := Definition{ID: "raw", Domain: "channels", Selector: Selector{Kind: "facilities", FacilityIDs: []string{"00000000-0000-4000-8000-000000000001"}}, IncludeFuture: true, FieldGroups: []string{"statistics.counts"}}
	if e := ValidateDefinition(&parent, false, "/scope"); e != nil {
		t.Fatal(e)
	}
	child := parent
	child.ID = ""
	child.ScopeID = "raw"
	if e := ValidateDefinition(&child, true, "/view"); e != nil {
		t.Fatal(e)
	}
	if !DefinitionSubset(child, parent) {
		t.Fatal("same future predicate rejected")
	}
	child.Selector = Selector{Kind: "project", SourceKinds: []string{"direct"}}
	if DefinitionSubset(child, parent) {
		t.Fatal("same current members cannot prove all-project future safe")
	}
	child = parent
	child.IncludeFuture = false
	parent.IncludeFuture = false
	child.IncludeFuture = true
	if DefinitionSubset(child, parent) {
		t.Fatal("one-sided future permission")
	}
	if _, e := Fields([]string{"statistics.counts"}, []string{"monitoring.values"}, []string{"monitoring.values"}); e == nil {
		t.Fatal("summary leaked values")
	}
	if out, e := Fields([]string{"resource.metadata", "resource.attributes"}, nil, []string{"resource.metadata"}); e != nil || !reflect.DeepEqual(out, []string{"resource.metadata"}) {
		t.Fatalf("optional fields defaulted: %v %v", out, e)
	}
	for _, d := range []Definition{{ID: "x", Domain: "resources", Selector: Selector{Kind: "project", ManufacturerIDs: []string{"00000000-0000-4000-8000-000000000001"}}, FieldGroups: []string{"resource.metadata"}}, {ID: "x", Domain: "channels", Selector: Selector{Kind: "project", SourceKinds: []string{"derived"}}, FieldGroups: []string{"statistics.counts"}}} {
		if ValidateDefinition(&d, false, "/scope") == nil {
			t.Fatal("invalid domain/derived selector accepted")
		}
	}
}
func TestReadWindowUnionKeepsGapsAndHalfOpenBounds(t *testing.T) {
	n := func(v int64) *int64 { return &v }
	in := []access.ReadWindow{{From: n(0), To: n(5)}, {From: n(3), To: n(8)}, {From: n(10), To: n(12)}, {From: n(12), To: n(13)}}
	out := Union(in)
	if len(out) != 2 || *out[0].From != 0 || *out[0].To != 8 || *out[1].From != 10 || *out[1].To != 13 {
		t.Fatalf("union filled gap: %+v", out)
	}
	if _, ok := Intersect(access.ReadWindow{From: n(0), To: n(5)}, access.ReadWindow{From: n(5), To: n(7)}); ok {
		t.Fatal("half-open edge included")
	}
	if Covers(access.ReadWindow{From: n(1), To: n(9)}, access.ReadWindow{}) {
		t.Fatal("finite ceiling covers all history")
	}
	from, to := "2026-11-01T00:00:00+08:00", "2026-10-31T16:05:00Z"
	w, e := ParseWindow(&Window{From: &from, To: &to}, "/range")
	if e != nil || *w.To-*w.From != 300000000000 {
		t.Fatalf("absolute range: %+v %v", w, e)
	}
}
func TestReadBudgetsAndTimeModes(t *testing.T) {
	members := make([]Member, MaxMembers)
	for i := range members {
		members[i] = Member{Kind: "channel", ID: fmt.Sprintf("%08d-0000-4000-8000-000000000001", i)}
	}
	if got, e := NormalizeMembers(members); e != nil || len(got) != MaxMembers {
		t.Fatal("50,000 boundary", e)
	}
	if _, e := NormalizeMembers(append(members, Member{Kind: "channel", ID: "extra"})); e == nil {
		t.Fatal("50,001 silently truncated")
	}
	ids := make([]string, 501)
	if e := ValidateIDs(ids, "/ids", 500); e == nil {
		t.Fatal("501 explicit ids accepted")
	}
	for _, raw := range []string{`{"target":{"view":"v"}}`, `{"target":{"view":"v"},"all_history":true,"follow_seconds":10}`, `{"target":{"view":"v"},"range":{"from":"2026-01-01T00:00:00Z"}}`} {
		if _, e := DecodeRequest("monitoring.details", []byte(raw)); e == nil {
			t.Fatal("ambiguous time accepted", raw)
		}
	}
	if _, e := DecodeRequest("monitoring.latest", []byte(`{"target":{"view":"v"}}`)); e != nil {
		t.Fatal(e)
	}
	if _, e := DecodeRequest("statistics.devices", []byte(`{"target":{"view":"v"},"population":"monitoring_bound","group_by":"none"}`)); e == nil {
		t.Fatal("source count without channel scope")
	}
	if _, e := DecodeRequest("monitoring.details", []byte(`{"target":{"selection":"opaque"},"all_history":true}`)); e == nil {
		t.Fatal("rewritten drilldown time")
	}
}
func TestReadGeographicBounds(t *testing.T) {
	b := BBox{West: 170, East: -170, South: -10, North: 10}
	if e := validateBBox(&b); e != nil {
		t.Fatal(e)
	}
	for lon, want := range map[float64]bool{170: true, 180: true, -180: true, -170: true, 0: false} {
		if b.Contains(lon, 0) != want {
			t.Fatalf("dateline %v", lon)
		}
	}
	b = BBox{West: -1, East: 1, South: -1, North: 1}
	if !b.Contains(0, 0) {
		t.Fatal("zero coordinate lost")
	}
	b.East = b.West
	if validateBBox(&b) == nil {
		t.Fatal("zero-width box accepted")
	}
}

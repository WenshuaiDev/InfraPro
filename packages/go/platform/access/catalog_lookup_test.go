package access

import (
	"slices"
	"testing"
)

func TestCatalogLookupsPreserveDefinitionsAndIsolation(t *testing.T) {
	checksum := Checksum()
	if checksum != calculateChecksum() {
		t.Fatal("indexed catalog changed the release checksum")
	}
	for _, permission := range Catalog() {
		if known(permission.Code) != (permission.Lifecycle == "active") {
			t.Fatal("permission lifecycle lookup changed", permission.Code)
		}
	}
	if known("unknown.permission") {
		t.Fatal("unknown permission accepted")
	}
	if _, ok := PolicyFor("unknown.operation"); ok {
		t.Fatal("unknown operation accepted")
	}
	if _, ok := RoutePolicy("GET", "/unknown/route"); ok {
		t.Fatal("unknown route accepted")
	}
	for _, expected := range Policies() {
		actual, ok := PolicyFor(expected.Operation)
		if !ok || actual.Operation != expected.Operation || actual.Method != expected.Method || actual.Path != expected.Path || actual.Class != expected.Class || actual.Recent != expected.Recent || actual.ChangesGraph != expected.ChangesGraph || !slices.Equal(actual.Permissions, expected.Permissions) {
			t.Fatal("operation policy changed", expected.Operation)
		}
		if len(actual.Permissions) > 0 {
			actual.Permissions[0] = "unknown.permission"
		}
		again, ok := RoutePolicy(expected.Method, expected.Path)
		if !ok || !slices.Equal(again.Permissions, expected.Permissions) {
			t.Fatal("caller mutation changed another policy lookup", expected.Operation)
		}
	}
	if Checksum() != checksum {
		t.Fatal("caller mutation changed release checksum")
	}
}

func BenchmarkAuthorizationCatalogLookups(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		known(ProjectMonitoringHistoryRead)
		Checksum()
		PolicyFor(OpAccountRead)
	}
}

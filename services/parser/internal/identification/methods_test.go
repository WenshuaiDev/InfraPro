package identification

import (
	"encoding/base64"
	i "github.com/WenshuaiDev/InfraPro/packages/go/foundation/ingestioncontract"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	"github.com/WenshuaiDev/InfraPro/services/parser/internal/methods"
	"os"
	"strings"
	"testing"
	"time"
)

func TestExactFixturesAndProvenance(t *testing.T) {
	for _, tt := range []struct {
		file, id    string
		size        int
		hash        string
		ids, values []string
	}{
		{"vendor-batch.json", Batch, 288, "7e9632a8c63659897b9e85d1a1d13a5653917439e7e00ee90f9fbafe8ba8df16", []string{"00000000001", "00000000002"}, []string{"12.34", "12.34"}},
		{"channels.bin", Channels, 205, "94f95b17b4a91fad143ccea7939a03d3018fb0a0e057581cbc2d24c10a34e469", []string{"00000000003", "00000000003"}, []string{"12.34", "8.5"}},
	} {
		t.Run(tt.file, func(t *testing.T) {
			b, e := os.ReadFile("../../../../samples/ingestion/" + tt.file)
			if e != nil {
				t.Fatal(e)
			}
			if len(b) != tt.size || c.Digest(b) != tt.hash {
				t.Fatal("independent fixture hash")
			}
			var m i.Identification
			for _, v := range Methods(strings.Repeat("a", 64), strings.Repeat("b", 64)) {
				if v.Method.ID == tt.id {
					m = v
				}
			}
			req := i.Request{Version: 1, ExecutionID: "00000000-0000-4000-8000-000000000001", Method: m.Method, Input: i.Input{BytesBase64: base64.StdEncoding.EncodeToString(b), ByteLength: int64(len(b)), SHA256: c.Digest(b)}, Context: i.Context{ReceivedAt: time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC), Transport: m.Transport, InputUnit: m.InputUnit}}
			out, e := Identify(req)
			if e != nil || i.ValidateResponse(req, m, out) != nil || len(out.Units) != 2 {
				t.Fatalf("identity: %+v %v", out, e)
			}
			for n, u := range out.Units {
				if u.TerminalKey != tt.ids[n] {
					t.Fatal("leading zero identity")
				}
				raw, e := i.Materialize(b, u.View)
				if e != nil {
					t.Fatal(e)
				}
				id := methods.Example
				if tt.id == Channels {
					id = methods.Reading
					if u.ChildKey != []string{"A", "B"}[n] {
						t.Fatal("child")
					}
				}
				var entry c.Entry
				for _, v := range methods.Manifests(strings.Repeat("a", 64), strings.Repeat("b", 64)) {
					if v.MethodID == id && v.MethodVersion == "1.0.0" {
						entry = methods.Entry(v)
					}
				}
				pr := c.Request{ContractVersion: 1, ExecutionID: req.ExecutionID, Method: entry.Method(), Input: c.Input{BytesBase64: base64.StdEncoding.EncodeToString(raw), SHA256: c.Digest(raw), ByteLength: len(raw)}, Context: c.Context{ReceivedAt: req.Context.ReceivedAt, Transport: u.View.ParseTransport}}
				result, e := methods.Parse(pr)
				result.ServiceRelease = "test"
				result.ServiceBuildDigest = strings.Repeat("c", 64)
				if e != nil || c.ValidateResponse(pr, entry, result) != nil || result.Status != "parsed" || *result.Records[0].Measurements[0].Value != tt.values[n] {
					t.Fatalf("reading: %+v %v", result, e)
				}
			}
			out.Units[1].Key = out.Units[0].Key
			if i.ValidateResponse(req, m, out) == nil {
				t.Fatal("duplicate unit accepted")
			}
		})
	}
}
func TestCatalogExactCombinations(t *testing.T) {
	measurement := c.Catalog{ContractVersion: 1}
	for _, m := range methods.Manifests(strings.Repeat("a", 64), strings.Repeat("b", 64)) {
		measurement.Entries = append(measurement.Entries, methods.Entry(m))
	}
	v := Catalog(Methods(strings.Repeat("c", 64), strings.Repeat("d", 64)), measurement)
	if e := i.ValidateCatalog(v, measurement); e != nil {
		t.Fatal(e)
	}
	v.Compatibilities[0].Measurement.ArtifactDigest = strings.Repeat("0", 64)
	v.Compatibilities[0].Digest = i.CompatibilityDigest(v.Compatibilities[0])
	if i.ValidateCatalog(v, measurement) == nil {
		t.Fatal("wrong digest compatibility accepted")
	}
}

func TestForgedViewsAndExclusiveAuthenticatedIdentity(t *testing.T) {
	all := Methods(strings.Repeat("a", 64), strings.Repeat("b", 64))
	var batch, fixed i.Identification
	for _, m := range all {
		if m.Method.ID == Batch {
			batch = m
		}
		if m.Method.ID == Fixed {
			fixed = m
		}
	}
	raw, e := os.ReadFile("../../../../samples/ingestion/vendor-batch.json")
	if e != nil {
		t.Fatal(e)
	}
	request := func(m i.Identification, b []byte) i.Request {
		return i.Request{Version: 1, ExecutionID: "00000000-0000-4000-8000-000000000001", Method: m.Method, Input: i.Input{BytesBase64: base64.StdEncoding.EncodeToString(b), ByteLength: int64(len(b)), SHA256: c.Digest(b)}, Context: i.Context{ReceivedAt: time.Now().UTC(), Transport: m.Transport, InputUnit: m.InputUnit}}
	}
	req := request(batch, raw)
	out, e := Identify(req)
	if e != nil {
		t.Fatal(e)
	}
	u := out.Units[0]
	u.View = view(raw, "whole", "complete_frame", transform(raw, raw, "identity", 0, len(raw)))
	out.Units = []i.Unit{u}
	if i.ValidateResponse(req, batch, out) == nil {
		t.Fatal("whole mixed body accepted as a complete frame")
	}
	reading := []byte(`{"observed_at":"2026-11-16T00:00:00Z","displacement_mm":"12.340"}`)
	req = request(fixed, reading)
	out, e = Identify(req)
	if e == nil && out.Status == "identified" {
		t.Fatal("unauthenticated fixed identity")
	}
	req.Context.FixedTerminal = "00000000001"
	req.Context.ExclusiveAuthenticated = true
	out, e = Identify(req)
	if e != nil || i.ValidateResponse(req, fixed, out) != nil {
		t.Fatal("exclusive source rejected", e)
	}
	req.Context.ExclusiveAuthenticated = false
	if i.ValidateResponse(req, fixed, out) == nil {
		t.Fatal("forged authenticated source accepted")
	}
}

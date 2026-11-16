package methods

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func request(b []byte, id, version string) c.Request {
	return c.Request{ContractVersion: 1, ExecutionID: "00000000-0000-4000-8000-000000000001", Method: c.Method{ID: id, Version: version, ArtifactDigest: strings.Repeat("a", 64), ManifestDigest: strings.Repeat("b", 64)}, Input: c.Input{BytesBase64: base64.StdEncoding.EncodeToString(b), SHA256: c.Digest(b), ByteLength: len(b)}, Context: c.Context{ReceivedAt: time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC), Transport: "tcp"}}
}
func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, e := os.ReadFile("../../../../samples/parsing/" + name)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func frame(payload []byte) []byte {
	b := append([]byte{123, 9, 0, 0}, []byte("00000000001")...)
	b = append(b, payload...)
	b = append(b, 123)
	binary.BigEndian.PutUint16(b[2:4], uint16(len(b)))
	return b
}
func TestDocumentFixtures(t *testing.T) {
	b := fixture(t, "measurement.bin")
	if len(b) != 81 || c.Digest(b) != "87b7f3f0e959ee6881bba0d4d0ddb045cddc2786f947b2f64f5976a2a501c63e" {
		t.Fatal("fixture drift")
	}
	for _, id := range []string{Frame, Example} {
		r, e := Parse(request(b, id, "1.0.0"))
		if e != nil || r.Status != "parsed" || r.Identities[0].Text != "00000000001" {
			t.Fatalf("%s: %+v %v", id, r, e)
		}
		if id == Frame {
			if len(r.Records) != 0 || r.Fields[2].Location.Range != (c.Range{Start: 15, End: 80}) {
				t.Fatal(r)
			}
		} else {
			m := r.Records[0].Measurements[0]
			if *m.Value != "12.34" || m.Unit != "mm" || r.Records[0].ObservedAt.Format(time.RFC3339) != "2026-11-16T00:00:00Z" || string(b[m.Location.Range.Start:m.Location.Range.End]) != `"12.340"` {
				t.Fatal(r)
			}
		}
		if r.Checks[0].Status != "not_applicable" {
			t.Fatal("invented checksum")
		}
	}
	for _, name := range []string{"logout.bin", "registration.bin", "ack.bin"} {
		r, e := Parse(request(fixture(t, name), Example, "1.0.0"))
		if e != nil || r.Status != "parsed" || r.MessageKind != "control" || len(r.Records) != 0 {
			t.Fatalf("%s: %+v %v", name, r, e)
		}
	}
}
func TestMalformedFramesAndLimits(t *testing.T) {
	valid := fixture(t, "measurement.bin")
	cases := map[string][]byte{"header": append([]byte{0}, valid[1:]...), "tail": append(bytes.Clone(valid[:len(valid)-1]), 0), "length": fixture(t, "bad-length.bin"), "two": fixture(t, "two-frames.bin"), "partial": valid[:20], "empty": {123}, "udp": append(fixture(t, "logout.bin"), []byte("payload")...)}
	for name, b := range cases {
		t.Run(name, func(t *testing.T) {
			r, e := Parse(request(b, Frame, "1.0.0"))
			if e == nil && r.Status != "rejected" {
				t.Fatal(r)
			}
		})
	}
	for _, n := range []int{1024, 1025} {
		r, e := Parse(request(frame(bytes.Repeat([]byte{123}, n)), Frame, "1.0.0"))
		if n == 1024 && (e != nil || r.Status != "parsed") {
			t.Fatal(r, e)
		}
		if n == 1025 && e == nil {
			t.Fatal("over limit")
		}
	}
	b := bytes.Clone(valid)
	b[1] = 11
	r, e := Parse(request(b, Frame, "1.0.0"))
	if e != nil || r.Status != "unsupported" {
		t.Fatal(r, e)
	}
	for _, id := range []string{Frame, Example} {
		r, e := Parse(request(fixture(t, "unknown-payload.bin"), id, "1.0.0"))
		if e != nil || id == Frame && r.Status != "parsed" || id == Example && r.Status != "rejected" {
			t.Fatal(r, e)
		}
	}
}
func TestStrictJSONValues(t *testing.T) {
	for _, payload := range []string{
		`{"observed_at":"2026-11-16T00:00:00Z","displacement_mm":"1","displacement_mm":"2"}`,
		`{"observed_at":"2026-11-16T00:00:00Z","displacement_mm":null}`,
		`{"observed_at":"2026-11-16T00:00:00Z","displacement_mm":12.34}`,
		`{"observed_at":"2026-11-16T00:00:00","displacement_mm":"1"}`,
		`{"displacement_mm":"1"}`,
		`{"observed_at":"2026-11-16T00:00:00Z","displacement_mm":"NaN"}`,
		`{"observed_at":"2026-11-16T00:00:00Z","displacement_mm":"100000000000000000000000000"}`,
		`{"observed_at":"2026-11-16T00:00:00Z","displacement_mm":"0.1234567890123"}`,
		`{"observed_at":"2026-11-16T00:00:00Z","displacement_mm":"1","unknown":0}`,
		"{\"observed_at\":\"\xff\"}",
	} {
		r, e := Parse(request(frame([]byte(payload)), Example, "1.0.0"))
		if e != nil || r.Status != "rejected" || len(r.Records) != 0 {
			t.Fatalf("%q: %+v %v", payload, r, e)
		}
	}
	for _, value := range []string{"0", "-0.000", "99999999999999999999999999.123456789012"} {
		payload := `{"observed_at":"2026-11-16T08:00:00+08:00","displacement_mm":"` + value + `"}`
		r, e := Parse(request(frame([]byte(payload)), Example, "1.0.0"))
		if e != nil || r.Status != "parsed" || r.Records[0].Measurements[0].Value == nil {
			t.Fatal(r, e)
		}
	}
}
func TestVersionsAndDeterminism(t *testing.T) {
	b := fixture(t, "source-status.bin")
	for _, version := range []string{"1.0.0", "1.1.0"} {
		r, e := Parse(request(b, Example, version))
		if e != nil {
			t.Fatal(e)
		}
		if version == "1.0.0" && r.Status != "rejected" || version == "1.1.0" && (r.Status != "parsed" || r.Records[0].SourceStatus != "suspect") {
			t.Fatal(r)
		}
	}
	req := request(fixture(t, "measurement.bin"), Example, "1.0.0")
	first, _ := Parse(req)
	req.Context.ReceivedAt = req.Context.ReceivedAt.Add(50 * time.Hour)
	second, _ := Parse(req)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("undeclared time dependency")
	}
}
func FuzzDDP(f *testing.F) {
	f.Add([]byte{123, 2, 0, 16, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 49, 123})
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 1040 {
			return
		}
		for _, id := range []string{Frame, Example} {
			r, e := Parse(request(b, id, "1.0.0"))
			if e == nil && r.Status != "parsed" && len(r.Records) != 0 {
				t.Fatal("partial success")
			}
		}
	})
}

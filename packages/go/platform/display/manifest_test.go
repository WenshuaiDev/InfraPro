package display

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

func testPackage(t *testing.T, change func(*Manifest), extra map[string][]byte) []byte {
	t.Helper()
	body := []byte(`<!doctype html><title>SDK integration</title>`)
	m := Manifest{Version: 1, AppKey: "test-app", AppVersion: "1.0.0", Entry: "index.html", Required: map[string]int{}, ConfigSchema: Schema{Type: "object", Properties: map[string]Schema{}}, Bindings: map[string]BindingSchema{}, Files: []File{{Path: "index.html", Size: int64(len(body)), MediaType: "text/html", SHA256: hashBytes(body)}}}
	m.Protocol.Major = 1
	m.Routing.Mode = "hash"
	if change != nil {
		change(&m)
	}
	raw, _ := json.Marshal(m)
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	for name, b := range map[string][]byte{"manifest.json": raw, "index.html": body} {
		f, e := w.Create(name)
		if e != nil {
			t.Fatal(e)
		}
		f.Write(b)
	}
	for name, b := range extra {
		f, e := w.Create(name)
		if e != nil {
			t.Fatal(e)
		}
		f.Write(b)
	}
	if e := w.Close(); e != nil {
		t.Fatal(e)
	}
	return out.Bytes()
}
func TestPackageBoundary(t *testing.T) {
	cases := []struct {
		name   string
		change func(*Manifest)
		extra  map[string][]byte
		valid  bool
	}{{"valid", nil, nil, true}, {"protocol", func(m *Manifest) { m.Protocol.Major = 2 }, nil, false}, {"digest", func(m *Manifest) { m.Files[0].SHA256 = hashBytes([]byte("wrong")) }, nil, false}, {"traversal", nil, map[string][]byte{"../outside": []byte("x")}, false}, {"unlisted", nil, map[string][]byte{"secret.txt": []byte("x")}, false}, {"case_collision", nil, map[string][]byte{"INDEX.HTML": []byte("x")}, false}, {"remote_schema", func(m *Manifest) { m.ConfigSchema.Additional = true }, nil, false}}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := testPackage(t, c.change, c.extra)
			_, e := ValidateZIP(context.Background(), bytes.NewReader(b), int64(len(b)))
			if (e == nil) != c.valid {
				t.Fatalf("valid=%v error=%v", c.valid, e)
			}
		})
	}
}
func TestStrictJSONAndExactInteger(t *testing.T) {
	var v map[string]any
	for _, raw := range []string{`{"x":1,"x":2}`, `{"a":{"b":1,"b":2}}`, `{} {}`} {
		if Decode([]byte(raw), &v) == nil {
			t.Fatal("ambiguous JSON accepted", raw)
		}
	}
	max := json.Number("9007199254740993")
	s := Schema{Type: "integer", Maximum: &max}
	if !validNumber(s, json.Number("9007199254740993")) || validNumber(s, json.Number("9007199254740994")) {
		t.Fatal("configuration precision lost")
	}
}

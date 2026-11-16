package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	"github.com/WenshuaiDev/InfraPro/services/parser/internal/methods"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func packageAt(t *testing.T, b []byte) string {
	t.Helper()
	dir := t.TempDir()
	digest := c.Digest(b)
	if e := os.MkdirAll(filepath.Join(dir, "artifacts"), 0755); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(dir, "artifacts", digest), b, 0555); e != nil {
		t.Fatal(e)
	}
	cat := c.Catalog{ContractVersion: 1, ServiceRelease: "test", ServiceBuildDigest: digest}
	for _, m := range methods.Manifests(digest, strings.Repeat("a", 64)) {
		entry := methods.Entry(m)
		cat.Entries = append(cat.Entries, entry)
		base := filepath.Join(dir, "manifests", entry.ManifestDigest)
		_ = os.MkdirAll(base, 0755)
		for name, s := range map[string]string{"manifest.json": entry.ManifestBase64, "outputs.json": entry.OutputContractBase64} {
			b, _ := base64.StdEncoding.DecodeString(s)
			_ = os.WriteFile(filepath.Join(base, name), b, 0444)
		}
	}
	raw, _ := json.Marshal(cat)
	if e := os.WriteFile(filepath.Join(dir, "catalog.json"), raw, 0444); e != nil {
		t.Fatal(e)
	}
	return dir
}
func requestFor(t *testing.T, s *Server) c.Request {
	b, e := os.ReadFile("../../../../samples/parsing/measurement.bin")
	if e != nil {
		t.Fatal(e)
	}
	return c.Request{ContractVersion: 1, ExecutionID: "00000000-0000-4000-8000-000000000001", Method: s.Catalog.Entries[1].Method(), Input: c.Input{BytesBase64: base64.StdEncoding.EncodeToString(b), SHA256: c.Digest(b), ByteLength: len(b)}, Context: c.Context{ReceivedAt: time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC), Transport: "tcp"}, TraceID: "machine-contract"}
}
func call(s *Server, path, token string, body []byte) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", path, bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}
func TestMachineAuthBudgetAndChildReaping(t *testing.T) {
	dir := packageAt(t, []byte("#!/bin/sh\nexec /bin/sleep 20\n"))
	token := strings.Repeat("x", 40)
	s, e := Open(dir, token, "test", strings.Repeat("b", 64), 4)
	if e != nil {
		t.Fatal(e)
	}
	in := requestFor(t, s)
	body, _ := json.Marshal(in)
	for _, bad := range []string{"", "wrong"} {
		w := call(s, "/v1/parse", bad, body)
		if w.Code != 401 || strings.Contains(w.Body.String(), token) {
			t.Fatal(w)
		}
	}
	r := httptest.NewRequest("GET", "/v1/catalog", nil)
	r.Header.Set("Origin", "http://localhost")
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("browser allowed")
	}
	w = call(s, "/v1/parse", token, bytes.Repeat([]byte(" "), c.MaxRequest+1))
	if w.Code != 413 {
		t.Fatal(w.Code)
	}
	var wg sync.WaitGroup
	codes := make(chan int, 4)
	start := time.Now()
	for range 4 {
		wg.Go(func() { codes <- call(s, "/v1/parse", token, body).Code })
	}
	for time.Since(start) < time.Second && len(s.slots) > 0 {
		time.Sleep(time.Millisecond)
	}
	if len(s.slots) != 0 {
		t.Fatal("children not started")
	}
	if w = call(s, "/v1/parse", token, body); w.Code != 429 {
		t.Fatal("concurrency bound", w.Code)
	}
	wg.Wait()
	close(codes)
	for code := range codes {
		if code != 504 {
			t.Fatal("timeout", code)
		}
	}
	if time.Since(start) > 7*time.Second || len(s.slots) != 4 {
		t.Fatal("children not reaped")
	}
	health := httptest.NewRecorder()
	s.ServeHTTP(health, httptest.NewRequest("GET", "/healthz", nil))
	if health.Code != 200 {
		t.Fatal("not recovered")
	}
}
func TestRealExecutableContractAndPackageCoverage(t *testing.T) {
	path := os.Getenv("INFRAPRO_PARSER_TEST_RUNNER")
	if path == "" {
		t.Skip("real built runner required")
	}
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	dir := packageAt(t, b)
	s, e := Open(dir, strings.Repeat("x", 40), "http-new-release", strings.Repeat("b", 64), 4)
	if e != nil {
		t.Fatal(e)
	}
	in := requestFor(t, s)
	body, _ := json.Marshal(in)
	w := call(s, "/v1/parse", s.Token, body)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var response c.Response
	if e = c.StrictJSON(w.Body.Bytes(), &response); e != nil {
		t.Fatal(e)
	}
	if e = c.ValidateResponse(in, s.Catalog.Entries[1], response); e != nil {
		t.Fatal(e)
	}
	if *response.Records[0].Measurements[0].Value != "12.34" {
		t.Fatal(response)
	}
	for _, mutate := range []func(*c.Response){func(v *c.Response) { v.ExecutionID = "mismatch" }, func(v *c.Response) { v.Method.Version = "9.0.0" }, func(v *c.Response) { v.Records[0].Measurements[0].OutputKey = "unknown" }, func(v *c.Response) { v.Fields[0].Location.Range.End = 9999 }, func(v *c.Response) { v.Steps[0].ParentIDs = []string{"future"} }, func(v *c.Response) { v.Records = make([]c.Record, c.MaxRecords+1) }, func(v *c.Response) { v.Steps = make([]c.Step, c.MaxSteps+1) }} {
		var v c.Response
		_ = json.Unmarshal(w.Body.Bytes(), &v)
		mutate(&v)
		if c.ValidateResponse(in, s.Catalog.Entries[1], v) == nil {
			t.Fatal("invalid response accepted")
		}
	}
	other, e := Open(dir, s.Token, "http-followup-release", strings.Repeat("c", 64), 4)
	if e != nil {
		t.Fatal(e)
	}
	otherResponse := call(other, "/v1/parse", s.Token, body)
	var replay c.Response
	if e = c.StrictJSON(otherResponse.Body.Bytes(), &replay); e != nil {
		t.Fatal(e)
	}
	if replay.ServiceRelease == response.ServiceRelease {
		t.Fatal("release metadata did not change")
	}
	replay.ServiceRelease = response.ServiceRelease
	replay.ServiceBuildDigest = response.ServiceBuildDigest
	if !reflect.DeepEqual(replay, response) {
		t.Fatal("HTTP update changed fixed parse content")
	}
	for _, mutate := range []func(*c.Request){func(v *c.Request) { v.Input.BytesBase64 = "invalid?" }, func(v *c.Request) { v.Input.ByteLength++ }, func(v *c.Request) { v.Input.SHA256 = strings.Repeat("0", 64) }, func(v *c.Request) { v.Input.ByteLength = c.MaxInput + 1 }} {
		bad := in
		mutate(&bad)
		raw, _ := json.Marshal(bad)
		got := call(s, "/v1/parse", s.Token, raw)
		if got.Code < 400 || got.Code >= 500 {
			t.Fatal("malformed input accepted", got.Code)
		}
	}
	required := append([]c.Entry{}, s.Catalog.Entries...)
	required[1].MethodVersion = "1.2.0"
	if _, e = c.VerifyPackage(dir, required); e == nil {
		t.Fatal("incompatible rollback")
	}
	if _, e = c.VerifyPackage(dir, s.Catalog.Entries); e != nil {
		t.Fatal(e)
	}
	artifact := filepath.Join(dir, "artifacts", s.Catalog.Entries[0].ArtifactDigest)
	if e = os.Rename(artifact, artifact+".missing"); e != nil {
		t.Fatal(e)
	}
	if _, e = c.VerifyPackage(dir, s.Catalog.Entries); e == nil {
		t.Fatal("missing historical artifact")
	}
	if w = call(s, "/v1/parse", s.Token, body); w.Code != 503 {
		t.Fatal(w.Code)
	}
}
func TestResponseByteBudget(t *testing.T) {
	dir := packageAt(t, []byte("#!/bin/sh\nexec /usr/bin/head -c 18000000 /dev/zero\n"))
	s, e := Open(dir, strings.Repeat("x", 40), "test", strings.Repeat("b", 64), 4)
	if e != nil {
		t.Fatal(e)
	}
	body, _ := json.Marshal(requestFor(t, s))
	w := call(s, "/v1/parse", s.Token, body)
	if w.Code == 200 || w.Body.Len() > 1024 {
		t.Fatal("unbounded output", w.Code)
	}
	_, _ = io.Copy(io.Discard, w.Body)
}

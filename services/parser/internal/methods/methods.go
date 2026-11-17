package methods

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	"github.com/WenshuaiDev/InfraPro/services/parser/internal/protocols"
	"strings"
	"time"
)

const Reading = "example.monitoring-json.reading"

const Frame = "hongdian.ddp.tcp.frame"
const Example = "example.hongdian.ddp.tcp.monitoring-json"

func Manifests(artifact, samples string) []c.Manifest {
	semantics := c.Semantics{ValueType: "decimal", Meaning: "示例位移", Unit: "mm", TimestampMeaning: "observation_time", Aggregation: "instantaneous", Direction: "示例正向", Reference: "模拟零点"}
	base := c.Manifest{ContractVersion: c.Version, MethodID: Frame, MethodVersion: "1.0.0", Name: "宏电 DDP TCP 完整帧", Category: "envelope", ProtocolReference: "H7710S-DDP 第1节及2.1—2.3节；TCP 0x01/0x02/0x05/0x09", InputUnit: "complete_frame", Transports: []string{"tcp"}, MaxLength: 1040, Outputs: []c.Output{}, ArtifactDigest: artifact, SampleSuiteDigest: samples, ReleaseNotes: "仅支持完整单帧；不包含 UDP、流分帧、身份认证或未知内层载荷解释。"}
	one := base
	one.MethodID = Example
	one.Name = "DDP 模拟监测 JSON（联调示例，非厂家测量协议）"
	one.Category = "measurement"
	one.Example = true
	one.Outputs = []c.Output{{Key: "displacement", Semantics: semantics}}
	one.ReleaseNotes = "联调示例；严格 observed_at 与 displacement_mm；不判定业务质量。"
	two := one
	two.MethodVersion = "1.1.0"
	two.ReleaseNotes = "联调示例；增加可选 source_status normal/suspect；旧版仍拒绝该字段。"
	reading := one
	reading.MethodID = Reading
	reading.Name = "模拟网关独立 reading（联调示例）"
	reading.InputUnit = "json_reading"
	reading.ReleaseNotes = "只接收经过固定解复用验证的独立 reading JSON，不接收完整帧。"
	return []c.Manifest{base, one, two, reading}
}
func Entry(m c.Manifest) c.Entry {
	outputs, _ := json.Marshal(m.Outputs)
	m.OutputContractDigest = c.Digest(outputs)
	raw, _ := json.Marshal(m)
	return c.Entry{Manifest: m, ManifestDigest: c.Digest(raw), ManifestBase64: base64.StdEncoding.EncodeToString(raw), OutputContractBase64: base64.StdEncoding.EncodeToString(outputs)}
}
func Parse(req c.Request) (c.Response, error) {
	var entry c.Entry
	for _, m := range Manifests(req.Method.ArtifactDigest, strings.Repeat("0", 64)) {
		if m.MethodID == req.Method.ID && m.MethodVersion == req.Method.Version {
			entry = Entry(m)
			entry.ManifestDigest = req.Method.ManifestDigest
		}
	}
	if entry.MethodID == "" {
		return c.Response{}, c.Fail("method_unavailable")
	}
	b, e := c.DecodeInput(req, entry)
	if e != nil {
		return c.Response{}, e
	}
	r := c.Response{ContractVersion: c.Version, ExecutionID: req.ExecutionID, Method: req.Method, InputSHA256: req.Input.SHA256, Identities: []c.Identity{}, Records: []c.Record{}, Fields: []c.Field{}, Steps: []c.Step{}, Diagnostics: []c.Diagnostic{}, Checks: []c.Check{}}
	payload, offset, parent := b, 0, "reading"
	if req.Method.ID == Reading {
		r.Status = "parsed"
		protocols.Step(&r, b, parent, "example.reading.input", 0, len(b), nil, []string{}, c.Digest(b))
	} else {
		typ, body := protocols.DDP(b, &r)
		if r.Status != "parsed" || typ != 9 || req.Method.ID == Frame {
			return r, nil
		}
		payload, offset, parent = body, 15, "payload"
	}
	r.MessageKind = "measurement"
	reject := func(code string, loc *c.Location) {
		r.Status = "rejected"
		r.Records = []c.Record{}
		r.Diagnostics = append(r.Diagnostics, c.Diagnostic{Code: code, Severity: "error", Message: code, Location: loc})
	}
	var fields map[string]json.RawMessage
	if e = c.StrictJSON(payload, &fields); e != nil {
		reject(e.Error(), &c.Location{Range: c.Range{Start: offset, End: offset + len(payload)}})
		return r, nil
	}
	if fields == nil {
		reject("invalid_json_fields", nil)
		return r, nil
	}
	for key := range fields {
		if key != "observed_at" && key != "displacement_mm" && (key != "source_status" || req.Method.Version != "1.1.0") {
			reject("unknown_field", nil)
			return r, nil
		}
	}
	// Decoder offsets refer to the original payload; JSON Pointer is auxiliary.
	locations := map[string]c.Location{}
	d := json.NewDecoder(bytes.NewReader(payload))
	_, _ = d.Token()
	for d.More() {
		key, _ := d.Token()
		start := int(d.InputOffset())
		for start < len(payload) && (payload[start] == ':' || payload[start] == ' ' || payload[start] == '\n' || payload[start] == '\r' || payload[start] == '\t') {
			start++
		}
		var raw json.RawMessage
		_ = d.Decode(&raw)
		locations[key.(string)] = c.Location{Range: c.Range{Start: offset + start, End: offset + int(d.InputOffset())}, Pointer: "/" + key.(string)}
	}
	for _, key := range []string{"observed_at", "displacement_mm"} {
		if _, ok := fields[key]; !ok {
			code := "missing_field"
			if key == "observed_at" {
				code = "missing_timestamp"
			}
			reject(code, nil)
			return r, nil
		}
	}
	get := func(key string) (string, bool) {
		var v string
		raw, ok := fields[key]
		if !ok || bytes.Equal(raw, []byte("null")) || json.Unmarshal(raw, &v) != nil {
			return "", false
		}
		return v, true
	}
	timestamp, ok := get("observed_at")
	observed, err := time.Parse(time.RFC3339Nano, timestamp)
	if !ok || err != nil {
		loc := locations["observed_at"]
		reject("invalid_timestamp", &loc)
		return r, nil
	}
	rawValue, ok := get("displacement_mm")
	value, err := c.Decimal(rawValue)
	if !ok || err != nil {
		loc := locations["displacement_mm"]
		code := "invalid_decimal"
		if err != nil {
			code = err.Error()
		}
		reject(code, &loc)
		return r, nil
	}
	status := ""
	if _, exists := fields["source_status"]; exists {
		status, ok = get("source_status")
		if !ok || (status != "normal" && status != "suspect") {
			loc := locations["source_status"]
			reject("unknown_source_status", &loc)
			return r, nil
		}
	}
	for _, key := range []string{"observed_at", "displacement_mm", "source_status"} {
		if _, exists := fields[key]; !exists {
			continue
		}
		loc := locations[key]
		v, _ := get(key)
		rule := "example.json.string"
		if key == "displacement_mm" {
			rule = "example.decimal38_12.canonical"
			v = value
		}
		protocols.Step(&r, b, key, rule, loc.Range.Start, loc.Range.End, []string{parent}, []string{key}, v)
		r.Fields = append(r.Fields, c.Field{Key: key, Value: v, RawText: string(fields[key]), Location: loc, StepIDs: []string{key}})
	}
	r.Checks = append(r.Checks, c.Check{RuleID: "example.strict_json", Status: "passed", Range: &c.Range{Start: offset, End: offset + len(payload)}})
	r.Records = append(r.Records, c.Record{Index: 0, ObservedAt: &observed, TimeSource: "protocol", SourceStatus: status, Measurements: []c.Measurement{{OutputKey: "displacement", ValueType: "decimal", Value: &value, Unit: "mm", RawText: string(fields["displacement_mm"]), Location: locations["displacement_mm"], StepIDs: []string{parent, "displacement_mm"}}}})
	return r, nil
}

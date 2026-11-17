// Package identification implements finite, code-published synthetic ingestion
// methods. It never returns or accepts an authorized business object identity.
package identification

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/ddp"
	i "github.com/WenshuaiDev/InfraPro/packages/go/foundation/ingestioncontract"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	"slices"
	"strings"
)

const DDP = "ingest.hongdian.ddp.tcp.identity"
const Batch = "example.vendor.ddp-batch.identity"
const Channels = "example.hongdian.ddp.channels"
const Fixed = "example.authenticated-reading.identity"
const Reading = "example.monitoring-json.reading"

func Methods(artifact, samples string) []i.Identification {
	list := []i.Identification{}
	for _, x := range []struct {
		id, name, purpose, input, transport, version string
		units, max                                   int
		example                                      bool
	}{
		{DDP, "H7710S DDP TCP 身份识别", "source_identify", "complete_frame", "tcp", "1.0.0", 1, 1040, false},
		{DDP, "H7710S DDP TCP 身份识别（身份规则相容升级）", "source_identify", "complete_frame", "tcp", "1.1.0", 1, 1040, false},
		{Batch, "模拟厂家 DDP 批次", "source_identify", "vendor_batch", "https_push", "1.0.0", 100, c.MaxInput, true},
		{Fixed, "独占认证来源 reading", "source_identify", "json_reading", "https_push", "1.0.0", 1, 4096, true},
		{Channels, "模拟网关多路 reading 解复用", "terminal_demultiplex", "complete_frame", "tcp", "1.0.0", 16, 1040, true},
	} {
		m := i.Identification{Method: c.Method{ID: x.id, Version: x.version, ArtifactDigest: artifact}, Name: x.name, Purpose: x.purpose, InputUnit: x.input, Transport: x.transport, MaxLength: x.max, MaxUnits: x.units, Seconds: 5, SampleSuiteDigest: samples, Example: x.example, ReleaseNotes: "固定代码及原字节定位；模拟内层不代表厂家测量协议。"}
		if x.version == "1.1.0" {
			m.ReleaseNotes = "保持身份和输入视图不变，增加已验证 ASCII 身份诊断；用于固定旧连接／新连接升级验收。"
		}
		m.Method.ManifestDigest = i.IdentificationDigest(m)
		list = append(list, m)
	}
	return list
}
func Catalog(methods []i.Identification, measurements c.Catalog) i.Catalog {
	v := i.Catalog{Version: 1, IdentificationMethods: methods, IngressProtocols: []i.Protocol{}, Compatibilities: []i.Compatibility{}, ReleaseCoverage: []string{}}
	cover := func(d string) {
		if !slices.Contains(v.ReleaseCoverage, d) {
			v.ReleaseCoverage = append(v.ReleaseCoverage, d)
		}
	}
	for _, m := range methods {
		cover(m.Method.ArtifactDigest)
	}
	for _, m := range measurements.Entries {
		cover(m.ArtifactDigest)
	}
	var demux i.Method
	for _, m := range methods {
		if m.Method.ID == Channels {
			demux = m.Method
		}
	}
	for _, m := range methods {
		if m.Purpose != "source_identify" {
			continue
		}
		p := i.Protocol{ID: "hongdian.ddp.tcp", Version: m.Method.Version, Name: m.Name, Transport: "ddp_tcp", Adapter: i.Adapter{ID: ddp.AdapterID, Version: ddp.AdapterVersion, ArtifactDigest: c.Digest([]byte(ddp.AdapterSource))}, Identification: m.Method, IdentityConvention: "exact-ascii-preserve-zero", InputViews: []string{"complete_frame", "json_reading"}, Example: m.Example}
		if m.Method.ID == Batch || m.Method.ID == Fixed {
			p.ID = "example.vendor.ddp-batch"
			p.Transport = "https_push"
			p.Adapter = i.Adapter{ID: "https.raw-body", Version: "1.0.0", ArtifactDigest: c.Digest([]byte("https.raw-body/v1:mtls,4MiB,raw-body,source-scoped-idempotency"))}
		}
		if m.Method.ID == Fixed {
			p.ID = "example.authenticated-reading"
			p.IdentityConvention = "exclusive-authenticated-source"
			p.InputViews = []string{"json_reading"}
		}
		p.Digest = i.ProtocolDigest(p)
		cover(p.Adapter.ArtifactDigest)
		v.IngressProtocols = append(v.IngressProtocols, p)
		for _, entry := range measurements.Entries {
			if entry.Category != "measurement" || !slices.Contains(p.InputViews, entry.InputUnit) {
				continue
			}
			x := i.Compatibility{ID: p.ID + "." + p.Version + "." + entry.MethodID + "." + entry.MethodVersion, Revision: 1, ProtocolID: p.ID, ProtocolVersion: p.Version, ProtocolDigest: p.Digest, InputView: entry.InputUnit, ParseTransport: "tcp", Measurement: entry.Method(), FixtureDigest: m.SampleSuiteDigest}
			if entry.MethodID == Reading && m.Method.ID != Fixed {
				x.Demultiplexer = &demux
			}
			x.Digest = i.CompatibilityDigest(x)
			v.Compatibilities = append(v.Compatibilities, x)
		}
	}
	return v
}
func transform(raw, out []byte, rule string, start, end int) i.Transform {
	return i.Transform{Rule: rule, Range: i.Range{Start: int64(start), End: int64(end)}, InputSHA256: c.Digest(raw[start:end]), OutputSHA256: c.Digest(out), OutputLength: int64(len(out))}
}
func view(raw []byte, kind, input string, t i.Transform) i.View {
	return i.View{Kind: kind, InputUnit: input, ParseTransport: "tcp", ByteLength: int64(len(raw)), SHA256: c.Digest(raw), Transforms: []i.Transform{t}}
}
func frameUnit(raw []byte, key string, t i.Transform, kind, parent string) (i.Unit, error) {
	f, e := ddp.Decode(raw)
	if e != nil {
		return i.Unit{}, e
	}
	mk := "measurement"
	if f.Type != 9 {
		mk = "control"
	}
	return i.Unit{Key: key, MessageKind: mk, TerminalKey: f.Terminal, View: view(raw, kind, "complete_frame", t), Identities: []i.Identity{{Kind: "terminal", Raw: f.Terminal, Value: f.Terminal, Parent: parent, Location: i.Range{Start: 4, End: 15}}}}, nil
}

// objectRanges obtains exact value spans; StrictJSON must be called first.
func objectRanges(raw []byte) map[string]i.Range {
	out := map[string]i.Range{}
	d := json.NewDecoder(bytes.NewReader(raw))
	_, _ = d.Token()
	for d.More() {
		k, _ := d.Token()
		start := int(d.InputOffset())
		for start < len(raw) && strings.ContainsRune(": \t\r\n", rune(raw[start])) {
			start++
		}
		var value json.RawMessage
		_ = d.Decode(&value)
		out[k.(string)] = i.Range{Start: int64(start), End: d.InputOffset()}
	}
	return out
}
func arrayRanges(raw []byte) []i.Range {
	out := []i.Range{}
	d := json.NewDecoder(bytes.NewReader(raw))
	_, _ = d.Token()
	for d.More() {
		start := int(d.InputOffset())
		for start < len(raw) && strings.ContainsRune(", \t\r\n", rune(raw[start])) {
			start++
		}
		var value json.RawMessage
		_ = d.Decode(&value)
		out = append(out, i.Range{Start: int64(start), End: d.InputOffset()})
	}
	return out
}
func Identify(req i.Request) (i.Response, error) {
	var m i.Identification
	for _, v := range Methods(req.Method.ArtifactDigest, strings.Repeat("0", 64)) {
		if v.Method.ID == req.Method.ID && v.Method.Version == req.Method.Version {
			m = v
			m.Method = req.Method
		}
	}
	if m.Method.ID == "" {
		return i.Response{}, c.Fail("method_unavailable")
	}
	b, e := i.DecodeInput(req, m)
	if e != nil {
		return i.Response{}, e
	}
	r := i.Response{Version: 1, ExecutionID: req.ExecutionID, Method: req.Method, InputSHA256: req.Input.SHA256, ServiceRelease: "runner", ServiceBuildDigest: req.Method.ArtifactDigest, Status: "identified", Units: []i.Unit{}, Diagnostics: []string{}}
	reject := func(e error) (i.Response, error) {
		r.Status = "rejected"
		r.Units = []i.Unit{}
		r.Diagnostics = append(r.Diagnostics, e.Error())
		return r, nil
	}
	switch m.Method.ID {
	case Fixed:
		if !req.Context.ExclusiveAuthenticated || len(req.Context.FixedTerminal) != 11 {
			return reject(c.Fail("identity_missing"))
		}
		var input map[string]json.RawMessage
		if c.StrictJSON(b, &input) != nil || input == nil {
			return reject(c.Fail("invalid_reading"))
		}
		r.Units = append(r.Units, i.Unit{Key: "reading", MessageKind: "measurement", TerminalKey: req.Context.FixedTerminal, View: view(b, "whole", "json_reading", transform(b, b, "identity", 0, len(b))), Identities: []i.Identity{{Kind: "terminal", Parent: "authenticated_source", Raw: req.Context.FixedTerminal, Value: req.Context.FixedTerminal}}})
	case DDP:
		u, e := frameUnit(b, "frame", transform(b, b, "identity", 0, len(b)), "whole", "input")
		if e != nil {
			return reject(e)
		}
		r.Units = append(r.Units, u)
		if m.Method.Version == "1.1.0" {
			r.Diagnostics = append(r.Diagnostics, "identity_ascii_verified")
		}
	case Batch:
		var body struct {
			Messages []struct {
				ID     string `json:"id"`
				Base64 string `json:"ddp_base64"`
			} `json:"messages"`
		}
		if e = c.StrictJSON(b, &body); e != nil {
			return reject(e)
		}
		if len(body.Messages) < 1 || len(body.Messages) > 100 {
			return reject(c.Fail("batch_limit"))
		}
		root := objectRanges(b)["messages"]
		items := arrayRanges(b[root.Start:root.End])
		keys := map[string]bool{}
		for n, item := range body.Messages {
			if item.ID == "" || len(item.ID) > 128 || keys[item.ID] {
				return reject(c.Fail("duplicate_or_missing_id"))
			}
			keys[item.ID] = true
			frame, e := base64.StdEncoding.Strict().DecodeString(item.Base64)
			if e != nil {
				return reject(c.Fail("invalid_base64"))
			}
			itemStart := root.Start + items[n].Start
			span := objectRanges(b[itemStart : root.Start+items[n].End])["ddp_base64"]
			u, e := frameUnit(frame, item.ID, transform(b, frame, "json.base64", int(itemStart+span.Start), int(itemStart+span.End)), "decoded", "unit")
			if e != nil {
				return reject(e)
			}
			u.ExternalID = item.ID
			r.Units = append(r.Units, u)
		}
	case Channels:
		frame, e := ddp.Decode(b)
		if e != nil {
			return reject(e)
		}
		if frame.Type != 9 {
			return reject(c.Fail("measurement_required"))
		}
		var body struct {
			Channels []struct {
				Key     string          `json:"key"`
				Reading json.RawMessage `json:"reading"`
			} `json:"channels"`
		}
		if e = c.StrictJSON(frame.Payload, &body); e != nil {
			return reject(e)
		}
		if len(body.Channels) < 1 || len(body.Channels) > 16 {
			return reject(c.Fail("channels_limit"))
		}
		root := objectRanges(frame.Payload)["channels"]
		items := arrayRanges(frame.Payload[root.Start:root.End])
		keys := map[string]bool{}
		for n, item := range body.Channels {
			if item.Key == "" || len(item.Key) > 128 || keys[item.Key] {
				return reject(c.Fail("duplicate_or_missing_child"))
			}
			keys[item.Key] = true
			itemStart := 15 + root.Start + items[n].Start
			ranges := objectRanges(b[itemStart : 15+root.Start+items[n].End])
			span := ranges["reading"]
			start, end := itemStart+span.Start, itemStart+span.End
			if len(item.Reading) < 2 || item.Reading[0] != '{' {
				return reject(c.Fail("invalid_reading"))
			}
			v := view(b[start:end], "range", "json_reading", transform(b, b[start:end], "range", int(start), int(end)))
			idSpan := ranges["key"]
			r.Units = append(r.Units, i.Unit{Key: fmt.Sprintf("child:%s", item.Key), MessageKind: "measurement", TerminalKey: frame.Terminal, ChildKey: item.Key, View: v, Identities: []i.Identity{{Kind: "terminal", Raw: frame.Terminal, Value: frame.Terminal, Parent: "input", Location: i.Range{Start: 4, End: 15}}, {Kind: "child", Raw: item.Key, Value: item.Key, Parent: "input", Location: i.Range{Start: itemStart + idSpan.Start, End: itemStart + idSpan.End}}}})
		}
	}
	return r, nil
}

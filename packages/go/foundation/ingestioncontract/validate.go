package ingestioncontract

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/ddp"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	"github.com/google/uuid"
	"regexp"
	"slices"
)

var idPattern = regexp.MustCompile(`^[a-z][a-z0-9.-]{1,159}$`)
var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

func Hash(v any) string                            { b, _ := json.Marshal(v); return c.Digest(b) }
func IdentificationDigest(v Identification) string { v.Method.ManifestDigest = ""; return Hash(v) }
func ProtocolDigest(v Protocol) string             { v.Digest = ""; return Hash(v) }
func CompatibilityDigest(v Compatibility) string   { v.Digest = ""; return Hash(v) }
func ValidMethod(v Method) bool {
	return idPattern.MatchString(v.ID) && versionPattern.MatchString(v.Version) && c.IsDigest(v.ManifestDigest) && c.IsDigest(v.ArtifactDigest)
}

func ValidateCatalog(v Catalog, measurement c.Catalog) error {
	bad := func() error { return c.Fail("invalid_ingestion_catalog") }
	if v.Version != Version || len(v.IdentificationMethods) == 0 || len(v.IngressProtocols) == 0 || len(v.Compatibilities) == 0 || len(v.IdentificationMethods) > 256 || len(v.IngressProtocols) > 256 || len(v.Compatibilities) > 4096 {
		return bad()
	}
	methods := map[Method]Identification{}
	ids := map[string]bool{}
	for _, m := range v.IdentificationMethods {
		key := m.Method.ID + "@" + m.Method.Version
		if ids[key] || !ValidMethod(m.Method) || m.Method.ManifestDigest != IdentificationDigest(m) || !slices.Contains([]string{"source_identify", "terminal_demultiplex"}, m.Purpose) || m.MaxLength < 1 || m.MaxLength > c.MaxInput || m.MaxUnits < 1 || m.MaxUnits > MaxUnits || m.Seconds != 5 || !c.IsDigest(m.SampleSuiteDigest) || !slices.Contains(v.ReleaseCoverage, m.Method.ArtifactDigest) {
			return bad()
		}
		methods[m.Method] = m
		ids[key] = true
	}
	protocols := map[string]Protocol{}
	for _, p := range v.IngressProtocols {
		key := p.ID + "@" + p.Version
		if _, ok := protocols[key]; ok {
			return bad()
		}
		m, ok := methods[p.Identification]
		if !ok || m.Purpose != "source_identify" || !idPattern.MatchString(p.ID) || !versionPattern.MatchString(p.Version) || p.Digest != ProtocolDigest(p) || !slices.Contains([]string{"ddp_tcp", "https_push"}, p.Transport) || !c.IsDigest(p.Adapter.ArtifactDigest) || !slices.Contains(v.ReleaseCoverage, p.Adapter.ArtifactDigest) || !slices.Contains([]string{"exact-ascii-preserve-zero", "exclusive-authenticated-source"}, p.IdentityConvention) || len(p.InputViews) == 0 {
			return bad()
		}
		protocols[key] = p
	}
	ids = map[string]bool{}
	for _, x := range v.Compatibilities {
		key := x.ID + "/" + stringID(x.Revision)
		p, ok := protocols[x.ProtocolID+"@"+x.ProtocolVersion]
		if ids[key] || !ok || p.Digest != x.ProtocolDigest || x.Revision < 1 || x.Digest != CompatibilityDigest(x) || !c.IsDigest(x.FixtureDigest) || !slices.Contains(p.InputViews, x.InputView) || x.ParseTransport != "tcp" {
			return bad()
		}
		if x.Demultiplexer != nil {
			m, ok := methods[*x.Demultiplexer]
			if !ok || m.Purpose != "terminal_demultiplex" {
				return bad()
			}
		}
		found := false
		for _, m := range measurement.Entries {
			if m.Method() == x.Measurement && m.Category == "measurement" && m.InputUnit == x.InputView && slices.Contains(m.Transports, x.ParseTransport) {
				found = true
			}
		}
		if !found || !slices.Contains(v.ReleaseCoverage, x.Measurement.ArtifactDigest) {
			return bad()
		}
		ids[key] = true
	}
	return nil
}
func stringID(n int64) string { b, _ := json.Marshal(n); return string(b) }

func DecodeInput(r Request, m Identification) ([]byte, error) {
	if r.Version != Version || r.Method != m.Method {
		return nil, c.Fail("version_conflict")
	}
	if _, e := uuid.Parse(r.ExecutionID); e != nil || r.Context.ReceivedAt.IsZero() || r.Context.InputUnit != m.InputUnit || r.Context.Transport != m.Transport || len(r.TraceID) > 128 {
		return nil, c.Fail("invalid_request")
	}
	if r.Input.ByteLength < 1 || r.Input.ByteLength > int64(m.MaxLength) || len(r.Input.BytesBase64) > base64.StdEncoding.EncodedLen(c.MaxInput) {
		return nil, c.Fail("limit_exceeded")
	}
	b, e := base64.StdEncoding.Strict().DecodeString(r.Input.BytesBase64)
	if e != nil || int64(len(b)) != r.Input.ByteLength || c.Digest(b) != r.Input.SHA256 {
		return nil, c.Fail("input_hash_mismatch")
	}
	return b, nil
}
func inRange(r Range, n int) bool { return r.Start >= 0 && r.Start < r.End && r.End <= int64(n) }

// Materialize independently checks every transform against original bytes. No
// executable code, remote URL or caller-selected object key is accepted.
func Materialize(raw []byte, v View) ([]byte, error) {
	bad := func() ([]byte, error) { return nil, c.Fail("invalid_input_view") }
	if !slices.Contains([]string{"whole", "range", "decoded"}, v.Kind) || v.ParseTransport != "tcp" || !slices.Contains([]string{"complete_frame", "json_reading"}, v.InputUnit) || len(v.Transforms) < 1 || len(v.Transforms) > 8 {
		return bad()
	}
	b := raw
	for _, t := range v.Transforms {
		if !inRange(t.Range, len(b)) {
			return bad()
		}
		part := b[t.Range.Start:t.Range.End]
		if c.Digest(part) != t.InputSHA256 {
			return bad()
		}
		switch t.Rule {
		case "identity", "range":
			b = part
		case "json.base64":
			var s string
			if c.StrictJSON(part, &s) != nil {
				return bad()
			}
			var e error
			b, e = base64.StdEncoding.Strict().DecodeString(s)
			if e != nil {
				return bad()
			}
		default:
			return bad()
		}
		if int64(len(b)) != t.OutputLength || c.Digest(b) != t.OutputSHA256 {
			return bad()
		}
	}
	if int64(len(b)) != v.ByteLength || c.Digest(b) != v.SHA256 {
		return bad()
	}
	if v.Kind == "whole" && (len(v.Transforms) != 1 || v.Transforms[0].Rule != "identity" || !bytes.Equal(b, raw)) {
		return bad()
	}
	return b, nil
}
func ValidateResponse(req Request, m Identification, r Response) error {
	raw, e := DecodeInput(req, m)
	if e != nil {
		return e
	}
	bad := func() error { return c.Fail("invalid_identification_response") }
	if r.Version != Version || r.ExecutionID != req.ExecutionID || r.Method != req.Method || r.InputSHA256 != req.Input.SHA256 || !c.IsDigest(r.ServiceBuildDigest) || r.ServiceRelease == "" || r.Units == nil || r.Diagnostics == nil || len(r.Diagnostics) > 100 || len(r.Units) > m.MaxUnits || !slices.Contains([]string{"identified", "rejected", "unsupported"}, r.Status) {
		return bad()
	}
	if r.Status != "identified" {
		if len(r.Units) != 0 {
			return bad()
		}
		return nil
	}
	if len(r.Units) == 0 {
		return bad()
	}
	keys := map[string]bool{}
	ranges := []Range{}
	for _, u := range r.Units {
		if u.Key == "" || len(u.Key) > 200 || keys[u.Key] || len(u.TerminalKey) != 11 || len(u.ChildKey) > 128 || !slices.Contains([]string{"measurement", "control"}, u.MessageKind) {
			return bad()
		}
		keys[u.Key] = true
		b, e := Materialize(raw, u.View)
		if e != nil {
			return e
		}
		// A claimed complete frame must independently decode as one exact frame;
		// an HTTP root containing siblings can never masquerade as a unit view.
		if u.View.InputUnit == "complete_frame" {
			f, e := ddp.Decode(b)
			if e != nil || f.Terminal != u.TerminalKey || u.ChildKey != "" || ((f.Type == 9) != (u.MessageKind == "measurement")) {
				return bad()
			}
		}
		for _, ch := range []byte(u.TerminalKey) {
			if ch < 32 || ch > 126 {
				return bad()
			}
		}
		first := u.View.Transforms[0].Range
		for _, old := range ranges {
			if first.Start < old.End && old.Start < first.End {
				return bad()
			}
		}
		ranges = append(ranges, first)
		terminal, child := false, u.ChildKey == ""
		for _, id := range u.Identities {
			if id.Parent == "authenticated_source" {
				if !req.Context.ExclusiveAuthenticated || len(req.Context.FixedTerminal) != 11 || id.Kind != "terminal" || id.Value != req.Context.FixedTerminal || id.Raw != id.Value || id.Location != (Range{}) {
					return bad()
				}
				terminal = id.Value == u.TerminalKey
				continue
			}
			parent := raw
			if id.Parent == "unit" {
				parent = b
			} else if id.Parent != "input" {
				return bad()
			}
			if !inRange(id.Location, len(parent)) {
				return bad()
			}
			part := parent[id.Location.Start:id.Location.End]
			actual := string(part)
			if len(part) > 0 && part[0] == '"' {
				if c.StrictJSON(part, &actual) != nil {
					return bad()
				}
			}
			if id.Raw != actual || id.Value != actual {
				return bad()
			}
			switch id.Kind {
			case "terminal":
				terminal = id.Value == u.TerminalKey
			case "child":
				child = id.Value == u.ChildKey
			default:
				return bad()
			}
		}
		if !terminal || !child {
			return bad()
		}
	}
	return nil
}

package parsingcontract

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

func Digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

var digestRE = regexp.MustCompile(`^[a-f0-9]{64}$`)
var idRE = regexp.MustCompile(`^[a-z][a-z0-9.-]{1,159}$`)
var versionRE = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var uuidRE = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
var decimalRE = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?$`)

func IsDigest(s string) bool { return digestRE.MatchString(s) }

// StrictJSON rejects duplicate keys, invalid UTF-8, trailing values, unknown
// members and excessive nesting before typed decoding. Numbers never use floats.
func StrictJSON(b []byte, out any) error {
	if !utf8.Valid(b) {
		return Fail("invalid_utf8")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if e := walkJSON(d, 0); e != nil {
		return e
	}
	if _, e := d.Token(); e != io.EOF {
		return Fail("invalid_json")
	}
	d = json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	d.DisallowUnknownFields()
	if e := d.Decode(out); e != nil {
		return Fail("invalid_json_fields")
	}
	return nil
}
func walkJSON(d *json.Decoder, depth int) error {
	if depth > MaxDepth {
		return Fail("limit_exceeded")
	}
	t, e := d.Token()
	if e != nil {
		return Fail("invalid_json")
	}
	x, ok := t.(json.Delim)
	if !ok {
		return nil
	}
	switch x {
	case '{':
		keys := map[string]bool{}
		for d.More() {
			k, e := d.Token()
			if e != nil {
				return Fail("invalid_json")
			}
			key, ok := k.(string)
			if !ok {
				return Fail("invalid_json")
			}
			if keys[key] {
				return Fail("duplicate_key")
			}
			keys[key] = true
			if e = walkJSON(d, depth+1); e != nil {
				return e
			}
		}
	case '[':
		for d.More() {
			if e = walkJSON(d, depth+1); e != nil {
				return e
			}
		}
	default:
		return Fail("invalid_json")
	}
	_, e = d.Token()
	if e != nil {
		return Fail("invalid_json")
	}
	return nil
}
func DecodeInput(r Request, entry Entry) ([]byte, error) {
	if r.ContractVersion != Version {
		return nil, Fail("contract_mismatch")
	}
	if r.Method != entry.Method() {
		return nil, Fail("version_conflict")
	}
	if !uuidRE.MatchString(r.ExecutionID) || len(r.TraceID) > 128 || r.Context.ReceivedAt.IsZero() {
		return nil, Fail("invalid_request")
	}
	if !slices.Contains(entry.Transports, r.Context.Transport) {
		return nil, Fail("unsupported_transport")
	}
	if r.Input.ByteLength < 1 || r.Input.ByteLength > MaxInput || r.Input.ByteLength > entry.MaxLength || len(r.Input.BytesBase64) > base64.StdEncoding.EncodedLen(MaxInput) {
		return nil, Fail("limit_exceeded")
	}
	b, e := base64.StdEncoding.Strict().DecodeString(r.Input.BytesBase64)
	if e != nil {
		return nil, Fail("invalid_base64")
	}
	if len(b) != r.Input.ByteLength || Digest(b) != r.Input.SHA256 {
		return nil, Fail("input_hash_mismatch")
	}
	return b, nil
}
func DecodeRepresentation(representation string, b []byte) ([]byte, error) {
	if len(b) > MaxRequest {
		return nil, Fail("limit_exceeded")
	}
	switch representation {
	case "binary":
	case "utf8":
		if !utf8.Valid(b) {
			return nil, Fail("invalid_utf8")
		}
	case "hex":
		var compact []byte
		for _, c := range b {
			if c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '\v' || c == '\f' {
				if len(compact)%2 != 0 {
					return nil, Fail("invalid_hex")
				}
				continue
			}
			compact = append(compact, c)
		}
		if len(compact)%2 != 0 {
			return nil, Fail("invalid_hex")
		}
		out := make([]byte, len(compact)/2)
		if _, e := hex.Decode(out, compact); e != nil {
			return nil, Fail("invalid_hex")
		}
		b = out
	default:
		return nil, Fail("invalid_representation")
	}
	if len(b) < 1 || len(b) > MaxInput {
		return nil, Fail("limit_exceeded")
	}
	return b, nil
}
func Decimal(s string) (string, error) {
	if len(s) > 80 || !decimalRE.MatchString(s) {
		return "", Fail("invalid_decimal")
	}
	negative := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	parts := strings.Split(s, ".")
	if len(parts[0]) > 26 || (len(parts) == 2 && len(parts[1]) > 12) {
		return "", Fail("decimal_overflow")
	}
	result := parts[0]
	if len(parts) == 2 {
		f := strings.TrimRight(parts[1], "0")
		if f != "" {
			result += "." + f
		}
	}
	if negative && result != "0" {
		result = "-" + result
	}
	return result, nil
}
func ValidateCatalog(c Catalog) error {
	if c.ContractVersion != Version || c.ServiceRelease == "" || !IsDigest(c.ServiceBuildDigest) || len(c.Entries) < 1 || len(c.Entries) > 1000 {
		return Fail("contract_mismatch")
	}
	seen := map[string]bool{}
	for _, entry := range c.Entries {
		key := entry.MethodID + "@" + entry.MethodVersion
		if seen[key] || !idRE.MatchString(entry.MethodID) || !versionRE.MatchString(entry.MethodVersion) {
			return Fail("invalid_manifest")
		}
		seen[key] = true
		if !IsDigest(entry.ArtifactDigest) || !IsDigest(entry.OutputContractDigest) || !IsDigest(entry.SampleSuiteDigest) || entry.MaxLength < 1 || entry.MaxLength > MaxInput || !slices.Contains([]string{"measurement", "envelope"}, entry.Category) || len(entry.Transports) < 1 {
			return Fail("invalid_manifest")
		}
		b, e := base64.StdEncoding.Strict().DecodeString(entry.ManifestBase64)
		if e != nil || Digest(b) != entry.ManifestDigest {
			return Fail("manifest_hash_mismatch")
		}
		var m Manifest
		if StrictJSON(b, &m) != nil || !reflect.DeepEqual(m, entry.Manifest) {
			return Fail("invalid_manifest")
		}
		b, e = base64.StdEncoding.Strict().DecodeString(entry.OutputContractBase64)
		if e != nil || Digest(b) != entry.OutputContractDigest {
			return Fail("output_contract_hash_mismatch")
		}
		var outputs []Output
		if StrictJSON(b, &outputs) != nil || !reflect.DeepEqual(outputs, entry.Outputs) {
			return Fail("invalid_output_contract")
		}
		keys := map[string]bool{}
		for _, o := range outputs {
			if o.Key == "" || keys[o.Key] || !slices.Contains([]string{"decimal", "integer", "string", "boolean"}, o.ValueType) {
				return Fail("invalid_output_contract")
			}
			keys[o.Key] = true
		}
	}
	return nil
}
func ValidateResponse(r Request, e Entry, v Response) error {
	bad := func() error { return Fail("invalid_response") }
	if v.ContractVersion != Version || v.ExecutionID != r.ExecutionID || v.Method != r.Method || v.InputSHA256 != r.Input.SHA256 || v.ServiceRelease == "" || !IsDigest(v.ServiceBuildDigest) {
		return bad()
	}
	if !slices.Contains([]string{"parsed", "rejected", "unsupported"}, v.Status) || !slices.Contains([]string{"measurement", "control", "envelope"}, v.MessageKind) || len(v.Records) > MaxRecords || len(v.Steps) > MaxSteps {
		return bad()
	}
	if (v.Status != "parsed" || v.MessageKind != "measurement") && len(v.Records) != 0 {
		return bad()
	}
	rangeOK := func(p Range) bool { return p.Start >= 0 && p.End >= p.Start && p.End <= r.Input.ByteLength }
	steps := map[string]bool{}
	for _, s := range v.Steps {
		if s.ID == "" || steps[s.ID] || s.RuleID == "" || !rangeOK(s.Range) || !IsDigest(s.InputDigest) || !IsDigest(s.OutputDigest) {
			return bad()
		}
		for _, id := range s.ParentIDs {
			if !steps[id] {
				return bad()
			}
		}
		steps[s.ID] = true
	}
	refsOK := func(ids []string) bool {
		if len(ids) == 0 {
			return false
		}
		for _, id := range ids {
			if !steps[id] {
				return false
			}
		}
		return true
	}
	for _, f := range v.Fields {
		if !rangeOK(f.Location.Range) || !refsOK(f.StepIDs) {
			return bad()
		}
	}
	for _, id := range v.Identities {
		if id.Authenticated || !rangeOK(id.Location.Range) {
			return bad()
		}
	}
	for _, d := range v.Diagnostics {
		if d.Location != nil && !rangeOK(d.Location.Range) {
			return bad()
		}
	}
	for _, c := range v.Checks {
		if !slices.Contains([]string{"passed", "failed", "not_applicable"}, c.Status) || c.Range != nil && !rangeOK(*c.Range) {
			return bad()
		}
	}
	outputs := map[string]Output{}
	for _, o := range e.Outputs {
		outputs[o.Key] = o
	}
	for i, rec := range v.Records {
		if rec.Index != i {
			return bad()
		}
		if rec.ObservedAt == nil {
			found := false
			for _, d := range v.Diagnostics {
				found = found || d.Code == "missing_timestamp"
			}
			if !found {
				return bad()
			}
		} else if rec.TimeSource != "protocol" {
			return bad()
		}
		seen := map[string]bool{}
		for _, m := range rec.Measurements {
			o, ok := outputs[m.OutputKey]
			if !ok || seen[m.OutputKey] || m.ValueType != o.ValueType || m.Unit != o.Unit || !rangeOK(m.Location.Range) || !refsOK(m.StepIDs) {
				return bad()
			}
			seen[m.OutputKey] = true
			if m.Value != nil {
				switch m.ValueType {
				case "decimal", "integer":
					n, err := Decimal(*m.Value)
					if err != nil || n != *m.Value || m.ValueType == "integer" && strings.Contains(n, ".") {
						return bad()
					}
				case "boolean":
					if *m.Value != "true" && *m.Value != "false" {
						return bad()
					}
				case "string":
					if len(*m.Value) > 65536 {
						return bad()
					}
				}
			}
		}
	}
	return nil
}

// RedactRaw preserves typed measurements and locations, never original text.
func RedactRaw(v Response) Response {
	v.Fields = slices.Clone(v.Fields)
	for i := range v.Fields {
		v.Fields[i].RawText = ""
	}
	v.Identities = slices.Clone(v.Identities)
	for i := range v.Identities {
		v.Identities[i].Hex = ""
	}
	v.Records = slices.Clone(v.Records)
	for i := range v.Records {
		v.Records[i].Measurements = slices.Clone(v.Records[i].Measurements)
		for j := range v.Records[i].Measurements {
			v.Records[i].Measurements[j].RawText = ""
		}
	}
	return v
}

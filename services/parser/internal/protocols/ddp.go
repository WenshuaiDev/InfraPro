// Package protocols implements the documented H7710S TCP complete-frame subset.
package protocols

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
)

func DDP(b []byte, r *c.Response) (byte, []byte) {
	r.Status = "rejected"
	r.MessageKind = "envelope"
	r.Checks = append(r.Checks, c.Check{RuleID: "ddp.checksum", Status: "not_applicable"})
	fail := func(code string, start, end int) {
		r.Diagnostics = append(r.Diagnostics, c.Diagnostic{Code: code, Severity: "error", Message: code, Location: &c.Location{Range: c.Range{Start: start, End: end}}})
		r.Checks = append(r.Checks, c.Check{RuleID: code, Status: "failed", Range: &c.Range{Start: start, End: end}})
	}
	if len(b) < 16 {
		fail("ddp.truncated", 0, len(b))
		return 0, nil
	}
	if len(b) > 1040 {
		fail("limit_exceeded", 0, len(b))
		return 0, nil
	}
	if b[0] != 0x7b {
		fail("ddp.start_marker", 0, 1)
		return 0, nil
	}
	if int(binary.BigEndian.Uint16(b[2:4])) != len(b) {
		fail("ddp.length", 2, 4)
		return 0, nil
	}
	if b[len(b)-1] != 0x7b {
		fail("ddp.end_marker", len(b)-1, len(b))
		return 0, nil
	}
	r.Checks = append(r.Checks, c.Check{RuleID: "ddp.complete_frame", Status: "passed", Range: &c.Range{Start: 0, End: len(b)}})
	Step(r, b, "frame", "ddp.tcp.complete_frame", 0, len(b), nil, []string{"packet_type", "length", "identity", "payload"}, fmt.Sprint(len(b)))
	Step(r, b, "header", "ddp.header.network_byte_order", 0, 15, []string{"frame"}, []string{"packet_type", "length", "identity"}, fmt.Sprintf("%02x/%d", b[1], len(b)))
	r.Fields = append(r.Fields, c.Field{Key: "packet_type", Value: fmt.Sprintf("0x%02X", b[1]), Location: c.Location{Range: c.Range{Start: 1, End: 2}}, StepIDs: []string{"header"}}, c.Field{Key: "length", Value: fmt.Sprint(len(b)), Location: c.Location{Range: c.Range{Start: 2, End: 4}}, StepIDs: []string{"header"}})
	id := c.Identity{Role: "dtu", Hex: hex.EncodeToString(b[4:15]), Location: c.Location{Range: c.Range{Start: 4, End: 15}}}
	printable := true
	for _, ch := range b[4:15] {
		if ch < 32 || ch > 126 {
			printable = false
		}
	}
	if printable {
		id.Text = string(b[4:15])
	}
	r.Identities = append(r.Identities, id)
	switch b[1] {
	case 1:
		if len(b) != 22 {
			fail("ddp.registration_length", 2, 4)
			return 0, nil
		}
		Step(r, b, "registration", "ddp.registration.ip_port", 15, 21, []string{"frame"}, []string{"mobile_ip", "mobile_port"}, hex.EncodeToString(b[15:21]))
		r.Fields = append(r.Fields, c.Field{Key: "mobile_ip", Value: fmt.Sprintf("%d.%d.%d.%d", b[15], b[16], b[17], b[18]), Location: c.Location{Range: c.Range{Start: 15, End: 19}}, StepIDs: []string{"registration"}}, c.Field{Key: "mobile_port", Value: fmt.Sprint(binary.BigEndian.Uint16(b[19:21])), Location: c.Location{Range: c.Range{Start: 19, End: 21}}, StepIDs: []string{"registration"}})
		r.MessageKind = "control"
	case 2, 5:
		if len(b) != 16 {
			fail("ddp.control_length", 2, 4)
			return 0, nil
		}
		r.MessageKind = "control"
	case 9:
		Step(r, b, "payload", "ddp.tcp.payload", 15, len(b)-1, []string{"frame"}, []string{"payload"}, c.Digest(b[15:len(b)-1]))
		r.Fields = append(r.Fields, c.Field{Key: "payload", Value: c.Digest(b[15 : len(b)-1]), Location: c.Location{Range: c.Range{Start: 15, End: len(b) - 1}}, StepIDs: []string{"payload"}})
	default:
		r.Status = "unsupported"
		r.Diagnostics = append(r.Diagnostics, c.Diagnostic{Code: "ddp.unsupported_packet_type", Severity: "error", Message: "该包型不在首批 TCP 完整帧支持范围内。", Location: &c.Location{Range: c.Range{Start: 1, End: 2}}})
		return b[1], nil
	}
	r.Status = "parsed"
	return b[1], b[15 : len(b)-1]
}
func Step(r *c.Response, b []byte, id, rule string, start, end int, parents, fields []string, value string) {
	if parents == nil {
		parents = []string{}
	}
	if fields == nil {
		fields = []string{}
	}
	r.Steps = append(r.Steps, c.Step{ID: id, RuleID: rule, Range: c.Range{Start: start, End: end}, Status: "passed", ParentIDs: parents, FieldKeys: fields, InputDigest: c.Digest(b[start:end]), OutputDigest: c.Digest([]byte(value))})
}

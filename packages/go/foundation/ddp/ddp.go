// Package ddp implements only the documented H7710S DDP TCP frame layout.
// It has no business routing, storage, authentication or measurement semantics.
package ddp

import (
	"encoding/binary"
	"errors"
	"io"
)

const MaxFrame = 1040
const AdapterID = "hongdian.ddp.tcp"
const AdapterVersion = "1.0.0"

// AdapterSource identifies the finite wire rules. Release packages additionally
// attest the executable digest; callers cannot select arbitrary adapters.
const AdapterSource = "H7710S/TCP:7b,type,u16be-length,11-byte-id,payload,7b;max1040;register01-81,data09-85,logout02-82;v1"

type Frame struct {
	Type     byte
	Terminal string
	Payload  []byte
}

func Decode(b []byte) (Frame, error) {
	bad := func(code string) (Frame, error) { return Frame{}, errors.New(code) }
	if len(b) < 16 || len(b) > MaxFrame {
		return bad("ddp.length")
	}
	if b[0] != 0x7b || b[len(b)-1] != 0x7b {
		return bad("ddp.marker")
	}
	if int(binary.BigEndian.Uint16(b[2:4])) != len(b) {
		return bad("ddp.length")
	}
	for _, ch := range b[4:15] {
		if ch < 32 || ch > 126 {
			return bad("identity_invalid")
		}
	}
	switch b[1] {
	case 1:
		if len(b) != 22 {
			return bad("ddp.registration_length")
		}
	case 2, 5, 0x81, 0x82, 0x85:
		if len(b) != 16 {
			return bad("ddp.control_length")
		}
	case 9:
	default:
		return bad("ddp.unsupported_packet_type")
	}
	return Frame{Type: b[1], Terminal: string(b[4:15]), Payload: b[15 : len(b)-1]}, nil
}

func Encode(typ byte, terminal string, payload []byte) ([]byte, error) {
	if len(terminal) != 11 || len(payload)+16 > MaxFrame {
		return nil, errors.New("ddp.length")
	}
	b := make([]byte, 16+len(payload))
	b[0] = 0x7b
	b[1] = typ
	binary.BigEndian.PutUint16(b[2:4], uint16(len(b)))
	copy(b[4:15], terminal)
	copy(b[15:], payload)
	b[len(b)-1] = 0x7b
	_, e := Decode(b)
	return b, e
}

// Read returns a complete frame or the exact bounded fragment received before
// failure. It never scans past a bad prefix or depends on TCP read boundaries.
func Read(r io.Reader) ([]byte, error) {
	header := make([]byte, 4)
	n, e := io.ReadFull(r, header)
	if e != nil {
		return header[:n], e
	}
	size := int(binary.BigEndian.Uint16(header[2:4]))
	if header[0] != 0x7b || size < 16 || size > MaxFrame {
		return header, errors.New("ddp.invalid_header")
	}
	b := make([]byte, size)
	copy(b, header)
	n, e = io.ReadFull(r, b[4:])
	b = b[:4+n]
	if e != nil {
		return b, e
	}
	_, e = Decode(b)
	return b, e
}

func Acknowledgement(f Frame) ([]byte, error) {
	typ := byte(0)
	switch f.Type {
	case 1:
		typ = 0x81
	case 9:
		typ = 0x85
	case 2:
		typ = 0x82
	default:
		return nil, errors.New("ddp.unsupported_packet_type")
	}
	return Encode(typ, f.Terminal, nil)
}

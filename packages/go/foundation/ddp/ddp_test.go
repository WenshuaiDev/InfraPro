package ddp

import (
	"bytes"
	"io"
	"testing"
)

type oneByte struct{ r io.Reader }

func (r oneByte) Read(b []byte) (int, error) { return r.r.Read(b[:1]) }
func TestStreamBoundariesAndAcknowledgements(t *testing.T) {
	for _, typ := range []byte{1, 9, 2} {
		payload := []byte{}
		if typ == 1 {
			payload = make([]byte, 6)
		}
		if typ == 9 {
			payload = []byte("{a{b}")
		}
		frame, e := Encode(typ, "00000000001", payload)
		if e != nil {
			t.Fatal(e)
		}
		stream := oneByte{bytes.NewReader(append(append([]byte{}, frame...), frame...))}
		for range 2 {
			actual, e := Read(stream)
			if e != nil || !bytes.Equal(actual, frame) {
				t.Fatal("framing", e)
			}
		}
		f, _ := Decode(frame)
		ack, e := Acknowledgement(f)
		if e != nil {
			t.Fatal(e)
		}
		expected := map[byte]byte{1: 0x81, 9: 0x85, 2: 0x82}[typ]
		if len(ack) != 16 || ack[1] != expected {
			t.Fatal("ack")
		}
	}
	for _, b := range [][]byte{{0, 9, 0, 16}, {0x7b, 9, 0xff, 0xff}, {0x7b, 9, 0, 16, 'x'}} {
		fragment, e := Read(bytes.NewReader(b))
		if e == nil || !bytes.Equal(fragment, b) {
			t.Fatal("fragment discarded", fragment, e)
		}
	}
}

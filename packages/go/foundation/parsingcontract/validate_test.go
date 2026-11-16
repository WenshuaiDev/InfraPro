package parsingcontract

import (
	"strings"
	"testing"
)

func TestStrictWireAndRepresentations(t *testing.T) {
	for _, raw := range []string{`{"x":1,"x":2}`, `{} {}`, `{"x":NaN}`, strings.Repeat("[", 65) + "0" + strings.Repeat("]", 65), "{\"x\":\"\xff\"}"} {
		var v any
		if StrictJSON([]byte(raw), &v) == nil {
			t.Fatal(raw)
		}
	}
	for _, raw := range []string{"0", "0 1", "GG", "7Ｂ", "0x12"} {
		if _, e := DecodeRepresentation("hex", []byte(raw)); e == nil {
			t.Fatal(raw)
		}
	}
	b, e := DecodeRepresentation("hex", []byte("7B 09\t00\r\n51"))
	if e != nil || len(b) != 4 || b[3] != 81 {
		t.Fatal(b, e)
	}
	for _, s := range []string{"NaN", "Infinity", "1e2", "+1", "01", "1.0000000000001", "100000000000000000000000000"} {
		if _, e := Decimal(s); e == nil {
			t.Fatal(s)
		}
	}
	if n, e := Decimal("-0.00"); e != nil || n != "0" {
		t.Fatal(n, e)
	}
}

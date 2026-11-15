package files

import (
	"strings"
	"testing"
)

func TestCheckerStrictWireRejectsAmbiguousResults(t *testing.T) {
	for _, raw := range []string{`{"status":"ok","status":"failed"}`, `{"status":"ok","Status":"failed"}`, `{"status":"ok","url":"https://untrusted"}`, `{"status":"ok"} {}`, strings.Repeat("[", 40) + "0" + strings.Repeat("]", 40)} {
		var r CheckResult
		if strictWire([]byte(raw), &r) == nil {
			t.Fatal("accepted ambiguous protocol", raw)
		}
	}
}

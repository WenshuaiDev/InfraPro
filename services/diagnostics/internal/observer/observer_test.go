package observer

import (
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/ingest"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/model"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/testfixture"
	"github.com/google/uuid"
	"strings"
	"testing"
	"time"
)

func TestOversizedEvidenceBecomesCompleteBoundedFailureFrame(t *testing.T) {
	m := testfixture.Manifest()
	f := ingest.Frame{SessionRequest: ingest.SessionRequest{NodeID: "node", BootID: uuid.NewString()}, Sequence: "1", GeneratedAt: time.Now().UTC().Truncate(time.Millisecond)}
	for _, p := range m.Probes {
		o := model.New(p, f.BootID, f.GeneratedAt)
		o.Metrics = []model.Metric{{Key: "process.rss_bytes", Value: strings.Repeat("0", 1<<20)}}
		f.Observations = append(f.Observations, o)
	}
	raw := encodeFrame(f, m)
	if len(raw) > 1<<20 {
		t.Fatal("wire buffer exceeded")
	}
	var result ingest.Frame
	if json.Unmarshal(raw, &result) != nil || len(result.Observations) != len(m.Probes) {
		t.Fatal("partial frame")
	}
	for i, o := range result.Observations {
		if o.Outcome != "invalid_report" || o.ReasonCode != "response_limit_exceeded" || model.Validate(o, m.Probes[i], f.BootID) != nil {
			t.Fatal("invalid failure evidence")
		}
	}
}

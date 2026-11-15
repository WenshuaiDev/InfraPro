package httptransport

import (
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/ingest"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/model"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
	"strconv"
	"time"
)

type resourceWarning struct {
	NodeID      string `json:"node_id"`
	ProbeID     string `json:"probe_id"`
	ReasonCode  string `json:"reason_code"`
	DimensionID string `json:"dimension_id,omitempty"`
}

func resourceWarnings(s ingest.Snapshot, h settings.ResourceHints) []resourceWarning {
	out := []resourceWarning{}
	if !h.Enabled {
		return out
	}
	diskBytes := settings.Number(h.DiskAvailableBytes)
	for _, p := range s.Manifest.Probes {
		stored, ok := s.Latest[p.ID]
		if !ok {
			continue
		}
		view := model.View(&stored, p, s.BootID, s.Now, s.Online[p.NodeID])
		if view.Freshness != "fresh" {
			continue
		}
		values := map[string]uint64{}
		for _, m := range view.Metrics {
			if m.Availability != "available" {
				continue
			}
			if value, ok := m.Value.(string); ok {
				n, e := strconv.ParseUint(value, 10, 64)
				if e == nil {
					values[m.Key+":"+m.DimensionID] = n
				}
			}
		}
		if total := values["memory.total_bytes:"]; total > 0 {
			if available, ok := values["memory.available_bytes:"]; ok && float64(available)/float64(total) < h.MemoryAvailableRatio {
				out = append(out, resourceWarning{NodeID: p.NodeID, ProbeID: p.ID, ReasonCode: "memory_pressure"})
			}
		}
		for _, m := range view.Metrics {
			if m.Key != "disk.total_bytes" || m.Availability != "available" {
				continue
			}
			total := values[m.Key+":"+m.DimensionID]
			available, ok := values["disk.available_bytes:"+m.DimensionID]
			if ok && total > 0 && (float64(available)/float64(total) < h.DiskAvailableRatio || available < diskBytes) {
				out = append(out, resourceWarning{NodeID: p.NodeID, ProbeID: p.ID, ReasonCode: "disk_pressure", DimensionID: m.DimensionID})
			}
		}
		points := s.CPUSamples[p.ID]
		if len(points) >= h.CPUSamples {
			high := true
			previous := s.Now
			for i := len(points) - 1; i >= len(points)-h.CPUSamples; i-- {
				point := points[i]
				value, ok := point.Value.(float64)
				if !ok || point.Availability != "available" || value <= h.CPURatio || previous.Sub(point.At) > time.Duration(p.StaleAfterMS)*time.Millisecond {
					high = false
					break
				}
				previous = point.At
			}
			if high {
				out = append(out, resourceWarning{NodeID: p.NodeID, ProbeID: p.ID, ReasonCode: "cpu_pressure"})
			}
		}
	}
	return out
}

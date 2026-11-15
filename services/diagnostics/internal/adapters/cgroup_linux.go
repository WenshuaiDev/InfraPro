//go:build linux

package adapters

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/model"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
)

func (r *Runtime) cgroup(ctx context.Context, p settings.Probe) []model.Metric {
	keys := []string{"container.memory_used_bytes", "container.memory_limit_bytes", "container.cpu_cores", "container.cpu_limit_cores"}
	unavailable := func() []model.Metric {
		out := []model.Metric{}
		for _, key := range keys {
			out = append(out, absentMetric(key, "", "unsupported", "adapter_unsupported"))
		}
		return out
	}
	if ctx.Err() != nil {
		return unavailable()
	}
	// Only the observer's own cgroup namespace root is read. A host observer may
	// never present host /proc memory as a container limit.
	membership, e := os.ReadFile("/proc/self/cgroup")
	if e != nil || strings.TrimSpace(string(membership)) != "0::/" {
		return unavailable()
	}
	values := map[string]string{}
	for _, name := range []string{"memory.current", "memory.max", "cpu.stat", "cpu.max"} {
		raw, e := os.ReadFile("/sys/fs/cgroup/" + name)
		if e != nil || len(raw) > 4096 {
			return unavailable()
		}
		values[name] = strings.TrimSpace(string(raw))
	}
	used, e := strconv.ParseUint(values["memory.current"], 10, 64)
	if e != nil {
		return unavailable()
	}
	out := []model.Metric{metric(keys[0], "", strconv.FormatUint(used, 10))}
	if values["memory.max"] == "max" {
		out = append(out, absentMetric(keys[1], "", "unlimited", "unlimited"))
	} else if v, e := strconv.ParseUint(values["memory.max"], 10, 64); e == nil {
		out = append(out, metric(keys[1], "", strconv.FormatUint(v, 10)))
	} else {
		return unavailable()
	}
	max := strings.Fields(values["cpu.max"])
	if len(max) != 2 {
		return unavailable()
	}
	if max[0] == "max" {
		out = append(out, absentMetric(keys[3], "", "unlimited", "unlimited"))
	} else {
		quota, e := strconv.ParseFloat(max[0], 64)
		period, e2 := strconv.ParseFloat(max[1], 64)
		if e != nil || e2 != nil || period <= 0 {
			return unavailable()
		}
		out = append(out, metric(keys[3], "", quota/period))
	}
	found := false
	for _, line := range strings.Split(values["cpu.stat"], "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[0] != "usage_usec" {
			continue
		}
		cpu, e := strconv.ParseFloat(fields[1], 64)
		if e != nil {
			return unavailable()
		}
		found = true
		next := sample{at: time.Now(), identity: r.BootID, cpu: cpu / 1e6}
		if old, ok := r.metricDelta(p.ID+":cgroup", next); ok && next.cpu >= old.cpu {
			out = append(out, metric(keys[2], "", (next.cpu-old.cpu)/next.at.Sub(old.at).Seconds()))
		} else {
			out = append(out, absentMetric(keys[2], "", "not_configured", "check_pending"))
		}
	}
	if !found {
		return unavailable()
	}
	return out
}

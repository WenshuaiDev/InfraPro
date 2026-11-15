//go:build !linux

package adapters

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/model"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
)

func (r *Runtime) cgroup(_ context.Context, _ settings.Probe) []model.Metric {
	out := []model.Metric{}
	for _, key := range []string{"container.memory_used_bytes", "container.memory_limit_bytes", "container.cpu_cores", "container.cpu_limit_cores"} {
		out = append(out, absentMetric(key, "", "unsupported", "adapter_unsupported"))
	}
	return out
}

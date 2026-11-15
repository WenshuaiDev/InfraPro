package adapters

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/model"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	psnet "github.com/shirou/gopsutil/v4/net"
	"golang.org/x/sys/unix"
)

func (r *Runtime) resources(ctx context.Context, p settings.Probe, b settings.Binding, o model.Observation) model.Observation {
	o.Evidence.Scope = b.Scope
	if b.Scope == "container" {
		o.Metrics = append(o.Metrics, r.cgroup(ctx, p)...)
	} else {
		times, e := cpu.TimesWithContext(ctx, false)
		if e == nil && len(times) == 1 {
			t := times[0]
			busy := t.User + t.System + t.Nice + t.Irq + t.Softirq + t.Steal
			next := sample{at: time.Now(), identity: r.BootID, cpu: busy, total: t.Total()}
			cores, e := cpu.CountsWithContext(ctx, true)
			old, ok := r.metricDelta(p.ID+":cpu", next)
			if e == nil && cores > 0 && ok && next.cpu >= old.cpu && next.total > old.total {
				ratio := (next.cpu - old.cpu) / next.at.Sub(old.at).Seconds() / float64(cores)
				o.Metrics = append(o.Metrics, metric("cpu.utilization_ratio", "", math.Max(0, math.Min(1, ratio))))
			} else {
				o.Metrics = append(o.Metrics, absentMetric("cpu.utilization_ratio", "", "not_configured", "check_pending"))
			}
		} else {
			o.Metrics = append(o.Metrics, absentMetric("cpu.utilization_ratio", "", "error", "adapter_error"))
		}
		memory, e := mem.VirtualMemoryWithContext(ctx)
		if e == nil {
			o.Metrics = append(o.Metrics, metric("memory.total_bytes", "", strconv.FormatUint(memory.Total, 10)), metric("memory.available_bytes", "", strconv.FormatUint(memory.Available, 10)), metric("memory.used_bytes", "", strconv.FormatUint(memory.Total-memory.Available, 10)))
		} else {
			for _, key := range []string{"memory.total_bytes", "memory.available_bytes", "memory.used_bytes"} {
				o.Metrics = append(o.Metrics, absentMetric(key, "", "error", "adapter_error"))
			}
		}
	}
	seenFS := map[string]bool{}
	for _, mount := range b.MountPaths {
		var st unix.Statfs_t
		e := unix.Statfs(mount.Path, &st)
		if e != nil {
			out, code := classify(e)
			availability := "error"
			if out == "permission_denied" {
				availability = out
			}
			for _, key := range []string{"disk.total_bytes", "disk.available_bytes", "disk.used_ratio"} {
				o.Metrics = append(o.Metrics, absentMetric(key, mount.ID, availability, code))
			}
			continue
		}
		fsid := fmt.Sprint(st.Fsid)
		if seenFS[fsid] {
			continue
		}
		seenFS[fsid] = true
		usage, e := disk.UsageWithContext(ctx, mount.Path)
		if e != nil {
			for _, key := range []string{"disk.total_bytes", "disk.available_bytes", "disk.used_ratio"} {
				o.Metrics = append(o.Metrics, absentMetric(key, mount.ID, "error", "adapter_error"))
			}
			continue
		}
		o.Metrics = append(o.Metrics, metric("disk.total_bytes", mount.ID, strconv.FormatUint(usage.Total, 10)), metric("disk.available_bytes", mount.ID, strconv.FormatUint(usage.Free, 10)))
		if usage.Total > 0 {
			o.Metrics = append(o.Metrics, metric("disk.used_ratio", mount.ID, math.Max(0, math.Min(1, float64(usage.Total-usage.Free)/float64(usage.Total)))))
		} else {
			o.Metrics = append(o.Metrics, absentMetric("disk.used_ratio", mount.ID, "error", "adapter_error"))
		}
	}
	if len(b.NetworkInterfaces) > 0 {
		counters, e := psnet.IOCountersWithContext(ctx, true)
		for _, nic := range b.NetworkInterfaces {
			found := false
			for _, count := range counters {
				if e != nil || count.Name != nic.Name {
					continue
				}
				found = true
				next := sample{at: time.Now(), identity: r.BootID, rx: float64(count.BytesRecv), tx: float64(count.BytesSent)}
				if old, ok := r.metricDelta(p.ID+":network:"+nic.ID, next); ok && next.rx >= old.rx && next.tx >= old.tx {
					seconds := next.at.Sub(old.at).Seconds()
					o.Metrics = append(o.Metrics, metric("network.rx_bytes_per_second", nic.ID, (next.rx-old.rx)/seconds), metric("network.tx_bytes_per_second", nic.ID, (next.tx-old.tx)/seconds))
				} else {
					for _, key := range []string{"network.rx_bytes_per_second", "network.tx_bytes_per_second"} {
						o.Metrics = append(o.Metrics, absentMetric(key, nic.ID, "not_configured", "check_pending"))
					}
				}
			}
			if !found {
				for _, key := range []string{"network.rx_bytes_per_second", "network.tx_bytes_per_second"} {
					o.Metrics = append(o.Metrics, absentMetric(key, nic.ID, "unsupported", "adapter_unsupported"))
				}
			}
		}
	}
	return o
}

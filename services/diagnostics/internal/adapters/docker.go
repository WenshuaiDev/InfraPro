package adapters

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/model"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
	"github.com/google/uuid"
)

type dockerReader struct {
	client  *http.Client
	version string
}
type dockerInspect struct {
	ID    string `json:"Id"`
	Path  string `json:"Path"`
	State struct {
		Running    bool
		Status     string
		StartedAt  time.Time
		FinishedAt time.Time
		ExitCode   int
		OOMKilled  bool
		Health     *struct{ Status string }
	} `json:"State"`
	RestartCount int `json:"RestartCount"`
	HostConfig   struct {
		PidMode   string
		Init      *bool
		Memory    int64
		NanoCPUs  int64
		CPUQuota  int64 `json:"CpuQuota"`
		CPUPeriod int64 `json:"CpuPeriod"`
	} `json:"HostConfig"`
	Config struct {
		User   string
		Labels map[string]string
	} `json:"Config"`
	Mounts []struct {
		Source, Destination string
		RW                  bool
	} `json:"Mounts"`
}

func dockerPath(path string) bool {
	return path == "/version" || regexp.MustCompile(`^/v1\.(4[1-9]|5[0-6])/containers/json\?`).MatchString(path) || regexp.MustCompile(`^/v1\.(4[1-9]|5[0-6])/containers/[a-f0-9]{12,64}/(json|stats\?stream=false&one-shot=true)$`).MatchString(path)
}
func (d *dockerReader) get(ctx context.Context, path string, out any) error {
	if !dockerPath(path) {
		return settings.Error("permission_denied")
	}
	request, e := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker"+path, nil)
	if e != nil {
		return e
	}
	response, e := d.client.Do(request)
	if e != nil {
		return e
	}
	defer response.Body.Close()
	if response.StatusCode == 403 || response.StatusCode == 401 {
		return settings.Error("permission_denied")
	}
	if response.StatusCode == 404 {
		return settings.Error("process_not_found")
	}
	if response.StatusCode != 200 {
		return settings.Error("adapter_error")
	}
	raw, e := readLimit(response.Body, 2<<20)
	if e != nil {
		return e
	}
	if json.Unmarshal(raw, out) != nil {
		return settings.Error("response_mismatch")
	}
	return nil
}
func newDocker(ctx context.Context, path string) (*dockerReader, error) {
	d := &dockerReader{client: &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: nil, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "unix", path)
	}, DisableKeepAlives: true, MaxConnsPerHost: 1, MaxResponseHeaderBytes: 16 << 10}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	var versions struct {
		APIVersion    string `json:"ApiVersion"`
		MinAPIVersion string `json:"MinAPIVersion"`
	}
	if e := d.get(ctx, "/version", &versions); e != nil {
		return nil, e
	}
	minor := func(s string) int {
		parts := strings.Split(s, ".")
		if len(parts) != 2 || parts[0] != "1" {
			return -1
		}
		n, e := strconv.Atoi(parts[1])
		if e != nil {
			return -1
		}
		return n
	}
	maxVersion, minVersion := minor(versions.APIVersion), minor(versions.MinAPIVersion)
	if minVersion < 0 || maxVersion < 41 || minVersion > 56 {
		return nil, settings.Error("adapter_unsupported")
	}
	d.version = "/v1." + strconv.Itoa(min(maxVersion, 56))
	return d, nil
}
func (d *dockerReader) find(ctx context.Context, b settings.Binding) (dockerInspect, error) {
	var out dockerInspect
	labels := []string{}
	for k, v := range b.SelectorLabels {
		labels = append(labels, k+"="+v)
	}
	filters, _ := json.Marshal(map[string][]string{"label": labels})
	query := url.Values{"all": {"true"}, "filters": {string(filters)}}
	var list []struct {
		ID     string            `json:"Id"`
		Labels map[string]string `json:"Labels"`
	}
	if e := d.get(ctx, d.version+"/containers/json?"+query.Encode(), &list); e != nil {
		return out, e
	}
	if len(list) == 0 {
		return out, settings.Error("process_not_found")
	}
	if len(list) != 1 {
		return out, settings.Error("ambiguous_identity")
	}
	if !regexp.MustCompile(`^[a-f0-9]{12,64}$`).MatchString(list[0].ID) {
		return out, settings.Error("identity_mismatch")
	}
	for k, v := range b.SelectorLabels {
		if list[0].Labels[k] != v {
			return out, settings.Error("identity_mismatch")
		}
	}
	if e := d.get(ctx, d.version+"/containers/"+list[0].ID+"/json", &out); e != nil {
		return out, e
	}
	if out.ID != list[0].ID {
		return out, settings.Error("identity_mismatch")
	}
	for k, v := range b.SelectorLabels {
		if out.Config.Labels[k] != v {
			return out, settings.Error("identity_mismatch")
		}
	}
	return out, nil
}
func dockerFailure(o model.Observation, e error) model.Observation {
	code := settings.Code(e)
	switch code {
	case "process_not_found":
		o.Facets.Presence = "missing"
		return model.Failure(o, "failed", code)
	case "ambiguous_identity", "identity_mismatch":
		return model.Failure(o, "invalid_report", code)
	case "adapter_unsupported":
		return model.Failure(o, "unsupported", code)
	case "permission_denied":
		return model.Failure(o, "permission_denied", code)
	}
	return failure(o, e)
}
func (r *Runtime) docker(ctx context.Context, p settings.Probe, b settings.Binding, o model.Observation) model.Observation {
	o.Facets.Presence = "unknown"
	o.Evidence.Scope = "container"
	d, e := newDocker(ctx, b.SocketPath)
	if e != nil {
		return dockerFailure(o, e)
	}
	defer d.client.CloseIdleConnections()
	info, e := d.find(ctx, b)
	if e != nil {
		return dockerFailure(o, e)
	}
	o.Facets.Presence = "stopped"
	if info.State.Running {
		o.Facets.Presence = "running"
	} else {
		o = model.Failure(o, "failed", "process_exited")
	}
	o.Evidence.ExitCode = &info.State.ExitCode
	o.Evidence.OOMKilled = &info.State.OOMKilled
	o.Evidence.RestartCount = &info.RestartCount
	o.Evidence.Health = "none"
	if info.State.Health != nil && settings.OneOf(info.State.Health.Status, "starting", "healthy", "unhealthy") {
		o.Evidence.Health = info.State.Health.Status
		o.Facets.Readiness = map[string]string{"starting": "starting", "healthy": "ready", "unhealthy": "not_ready"}[info.State.Health.Status]
		if info.State.Health.Status == "unhealthy" {
			o = model.Failure(o, "failed", "response_mismatch")
		}
	}
	if p.Facet == "readiness" && info.State.Running && o.Evidence.Health == "none" {
		o.Facets.Readiness = "unknown"
		o = model.Failure(o, "not_configured", "not_configured")
	}
	id := uuid.NewSHA1(uuid.NameSpaceOID, []byte(info.ID+":"+info.State.StartedAt.Format(time.RFC3339Nano))).String()
	o.Process = &model.ProcessIdentity{ReplicaID: b.ExpectedReplicaID, BootID: id, ContainerID: info.ID, ContainerStartedAt: &info.State.StartedAt}
	o.Metrics = append(o.Metrics, metric("process.restart_count", "", float64(info.RestartCount)))
	if b.StatsEnabled && info.State.Running {
		var stats struct {
			CPU struct {
				Usage struct {
					Total uint64 `json:"total_usage"`
				} `json:"cpu_usage"`
			} `json:"cpu_stats"`
			Memory struct {
				Usage uint64 `json:"usage"`
			} `json:"memory_stats"`
		}
		if e = d.get(ctx, d.version+"/containers/"+info.ID+"/stats?stream=false&one-shot=true", &stats); e != nil {
			o.Metrics = append(o.Metrics, absentMetric("container.cpu_cores", "", "error", "adapter_error"), absentMetric("container.memory_used_bytes", "", "error", "adapter_error"))
		} else {
			next := sample{at: time.Now(), identity: id, cpu: float64(stats.CPU.Usage.Total) / 1e9}
			if old, ok := r.metricDelta(p.ID+":docker", next); ok && next.cpu >= old.cpu {
				o.Metrics = append(o.Metrics, metric("container.cpu_cores", "", (next.cpu-old.cpu)/next.at.Sub(old.at).Seconds()))
			} else {
				o.Metrics = append(o.Metrics, absentMetric("container.cpu_cores", "", "not_configured", "check_pending"))
			}
			o.Metrics = append(o.Metrics, metric("container.memory_used_bytes", "", strconv.FormatUint(stats.Memory.Usage, 10)))
		}
		if info.HostConfig.Memory > 0 {
			o.Metrics = append(o.Metrics, metric("container.memory_limit_bytes", "", strconv.FormatInt(info.HostConfig.Memory, 10)))
		} else {
			o.Metrics = append(o.Metrics, absentMetric("container.memory_limit_bytes", "", "unlimited", "unlimited"))
		}
		cpuLimit := float64(info.HostConfig.NanoCPUs) / 1e9
		if cpuLimit <= 0 && info.HostConfig.CPUQuota > 0 && info.HostConfig.CPUPeriod > 0 {
			cpuLimit = float64(info.HostConfig.CPUQuota) / float64(info.HostConfig.CPUPeriod)
		}
		if cpuLimit > 0 {
			o.Metrics = append(o.Metrics, metric("container.cpu_limit_cores", "", cpuLimit))
		} else {
			o.Metrics = append(o.Metrics, absentMetric("container.cpu_limit_cores", "", "unlimited", "unlimited"))
		}
	}
	return o
}
func (r *Runtime) verifyContainerReport(ctx context.Context, p settings.Probe, b settings.Binding, report diagnosticreport.Report) bool {
	for _, candidate := range r.Bindings.Bindings {
		if candidate.Adapter != "docker_container" || candidate.ExpectedReplicaID != b.ExpectedReplicaID {
			continue
		}
		d, e := newDocker(ctx, candidate.SocketPath)
		if e != nil {
			return false
		}
		info, e := d.find(ctx, candidate)
		d.client.CloseIdleConnections()
		return e == nil && matchesReportIdentity(info, b, report)
	}
	return false
}

// The split images invoke the declared binary directly as PID 1 in a private
// namespace. Containers with an init wrapper need a different source of
// process identity. Inspect never grants access to exec, logs or environment.
func matchesReportIdentity(info dockerInspect, b settings.Binding, report diagnosticreport.Report) bool {
	if !info.State.Running || info.State.StartedAt.IsZero() || report.EmittedAt.Before(info.State.StartedAt) ||
		info.Path != b.ExpectedExecutable || report.Process.PID != 1 || b.ExpectedUID == nil ||
		info.HostConfig.PidMode != "" && info.HostConfig.PidMode != "private" || info.HostConfig.Init != nil && *info.HostConfig.Init {
		return false
	}
	user, _, _ := strings.Cut(info.Config.User, ":")
	if user == "" {
		user = "0"
	}
	uid, err := strconv.Atoi(user)
	if err != nil || uid != *b.ExpectedUID {
		return false
	}
	// Linux /proc provides boot time in whole seconds. Compare it with Docker's
	// nanosecond start time, while still requiring a report emitted after this
	// container generation.
	start := time.UnixMilli(int64(settings.Number(report.Process.OSStartTimeMS)))
	if start.Before(info.State.StartedAt.Add(-2*time.Second)) || start.After(info.State.StartedAt.Add(2*time.Second)) {
		return false
	}
	for _, mount := range info.Mounts {
		if mount.RW && filepath.Clean(mount.Source) == b.AllowedRoot && filepath.IsAbs(mount.Destination) && mount.Destination != "/" && settings.Within(b.ReportFile, b.AllowedRoot) {
			return true
		}
	}
	return false
}

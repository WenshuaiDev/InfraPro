package settings

import (
	"encoding/json"
	"net"
	"net/netip"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var BindingFields = map[string][]string{
	"native_process":   {"report_file", "pid_file", "pid_file_format", "allowed_root", "expected_executable", "expected_uid"},
	"self_report":      {"report_file", "allowed_root", "expected_replica_id", "expected_executable", "expected_uid", "pid_namespace", "dependency_key", "lane_id"},
	"host_resources":   {"scope", "mount_paths", "network_interfaces"},
	"systemd_unit":     {"unit_name", "expected_uid"},
	"docker_container": {"socket_path", "selector_labels", "expected_replica_id", "stats_enabled"},
	"http_live":        {"url", "expected_status", "expected_json", "credential_file", "tls_ca_file"},
	"http_ready":       {"url", "expected_status", "expected_json", "credential_file", "tls_ca_file"},
	"tcp":              {"host", "port", "tls", "server_name", "tls_ca_file"},
	"proxy_listener":   {"host", "port"},
	"postgres":         {"host", "port", "database", "username", "credential_file", "tls_mode", "tls_ca_file"},
	"redis":            {"address", "username", "credential_file", "database", "tls", "tls_ca_file"},
	"clickhouse":       {"address", "database", "username", "credential_file", "tls", "tls_ca_file"},
	"s3":               {"endpoint", "region", "bucket", "credential_file", "tls_ca_file"},
	"jetstream_health": {"host", "port"},
	"keeper_live":      {"host", "port", "enabled"},
}

func (b *Binding) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if e := json.Unmarshal(data, &fields); e != nil {
		return Invalid
	}
	var adapter string
	if json.Unmarshal(fields["adapter"], &adapter) != nil {
		return Invalid
	}
	allowed, ok := BindingFields[adapter]
	if !ok {
		return Invalid
	}
	for k := range fields {
		if k != "id" && k != "adapter" && !OneOf(k, allowed...) {
			return Invalid
		}
	}
	if adapter == "redis" && len(fields["database"]) > 0 {
		var n int
		if json.Unmarshal(fields["database"], &n) != nil || n < 0 || n > 15 {
			return Invalid
		}
		fields["database"], _ = json.Marshal(strconv.Itoa(n))
		data, _ = json.Marshal(fields)
	}
	type plain Binding
	var v plain
	if e := json.Unmarshal(data, &v); e != nil {
		return Invalid
	}
	*b = Binding(v)
	return nil
}
func (b Binding) MarshalJSON() ([]byte, error) {
	type plain Binding
	data, e := json.Marshal(plain(b))
	if e != nil || b.Adapter != "redis" {
		return data, e
	}
	var m map[string]json.RawMessage
	_ = json.Unmarshal(data, &m)
	n, e := strconv.Atoi(b.Database)
	if e != nil {
		return nil, Invalid
	}
	m["database"], _ = json.Marshal(n)
	return json.Marshal(m)
}
func Absolute(path string) bool {
	return path != "" && filepath.IsAbs(path) && filepath.Clean(path) == path && !strings.ContainsRune(path, 0)
}
func Within(path, root string) bool {
	if !Absolute(path) || !Absolute(root) {
		return false
	}
	r, e := filepath.Rel(root, path)
	return e == nil && r != "." && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator))
}
func PrivateIP(ip net.IP) bool { return ip.IsLoopback() || ip.IsPrivate() }
func SafeIP(ip net.IP, public bool) bool {
	return ip != nil && !ip.IsUnspecified() && !ip.IsMulticast() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && (public || PrivateIP(ip))
}
func TargetHost(host string, allow []string) bool {
	if len(host) < 1 || len(host) > 253 || strings.ContainsAny(host, "/\\@?#% \t\r\n") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return SafeIP(ip, OneOf(host, allow...))
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	return regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.-]*$`).MatchString(host)
}
func TargetURL(raw string, allow []string) (*url.URL, error) {
	u, e := url.Parse(raw)
	if e != nil || !OneOf(u.Scheme, "http", "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Host == "" || !TargetHost(u.Hostname(), allow) {
		return nil, Invalid
	}
	if p := u.Port(); p != "" {
		n, e := strconv.Atoi(p)
		if e != nil || n < 1 || n > 65535 {
			return nil, Invalid
		}
	}
	return u, nil
}
func Origin(raw string, loopHTTP bool) bool {
	u, e := url.Parse(raw)
	if e != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || u.Host == "" || u.Scheme != "https" && !(loopHTTP && u.Scheme == "http" && isLoop(u.Hostname())) {
		return false
	}
	return u.String() == raw
}
func isLoop(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback() || host == "localhost"
}
func (b Bindings) Validate(m Manifest) error {
	node, ok := m.Node(b.NodeID)
	if b.SchemaVersion != 1 || b.InstanceID != m.InstanceID || !ok || b.AssignmentRevision != node.AssignmentRevision || b.Bindings == nil || len(b.Bindings) > 256 || len(b.AllowedTargetHosts) > 64 {
		return Invalid
	}
	for _, h := range b.AllowedTargetHosts {
		if !TargetHost(h, b.AllowedTargetHosts) || strings.Contains(h, "*") {
			return Invalid
		}
	}
	seen := map[string]Binding{}
	for _, v := range b.Bindings {
		if !ValidID(v.ID) || seen[v.ID].ID != "" || v.validate(b) != nil {
			return Invalid
		}
		seen[v.ID] = v
	}
	for _, p := range m.Assigned(b.NodeID) {
		v, exists := seen[p.BindingID]
		if !exists {
			continue
		}
		if v.Adapter != p.Adapter {
			return Invalid
		}
		if p.TargetKind == "dependency" && v.Adapter == "self_report" && v.DependencyKey == "" {
			return Invalid
		}
		if v.Adapter == "proxy_listener" {
			replica, ok := m.ReplicaFor(p)
			if !ok || replica.ComponentID != "replica-proxy" || p.Facet != "readiness" {
				return Invalid
			}
		}
		if v.Adapter == "self_report" || v.Adapter == "docker_container" {
			r, ok := m.ReplicaFor(p)
			if !ok || v.ExpectedReplicaID != r.ID {
				return Invalid
			}
		}
	}
	return nil
}
func (b Binding) validate(root Bindings) error {
	if BindingFields[b.Adapter] == nil {
		return Invalid
	}
	for _, p := range []string{b.CredentialFile, b.TLSCAFile} {
		if p != "" && !Absolute(p) {
			return Invalid
		}
	}
	if b.ExpectedUID != nil && (*b.ExpectedUID < 0 || *b.ExpectedUID > 2147483647) {
		return Invalid
	}
	switch b.Adapter {
	case "native_process", "self_report":
		if !Absolute(b.ExpectedExecutable) || !Absolute(b.AllowedRoot) || b.ExpectedUID == nil {
			return Invalid
		}
		if b.ReportFile == "" && b.PIDFile == "" {
			return Invalid
		}
		for _, p := range []string{b.ReportFile, b.PIDFile} {
			if p != "" && !Within(p, b.AllowedRoot) {
				return Invalid
			}
		}
		if b.PIDFile != "" && !OneOf(b.PIDFileFormat, "diagnostics", "decimal", "postgres") {
			return Invalid
		}
		if b.Adapter == "self_report" {
			if b.ReportFile == "" || !ValidID(b.ExpectedReplicaID) || !OneOf(b.PIDNamespace, "host", "container") {
				return Invalid
			}
			for _, s := range []string{b.LaneID, b.DependencyKey} {
				if s != "" && !ValidID(s) {
					return Invalid
				}
			}
		}
	case "host_resources":
		if !OneOf(b.Scope, "host", "vm", "container") || len(b.MountPaths) > 8 || len(b.NetworkInterfaces) > 8 {
			return Invalid
		}
		seen := map[string]bool{}
		for _, p := range b.MountPaths {
			if !ValidID(p.ID) || seen[p.ID] || !Absolute(p.Path) {
				return Invalid
			}
			seen[p.ID] = true
		}
		seen = map[string]bool{}
		for _, n := range b.NetworkInterfaces {
			if !ValidID(n.ID) || seen[n.ID] || !regexp.MustCompile(`^[a-zA-Z0-9_.:-]{1,64}$`).MatchString(n.Name) {
				return Invalid
			}
			seen[n.ID] = true
		}
	case "systemd_unit":
		if !regexp.MustCompile(`^[a-zA-Z0-9_.@:-]{1,240}\.service$`).MatchString(b.UnitName) {
			return Invalid
		}
	case "jetstream_health":
		if !isLoop(b.Host) || b.Port < 1 || b.Port > 65535 {
			return Invalid
		}
	case "docker_container":
		if !Absolute(b.SocketPath) || !ValidID(b.ExpectedReplicaID) || len(b.SelectorLabels) < 2 || len(b.SelectorLabels) > 8 || b.SelectorLabels["io.infrapro.instance"] != root.InstanceID || b.SelectorLabels["io.infrapro.replica"] != b.ExpectedReplicaID {
			return Invalid
		}
		for k, v := range b.SelectorLabels {
			if !regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,64}$`).MatchString(k) || !ValidID(v) {
				return Invalid
			}
		}
	case "http_live", "http_ready":
		if _, e := TargetURL(b.URL, root.AllowedTargetHosts); e != nil || b.ExpectedStatus < 200 || b.ExpectedStatus > 399 || len(b.ExpectedJSON) > 4 {
			return Invalid
		}
		for k, v := range b.ExpectedJSON {
			if !ValidID(k) || len(v) > 256 {
				return Invalid
			}
			var value any
			if json.Unmarshal(v, &value) != nil {
				return Invalid
			}
			switch value.(type) {
			case string, float64, bool, nil:
			default:
				return Invalid
			}
		}
	case "tcp", "proxy_listener", "postgres", "keeper_live":
		if !TargetHost(b.Host, root.AllowedTargetHosts) || b.Port < 1 || b.Port > 65535 {
			return Invalid
		}
		if b.Adapter == "postgres" && (!OneOf(b.TLSMode, "disable", "verify-full") || b.Database == "" || b.Username == "") {
			return Invalid
		}
	case "redis", "clickhouse":
		h, p, e := net.SplitHostPort(b.Address)
		if e != nil || !TargetHost(h, root.AllowedTargetHosts) {
			return Invalid
		}
		n, e := strconv.Atoi(p)
		if e != nil || n < 1 || n > 65535 {
			return Invalid
		}
		if b.Adapter == "redis" {
			n, e = strconv.Atoi(b.Database)
			if e != nil || n < 0 || n > 15 {
				return Invalid
			}
		} else if b.Username == "" || b.Database == "" {
			return Invalid
		}
	case "s3":
		u, e := TargetURL(b.Endpoint, root.AllowedTargetHosts)
		if e != nil || u.Path != "" || b.Bucket == "" || len(b.Bucket) > 63 || strings.ContainsAny(b.Bucket, "/\\\x00") {
			return Invalid
		}
	}
	for _, s := range []string{b.Database, b.Username, b.Region, b.ServerName} {
		if len(s) > 128 || strings.ContainsAny(s, "\x00\r\n") {
			return Invalid
		}
	}
	return nil
}
func (s Server) Validate(development bool) error {
	if s.SchemaVersion != 1 || !ValidID(s.InstanceID) || s.Limits.Normalize() != nil {
		return Invalid
	}
	addr, e := netip.ParseAddrPort(s.Listen)
	if e != nil || addr.Port() == 0 || addr.Addr().IsUnspecified() && s.TLS == nil {
		return Invalid
	}
	if !addr.Addr().IsLoopback() && s.TLS == nil {
		return Invalid
	}
	if !Origin(s.PublicOrigin, addr.Addr().IsLoopback()) || len(s.AllowedHosts) < 1 || len(s.AllowedHosts) > 8 {
		return Invalid
	}
	u, _ := url.Parse(s.PublicOrigin)
	if !OneOf(u.Host, s.AllowedHosts...) {
		return Invalid
	}
	for _, h := range s.AllowedHosts {
		if strings.ContainsAny(h, "/*@?# \r\n") {
			return Invalid
		}
	}
	if s.TLS != nil {
		if !Absolute(s.TLS.CertFile) || !Absolute(s.TLS.KeyFile) || u.Scheme != "https" {
			return Invalid
		}
	}
	for _, p := range []string{s.ManifestFile, s.AuthFile, s.NodeKeysFile, s.StateDir} {
		if !Absolute(p) {
			return Invalid
		}
	}
	if s.LocalObserver != nil {
		if !ValidID(s.LocalObserver.NodeID) || !Decimal(s.LocalObserver.AssignmentRevision, false) || !Absolute(s.LocalObserver.BindingsFile) {
			return Invalid
		}
	}
	if len(s.TrustedProxyCIDRs) > 8 {
		return Invalid
	}
	for _, c := range s.TrustedProxyCIDRs {
		if _, e := netip.ParsePrefix(c); e != nil {
			return Invalid
		}
	}
	if len(s.DevelopmentOrigins) > 0 && (!development || !addr.Addr().IsLoopback()) || len(s.DevelopmentOrigins) > 2 {
		return Invalid
	}
	for _, o := range s.DevelopmentOrigins {
		u, e := url.Parse(o)
		if e != nil || !isLoop(u.Hostname()) || !Origin(o, true) {
			return Invalid
		}
	}
	h := s.ResourceHints
	if h.Enabled && (h.CPURatio <= 0 || h.CPURatio > 1 || h.CPUSamples < 1 || h.CPUSamples > 60 || h.MemoryAvailableRatio <= 0 || h.MemoryAvailableRatio >= 1 || h.DiskAvailableRatio <= 0 || h.DiskAvailableRatio >= 1 || !Decimal(h.DiskAvailableBytes, true)) {
		return Invalid
	}
	return nil
}
func (o Observer) Validate() error {
	if o.SchemaVersion != 1 || !ValidID(o.InstanceID) || !ValidID(o.NodeID) || !Decimal(o.AssignmentRevision, false) || !Origin(o.ServerURL, true) {
		return Invalid
	}
	for _, p := range []string{o.ManifestFile, o.BindingsFile, o.CredentialFile, o.StateDir} {
		if !Absolute(p) {
			return Invalid
		}
	}
	if o.TLSCAFile != "" && !Absolute(o.TLSCAFile) {
		return Invalid
	}
	return nil
}
func (b Bindings) Find(id string) (Binding, bool) {
	for _, v := range b.Bindings {
		if v.ID == id {
			return v, true
		}
	}
	return Binding{}, false
}

package settings

import (
	"path/filepath"
	"slices"
	"strconv"
)

// SplitOptions describes process identities that must be checked outside the
// application. Each directory contains reports from the named replica only.
type SplitOptions struct {
	BinDirectory             string
	ReportDirectory          string
	ContainerReportDirectory string
	ContainerRoles           []string
	FlatLocalReports         bool
	ProxyExecutable          string
	ProxyListeners           []ProxyListener
	RuntimeUID               int
	RoleUIDs                 map[string]int
	EnabledCompat            bool
	EnabledCompatWorkers     bool
	BrokerReplicas           int
	BrokerPIDFile            string
	BrokerExecutable         string
	BrokerMonitorPort        int
	ReplicaNodes             map[string]string
	ReplicaSlots             map[string][]string
}

type ProxyListener struct {
	Role     string
	Protocol string
	Host     string
	Port     int
}

// These keys are reported by the owning domain on real operations. A healthy
// direct connection from the observer cannot substitute for that evidence.
// Event consumers retain a functioning PG path if JetStream is unavailable.
var splitDependencies = map[string][]string{
	"event-relay":                   {"postgres", "jetstream"},
	"ingest-relay":                  {"ingest-relay-postgres"},
	"ingest-custody-worker":         {"postgres", "s3"},
	"parsing-dispatch-worker":       {"parser", "jetstream"},
	"monitoring-incremental-worker": {"jetstream"},
	"monitoring-read-worker":        {"jetstream"},
	"task-index-worker":             {"postgres", "jetstream"},
	"notification-worker":           {"postgres", "jetstream"},
	"audit-worker":                  {"postgres", "jetstream"},
	"files-worker":                  {"postgres"},
}

func splitTopology(m *Manifest, local *Bindings, profile string, opt SplitOptions) (map[string]*Bindings, error) {
	if !Absolute(opt.BinDirectory) || opt.RuntimeUID < 0 || opt.ReportDirectory != "" && !Absolute(opt.ReportDirectory) || opt.ContainerReportDirectory != "" && !Absolute(opt.ContainerReportDirectory) {
		return nil, Invalid
	}
	if opt.FlatLocalReports && (profile != "native" || opt.ReportDirectory == "") {
		return nil, Invalid
	}
	if len(opt.ProxyListeners) > 16 || profile == "docker" && len(opt.ProxyListeners) != 0 ||
		opt.FlatLocalReports && len(opt.ProxyListeners) != 0 && !Absolute(opt.ProxyExecutable) {
		return nil, Invalid
	}
	containerRoles := opt.ContainerRoles
	if containerRoles != nil && profile != "mixed" {
		return nil, Invalid
	}
	if containerRoles == nil {
		containerRoles = []string{"monitoring-history-worker", "monitoring-read-worker"}
	}
	inContainer := func(role string) bool {
		return profile == "docker" && role != "ingest-relay" || profile == "mixed" && slices.Contains(containerRoles, role)
	}
	brokers := opt.BrokerReplicas
	if slots := opt.ReplicaSlots["nats-server"]; len(slots) != 0 {
		if brokers != 0 && brokers != len(slots) {
			return nil, Invalid
		}
		for i, name := range slots {
			if name != "nats-server-"+strconv.Itoa(i+1) {
				return nil, Invalid
			}
		}
		brokers = len(slots)
	}
	if brokers == 0 {
		brokers = 1
		if inContainer("nats-server") {
			brokers = 3
		}
	}
	if brokers != 1 && brokers != 3 {
		return nil, Invalid
	}
	if opt.BrokerPIDFile != "" && (brokers != 1 || inContainer("nats-server") || !Absolute(opt.BrokerPIDFile)) ||
		opt.BrokerExecutable != "" && (opt.BrokerPIDFile == "" || !Absolute(opt.BrokerExecutable)) {
		return nil, Invalid
	}
	if opt.BrokerMonitorPort == 0 {
		opt.BrokerMonitorPort = 8222
	}
	if opt.BrokerMonitorPort < 1 || opt.BrokerMonitorPort > 65535 {
		return nil, Invalid
	}
	for i := 2; i <= brokers; i++ {
		m.Replicas = append(m.Replicas, Replica{ID: "nats-server-" + strconv.Itoa(i), ComponentID: "nats-server", NodeID: "local", Expectation: "disabled", RequiredProbeIDs: []string{}})
	}
	observers := map[string]*Bindings{"local": local}
	assigned := map[string]bool{}
	// One observer per declared node. A daemon observer and a host observer
	// cannot independently attest different namespaces under the same identity.
	vantages := map[string]bool{"local": false}
	bindingsFor := func(id string, container bool) (*Bindings, error) {
		if !ValidID(id) || id == "docker-vm" {
			return nil, Invalid
		}
		if current, ok := observers[id]; ok {
			if vantages[id] != container {
				return nil, Invalid
			}
			return current, nil
		}
		kind, scope := "physical", "host"
		if container {
			kind, scope = "container", "container"
		}
		node := Node{ID: id, DisplayName: id, Kind: kind, ResourceScope: scope, AssignmentRevision: "1", Enabled: true}
		if i := slices.IndexFunc(m.Nodes, func(n Node) bool { return n.ID == id }); i >= 0 {
			m.Nodes[i] = node
		} else {
			m.Nodes = append(m.Nodes, node)
		}
		vantages[id] = container
		observers[id] = &Bindings{SchemaVersion: 1, InstanceID: m.InstanceID, NodeID: id, AssignmentRevision: "1", Bindings: []Binding{}}
		return observers[id], nil
	}
	brokerNodes := map[string]bool{}
	catalog := map[string]CatalogEntry{}
	for _, entry := range RuntimeCatalog {
		catalog[entry.ID] = entry
	}
	for _, name := range containerRoles {
		if entry, ok := catalog[name]; !ok || entry.EntryPoint == "" && name != "nats-server" || entry.Kind == "tool" {
			return nil, Invalid
		}
	}
	for name, ids := range opt.ReplicaSlots {
		if _, ok := catalog[name]; !ok || len(ids) == 0 || len(ids) > 128 {
			return nil, Invalid
		}
	}
	var replicas []Replica
	known := map[string]bool{}
	for _, r := range m.Replicas {
		ids := opt.ReplicaSlots[r.ComponentID]
		if len(ids) == 0 || r.ComponentID == "nats-server" {
			ids = []string{r.ID}
		}
		for _, id := range ids {
			if !ValidID(id) || known[id] || len(r.RequiredProbeIDs) != 0 && id != r.ID {
				return nil, Invalid
			}
			known[id] = true
			copy := r
			copy.ID = id
			replicas = append(replicas, copy)
		}
	}
	m.Replicas = replicas
	for name, n := range opt.RoleUIDs {
		if _, ok := catalog[name]; !ok && !known[name] || n < 0 || int64(n) > 2147483647 {
			return nil, Invalid
		}
	}
	for i := range m.Replicas {
		r := &m.Replicas[i]
		entry := catalog[r.ComponentID]
		if entry.ID == "replica-proxy" && len(opt.ProxyListeners) == 0 {
			continue
		}
		legacyWorker := entry.ID == "ingest-worker" || entry.ID == "monitoring-worker"
		if (entry.EntryPoint == "" && entry.ID != "nats-server") || entry.Kind == "tool" || entry.ID == "compat-api" && !opt.EnabledCompat || legacyWorker && !opt.EnabledCompatWorkers {
			if node, ok := opt.ReplicaNodes[r.ID]; ok && len(opt.ReplicaSlots[entry.ID]) > 0 {
				owner, err := bindingsFor(node, inContainer(entry.ID))
				if err != nil {
					return nil, err
				}
				r.NodeID = owner.NodeID
				assigned[r.ID] = true
			}
			continue
		}
		container := inContainer(entry.ID)
		node := "local"
		if container {
			node = "docker-observer"
		} else if profile == "docker" && entry.ID == "ingest-relay" {
			node = "edge"
		} else if entry.ID == "nats-server" && r.ID != "nats-server-1" {
			node = "broker-node-" + r.ID[len("nats-server-"):]
		}
		if override, ok := opt.ReplicaNodes[r.ID]; ok {
			node = override
			assigned[r.ID] = true
		}
		target, err := bindingsFor(node, container)
		if err != nil {
			return nil, err
		}
		if entry.ID == "nats-server" && !container {
			// Fixed systemd unit and loopback port identify a single broker per host.
			if brokerNodes[node] {
				return nil, Invalid
			}
			brokerNodes[node] = true
		}
		r.NodeID = target.NodeID
		r.Expectation = "continuous"
		for j, c := range m.Components {
			if c.ID == entry.ID {
				m.Components[j].Expectation = "continuous"
				m.Components[j].MinReadyReplicas = 1
				if entry.ID == "nats-server" {
					m.Components[j].MinReadyReplicas = brokers/2 + 1
				}
				m.Components[j].ExpectationReason = entry.Reason
				break
			}
		}
		attach := func(b Binding, facet string, required bool) {
			p := Probe{ID: b.ID, NodeID: r.NodeID, TargetKind: "replica", TargetID: r.ID, Adapter: b.Adapter, BindingID: b.ID, Facet: facet, Required: required, IntervalMS: 5000, TimeoutMS: 2000, StaleAfterMS: 20000}
			m.Probes = append(m.Probes, p)
			target.Bindings = append(target.Bindings, b)
			if required {
				r.RequiredProbeIDs = append(r.RequiredProbeIDs, p.ID)
			}
		}
		uid := opt.RuntimeUID
		if n, ok := opt.RoleUIDs[entry.ID]; ok {
			uid = n
		}
		if n, ok := opt.RoleUIDs[r.ID]; ok {
			uid = n
		}
		reportOwner := entry.ID
		if len(opt.ReplicaSlots[entry.ID]) > 1 {
			reportOwner = r.ID
		}
		root := filepath.Join("/run", "infrapro-"+reportOwner)
		reportDir := opt.ReportDirectory
		if container && opt.ContainerReportDirectory != "" {
			reportDir = opt.ContainerReportDirectory
		}
		if reportDir != "" {
			root = filepath.Join(reportDir, reportOwner)
		}
		file := filepath.Join(root, "process.json")
		if opt.FlatLocalReports {
			root = reportDir
			file = filepath.Join(root, reportOwner+".json")
		}
		executable := filepath.Join(opt.BinDirectory, entry.ID)
		probePrefix := entry.ID
		if reportOwner != entry.ID || entry.ID == "nats-server" && r.ID != "nats-server-1" {
			probePrefix = r.ID
		}
		namespace := "host"
		if entry.ID == "replica-proxy" {
			if container {
				return nil, Invalid
			}
			presence := Binding{ID: "replica-proxy-presence", Adapter: "systemd_unit", UnitName: "infrapro-replica-proxy.service"}
			if opt.FlatLocalReports {
				presence = Binding{ID: presence.ID, Adapter: "native_process", PIDFile: file, PIDFileFormat: "decimal", AllowedRoot: root, ExpectedExecutable: opt.ProxyExecutable, ExpectedUID: &uid}
			}
			attach(presence, "presence", true)
			for _, socket := range opt.ProxyListeners {
				if !ValidID(socket.Role) || !OneOf(socket.Protocol, "private-tls", "external-http") || !TargetHost(socket.Host, nil) || socket.Port < 1 || socket.Port > 65535 {
					return nil, Invalid
				}
				b := Binding{ID: "proxy-" + socket.Role + "-" + socket.Protocol, Adapter: "proxy_listener", Host: socket.Host, Port: socket.Port}
				attach(b, "readiness", true)
			}
			continue
		}
		if container {
			namespace = "container"
			executable = filepath.Join("/usr/local/bin", entry.ID)
			if entry.ID == "parser" {
				executable = "/parser"
			}
			attach(Binding{ID: probePrefix + "-presence", Adapter: "docker_container", ExpectedReplicaID: r.ID, SocketPath: "/var/run/docker.sock", SelectorLabels: map[string]string{"io.infrapro.instance": m.InstanceID, "io.infrapro.replica": r.ID}, StatsEnabled: true}, "presence", true)
		} else if entry.ID == "nats-server" {
			presence := Binding{ID: probePrefix + "-presence", Adapter: "systemd_unit", UnitName: "infrapro-nats-server.service", ExpectedUID: &uid}
			if opt.BrokerPIDFile != "" {
				if opt.BrokerExecutable != "" {
					executable = opt.BrokerExecutable
				}
				presence = Binding{ID: presence.ID, Adapter: "native_process", PIDFile: opt.BrokerPIDFile, PIDFileFormat: "decimal", AllowedRoot: filepath.Dir(opt.BrokerPIDFile), ExpectedExecutable: executable, ExpectedUID: &uid}
			}
			attach(presence, "presence", true)
		} else {
			attach(Binding{ID: probePrefix + "-presence", Adapter: "native_process", ReportFile: file, AllowedRoot: root, ExpectedExecutable: executable, ExpectedUID: &uid}, "presence", true)
		}
		if entry.ID == "nats-server" {
			if container {
				attach(Binding{ID: probePrefix + "-ready", Adapter: "docker_container", ExpectedReplicaID: r.ID, SocketPath: "/var/run/docker.sock", SelectorLabels: map[string]string{"io.infrapro.instance": m.InstanceID, "io.infrapro.replica": r.ID}}, "readiness", true)
			} else {
				attach(Binding{ID: probePrefix + "-ready", Adapter: "jetstream_health", Host: "127.0.0.1", Port: opt.BrokerMonitorPort}, "readiness", true)
			}
			continue
		}
		report := Binding{ID: probePrefix + "-report", Adapter: "self_report", ReportFile: file, AllowedRoot: root, ExpectedExecutable: executable, ExpectedUID: &uid, ExpectedReplicaID: r.ID, PIDNamespace: namespace}
		attach(report, "loop", true)
		for _, key := range splitDependencies[entry.ID] {
			id := probePrefix + "-" + key
			component := key
			if key == "jetstream" {
				component = "nats-server"
			}
			check := report
			check.ID, check.DependencyKey = id+"-check", key
			m.Dependencies = append(m.Dependencies, Dependency{ID: id, FromReplicaID: r.ID, ToComponentID: component, ProbeID: check.ID, EvidenceMode: "self_report"})
			m.Probes = append(m.Probes, Probe{ID: check.ID, NodeID: r.NodeID, TargetKind: "dependency", TargetID: id, Adapter: check.Adapter, BindingID: check.ID, Facet: "readiness", IntervalMS: 5000, TimeoutMS: 2000, StaleAfterMS: 20000})
			target.Bindings = append(target.Bindings, check)
		}
	}
	if len(assigned) != len(opt.ReplicaNodes) {
		return nil, Invalid
	}
	delete(observers, "local")
	return observers, nil
}

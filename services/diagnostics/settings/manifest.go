package settings

func DefaultLimits() Limits { return Limits{32, 512, 2048, 256, 8, 256 << 20, 50000} }
func (l *Limits) Normalize() error {
	max := DefaultLimits()
	pairs := [][2]*int{{&l.MaxNodes, &max.MaxNodes}, {&l.MaxReplicas, &max.MaxReplicas}, {&l.MaxProbes, &max.MaxProbes}, {&l.ProbesPerNode, &max.ProbesPerNode}, {&l.ConcurrentProbes, &max.ConcurrentProbes}, {&l.HistoryPoints, &max.HistoryPoints}}
	for _, p := range pairs {
		if *p[0] == 0 {
			*p[0] = *p[1]
		}
		if *p[0] < 1 || *p[0] > *p[1] {
			return Invalid
		}
	}
	if l.StoreMaxBytes == 0 {
		l.StoreMaxBytes = max.StoreMaxBytes
	}
	if l.StoreMaxBytes < 1<<20 || l.StoreMaxBytes > max.StoreMaxBytes {
		return Invalid
	}
	return nil
}
func Expectation(s string) bool { return OneOf(s, "continuous", "on_demand", "disabled", "external") }
func (m Manifest) Validate(l Limits) error {
	if l.Normalize() != nil || m.SchemaVersion != 1 || !ValidID(m.InstanceID) || !Decimal(m.Revision, false) || !Name(m.DisplayName) || len(m.Nodes) > l.MaxNodes || len(m.Components) > l.MaxReplicas || len(m.Replicas) > l.MaxReplicas || len(m.Endpoints) > l.MaxProbes || len(m.Dependencies) > l.MaxProbes || len(m.Probes) > l.MaxProbes {
		return Invalid
	}
	if m.Nodes == nil || m.Components == nil || m.Replicas == nil || m.Endpoints == nil || m.Dependencies == nil || m.Probes == nil {
		return Invalid
	}
	nodes := map[string]Node{}
	comps := map[string]Component{}
	reps := map[string]Replica{}
	ends := map[string]Endpoint{}
	deps := map[string]Dependency{}
	probes := map[string]Probe{}
	for _, n := range m.Nodes {
		if !ValidID(n.ID) || nodes[n.ID].ID != "" || !Name(n.DisplayName) || !OneOf(n.Kind, "physical", "vm", "container", "logical_test", "external") || !OneOf(n.ResourceScope, "host", "vm", "container", "none") || !Decimal(n.AssignmentRevision, false) {
			return Invalid
		}
		if n.Kind == "logical_test" && n.ResourceScope != "none" {
			return Invalid
		}
		if n.Kind == "container" && n.ResourceScope != "container" {
			return Invalid
		}
		if n.Kind == "external" && n.ResourceScope != "none" {
			return Invalid
		}
		nodes[n.ID] = n
	}
	for _, n := range m.Nodes {
		seen := map[string]bool{n.ID: true}
		p := n.ParentNodeID
		for p != "" {
			if seen[p] || nodes[p].ID == "" {
				return Invalid
			}
			seen[p] = true
			p = nodes[p].ParentNodeID
		}
	}
	for _, c := range m.Components {
		if !ValidID(c.ID) || comps[c.ID].ID != "" || !Name(c.DisplayName) || !OneOf(c.Kind, "application", "worker", "database", "cache", "object_store", "gateway", "tool", "diagnostics") || !Expectation(c.Expectation) || c.MinReadyReplicas < 0 || c.MinReadyReplicas > l.MaxReplicas || c.ExpectationReason != "" && !Name(c.ExpectationReason) {
			return Invalid
		}
		comps[c.ID] = c
	}
	counts := map[string]int{}
	for _, r := range m.Replicas {
		if !ValidID(r.ID) || reps[r.ID].ID != "" || comps[r.ComponentID].ID == "" || nodes[r.NodeID].ID == "" || !Expectation(r.Expectation) || r.RequiredProbeIDs == nil || len(r.RequiredProbeIDs) > 32 {
			return Invalid
		}
		reps[r.ID] = r
		if r.Expectation == "continuous" {
			counts[r.ComponentID]++
		}
	}
	for _, c := range m.Components {
		if c.MinReadyReplicas > counts[c.ID] {
			return Invalid
		}
	}
	direct := map[string]int{}
	for _, e := range m.Endpoints {
		if !ValidID(e.ID) || ends[e.ID].ID != "" || reps[e.ReplicaID].ID == "" || !Name(e.DisplayName) || !OneOf(e.Purpose, "liveness", "readiness", "serving", "storage", "asset", "control_plane") {
			return Invalid
		}
		ends[e.ID] = e
		direct[e.ReplicaID]++
	}
	for _, d := range m.Dependencies {
		if !ValidID(d.ID) || deps[d.ID].ID != "" || reps[d.FromReplicaID].ID == "" || comps[d.ToComponentID].ID == "" || !OneOf(d.EvidenceMode, "self_report", "shared_network", "external_probe") {
			return Invalid
		}
		deps[d.ID] = d
		direct[d.FromReplicaID]++
	}
	perNode := map[string]int{}
	presence := map[string]bool{}
	report := map[string]bool{}
	for _, p := range m.Probes {
		if !ValidID(p.ID) || probes[p.ID].ID != "" || nodes[p.NodeID].ID == "" || !ValidID(p.BindingID) || BindingFields[p.Adapter] == nil || !OneOf(p.Facet, "presence", "readiness", "loop", "resource") || p.IntervalMS < 1000 || p.IntervalMS > 300000 || p.TimeoutMS < 100 || p.TimeoutMS > 5000 || p.StaleAfterMS < 3*p.IntervalMS || p.StaleAfterMS <= p.TimeoutMS || p.StaleAfterMS > 900000 {
			return Invalid
		}
		rid := ""
		switch p.TargetKind {
		case "node":
			if nodes[p.TargetID].ID == "" {
				return Invalid
			}
		case "replica":
			rid = p.TargetID
			if reps[rid].ID == "" {
				return Invalid
			}
		case "endpoint":
			rid = ends[p.TargetID].ReplicaID
			if rid == "" {
				return Invalid
			}
		case "dependency":
			if deps[p.TargetID].ID == "" {
				return Invalid
			}
		default:
			return Invalid
		}
		if rid != "" {
			direct[rid]++
			if p.Required && p.Facet == "presence" {
				presence[rid] = true
			}
			if p.Required && (p.Adapter == "self_report" || p.Facet == "readiness") {
				report[rid] = true
			}
		}
		perNode[p.NodeID]++
		if perNode[p.NodeID] > l.ProbesPerNode {
			return Invalid
		}
		probes[p.ID] = p
	}
	for _, r := range m.Replicas {
		if direct[r.ID] > 32 {
			return Invalid
		}
		seen := map[string]bool{}
		presence[r.ID], report[r.ID] = false, false
		for _, id := range r.RequiredProbeIDs {
			p := probes[id]
			if p.ID == "" || !p.Required || seen[id] || !(p.TargetKind == "replica" && p.TargetID == r.ID || p.TargetKind == "endpoint" && ends[p.TargetID].ReplicaID == r.ID) {
				return Invalid
			}
			seen[id] = true
			presence[r.ID] = presence[r.ID] || p.Facet == "presence"
			report[r.ID] = report[r.ID] || p.Adapter == "self_report" || p.Facet == "readiness"
		}
		if r.Expectation == "continuous" {
			if !presence[r.ID] {
				return Invalid
			}
			kind := comps[r.ComponentID].Kind
			if (kind == "application" || kind == "worker" || kind == "diagnostics") && !report[r.ID] {
				return Invalid
			}
			if len(r.RequiredProbeIDs) == 0 {
				return Invalid
			}
		}
	}
	for _, d := range m.Dependencies {
		p := probes[d.ProbeID]
		if p.ID == "" || p.TargetKind != "dependency" || p.TargetID != d.ID || p.Required != d.Required {
			return Invalid
		}
		if d.EvidenceMode == "self_report" && p.Adapter != "self_report" {
			return Invalid
		}
		if d.EvidenceMode != "self_report" && p.Adapter == "self_report" {
			return Invalid
		}
		if d.EvidenceMode == "shared_network" && p.NodeID != reps[d.FromReplicaID].NodeID {
			a, b := nodes[p.NodeID], nodes[reps[d.FromReplicaID].NodeID]
			if a.ParentNodeID == "" || a.ParentNodeID != b.ParentNodeID {
				return Invalid
			}
		}
	}
	return nil
}
func (m Manifest) Node(id string) (Node, bool) {
	for _, n := range m.Nodes {
		if n.ID == id {
			return n, true
		}
	}
	return Node{}, false
}
func (m Manifest) Assigned(node string) []Probe {
	out := []Probe{}
	n, ok := m.Node(node)
	if !ok || !n.Enabled {
		return out
	}
	for _, p := range m.Probes {
		if p.NodeID == node {
			out = append(out, p)
		}
	}
	return out
}
func (m Manifest) ReplicaFor(p Probe) (Replica, bool) {
	rid := p.TargetID
	if p.TargetKind == "endpoint" {
		for _, e := range m.Endpoints {
			if e.ID == p.TargetID {
				rid = e.ReplicaID
			}
		}
	}
	if p.TargetKind == "dependency" {
		for _, d := range m.Dependencies {
			if d.ID == p.TargetID {
				rid = d.FromReplicaID
			}
		}
	}
	for _, r := range m.Replicas {
		if r.ID == rid {
			return r, true
		}
	}
	return Replica{}, false
}

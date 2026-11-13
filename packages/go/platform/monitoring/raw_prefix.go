package monitoring

import (
	"context"
	"slices"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
	"github.com/jackc/pgx/v5"
)

// RawPrefix asks for actual predecessors from the same fixed source manifest.
// The search is not an authorization grant. The expanded candidate/non-hit
// range must pass current authorization before any evidence becomes usable.
type RawPrefix struct {
	Channel      string   `json:"channel_id"`
	Before       int64    `json:"before_ns,string"`
	Count        int      `json:"count"`
	Qualities    []string `json:"qualities"`
	WholeHistory bool     `json:"whole_history"`
}

// PrefixAuthorityTx makes future predecessor searches explicit in the control
// preview. It extends only to the start of the same currently granted read
// interval; it cannot bridge a permission gap or add future authority.
func (s *Service) PrefixAuthorityTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, needs []RawNeed, channels []string) ([]RawNeed, error) {
	auth, e := s.rawAuthorizationTx(ctx, tx, a, scope)
	if e != nil {
		return nil, e
	}
	allowed, e := authorizedWindowsTx(ctx, tx, auth, channels, "read")
	if e != nil {
		return nil, e
	}
	out := slices.Clone(needs)
	for i, n := range out {
		if !slices.Contains(channels, n.Channel) {
			continue
		}
		for _, w := range allowed[n.Channel] {
			if w.Covers(Window{From: n.From, To: n.To}) {
				out[i].From = w.From
				if out[i].From == nil {
					minimum := monitoringvalue.MinTime.UnixNano()
					out[i].From = &minimum
				}
				break
			}
		}
	}
	if e = authorizeRawNeedsTx(ctx, tx, auth, out, false); e != nil {
		return nil, e
	}
	return out, nil
}

// CalculationPrefixAuthorityTx includes formal derived inputs; the raw reader
// still rejects derived identities when preparing native snapshots.
func (s *Service) CalculationPrefixAuthorityTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, needs []RawNeed, channels []string) ([]RawNeed, error) {
	auth, e := s.rawAuthorizationTx(ctx, tx, a, scope)
	if e != nil {
		return nil, e
	}
	allowed, e := authorizedWindowsTx(ctx, tx, auth, channels, "read")
	if e != nil {
		return nil, e
	}
	out := slices.Clone(needs)
	for i, n := range out {
		if !slices.Contains(channels, n.Channel) {
			continue
		}
		for _, w := range allowed[n.Channel] {
			if w.Covers(Window{From: n.From, To: n.To}) {
				out[i].From = w.From
				if out[i].From == nil {
					minimum := monitoringvalue.MinTime.UnixNano()
					out[i].From = &minimum
				}
				break
			}
		}
	}
	for _, need := range out {
		var kind string
		if e = tx.QueryRow(ctx, `SELECT source_kind FROM engineering_channels WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL`, scope.Tenant, need.Channel).Scan(&kind); e != nil {
			return nil, e
		}
		if kind == "derived" {
			e = s.AuthorizeCalculationOutputTx(ctx, tx, a, scope, need.Channel, []Window{{From: need.From, To: need.To}}, false)
		} else {
			e = authorizeRawNeedsTx(ctx, tx, auth, []RawNeed{need}, false)
		}
		if e != nil {
			return nil, e
		}
	}
	return out, nil
}

func prefixManifestWindows(windows map[string][]Window, prefixes []RawPrefix) (map[string][]Window, error) {
	out := map[string][]Window{}
	for id, parts := range windows {
		out[id] = slices.Clone(parts)
	}
	if len(prefixes) > 512 {
		return nil, invalid("prefixes", "前驱规划超过预算。")
	}
	for _, p := range prefixes {
		if len(windows[p.Channel]) == 0 || p.Before < monitoringvalue.MinTime.UnixNano() || p.Count < 0 || p.Count > 1_000_000 || !p.WholeHistory && p.Count == 0 {
			return nil, invalid("prefixes", "需要有效且有界的前驱请求。")
		}
		for _, q := range p.Qualities {
			if !slices.Contains([]string{"good", "unmarked", "suspect", "bad"}, q) {
				return nil, invalid("prefixes", "前驱质量筛选无效。")
			}
		}
		if p.Before > monitoringvalue.MinTime.UnixNano() {
			out[p.Channel] = UnionWindows(append(out[p.Channel], Window{From: nil, To: &p.Before}))
		}
	}
	return out, nil
}

func (r rawReader) expandPrefixes(ctx context.Context, tenant string, evidence *rawEvidence, prefixes []RawPrefix) error {
	if len(prefixes) == 0 {
		return nil
	}
	_, windows, _, e := normalizeRawNeeds(evidence.Candidates)
	if e != nil {
		return e
	}
	if _, e = prefixManifestWindows(windows, prefixes); e != nil {
		return e
	}
	needs := slices.Clone(evidence.Candidates)
	for _, p := range prefixes {
		start := monitoringvalue.MinTime.UnixNano()
		minimum := start
		if p.Before <= start {
			continue
		}
		if !p.WholeHistory && len(evidence.Manifest) > 0 {
			if e = r.service.requireStorage(); e != nil {
				return e
			}
			count := 0
			e = r.service.storage.read(ctx, rawRead{Tenant: tenant, Manifest: evidence.Manifest, Windows: map[string][]Window{p.Channel: {{From: &minimum, To: &p.Before}}}, Qualities: p.Qualities, Reverse: true, Order: "channel", EndpointOnly: true, Limit: p.Count}, func(row rawObservation) error { count++; start = row.At; return ctx.Err() })
			if e != nil {
				return e
			}
			if count < p.Count {
				start = monitoringvalue.MinTime.UnixNano()
			}
		}
		needs = append(needs, RawNeed{Channel: p.Channel, From: &start, To: &p.Before})
	}
	evidence.Candidates, _, _, e = normalizeRawNeeds(needs)
	if e != nil {
		return e
	}
	evidence.Needs = slices.Clone(evidence.Candidates)
	return nil
}

func (r rawReader) resolvePart(ctx context.Context, tenant string, original rawEvidence, in RawFreezeRequest) (rawEvidence, error) {
	part, e := selectRawPart(original, in.Needs)
	if e != nil {
		return part, e
	}
	part.Needs = slices.Clone(part.Candidates)
	if e = r.expandPrefixes(ctx, tenant, &part, in.Prefixes); e != nil {
		return part, e
	}
	if _, e = selectRawPart(original, part.Candidates); e != nil {
		return part, e
	}
	return part, nil
}

func (r rawReader) ResolvePartUser(ctx context.Context, a access.Actor, scope Scope, id string, in RawFreezeRequest) (RawSnapshotDetails, error) {
	var original, part rawEvidence
	var state RawSnapshotState
	complete := false
	e := r.service.transact(ctx, a, scope, "details", false, false, func(tx pgx.Tx, auth authorization) error {
		if e := authorizeRawNeedsTx(ctx, tx, auth, in.Needs, in.History); e != nil {
			return e
		}
		var e error
		original, e = loadRawEvidenceTx(ctx, tx, scope.Tenant, id)
		if e != nil {
			return e
		}
		// Instantaneous inputs without predecessor searches require no
		// external I/O. Project and qualify them under this same authority
		// transaction; interval and predecessor expansion keep both checks.
		if len(in.Prefixes) == 0 && len(original.intervalChannels) == 0 {
			part, e = r.resolvePart(ctx, scope.Tenant, original, in)
			if e != nil {
				return e
			}
			complete = true
			state, e = checkRawEvidenceTx(ctx, tx, scope.Tenant, part, in.History)
		}
		return e
	})
	if complete {
		return RawSnapshotDetails{part.Snapshot, part.Needs, part.Candidates, state}, e
	}
	if e != nil {
		return RawSnapshotDetails{}, e
	}
	part, e = r.resolvePart(ctx, scope.Tenant, original, in)
	if e != nil {
		return RawSnapshotDetails{}, e
	}
	if e = r.expandIntervals(ctx, scope.Tenant, &part, func(needs []RawNeed) error {
		return r.service.transact(ctx, a, scope, "details", false, false, func(tx pgx.Tx, auth authorization) error {
			return authorizeRawNeedsTx(ctx, tx, auth, needs, in.History)
		})
	}); e != nil {
		return RawSnapshotDetails{}, e
	}
	e = r.service.transact(ctx, a, scope, "details", false, false, func(tx pgx.Tx, auth authorization) error {
		if e := authorizeRawNeedsTx(ctx, tx, auth, part.Needs, in.History); e != nil {
			return e
		}
		var e error
		state, e = checkRawEvidenceTx(ctx, tx, scope.Tenant, part, in.History)
		return e
	})
	return RawSnapshotDetails{part.Snapshot, part.Needs, part.Candidates, state}, e
}

func (r rawReader) ResolvePartSystem(ctx context.Context, p RawSystemPermit, id string, in RawFreezeRequest) (RawSnapshotDetails, error) {
	if in.History {
		return RawSnapshotDetails{}, invalid("history", "系统当前计算不能混入历史输入。")
	}
	original, _, e := r.systemEvidence(ctx, p, id, false)
	if e != nil {
		return RawSnapshotDetails{}, e
	}
	part, e := r.resolvePart(ctx, p.Tenant, original, in)
	if e != nil {
		return RawSnapshotDetails{}, e
	}
	if e = r.expandIntervals(ctx, p.Tenant, &part, func(needs []RawNeed) error {
		return r.systemTx(ctx, p, func(tx pgx.Tx) error { return r.service.rawSystem.AuthorizeRawSystemTx(ctx, tx, p, needs, false) })
	}); e != nil {
		return RawSnapshotDetails{}, e
	}
	state, e := r.checkKnownSystem(ctx, p, part, false)
	return RawSnapshotDetails{part.Snapshot, part.Needs, part.Candidates, state}, e
}

// ResolveDependencySystem checks a prior generation's raw evidence under the
// consuming run's current mandate. It neither borrows the original actor nor
// lets a system computation read revoked historical observations.
func (r rawReader) ResolveDependencySystem(ctx context.Context, p RawSystemPermit, id string, in RawFreezeRequest) (RawSnapshotDetails, error) {
	if in.History {
		return RawSnapshotDetails{}, access.ErrDenied
	}
	var original rawEvidence
	e := r.systemTx(ctx, p, func(tx pgx.Tx) error {
		if e := r.service.rawSystem.AuthorizeRawSystemTx(ctx, tx, p, in.Needs, false); e != nil {
			return e
		}
		var e error
		original, e = loadRawEvidenceTx(ctx, tx, p.Tenant, id)
		return e
	})
	if e != nil {
		return RawSnapshotDetails{}, e
	}
	part, e := r.resolvePart(ctx, p.Tenant, original, in)
	if e != nil {
		return RawSnapshotDetails{}, e
	}
	if e = r.expandIntervals(ctx, p.Tenant, &part, func(needs []RawNeed) error {
		return r.systemTx(ctx, p, func(tx pgx.Tx) error { return r.service.rawSystem.AuthorizeRawSystemTx(ctx, tx, p, needs, false) })
	}); e != nil {
		return RawSnapshotDetails{}, e
	}
	state, e := r.checkKnownSystem(ctx, p, part, false)
	return RawSnapshotDetails{part.Snapshot, part.Needs, part.Candidates, state}, e
}

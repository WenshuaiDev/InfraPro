package monitoring

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicecoord"
	"github.com/jackc/pgx/v5"
)

// RawReader is the sole calculation-facing raw storage seam. Evidence stays in
// monitoring; callers receive immutable identities and canonical numeric text.
// User and system paths are deliberately distinct. No HTTP field can turn a
// user read into a system read.
type RawReader interface {
	FreezeUser(context.Context, access.Actor, Scope, RawFreezeRequest) (RawSnapshot, error)
	FreezeSystem(context.Context, RawSystemPermit, RawFreezeRequest) (RawSnapshot, error)
	CheckUser(context.Context, access.Actor, Scope, string, bool) (RawSnapshotState, error)
	CheckSystem(context.Context, RawSystemPermit, string, bool) (RawSnapshotState, error)
	ReadUser(context.Context, access.Actor, Scope, string, RawReadBudget, func(RawSample) error) (RawReadCount, error)
	ReadSystem(context.Context, RawSystemPermit, string, RawReadBudget, func(RawSample) error) (RawReadCount, error)
	ReadPartUser(context.Context, access.Actor, Scope, string, []RawNeed, RawReadBudget, func(RawSample) error) (RawReadCount, error)
	ReadSelectedPartUser(context.Context, access.Actor, Scope, string, []RawNeed, bool, RawReadBudget, func(RawSample) error) (RawReadCount, error)
	ReadPartSystem(context.Context, RawSystemPermit, string, []RawNeed, RawReadBudget, func(RawSample) error) (RawReadCount, error)
	ReplayUser(context.Context, access.Actor, Scope, string, RawReadBudget, func(RawSample) error) (RawReadCount, error)
	ReplayPartUser(context.Context, access.Actor, Scope, string, []RawNeed, RawReadBudget, func(RawSample) error) (RawReadCount, error)
	ReplaySystem(context.Context, RawSystemPermit, string, RawReadBudget, func(RawSample) error) (RawReadCount, error)
	InspectUser(context.Context, access.Actor, Scope, string, bool) (RawSnapshotDetails, error)
	InspectSystem(context.Context, RawSystemPermit, string, bool) (RawSnapshotDetails, error)
	ResolvePartUser(context.Context, access.Actor, Scope, string, RawFreezeRequest) (RawSnapshotDetails, error)
	ResolvePartSystem(context.Context, RawSystemPermit, string, RawFreezeRequest) (RawSnapshotDetails, error)
	ResolveDependencySystem(context.Context, RawSystemPermit, string, RawFreezeRequest) (RawSnapshotDetails, error)
}
type RawSnapshotDetails struct {
	Snapshot   RawSnapshot
	Needs      []RawNeed
	Candidates []RawNeed
	State      RawSnapshotState
}

func (r rawReader) InspectUser(ctx context.Context, a access.Actor, scope Scope, id string, replay bool) (RawSnapshotDetails, error) {
	evidence, state, e := r.userEvidence(ctx, a, scope, id, replay)
	return RawSnapshotDetails{evidence.Snapshot, evidence.Needs, evidence.Candidates, state}, e
}
func (r rawReader) InspectSystem(ctx context.Context, p RawSystemPermit, id string, replay bool) (RawSnapshotDetails, error) {
	evidence, state, e := r.systemEvidence(ctx, p, id, replay)
	return RawSnapshotDetails{evidence.Snapshot, evidence.Needs, evidence.Candidates, state}, e
}

// AuthorizeRawInputTx and CheckRawSnapshotTx join an already-open domain
// transaction. They perform no physical I/O. They repeat current monitoring
// qualification so a successful trial/publication cannot race a revocation.
func (s *Service) rawAuthorizationTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope) (authorization, error) {
	auth := authorization{Scope: scope, Account: a.ID, Subject: a.ReadSubject()}
	if !scope.valid() || a.Check == nil {
		return auth, access.ErrDenied
	}
	var e error
	auth.Revision, e = access.Lock(ctx, tx, false)
	if e != nil {
		return auth, e
	}
	if e = a.CheckInputReadTx(ctx, tx); e != nil {
		return auth, e
	}
	op := access.MonitoringOperation(scope.Entry, "details")
	if scope.Entry == "project" {
		if _, e = access.RequireActorProjectTx(ctx, tx, a, op, scope.Tenant, scope.Project); e != nil {
			return auth, e
		}
	} else if e = access.RequireScopedTx(ctx, tx, a.ID, op, scope.Tenant); e != nil {
		return auth, e
	}
	if _, e = tx.Exec(ctx, `SELECT id FROM engineering_catalog_state WHERE id=1 FOR SHARE`); e != nil {
		return auth, e
	}
	if e = devicecoord.Catalog(ctx, tx, false); e != nil {
		return auth, e
	}
	if e = devicecoord.Tenant(ctx, tx, scope.Tenant, false); e != nil {
		return auth, e
	}
	var readable bool
	if e = tx.QueryRow(ctx, `SELECT mode<>'restoring' FROM monitoring_environment_state WHERE id=1`).Scan(&readable); e != nil {
		return auth, e
	}
	if !readable {
		return auth, problem("monitoring_restoring", "监测环境尚未恢复就绪。", 503)
	}
	return auth, nil
}
func (s *Service) AuthorizeRawInputTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, needs []RawNeed, history bool, capabilities ...string) error {
	auth, e := s.rawAuthorizationTx(ctx, tx, a, scope)
	if e != nil {
		return e
	}
	return authorizeRawNeedsTx(ctx, tx, auth, needs, history, capabilities...)
}
func (s *Service) CheckRawSnapshotTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, id string, replay bool, capabilities ...string) (RawSnapshotState, error) {
	auth, e := s.rawAuthorizationTx(ctx, tx, a, scope)
	if e != nil {
		return RawSnapshotState{}, e
	}
	evidence, e := loadRawEvidenceTx(ctx, tx, scope.Tenant, id)
	if e != nil {
		return RawSnapshotState{}, e
	}
	if e = authorizeRawNeedsTx(ctx, tx, auth, evidence.Needs, replay || evidence.History, capabilities...); e != nil {
		return RawSnapshotState{}, e
	}
	return checkRawEvidenceTx(ctx, tx, scope.Tenant, evidence, replay)
}

// CheckRawSystemSnapshotTx closes the same authorization/invalidation race at
// a calculation publication boundary without external I/O in the transaction.
func (s *Service) CheckRawSystemSnapshotTx(ctx context.Context, tx pgx.Tx, p RawSystemPermit, id string) (RawSnapshotState, error) {
	if s.rawSystem == nil {
		return RawSnapshotState{}, access.ErrDenied
	}
	var own bool
	if e := tx.QueryRow(ctx, `SELECT system_run_id=$3::uuid AND actor_id IS NULL AND project_id IS NULL FROM monitoring_raw_snapshots WHERE tenant_id=$1 AND id=$2`, p.Tenant, id, p.RunID).Scan(&own); e != nil {
		return RawSnapshotState{}, e
	}
	if !own {
		return RawSnapshotState{}, access.ErrDenied
	}
	evidence, e := loadRawEvidenceTx(ctx, tx, p.Tenant, id)
	if e != nil {
		return RawSnapshotState{}, e
	}
	if e = s.rawSystem.AuthorizeRawSystemTx(ctx, tx, p, evidence.Needs, evidence.History); e != nil {
		return RawSnapshotState{}, e
	}
	return checkRawEvidenceTx(ctx, tx, p.Tenant, evidence, false)
}

// CheckRawSnapshotPartTx authorizes and checks a complete dependency subrange of
// a sealed snapshot. This allows a project to read its permitted portion of a
// larger generation without granting it the rest of the generation's inputs.
func (s *Service) CheckRawSnapshotPartTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, id string, needs []RawNeed, replay bool, capabilities ...string) (RawSnapshotState, error) {
	auth, e := s.rawAuthorizationTx(ctx, tx, a, scope)
	if e != nil {
		return RawSnapshotState{}, e
	}
	original, e := loadRawEvidenceTx(ctx, tx, scope.Tenant, id)
	if e != nil {
		return RawSnapshotState{}, e
	}
	part, e := rawDependencyPart(original, needs)
	if e != nil {
		return RawSnapshotState{}, e
	}
	if e = authorizeRawNeedsTx(ctx, tx, auth, part.Needs, replay || part.History, capabilities...); e != nil {
		return RawSnapshotState{}, e
	}
	return checkRawEvidenceTx(ctx, tx, scope.Tenant, part, replay)
}

// Preserve complete interval evidence for transaction-only requalification.
func rawDependencyPart(original rawEvidence, needs []RawNeed) (rawEvidence, error) {
	part, e := selectRawPart(original, needs)
	if e != nil {
		return part, e
	}
	part.Needs = slices.Clone(needs)
	// An interval proof can begin before its first selected endpoint. Preserve
	// that proof conservatively when subdividing; never clip an interval's owner
	// authorization merely because only its endpoint falls in this result page.
	for i, n := range part.Needs {
		for _, candidate := range original.Candidates {
			if candidate.Channel != n.Channel || candidate.From == nil {
				continue
			}
			for _, proof := range original.Needs {
				if proof.Channel == n.Channel && proof.From != nil && *proof.From < *candidate.From && (n.From == nil || *proof.From < *n.From) {
					start := *proof.From
					part.Needs[i].From = &start
				}
			}
		}
	}

	return part, nil
}

func (s *Service) CheckRawDependencyPartSystemTx(ctx context.Context, tx pgx.Tx, p RawSystemPermit, id string, needs []RawNeed) (RawSnapshotState, error) {
	if s.rawSystem == nil {
		return RawSnapshotState{}, access.ErrDenied
	}
	original, e := loadRawEvidenceTx(ctx, tx, p.Tenant, id)
	if e != nil {
		return RawSnapshotState{}, e
	}
	part, e := rawDependencyPart(original, needs)
	if e != nil {
		return RawSnapshotState{}, e
	}
	if e = s.rawSystem.AuthorizeRawSystemTx(ctx, tx, p, part.Needs, false); e != nil {
		return RawSnapshotState{}, e
	}
	return checkRawEvidenceTx(ctx, tx, p.Tenant, part, false)
}

type RawNeed struct {
	Channel string `json:"channel_id"`
	From    *int64 `json:"from_ns,string"`
	To      *int64 `json:"to_ns,string"`
}
type RawFreezeRequest struct {
	Needs    []RawNeed   `json:"needs"`
	History  bool        `json:"history"`
	Prefixes []RawPrefix `json:"prefixes,omitempty"`
}
type RawSnapshot struct {
	ID             string `json:"id"`
	Schema         string `json:"schema_version"`
	SourcePosition string `json:"source_position"`
	RouteRevision  string `json:"route_revision"`
	Hash           string `json:"content_hash"`
}
type RawSnapshotState struct {
	Changed        bool   `json:"changed"`
	Revoked        bool   `json:"revoked"`
	SourcePosition string `json:"source_position"`
}
type RawSample struct {
	Channel       string `json:"channel_id"`
	Observation   string `json:"observation_id"`
	Adoption      string `json:"adoption_id"`
	Batch         string `json:"batch_id"`
	At            int64  `json:"observed_at_ns,string"`
	Start         *int64 `json:"interval_start_ns,string"`
	Kind          string `json:"value_kind"`
	Value         string `json:"value"`
	Quality       string `json:"quality"`
	QualityReason string `json:"quality_reason"`
}
type RawReadBudget struct{ Rows, Bytes uint64 }
type RawReadCount struct {
	Rows  uint64 `json:"rows,string"`
	Bytes uint64 `json:"bytes,string"`
}
type RawSystemPermit struct{ Tenant, RunID, MandateRevision string }
type RawSystemAuthorizer interface {
	AuthorizeRawSystemTx(context.Context, pgx.Tx, RawSystemPermit, []RawNeed, bool) error
}
type rawReader struct{ service *Service }

func (s *Service) RawReader() RawReader                                   { return rawReader{s} }
func (s *Service) WithRawSystemAuthorizer(a RawSystemAuthorizer) *Service { s.rawSystem = a; return s }

type rawEvidence struct {
	Snapshot         RawSnapshot       `json:"snapshot"`
	Needs            []RawNeed         `json:"needs"`
	Candidates       []RawNeed         `json:"candidates"`
	Manifest         []selectedChunk   `json:"manifest"`
	Integrity        map[string]string `json:"integrity"`
	History          bool              `json:"history"`
	intervalChannels []string
}

func (e rawEvidence) hash() string {
	e.Snapshot.ID = ""
	e.Snapshot.Hash = ""
	return digest(e)
}
func normalizeRawNeeds(needs []RawNeed) ([]RawNeed, map[string][]Window, []string, error) {
	if len(needs) == 0 || len(needs) > 50000 {
		return nil, nil, nil, invalid("needs", "需要 1–50000 个输入范围。")
	}
	windows := map[string][]Window{}
	for _, n := range needs {
		if !access.ValidID(n.Channel) {
			return nil, nil, nil, notFound()
		}
		if n.From != nil && *n.From < -2208988800000000000 || n.To != nil && *n.To < -2208988800000000000 || n.From != nil && n.To != nil && *n.From >= *n.To {
			return nil, nil, nil, invalid("needs", "输入范围必须位于存储时间域且非空。")
		}
		windows[n.Channel] = append(windows[n.Channel], Window{n.From, n.To})
	}
	ids := make([]string, 0, len(windows))
	for id := range windows {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	out := []RawNeed{}
	for _, id := range ids {
		windows[id] = UnionWindows(windows[id])
		for _, w := range windows[id] {
			out = append(out, RawNeed{id, w.From, w.To})
		}
	}
	return out, windows, ids, nil
}
func authorizeRawNeedsTx(ctx context.Context, tx pgx.Tx, auth authorization, needs []RawNeed, history bool, capabilities ...string) error {
	_, windows, ids, e := normalizeRawNeeds(needs)
	if e != nil {
		return e
	}
	allowed, e := authorizedWindowsTx(ctx, tx, auth, ids, "read")
	if e != nil {
		return e
	}
	if history {
		h, e := authorizedWindowsTx(ctx, tx, auth, ids, "history")
		if e != nil {
			return e
		}
		allowed = intersectWindows(allowed, h)
	}
	for _, capability := range capabilities {
		if capability == "" || capability == "read" {
			continue
		}
		if capability != "export" {
			return access.ErrDenied
		}
		extra, e := authorizedWindowsTx(ctx, tx, auth, ids, capability)
		if e != nil {
			return e
		}
		allowed = intersectWindows(allowed, extra)
	}
	for id, ws := range windows {
		for _, w := range ws {
			covered := false
			for _, a := range allowed[id] {
				if a.Covers(w) {
					covered = true
					break
				}
			}
			if !covered {
				return notFound()
			}
		}
	}
	var count int
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM engineering_channels WHERE tenant_id=$1 AND id=ANY($2::uuid[]) AND source_kind='direct' AND deleted_at IS NULL`, auth.Tenant, ids).Scan(&count); e != nil {
		return e
	}
	if count != len(ids) {
		return notFound()
	}
	return nil
}
func rawIntegrityTx(ctx context.Context, tx pgx.Tx, tenant string, ids []string) (map[string]string, error) {
	rows, e := tx.Query(ctx, `SELECT c.id::text,COALESCE(v.integrity_revision,0)::text,COALESCE(v.quarantined,false) FROM engineering_channels c LEFT JOIN monitoring_channel_versions v ON v.tenant_id=c.tenant_id AND v.channel_id=c.id WHERE c.tenant_id=$1 AND c.id=ANY($2::uuid[]) AND c.source_kind='direct' AND c.deleted_at IS NULL ORDER BY c.id`, tenant, ids)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, revision string
		var quarantined bool
		if e = rows.Scan(&id, &revision, &quarantined); e != nil {
			return nil, e
		}
		if quarantined {
			return nil, integrityUnavailable()
		}
		out[id] = revision
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	if len(out) != len(ids) {
		return nil, notFound()
	}
	return out, nil
}
func prepareRawEvidenceTx(ctx context.Context, tx pgx.Tx, tenant string, in RawFreezeRequest) (rawEvidence, error) {
	out := rawEvidence{Snapshot: RawSnapshot{Schema: "raw-input-snapshot/1"}, History: in.History, Manifest: []selectedChunk{}}
	needs, windows, ids, e := normalizeRawNeeds(in.Needs)
	if e != nil {
		return out, e
	}
	out.Needs = needs
	out.Candidates = slices.Clone(needs)
	out.Integrity, e = rawIntegrityTx(ctx, tx, tenant, ids)
	if e != nil {
		return out, e
	}
	position, route := int64(0), int64(1)
	e = tx.QueryRow(ctx, `SELECT commit_sequence,route_revision FROM monitoring_tenant_states WHERE tenant_id=$1`, tenant).Scan(&position, &route)
	if e != nil && e != pgx.ErrNoRows {
		return out, e
	}
	out.Snapshot.SourcePosition = strconv.FormatInt(position, 10)
	out.Snapshot.RouteRevision = strconv.FormatInt(route, 10)
	if position > 0 {
		status := "current"
		if in.History {
			status = "history"
		}
		manifestWindows, e := prefixManifestWindows(windows, in.Prefixes)
		if e != nil {
			return out, e
		}
		out.Manifest, e = manifestTx(ctx, tx, tenant, manifestWindows, position, status, route)
		if e != nil {
			return out, e
		}
	}
	e = tx.QueryRow(ctx, `SELECT COALESCE(array_agg(id::text ORDER BY id),'{}') FROM engineering_channels WHERE tenant_id=$1 AND id=ANY($2::uuid[]) AND timestamp_meaning<>'observation_time'`, tenant, ids).Scan(&out.intervalChannels)
	return out, e
}

// expandIntervals performs physical I/O outside PostgreSQL transactions. It
// examines only fixed-manifest candidates, then widens the permission proof to
// include the complete intervals. A subsequent authorization transaction must
// accept that proof before the snapshot can become visible or be used.
func (r rawReader) expandIntervals(ctx context.Context, tenant string, evidence *rawEvidence, recheck func([]RawNeed) error) error {
	if len(evidence.intervalChannels) == 0 || len(evidence.Manifest) == 0 {
		return nil
	}
	if e := r.service.requireStorage(); e != nil {
		return e
	}
	_, windows, _, e := normalizeRawNeeds(evidence.Candidates)
	if e != nil {
		return e
	}
	for id := range windows {
		if !slices.Contains(evidence.intervalChannels, id) {
			delete(windows, id)
		}
	}
	needs := slices.Clone(evidence.Needs)
	count, lastCheck := 0, time.Now()
	e = r.service.storage.read(ctx, rawRead{Tenant: tenant, Manifest: evidence.Manifest, Windows: windows, Order: "channel", EndpointOnly: true}, func(row rawObservation) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		if row.Start == nil || *row.Start >= row.At {
			return integrityUnavailable()
		}
		for i, candidate := range evidence.Candidates {
			if candidate.Channel == row.Channel && (Window{candidate.From, candidate.To}).Contains(row.At, nil) {
				if needs[i].From != nil && *row.Start < *needs[i].From {
					v := *row.Start
					needs[i].From = &v
				}
			}
		}
		count++
		if count == 1 || count%1024 == 0 || time.Since(lastCheck) >= time.Second {
			if e := recheck(needs); e != nil {
				return e
			}
			lastCheck = time.Now()
		}
		return nil
	})
	if e != nil {
		return e
	}
	evidence.Needs, _, _, e = normalizeRawNeeds(needs)
	return e
}
func saveRawEvidenceTx(ctx context.Context, tx pgx.Tx, tenant, project, actor, run string, evidence rawEvidence) (RawSnapshot, error) {
	if _, e := checkRawEvidenceTx(ctx, tx, tenant, evidence, evidence.History); e != nil {
		return RawSnapshot{}, e
	}
	evidence.Snapshot.Hash = evidence.hash()
	e := tx.QueryRow(ctx, `INSERT INTO monitoring_raw_snapshots(tenant_id,project_id,actor_id,system_run_id,schema_version,source_position,route_revision,needs,candidates,manifest,integrity,history,content_hash) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING id::text`, tenant, nullable(project), nullable(actor), nullable(run), evidence.Snapshot.Schema, mustRevision(evidence.Snapshot.SourcePosition), mustRevision(evidence.Snapshot.RouteRevision), encodeJSON(evidence.Needs), encodeJSON(evidence.Candidates), encodeJSON(evidence.Manifest), encodeJSON(evidence.Integrity), evidence.History, evidence.Snapshot.Hash).Scan(&evidence.Snapshot.ID)
	return evidence.Snapshot, e
}
func (r rawReader) FreezeUser(ctx context.Context, a access.Actor, scope Scope, in RawFreezeRequest) (RawSnapshot, error) {
	var evidence rawEvidence
	e := r.service.transact(ctx, a, scope, "details", false, false, func(tx pgx.Tx, auth authorization) error {
		if e := authorizeRawNeedsTx(ctx, tx, auth, in.Needs, in.History); e != nil {
			return e
		}
		var e error
		evidence, e = prepareRawEvidenceTx(ctx, tx, scope.Tenant, in)
		return e
	})
	if e != nil {
		return RawSnapshot{}, e
	}
	if e = r.expandPrefixes(ctx, scope.Tenant, &evidence, in.Prefixes); e != nil {
		return RawSnapshot{}, e
	}
	if e = r.expandIntervals(ctx, scope.Tenant, &evidence, func(needs []RawNeed) error {
		return r.service.transact(ctx, a, scope, "details", false, false, func(tx pgx.Tx, auth authorization) error {
			return authorizeRawNeedsTx(ctx, tx, auth, needs, in.History)
		})
	}); e != nil {
		return RawSnapshot{}, e
	}
	var out RawSnapshot
	e = r.service.transact(ctx, a, scope, "details", false, false, func(tx pgx.Tx, auth authorization) error {
		if e := authorizeRawNeedsTx(ctx, tx, auth, evidence.Needs, evidence.History); e != nil {
			return e
		}
		var e error
		out, e = saveRawEvidenceTx(ctx, tx, scope.Tenant, scope.Project, a.ID, "", evidence)
		return e
	})
	return out, e
}
func (r rawReader) systemTx(ctx context.Context, p RawSystemPermit, fn func(pgx.Tx) error) error {
	if !access.ValidID(p.Tenant) || !access.ValidID(p.RunID) || r.service.rawSystem == nil {
		return access.ErrDenied
	}
	return r.service.db(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `SELECT id FROM engineering_catalog_state WHERE id=1 FOR SHARE`); e != nil {
			return e
		}
		if e := devicecoord.Catalog(ctx, tx, false); e != nil {
			return e
		}
		if e := devicecoord.Tenant(ctx, tx, p.Tenant, false); e != nil {
			return e
		}
		var ready bool
		if e := tx.QueryRow(ctx, `SELECT mode='ready' FROM monitoring_environment_state WHERE id=1`).Scan(&ready); e != nil {
			return e
		}
		if !ready {
			return problem("monitoring_restoring", "监测环境尚未恢复就绪。", 503)
		}
		return fn(tx)
	})
}
func (r rawReader) FreezeSystem(ctx context.Context, p RawSystemPermit, in RawFreezeRequest) (RawSnapshot, error) {
	var evidence rawEvidence
	e := r.systemTx(ctx, p, func(tx pgx.Tx) error {
		needs, _, _, e := normalizeRawNeeds(in.Needs)
		if e != nil {
			return e
		}
		if e = r.service.rawSystem.AuthorizeRawSystemTx(ctx, tx, p, needs, in.History); e != nil {
			return e
		}
		evidence, e = prepareRawEvidenceTx(ctx, tx, p.Tenant, in)
		return e
	})
	if e != nil {
		return RawSnapshot{}, e
	}
	if e = r.expandPrefixes(ctx, p.Tenant, &evidence, in.Prefixes); e != nil {
		return RawSnapshot{}, e
	}
	if e = r.expandIntervals(ctx, p.Tenant, &evidence, func(needs []RawNeed) error {
		return r.systemTx(ctx, p, func(tx pgx.Tx) error { return r.service.rawSystem.AuthorizeRawSystemTx(ctx, tx, p, needs, in.History) })
	}); e != nil {
		return RawSnapshot{}, e
	}
	var out RawSnapshot
	e = r.systemTx(ctx, p, func(tx pgx.Tx) error {
		if e := r.service.rawSystem.AuthorizeRawSystemTx(ctx, tx, p, evidence.Needs, evidence.History); e != nil {
			return e
		}
		var e error
		out, e = saveRawEvidenceTx(ctx, tx, p.Tenant, "", "", p.RunID, evidence)
		return e
	})
	return out, e
}
func loadRawEvidenceTx(ctx context.Context, tx pgx.Tx, tenant, id string) (rawEvidence, error) {
	if !access.ValidID(id) {
		return rawEvidence{}, notFound()
	}
	out, e := scanRawEvidence(tx.QueryRow(ctx, rawEvidenceSelect+` WHERE tenant_id=$1 AND id=$2`, tenant, id))
	if e != nil {
		return out, e
	}
	// intervalChannels is an optimization, not serialized evidence.
	_, _, ids, e := normalizeRawNeeds(out.Candidates)
	if e != nil {
		return out, e
	}
	e = tx.QueryRow(ctx, `SELECT COALESCE(array_agg(id::text ORDER BY id),'{}') FROM engineering_channels WHERE tenant_id=$1 AND id=ANY($2::uuid[]) AND timestamp_meaning<>'observation_time'`, tenant, ids).Scan(&out.intervalChannels)
	return out, e
}

const rawEvidenceSelect = `SELECT id::text,schema_version,source_position::text,route_revision::text,needs,candidates,manifest,integrity,history,content_hash FROM monitoring_raw_snapshots`

func scanRawEvidence(row interface{ Scan(...any) error }) (rawEvidence, error) {
	var out rawEvidence
	var needs, candidates, manifest, integrity []byte
	e := row.Scan(&out.Snapshot.ID, &out.Snapshot.Schema, &out.Snapshot.SourcePosition, &out.Snapshot.RouteRevision, &needs, &candidates, &manifest, &integrity, &out.History, &out.Snapshot.Hash)
	if e == pgx.ErrNoRows {
		return out, notFound()
	}
	if e != nil {
		return out, e
	}
	if e = json.Unmarshal(needs, &out.Needs); e != nil {
		return out, e
	}
	if e = json.Unmarshal(candidates, &out.Candidates); e != nil {
		return out, e
	}
	if e = json.Unmarshal(manifest, &out.Manifest); e != nil {
		return out, e
	}
	if e = json.Unmarshal(integrity, &out.Integrity); e != nil {
		return out, e
	}
	if out.hash() != out.Snapshot.Hash {
		return out, integrityUnavailable()
	}
	return out, nil
}
func checkRawEvidenceTx(ctx context.Context, tx pgx.Tx, tenant string, evidence rawEvidence, historicalReplay bool) (RawSnapshotState, error) {
	out := RawSnapshotState{SourcePosition: evidence.Snapshot.SourcePosition}
	_, _, ids, e := normalizeRawNeeds(evidence.Needs)
	if e != nil {
		return out, e
	}
	integrity, e := rawIntegrityTx(ctx, tx, tenant, ids)
	if e != nil {
		return out, e
	}
	if digest(integrity) != digest(evidence.Integrity) {
		return out, integrityUnavailable()
	}
	channels := []string{}
	ranges := []string{}
	for _, n := range evidence.Candidates {
		channels = append(channels, n.Channel)
		ranges = append(ranges, (Window{n.From, n.To}).pg())
	}
	e = tx.QueryRow(ctx, `SELECT count(*)>0,COALESCE(bool_or(e.change_type='raw_revoke'),false),COALESCE(max(e.source_position),$2)::text FROM monitoring_changes e JOIN monitoring_change_ranges r ON r.tenant_id=e.tenant_id AND r.event_id=e.id JOIN unnest($3::uuid[],$4::int8range[]) AS n(channel_id,affected) ON n.channel_id=r.channel_id AND n.affected&&r.affected WHERE e.tenant_id=$1 AND e.source_position>$2`, tenant, mustRevision(evidence.Snapshot.SourcePosition), channels, ranges).Scan(&out.Changed, &out.Revoked, &out.SourcePosition)
	if e != nil {
		return out, e
	}
	if out.Revoked && !historicalReplay {
		return out, problem("snapshot_stale", "固定输入范围的采用已被撤销，需要重新建立输入快照。", 409)
	}
	return out, nil
}
func (r rawReader) userEvidence(ctx context.Context, a access.Actor, scope Scope, id string, replay bool) (rawEvidence, RawSnapshotState, error) {
	var evidence rawEvidence
	var state RawSnapshotState
	e := r.service.transact(ctx, a, scope, "details", false, false, func(tx pgx.Tx, auth authorization) error {
		var e error
		evidence, e = loadRawEvidenceTx(ctx, tx, scope.Tenant, id)
		if e != nil {
			return e
		}
		if e = authorizeRawNeedsTx(ctx, tx, auth, evidence.Needs, replay || evidence.History); e != nil {
			return e
		}
		state, e = checkRawEvidenceTx(ctx, tx, scope.Tenant, evidence, replay)
		return e
	})
	return evidence, state, e
}
func (r rawReader) systemEvidence(ctx context.Context, p RawSystemPermit, id string, replay bool) (rawEvidence, RawSnapshotState, error) {
	var evidence rawEvidence
	var state RawSnapshotState
	e := r.systemTx(ctx, p, func(tx pgx.Tx) error {
		var own bool
		if e := tx.QueryRow(ctx, `SELECT system_run_id=$3::uuid AND actor_id IS NULL AND project_id IS NULL FROM monitoring_raw_snapshots WHERE tenant_id=$1 AND id=$2`, p.Tenant, id, p.RunID).Scan(&own); e != nil {
			return e
		}
		if !own {
			return access.ErrDenied
		}
		var e error
		evidence, e = loadRawEvidenceTx(ctx, tx, p.Tenant, id)
		if e != nil {
			return e
		}
		if e = r.service.rawSystem.AuthorizeRawSystemTx(ctx, tx, p, evidence.Needs, replay || evidence.History); e != nil {
			return e
		}
		state, e = checkRawEvidenceTx(ctx, tx, p.Tenant, evidence, replay)
		return e
	})
	return evidence, state, e
}
func (r rawReader) CheckUser(ctx context.Context, a access.Actor, scope Scope, id string, replay bool) (RawSnapshotState, error) {
	_, state, e := r.userEvidence(ctx, a, scope, id, replay)
	return state, e
}
func (r rawReader) CheckSystem(ctx context.Context, p RawSystemPermit, id string, replay bool) (RawSnapshotState, error) {
	_, state, e := r.systemEvidence(ctx, p, id, replay)
	return state, e
}
func (r rawReader) read(ctx context.Context, tenant string, evidence rawEvidence, budget RawReadBudget, check func() error, consume func(RawSample) error) (RawReadCount, error) {
	count := RawReadCount{}
	if e := r.service.requireStorage(); e != nil {
		return count, e
	}
	if budget.Rows == 0 || budget.Bytes == 0 || consume == nil {
		return count, invalid("budget", "固定快照读取必须声明非零行数及字节预算。")
	}
	_, windows, _, e := normalizeRawNeeds(evidence.Candidates)
	if e != nil {
		return count, e
	}
	lastCheck := time.Now()
	e = r.service.storage.read(ctx, rawRead{Tenant: tenant, Manifest: evidence.Manifest, Windows: windows, Order: "channel", EndpointOnly: true}, func(row rawObservation) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		if count.Rows%1024 == 0 || time.Since(lastCheck) >= time.Second {
			if e := check(); e != nil {
				return e
			}
			lastCheck = time.Now()
		}
		sample := RawSample{row.Channel, row.Observation, row.Adoption, row.Batch, row.At, row.Start, row.Kind, row.Value, row.Quality, row.QualityReason}
		size := uint64(len(encodeJSON(sample)))
		if count.Rows >= budget.Rows || size > budget.Bytes-count.Bytes {
			return problem("budget_exceeded", "输入超过当前任务读取预算，需要完整后台运行或调整配置。", 422)
		}
		count.Rows++
		count.Bytes += size
		return consume(sample)
	})
	if e != nil {
		return count, e
	}
	return count, check()
}
func (r rawReader) ReadUser(ctx context.Context, a access.Actor, scope Scope, id string, budget RawReadBudget, consume func(RawSample) error) (RawReadCount, error) {
	evidence, _, e := r.userEvidence(ctx, a, scope, id, false)
	if e != nil {
		return RawReadCount{}, e
	}
	return r.read(ctx, scope.Tenant, evidence, budget, func() error { _, e := r.checkKnownUser(ctx, a, scope, evidence, false); return e }, consume)
}
func (r rawReader) ReadSystem(ctx context.Context, p RawSystemPermit, id string, budget RawReadBudget, consume func(RawSample) error) (RawReadCount, error) {
	evidence, _, e := r.systemEvidence(ctx, p, id, false)
	if e != nil {
		return RawReadCount{}, e
	}
	return r.read(ctx, p.Tenant, evidence, budget, func() error { _, e := r.checkKnownSystem(ctx, p, evidence, false); return e }, consume)
}

func selectRawPart(evidence rawEvidence, needs []RawNeed) (rawEvidence, error) {
	selected, windows, _, e := normalizeRawNeeds(needs)
	if e != nil {
		return evidence, e
	}
	_, full, _, e := normalizeRawNeeds(evidence.Candidates)
	if e != nil {
		return evidence, e
	}
	for id, parts := range windows {
		for _, part := range parts {
			covered := false
			for _, allowed := range full[id] {
				if allowed.Covers(part) {
					covered = true
					break
				}
			}
			if !covered {
				return evidence, access.ErrDenied
			}
		}
	}
	evidence.Candidates = selected
	evidence.Needs = slices.Clone(selected)
	// loadRawEvidenceTx already verified the complete immutable manifest hash.
	// A qualified part must compare integrity revisions for precisely its own
	// channels; retaining the full map would reject a valid one-channel slice
	// from a snapshot that also contains unrelated channels.
	partIntegrity := make(map[string]string, len(windows))
	for channel := range windows {
		revision, exists := evidence.Integrity[channel]
		if !exists {
			return evidence, integrityUnavailable()
		}
		partIntegrity[channel] = revision
	}
	evidence.Integrity = partIntegrity
	return evidence, nil
}

// Part readers retain one immutable run snapshot and its full qualification,
// while reading a complete, explicitly bounded subrange (including its halo).
func (r rawReader) ReadPartUser(ctx context.Context, a access.Actor, scope Scope, id string, needs []RawNeed, budget RawReadBudget, consume func(RawSample) error) (RawReadCount, error) {
	original, _, e := r.userEvidence(ctx, a, scope, id, false)
	if e != nil {
		return RawReadCount{}, e
	}
	part, e := selectRawPart(original, needs)
	if e != nil {
		return RawReadCount{}, e
	}
	return r.read(ctx, scope.Tenant, part, budget, func() error { _, e := r.checkKnownUser(ctx, a, scope, original, false); return e }, consume)
}
func (r rawReader) ReadPartSystem(ctx context.Context, p RawSystemPermit, id string, needs []RawNeed, budget RawReadBudget, consume func(RawSample) error) (RawReadCount, error) {
	original, _, e := r.systemEvidence(ctx, p, id, false)
	if e != nil {
		return RawReadCount{}, e
	}
	part, e := selectRawPart(original, needs)
	if e != nil {
		return RawReadCount{}, e
	}
	return r.read(ctx, p.Tenant, part, budget, func() error { _, e := r.checkKnownSystem(ctx, p, original, false); return e }, consume)
}

// Replay reads the original adoption manifest, including subsequently revoked
// evidence, only after the separate history permission/mandate check.
func (r rawReader) ReplayUser(ctx context.Context, a access.Actor, scope Scope, id string, budget RawReadBudget, consume func(RawSample) error) (RawReadCount, error) {
	evidence, _, e := r.userEvidence(ctx, a, scope, id, true)
	if e != nil {
		return RawReadCount{}, e
	}
	return r.read(ctx, scope.Tenant, evidence, budget, func() error { _, e := r.checkKnownUser(ctx, a, scope, evidence, true); return e }, consume)
}

// ReplayPartUser checks only the requested historical dependency interval,
// including complete source intervals. Unrelated snapshot ranges confer no
// authority and do not make an otherwise authorized partial replay fail.
func (r rawReader) ReplayPartUser(ctx context.Context, a access.Actor, scope Scope, id string, needs []RawNeed, budget RawReadBudget, consume func(RawSample) error) (RawReadCount, error) {
	return r.ReadSelectedPartUser(ctx, a, scope, id, needs, true, budget, consume)
}

// ReadSelectedPartUser independently qualifies a published result's selected
// inputs, including any expanded observation intervals. Revoked evidence also
// requires history permission.
func (r rawReader) ReadSelectedPartUser(ctx context.Context, a access.Actor, scope Scope, id string, needs []RawNeed, history bool, budget RawReadBudget, consume func(RawSample) error) (RawReadCount, error) {
	var original rawEvidence
	e := r.service.transact(ctx, a, scope, "details", false, false, func(tx pgx.Tx, auth authorization) error {
		if e := authorizeRawNeedsTx(ctx, tx, auth, needs, history); e != nil {
			return e
		}
		var e error
		original, e = loadRawEvidenceTx(ctx, tx, scope.Tenant, id)
		return e
	})
	if e != nil {
		return RawReadCount{}, e
	}
	part, e := selectRawPart(original, needs)
	if e != nil {
		return RawReadCount{}, e
	}
	part.Needs = slices.Clone(part.Candidates)
	if e = r.expandIntervals(ctx, scope.Tenant, &part, func(expanded []RawNeed) error {
		return r.service.transact(ctx, a, scope, "details", false, false, func(tx pgx.Tx, auth authorization) error {
			return authorizeRawNeedsTx(ctx, tx, auth, expanded, history || part.History)
		})
	}); e != nil {
		return RawReadCount{}, e
	}
	check := func() error { _, e := r.checkKnownUser(ctx, a, scope, part, history); return e }
	if e = check(); e != nil {
		return RawReadCount{}, e
	}
	return r.read(ctx, scope.Tenant, part, budget, check, consume)
}
func (r rawReader) ReplaySystem(ctx context.Context, p RawSystemPermit, id string, budget RawReadBudget, consume func(RawSample) error) (RawReadCount, error) {
	evidence, _, e := r.systemEvidence(ctx, p, id, true)
	if e != nil {
		return RawReadCount{}, e
	}
	return r.read(ctx, p.Tenant, evidence, budget, func() error { _, e := r.checkKnownSystem(ctx, p, evidence, true); return e }, consume)
}

// Known evidence was loaded and hashed once. Revalidation checks mutable
// authority and changes without repeatedly decoding the immutable manifest.
func (r rawReader) checkKnownUser(ctx context.Context, a access.Actor, scope Scope, evidence rawEvidence, replay bool) (RawSnapshotState, error) {
	var state RawSnapshotState
	e := r.service.transact(ctx, a, scope, "details", false, false, func(tx pgx.Tx, auth authorization) error {
		if e := authorizeRawNeedsTx(ctx, tx, auth, evidence.Needs, replay || evidence.History); e != nil {
			return e
		}
		var e error
		state, e = checkRawEvidenceTx(ctx, tx, scope.Tenant, evidence, replay)
		return e
	})
	return state, e
}
func (r rawReader) checkKnownSystem(ctx context.Context, p RawSystemPermit, evidence rawEvidence, replay bool) (RawSnapshotState, error) {
	var state RawSnapshotState
	e := r.systemTx(ctx, p, func(tx pgx.Tx) error {
		if e := r.service.rawSystem.AuthorizeRawSystemTx(ctx, tx, p, evidence.Needs, replay || evidence.History); e != nil {
			return e
		}
		var e error
		state, e = checkRawEvidenceTx(ctx, tx, p.Tenant, evidence, replay)
		return e
	})
	return state, e
}

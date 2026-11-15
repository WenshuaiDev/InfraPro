package monitoring

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

type SelectionInput struct {
	All      bool     `json:"all_workspace"`
	Channels []string `json:"channel_ids"`
	Points   []string `json:"point_ids"`
}
type Selection struct {
	ID        string    `json:"id"`
	Hash      string    `json:"hash"`
	Count     string    `json:"channel_count"`
	ExpiresAt time.Time `json:"expires_at"`
}
type QueryInput struct {
	Selection  string   `json:"selection_id"`
	Snapshot   string   `json:"snapshot_id"`
	Range      *Range   `json:"range"`
	ViewRange  *Range   `json:"view_range,omitempty"`
	AllHistory bool     `json:"all_history"`
	Qualities  []string `json:"qualities"`
	Timezone   string   `json:"timezone"`
	Status     string   `json:"status"`
	ValueKind  string   `json:"value_kind"`
	Cursor     string   `json:"cursor"`
	Limit      int      `json:"limit"`
	Direction  string   `json:"direction"`
	PlotWidth  int      `json:"plot_width_px"`
	PlotBudget int      `json:"plot_point_budget,omitempty"`
	Period     string   `json:"period"`
	Digits     *int     `json:"digits"`
}
type Snapshot struct {
	ID            string    `json:"id"`
	Watermark     string    `json:"watermark"`
	RouteRevision string    `json:"route_revision"`
	ExpiresAt     time.Time `json:"expires_at"`
	QueryHash     string    `json:"query_hash"`
}
type Observation struct {
	SourceKind    string                 `json:"source_kind"`
	Calculated    *CalculatedObservation `json:"-"`
	ID            string                 `json:"observation_id"`
	Adoption      string                 `json:"adoption_id"`
	Channel       string                 `json:"channel_id"`
	Batch         string                 `json:"batch_id"`
	At            string                 `json:"observed_at"`
	Start         *string                `json:"interval_start"`
	Kind          string                 `json:"value_kind"`
	Value         string                 `json:"value"`
	Quality       string                 `json:"quality"`
	QualityReason string                 `json:"quality_reason"`
	Revoked       bool                   `json:"revoked"`
}
type DetailPage struct {
	Items      []Observation         `json:"items"`
	Snapshot   Snapshot              `json:"snapshot"`
	HasMore    bool                  `json:"has_more"`
	NextCursor string                `json:"next_cursor"`
	PrevCursor string                `json:"prev_cursor"`
	Coverage   []CalculationCoverage `json:"coverage,omitempty"`
}
type snapshotVector struct {
	Integrity  map[string]string          `json:"integrity"`
	Revokes    map[string]string          `json:"revokes"`
	Contents   map[string]string          `json:"contents"`
	Secret     string                     `json:"cursor_secret"`
	Calculated map[string]CalculationHead `json:"calculated,omitempty"`
}
type queryPlan struct {
	snapshot      Snapshot
	vector        snapshotVector
	manifest      []selectedChunk
	windows       map[string][]Window
	channels      []string
	authorization string
	filter        QueryInput
}

func (s *Service) Select(ctx context.Context, a access.Actor, scope Scope, in SelectionInput) (Selection, error) {
	out := Selection{}
	if (in.All && (len(in.Channels) > 0 || len(in.Points) > 0)) || (!in.All && len(in.Channels) == 0 && len(in.Points) == 0) || len(in.Channels) > 50000 || len(in.Points) > 50000 {
		return out, invalid("selection", "请选择全工作空间或明确的点／通道。")
	}
	for _, id := range append(slices.Clone(in.Channels), in.Points...) {
		if !access.ValidID(id) {
			return out, notFound()
		}
	}
	e := s.transact(ctx, a, scope, "selections.create", false, false, func(tx pgx.Tx, auth authorization) error {
		ids := append([]string{}, in.Channels...)
		if in.All || len(in.Points) > 0 {
			rows, e := tx.Query(ctx, `SELECT c.id::text FROM engineering_channels c WHERE c.tenant_id=$1 AND c.deleted_at IS NULL AND ($2 OR c.point_id=ANY($3::uuid[])) AND ($4='' OR EXISTS(SELECT 1 FROM monitoring_data_grants d JOIN project_resource_grants g ON g.id=d.resource_grant_id JOIN project_facility_links l ON l.id=g.link_id WHERE d.tenant_id=c.tenant_id AND d.channel_id=c.id AND d.project_id::text=$4 AND d.revoked_at IS NULL AND 'read'=ANY(d.capabilities) AND g.ended_at IS NULL AND l.status='active')) ORDER BY c.id LIMIT 50001`, scope.Tenant, in.All, in.Points, scope.Project)
			if e != nil {
				return e
			}
			for rows.Next() {
				var id string
				if e = rows.Scan(&id); e != nil {
					rows.Close()
					return e
				}
				ids = append(ids, id)
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				return e
			}
		}
		slices.Sort(ids)
		ids = slices.Compact(ids)
		if len(ids) > 50000 {
			return invalid("selection", "选择超过 50,000 通道，请缩小范围。")
		}
		if len(ids) > 0 {
			if _, e := authorizedWindowsTx(ctx, tx, auth, ids, "read"); e != nil {
				return e
			}
		}
		out.Hash = digest(ids)
		out.Count = fmt.Sprint(len(ids))
		return tx.QueryRow(ctx, `INSERT INTO monitoring_selections(tenant_id,project_id,actor_id,display_subject_id,channel_ids,selection_hash,authorization_revision,expires_at) VALUES($1,$2,$3,$7,$4,$5,$6,clock_timestamp()+interval '15 minutes') RETURNING id::text,expires_at`, scope.Tenant, nullable(scope.Project), a.OwnerAccount(), ids, out.Hash, authorizationVersion(auth), a.OwnerSubject()).Scan(&out.ID, &out.ExpiresAt)
	})
	return out, e
}
func normalizedQuery(in QueryInput) (QueryInput, error) {
	if !access.ValidID(in.Selection) || in.Snapshot != "" && !access.ValidID(in.Snapshot) {
		return in, invalid("selection_id", "需要有效选择和快照。")
	}
	if in.AllHistory {
		if in.Range != nil {
			return in, invalid("range", "全部历史和有限范围不能同时选择。")
		}
	} else {
		if in.Range == nil || (in.Range.From == nil && in.Range.To == nil) {
			return in, invalid("range", "范围查询至少需要一个明确端点；完全无界请使用全部历史。")
		}
		if _, e := in.Range.window(); e != nil {
			return in, e
		}
	}
	// A view may narrow a fixed snapshot's base range. It never widens that
	// range or its authorization; stable snapshots can serve adjacent tiles.
	if in.ViewRange != nil {
		view, e := in.ViewRange.window()
		if e != nil {
			return in, e
		}
		if in.ViewRange.From == nil && in.ViewRange.To == nil {
			return in, invalid("view_range", "视窗至少需要一个端点。")
		}
		if !in.AllHistory {
			base, _ := in.Range.window()
			if !base.Covers(view) {
				return in, invalid("view_range", "视窗必须位于快照的固定范围内。")
			}
		}
	}
	if in.PlotBudget != 0 && (in.PlotBudget < 4 || in.PlotBudget > 2000) {
		return in, invalid("plot_point_budget", "每曲线绘图预算应为 4–2000 点。")
	}
	if len(in.Qualities) == 0 {
		in.Qualities = []string{"good", "unmarked"}
	}
	slices.Sort(in.Qualities)
	in.Qualities = slices.Compact(in.Qualities)
	for _, q := range in.Qualities {
		if !slices.Contains([]string{"good", "unmarked", "suspect", "bad"}, q) {
			return in, invalid("qualities", "无效质量筛选。")
		}
	}
	if in.Status == "" {
		in.Status = "active"
	}
	if in.Status != "active" && in.Status != "history" {
		return in, invalid("status", "请选择当前在用或采用历史。")
	}
	if in.Timezone == "" {
		in.Timezone = "UTC"
	}
	if _, e := time.LoadLocation(in.Timezone); e != nil {
		return in, invalid("timezone", "无效 IANA 时区。")
	}
	if in.ValueKind != "" && !slices.Contains([]string{"integer", "decimal", "enum"}, in.ValueKind) {
		return in, invalid("value_kind", "无效值类型。")
	}
	if in.Limit == 0 {
		in.Limit = 200
	}
	if in.Limit < 1 || in.Limit > 1000 {
		return in, invalid("limit", "每页应为 1–1000 条。")
	}
	if in.Direction == "" {
		in.Direction = "forward"
	}
	if in.Direction != "forward" && in.Direction != "backward" {
		return in, invalid("direction", "无效翻页方向。")
	}
	return in, nil
}
func filterHash(in QueryInput) string {
	return digest([]any{in.Selection, in.Range, in.AllHistory, in.Qualities, in.Timezone, in.Status, in.ValueKind})
}
func (s *Service) queryPlan(ctx context.Context, a access.Actor, scope Scope, in QueryInput, action, capability string, leased ...bool) (queryPlan, error) {
	plan := queryPlan{filter: in}
	useLease := len(leased) > 0 && leased[0]
	if e := s.requireStorage(); e != nil {
		return plan, e
	}
	e := s.transact(ctx, a, scope, action, false, false, func(tx pgx.Tx, auth authorization) error {
		var selectionHash, selectionAuth string
		var expired bool
		e := tx.QueryRow(ctx, `SELECT channel_ids::text[],selection_hash,authorization_revision,expires_at<=clock_timestamp() FROM monitoring_selections WHERE tenant_id=$1 AND id=$2 AND COALESCE(actor_id,display_subject_id)=$3 AND project_id IS NOT DISTINCT FROM $4::uuid`, scope.Tenant, in.Selection, a.ID, nullable(scope.Project)).Scan(&plan.channels, &selectionHash, &selectionAuth, &expired)
		if e == pgx.ErrNoRows {
			return notFound()
		}
		if e != nil {
			return e
		}
		plan.authorization = authorizationVersion(auth)
		if selectionAuth != plan.authorization {
			return problem("selection_stale", "授权已变化，请重新选择通道。", 409)
		}
		if expired && in.Snapshot == "" {
			return problem("selection_expired", "选择已过期，请重新选择。", 409)
		}
		plan.windows = map[string][]Window{}
		if len(plan.channels) > 0 {
			plan.windows, e = authorizedWindowsTx(ctx, tx, auth, plan.channels, capability)
			if e != nil {
				return e
			}
			if capability == "export" {
				read, e := authorizedWindowsTx(ctx, tx, auth, plan.channels, "read")
				if e != nil {
					return e
				}
				plan.windows = intersectWindows(plan.windows, read)
			}
			if in.Status == "history" {
				history, e := authorizedWindowsTx(ctx, tx, auth, plan.channels, "history")
				if e != nil {
					return e
				}
				plan.windows = intersectWindows(plan.windows, history)
			}
			if in.ValueKind != "" {
				rows, e := tx.Query(ctx, `SELECT c.id::text,CASE WHEN c.source_kind='derived' THEN 'decimal' ELSE v.content->'metric'->>'shape' END FROM engineering_channels c LEFT JOIN engineering_versions v ON v.id=c.metric_version_id WHERE c.tenant_id=$1 AND c.id=ANY($2::uuid[]) AND c.deleted_at IS NULL`, scope.Tenant, plan.channels)
				if e != nil {
					return e
				}
				for rows.Next() {
					var id, kind string
					if e = rows.Scan(&id, &kind); e != nil {
						rows.Close()
						return e
					}
					if kind != in.ValueKind {
						delete(plan.windows, id)
					}
				}
				e = rows.Err()
				rows.Close()
				if e != nil {
					return e
				}
			}

		}
		current := map[string]string{}
		contents := map[string]string{}
		integrity := map[string]string{}
		rows, e := tx.Query(ctx, `SELECT c.id::text,COALESCE(v.revoke_revision,0)::text,COALESCE(v.content_revision,0)::text,COALESCE(v.integrity_revision,0)::text,COALESCE(v.quarantined,false) FROM engineering_channels c LEFT JOIN monitoring_channel_versions v ON v.tenant_id=c.tenant_id AND v.channel_id=c.id WHERE c.tenant_id=$1 AND c.id=ANY($2::uuid[]) ORDER BY c.id`, scope.Tenant, plan.channels)
		if e != nil {
			return e
		}
		for rows.Next() {
			var id, rev, content, repair string
			var quarantined bool
			if e = rows.Scan(&id, &rev, &content, &repair, &quarantined); e != nil {
				rows.Close()
				return e
			}
			if quarantined {
				rows.Close()
				return integrityUnavailable()
			}
			integrity[id] = repair
			current[id] = rev
			contents[id] = content
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if in.Snapshot == "" {
			seq, route := int64(0), int64(1)
			e = tx.QueryRow(ctx, `SELECT commit_sequence,route_revision FROM monitoring_tenant_states WHERE tenant_id=$1`, scope.Tenant).Scan(&seq, &route)
			if e != nil && e != pgx.ErrNoRows {
				return e
			}
			secret := make([]byte, 32)
			if _, e = rand.Read(secret); e != nil {
				return e
			}
			plan.vector = snapshotVector{Integrity: integrity, Revokes: current, Contents: contents, Secret: base64.RawURLEncoding.EncodeToString(secret)}
			if s.calculationReader != nil {
				plan.vector.Calculated, e = s.calculationReader.FreezeCalculatedTx(ctx, tx, scope.Tenant, plan.channels)
				if e != nil {
					return e
				}
			}
			plan.snapshot.QueryHash = filterHash(in)
			e = tx.QueryRow(ctx, `INSERT INTO monitoring_snapshots(tenant_id,selection_id,actor_id,display_subject_id,watermark,route_revision,query_hash,revision_vector,authorization_revision,expires_at) VALUES($1,$2,$3,$9,$4,$5,$6,$7,$8,clock_timestamp()+interval '15 minutes') RETURNING id::text,watermark::text,route_revision::text,expires_at`, scope.Tenant, in.Selection, a.OwnerAccount(), seq, route, plan.snapshot.QueryHash, encodeJSON(plan.vector), plan.authorization, a.OwnerSubject()).Scan(&plan.snapshot.ID, &plan.snapshot.Watermark, &plan.snapshot.RouteRevision, &plan.snapshot.ExpiresAt)
			if e != nil {
				return e
			}
		} else {
			var vector []byte
			var authRevision string
			e = tx.QueryRow(ctx, `SELECT id::text,watermark::text,route_revision::text,expires_at,query_hash,revision_vector,authorization_revision,(expires_at<=clock_timestamp() AND (NOT $5::bool OR lease_until IS NULL OR lease_until<=clock_timestamp())) FROM monitoring_snapshots WHERE tenant_id=$1 AND id=$2 AND selection_id=$3 AND COALESCE(actor_id,display_subject_id)=$4`, scope.Tenant, in.Snapshot, in.Selection, a.ID, useLease).Scan(&plan.snapshot.ID, &plan.snapshot.Watermark, &plan.snapshot.RouteRevision, &plan.snapshot.ExpiresAt, &plan.snapshot.QueryHash, &vector, &authRevision, &expired)
			if e == pgx.ErrNoRows {
				return notFound()
			}
			if e != nil {
				return e
			}
			if expired {
				return problem("snapshot_expired", "快照已过期，请刷新。", 409)
			}
			if e = json.Unmarshal(vector, &plan.vector); e != nil {
				return e
			}
			if authRevision != plan.authorization || digest(current) != digest(plan.vector.Revokes) || digest(integrity) != digest(plan.vector.Integrity) {
				return problem("snapshot_stale", "授权或相关数据撤销状态已变化，请刷新。", 409)
			}
			if plan.snapshot.QueryHash != filterHash(in) {
				return problem("snapshot_query_changed", "快照不能用于不同筛选条件。", 409)
			}
		}
		watermark, _ := revisionNumber(plan.snapshot.Watermark)
		if watermark == 0 || action != "details" {
			plan.manifest = []selectedChunk{}
			return nil
		}
		plan.manifest, e = manifestTx(ctx, tx, scope.Tenant, plan.windows, watermark, in.Status, mustRevision(plan.snapshot.RouteRevision))
		return e
	})
	return plan, e
}
func intersectWindows(a, b map[string][]Window) map[string][]Window {
	out := map[string][]Window{}
	for id, ws := range a {
		for _, x := range ws {
			for _, y := range b[id] {
				if w, ok := x.Intersect(y); ok {
					out[id] = append(out[id], w)
				}
			}
		}
		out[id] = UnionWindows(out[id])
	}
	return out
}

type detailCursor struct {
	Snapshot, Direction, Channel, Observation, Adoption, View string
	At                                                        int64 `json:",string"`
}

func signCursor(plan queryPlan, r rawObservation, direction string) string {
	raw := encodeJSON(detailCursor{plan.snapshot.ID, direction, r.Channel, r.Observation, r.Adoption, digest(queryWindow(plan.filter)), r.At})
	h := hmac.New(sha256.New, []byte(plan.vector.Secret))
	h.Write(raw)
	return base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
func decodeCursor(plan queryPlan, raw, direction string) (*rawObservation, error) {
	if raw == "" {
		return nil, nil
	}
	if len(raw) > 2048 {
		return nil, invalid("cursor", "无效游标。")
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 2 {
		return nil, invalid("cursor", "无效游标。")
	}
	payload, e := base64.RawURLEncoding.DecodeString(parts[0])
	if e != nil {
		return nil, invalid("cursor", "无效游标。")
	}
	signature, e := base64.RawURLEncoding.DecodeString(parts[1])
	if e != nil {
		return nil, invalid("cursor", "无效游标。")
	}
	h := hmac.New(sha256.New, []byte(plan.vector.Secret))
	h.Write(payload)
	if !hmac.Equal(signature, h.Sum(nil)) {
		return nil, invalid("cursor", "游标签名无效。")
	}
	var value detailCursor
	if e = json.Unmarshal(payload, &value); e != nil || value.Snapshot != plan.snapshot.ID || value.Direction != direction || value.View != digest(queryWindow(plan.filter)) || !access.ValidID(value.Channel) || !access.ValidID(value.Observation) || !access.ValidID(value.Adoption) {
		return nil, invalid("cursor", "游标与快照或方向不匹配。")
	}
	return &rawObservation{At: value.At, Channel: value.Channel, Observation: value.Observation, Adoption: value.Adoption}, nil
}
func observationDTO(r rawObservation, revoked bool) Observation {
	out := Observation{ID: r.Observation, Adoption: r.Adoption, Channel: r.Channel, Batch: r.Batch, At: time.Unix(0, r.At).UTC().Format(time.RFC3339Nano), Kind: r.Kind, Value: r.Value, Quality: r.Quality, QualityReason: r.QualityReason, Revoked: revoked}
	if r.Start != nil {
		value := time.Unix(0, *r.Start).UTC().Format(time.RFC3339Nano)
		out.Start = &value
	}
	return out
}

func (s *Service) Details(ctx context.Context, a access.Actor, scope Scope, input QueryInput) (DetailPage, error) {
	out := DetailPage{Items: []Observation{}}
	in, e := normalizedQuery(input)
	if e != nil {
		return out, e
	}
	plan, e := s.queryPlan(ctx, a, scope, in, "details", "read")
	if e != nil {
		return out, e
	}
	out.Snapshot = plan.snapshot
	boundary, e := decodeCursor(plan, in.Cursor, in.Direction)
	if e != nil {
		return out, e
	}
	raw := []rawObservation{}
	e = s.storage.read(ctx, rawRead{Manifest: plan.manifest, Tenant: scope.Tenant, Windows: plan.windows, Qualities: in.Qualities, Range: queryWindow(in), Limit: in.Limit + 1, Boundary: boundary, Reverse: in.Direction == "forward"}, func(r rawObservation) error { raw = append(raw, r); return nil })
	if e != nil {
		return out, storageError(e)
	}
	type displayRow struct {
		raw        rawObservation
		calculated *CalculatedObservation
	}
	display := make([]displayRow, 0, len(raw))
	for _, r := range raw {
		display = append(display, displayRow{raw: r})
	}
	if len(plan.vector.Calculated) > 0 {
		if s.calculationReader == nil {
			return out, problem("calculation_unavailable", "派生结果读取服务尚未配置。", 503)
		}
		var after *CalculationBoundary
		if boundary != nil {
			after = &CalculationBoundary{At: boundary.At, Channel: boundary.Channel, Result: boundary.Observation, Generation: boundary.Adoption}
		}
		calculated, e := s.calculationReader.ReadCalculated(ctx, a, scope, CalculationQuery{Heads: plan.vector.Calculated, Windows: plan.windows, Range: queryWindow(in), Qualities: in.Qualities, Boundary: after, Limit: in.Limit + 1, Reverse: in.Direction == "forward", History: in.Status == "history"})
		if e != nil {
			return DetailPage{Items: []Observation{}}, e
		}
		out.Coverage = calculated.Coverage
		for _, point := range calculated.Items {
			display = append(display, displayRow{raw: rawObservation{At: point.AtNS, Channel: point.Channel, Observation: point.Result, Adoption: point.Generation}, calculated: &point})
		}
		slices.SortFunc(display, func(a, b displayRow) int {
			v := compareRead(a.raw, b.raw, "")
			if in.Direction == "forward" {
				return -v
			}
			return v
		})
	}
	raw = nil
	calculatedByID := map[string]*CalculatedObservation{}
	for _, row := range display {
		raw = append(raw, row.raw)
		if row.calculated != nil {
			calculatedByID[row.raw.Observation+":"+row.raw.Adoption] = row.calculated
		}
	}
	out.HasMore = len(raw) > in.Limit
	if out.HasMore {
		raw = raw[:in.Limit]
	}
	if in.Direction == "backward" {
		slices.Reverse(raw)
	}
	revoked := map[string]bool{}
	for _, entry := range plan.manifest {
		revoked[entry.Batch] = entry.Revoked
	}
	for _, r := range raw {
		item := observationDTO(r, revoked[r.Batch])
		item.Calculated = calculatedByID[r.Observation+":"+r.Adoption]
		if item.Calculated != nil {
			item.SourceKind = "derived"
		}
		out.Items = append(out.Items, item)
	}
	if len(raw) > 0 {
		out.NextCursor = signCursor(plan, raw[len(raw)-1], "forward")
		out.PrevCursor = signCursor(plan, raw[0], "backward")
	}
	in.Snapshot = plan.snapshot.ID
	_, e = s.queryPlan(ctx, a, scope, in, "details", "read")
	if e != nil {
		return DetailPage{Items: []Observation{}}, e
	}
	return out, nil
}

func (s *Service) RenewSnapshot(ctx context.Context, a access.Actor, scope Scope, id string, in QueryInput) (Snapshot, error) {
	in.Snapshot = id
	normalized, e := normalizedQuery(in)
	if e != nil {
		return Snapshot{}, e
	}
	plan, e := s.queryPlan(ctx, a, scope, normalized, "snapshots.renew", "read")
	if e != nil {
		return Snapshot{}, e
	}
	e = s.transact(ctx, a, scope, "snapshots.renew", false, false, func(tx pgx.Tx, auth authorization) error {
		if authorizationVersion(auth) != plan.authorization {
			return revisionChanged()
		}
		return tx.QueryRow(ctx, `UPDATE monitoring_snapshots SET expires_at=clock_timestamp()+interval '15 minutes' WHERE tenant_id=$1 AND id=$2 AND COALESCE(actor_id,display_subject_id)=$3 RETURNING expires_at`, scope.Tenant, id, a.ID).Scan(&plan.snapshot.ExpiresAt)
	})
	return plan.snapshot, e
}

func queryWindow(in QueryInput) *Window {
	if in.ViewRange != nil {
		w, _ := in.ViewRange.window()
		return &w
	}
	if in.AllHistory || in.Range == nil {
		return nil
	}
	w, _ := in.Range.window()
	return &w
}

// CheckSelectionTargets prevents a caller from reusing a larger selection when
// narrowing a named display binding. Ownership alone is not a scope proof.
func (s *Service) CheckSelectionTargets(ctx context.Context, a access.Actor, scope Scope, id string, ids []string) error {
	if !access.ValidID(id) {
		return notFound()
	}
	return s.transact(ctx, a, scope, "selections.create", false, false, func(tx pgx.Tx, _ authorization) error {
		var selected []string
		e := tx.QueryRow(ctx, `SELECT channel_ids::text[] FROM monitoring_selections WHERE tenant_id=$1 AND project_id IS NOT DISTINCT FROM $2::uuid AND id=$3 AND COALESCE(actor_id,display_subject_id)=$4 AND expires_at>clock_timestamp()`, scope.Tenant, nullable(scope.Project), id, a.ID).Scan(&selected)
		if e == pgx.ErrNoRows {
			return notFound()
		}
		if e != nil {
			return e
		}
		selected = slices.Clone(selected)
		ids = slices.Clone(ids)
		slices.Sort(selected)
		slices.Sort(ids)
		if !slices.Equal(selected, ids) {
			return notFound()
		}
		return nil
	})
}

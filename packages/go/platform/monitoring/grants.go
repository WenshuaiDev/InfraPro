package monitoring

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

var capabilities = []string{"read", "export", "import", "revoke", "history", "source"}

func projectState(ctx context.Context, tx pgx.Tx, tenant, project string, write bool) (string, string, error) {
	if !access.ValidID(project) {
		return "", "", notFound()
	}
	var status, revision string
	e := tx.QueryRow(ctx, `SELECT p.status,COALESCE(m.revision,0)::text FROM projects p LEFT JOIN monitoring_access_states m ON m.tenant_id=p.tenant_id AND m.project_id=p.id WHERE p.tenant_id=$1 AND p.id=$2`, tenant, project).Scan(&status, &revision)
	if e == pgx.ErrNoRows {
		return "", "", notFound()
	}
	if e == nil && write && status != "active" {
		return "", "", problem("project_archived", "项目已归档，不能变更数据授权。", 409)
	}
	return status, revision, e
}

func receiptTx(ctx context.Context, tx pgx.Tx, auth authorization, operation string, request string, payload any, out any) (bool, error) {
	var hash string
	var result []byte
	e := tx.QueryRow(ctx, `SELECT payload_hash,result FROM monitoring_receipts WHERE scope_kind=$1 AND tenant_id=$2 AND scope_id=$3 AND actor_id=$4 AND operation=$5 AND request_id=$6`, auth.Entry, auth.Tenant, auth.scopeID(), auth.Account, operation, request).Scan(&hash, &result)
	if e == pgx.ErrNoRows {
		return false, auditBeforeCommandTx(ctx, tx, auth, operation)
	}
	if e != nil {
		return false, e
	}
	if hash != digest(payload) {
		return false, problem("request_id_reused", "同一请求标识不能用于不同正文。", 409)
	}
	return true, json.Unmarshal(result, out)
}
func saveReceiptTx(ctx context.Context, tx pgx.Tx, auth authorization, op, request string, payload, result any) error {
	b, e := json.Marshal(result)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO monitoring_receipts(scope_kind,tenant_id,scope_id,actor_id,operation,request_id,payload_hash,result) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, auth.Entry, auth.Tenant, auth.scopeID(), auth.Account, op, request, digest(payload), b)
	if e != nil {
		return e
	}
	return auditCommandTx(ctx, tx, auth, op, request, payload, result)
}

func (s *Service) GrantData(ctx context.Context, a access.Actor, tenant, project string, input GrantInput) (GrantResult, error) {
	out := GrantResult{}
	if e := input.WriteMeta.validate(); e != nil {
		return out, e
	}
	if !access.ValidID(input.ChannelID) || !access.ValidID(input.ResourceGrantID) {
		return out, invalid("channel_id", "请选择明确通道及其资源授权。")
	}
	if input.Range == nil {
		return out, invalid("range", "请明确选择全部历史或时间范围。")
	}
	w, e := input.Range.window()
	if e != nil {
		return out, e
	}
	if len(input.Capabilities) == 0 || len(input.Capabilities) > 6 {
		return out, invalid("capabilities", "请选择数据能力。")
	}
	for _, c := range input.Capabilities {
		if !slices.Contains(capabilities, c) {
			return out, invalid("capabilities", "数据能力无效。")
		}
	}
	input.Capabilities = slices.Clone(input.Capabilities)
	slices.Sort(input.Capabilities)
	if len(slices.Compact(input.Capabilities)) != len(input.Capabilities) {
		return out, invalid("capabilities", "数据能力不能重复。")
	}
	_, e = revisionNumber(input.ExpectedRevision)
	if e != nil {
		return out, e
	}
	e = s.transact(ctx, a, TenantScope(tenant), "access.grant", true, true, func(tx pgx.Tx, auth authorization) error {
		_, revision, e := projectState(ctx, tx, tenant, project, true)
		if e != nil {
			return e
		}
		payload := []any{project, input}
		replayed, e := receiptTx(ctx, tx, auth, "access.grant", input.RequestID, payload, &out)
		if e != nil {
			return e
		}
		if replayed {
			out.Replayed = true
			return nil
		}
		if revision != input.ExpectedRevision {
			return revisionChanged()
		}
		var valid bool
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project_resource_grants g JOIN project_facility_links l ON l.id=g.link_id JOIN engineering_channels c ON c.id=g.channel_id AND c.tenant_id=g.tenant_id WHERE g.tenant_id=$1 AND g.project_id=$2 AND g.id=$3 AND g.channel_id=$4 AND g.resource_kind='channel' AND g.ended_at IS NULL AND l.status='active' AND c.deleted_at IS NULL)`, tenant, project, input.ResourceGrantID, input.ChannelID).Scan(&valid)
		if e != nil {
			return e
		}
		if !valid {
			return notFound()
		}
		e = tx.QueryRow(ctx, `INSERT INTO monitoring_data_grants(tenant_id,project_id,channel_id,resource_grant_id,permitted,capabilities,granted_by,reason) VALUES($1,$2,$3,$4,$5::int8range,$6,$7,$8) RETURNING id::text,revision::text`, tenant, project, input.ChannelID, input.ResourceGrantID, w.pg(), input.Capabilities, a.ID, input.Reason).Scan(&out.ID, &out.Revision)
		if e != nil {
			return e
		}
		out.AccessRevision, e = bumpDataAccess(ctx, tx, auth, project, out.ID, "grant", input.WriteMeta)
		if e != nil {
			return e
		}
		return saveReceiptTx(ctx, tx, auth, "access.grant", input.RequestID, payload, out)
	})
	return out, e
}

const grantSelect = `SELECT d.id::text,d.tenant_id::text,d.project_id::text,d.channel_id::text,d.resource_grant_id::text,lower(d.permitted),upper(d.permitted),d.capabilities,d.revision::text,d.granted_at,d.revoked_at,(d.revoked_at IS NULL AND g.ended_at IS NULL AND l.status='active'),c.name,c.code,COALESCE(p.name,''),f.name FROM monitoring_data_grants d JOIN project_resource_grants g ON g.id=d.resource_grant_id JOIN project_facility_links l ON l.id=g.link_id JOIN engineering_channels c ON c.id=d.channel_id AND c.tenant_id=d.tenant_id LEFT JOIN engineering_points p ON p.id=c.point_id AND p.tenant_id=c.tenant_id JOIN engineering_facilities f ON f.id=c.facility_id AND f.tenant_id=c.tenant_id`

func scanGrant(row pgx.Row) (DataGrant, error) {
	var out DataGrant
	var lo, hi *int64
	e := row.Scan(&out.ID, &out.TenantID, &out.ProjectID, &out.ChannelID, &out.ResourceGrantID, &lo, &hi, &out.Capabilities, &out.Revision, &out.GrantedAt, &out.RevokedAt, &out.Effective, &out.ChannelName, &out.ChannelCode, &out.PointName, &out.FacilityName)
	out.Range = Window{lo, hi}.rangeDTO()
	return out, e
}

func (s *Service) ListDataGrants(ctx context.Context, a access.Actor, tenant, project, cursor string) (GrantPage, error) {
	out := GrantPage{Items: []DataGrant{}}
	if cursor != "" && !access.ValidID(cursor) {
		return out, invalid("cursor", "分页位置无效。")
	}
	e := s.transact(ctx, a, TenantScope(tenant), "access.list", false, false, func(tx pgx.Tx, auth authorization) error {
		_, revision, e := projectState(ctx, tx, tenant, project, false)
		if e != nil {
			return e
		}
		out.AccessRevision = revision
		rows, e := tx.Query(ctx, grantSelect+` WHERE d.tenant_id=$1 AND d.project_id=$2 AND ($3::uuid IS NULL OR d.id>$3) ORDER BY d.id LIMIT 101`, tenant, project, nullable(cursor))
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			g, e := scanGrant(rows)
			if e != nil {
				return e
			}
			out.Items = append(out.Items, g)
		}
		if e = rows.Err(); e != nil {
			return e
		}
		if len(out.Items) > 100 {
			out.Items = out.Items[:100]
			out.NextCursor = out.Items[99].ID
		}
		return nil
	})
	return out, e
}

func grantPreviewTx(ctx context.Context, tx pgx.Tx, auth authorization, project, id string) (GrantPreview, error) {
	out := GrantPreview{}
	_, rev, e := projectState(ctx, tx, auth.Tenant, project, true)
	if e != nil {
		return out, e
	}
	out.AccessRevision = rev
	if !access.ValidID(id) {
		return out, notFound()
	}
	out.Grant, e = scanGrant(tx.QueryRow(ctx, grantSelect+` WHERE d.tenant_id=$1 AND d.project_id=$2 AND d.id=$3`, auth.Tenant, project, id))
	if e == pgx.ErrNoRows {
		return out, notFound()
	}
	if e != nil {
		return out, e
	}
	out.Hash = digest([]any{"data-grant-preview-v1", out.Grant, rev, auth.Revision})
	return out, nil
}
func (s *Service) PreviewRevokeData(ctx context.Context, a access.Actor, tenant, project, id string) (GrantPreview, error) {
	out := GrantPreview{}
	e := s.transact(ctx, a, TenantScope(tenant), "access.preview", false, false, func(tx pgx.Tx, auth authorization) error {
		var e error
		out, e = grantPreviewTx(ctx, tx, auth, project, id)
		return e
	})
	return out, e
}
func (s *Service) RevokeData(ctx context.Context, a access.Actor, tenant, project, id string, in RevokeInput) (GrantResult, error) {
	out := GrantResult{}
	if e := in.WriteMeta.validate(); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, TenantScope(tenant), "access.revoke", true, true, func(tx pgx.Tx, auth authorization) error {
		if _, _, e := projectState(ctx, tx, tenant, project, true); e != nil {
			return e
		}
		payload := []any{project, id, in}
		replayed, e := receiptTx(ctx, tx, auth, "access.revoke", in.RequestID, payload, &out)
		if e != nil {
			return e
		}
		if replayed {
			out.Replayed = true
			return nil
		}
		preview, e := grantPreviewTx(ctx, tx, auth, project, id)
		if e != nil {
			return e
		}
		if preview.Grant.Revision != in.ExpectedRevision || preview.Hash != in.PreviewHash {
			return revisionChanged()
		}
		if preview.Grant.RevokedAt != nil {
			return revisionChanged()
		}
		e = tx.QueryRow(ctx, `UPDATE monitoring_data_grants SET revision=revision+1,revoked_at=clock_timestamp(),revoked_by=$4,revoke_reason=$5 WHERE tenant_id=$1 AND project_id=$2 AND id=$3 AND revoked_at IS NULL RETURNING id::text,revision::text`, tenant, project, id, a.ID, in.Reason).Scan(&out.ID, &out.Revision)
		if e != nil {
			return e
		}
		out.AccessRevision, e = bumpDataAccess(ctx, tx, auth, project, id, "revoke", in.WriteMeta)
		if e != nil {
			return e
		}
		return saveReceiptTx(ctx, tx, auth, "access.revoke", in.RequestID, payload, out)
	})
	return out, e
}
func bumpDataAccess(ctx context.Context, tx pgx.Tx, auth authorization, project, id, action string, m WriteMeta) (string, error) {
	var revision string
	e := tx.QueryRow(ctx, `INSERT INTO monitoring_access_states(tenant_id,project_id) VALUES($1,$2) ON CONFLICT(tenant_id,project_id) DO UPDATE SET revision=monitoring_access_states.revision+1 RETURNING revision::text`, auth.Tenant, project).Scan(&revision)
	if e != nil {
		return "", e
	}
	if e = access.Bump(ctx, tx); e != nil {
		return "", e
	}
	b, _ := json.Marshal(map[string]string{"access_revision": revision, "action": action})
	_, e = tx.Exec(ctx, `INSERT INTO monitoring_audit(tenant_id,project_id,actor_id,operation,target_id,reason,trace_id,summary) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, auth.Tenant, project, auth.Account, "access."+action, id, m.Reason, m.RequestID, b)
	if e != nil {
		return "", e
	}
	e = access.Audit(ctx, tx, "monitoring.access."+action, "monitoring_data_grant", id, "", nil, map[string]string{"access_revision": revision}, access.Meta{TenantID: auth.Tenant, ActorID: auth.Account, ActorScope: "tenant", RequestID: m.RequestID, Reason: m.Reason})
	if e == nil {
		e = access.NotifyProjectGrantTx(ctx, tx, "monitoring", auth.Tenant, project, auth.Account)
	}
	return revision, e
}

// AuthorizedWindows is used by ingestion and query planning, always within the
// current operation/session authorization. It does not infer permission from
// file ownership or from a resource's navigation context.
func (s *Service) AuthorizedWindows(ctx context.Context, a access.Actor, scope Scope, channels []string, capability string) (map[string][]Window, string, error) {
	out := map[string][]Window{}
	revision := ""
	action := map[string]string{"read": "details", "export": "exports.create", "import": "imports.validate", "revoke": "batches.preview", "history": "details", "source": "sources.download"}[capability]
	if action == "" {
		return out, revision, access.ErrDenied
	}
	e := s.transact(ctx, a, scope, action, false, false, func(tx pgx.Tx, auth authorization) error {
		var e error
		out, e = authorizedWindowsTx(ctx, tx, auth, channels, capability)
		revision = auth.Revision + ":" + auth.ResourceRevision + ":" + auth.DataRevision
		return e
	})
	return out, revision, e
}
func authorizedWindowsTx(ctx context.Context, tx pgx.Tx, auth authorization, channels []string, capability string) (map[string][]Window, error) {
	return availableWindowsTx(ctx, tx, auth, channels, capability, true)
}
func availableWindowsTx(ctx context.Context, tx pgx.Tx, auth authorization, channels []string, capability string, strict bool) (map[string][]Window, error) {
	out := map[string][]Window{}
	if len(channels) == 0 || len(channels) > 50000 {
		return out, invalid("channel_ids", "通道选择须为 1–50000 个。")
	}
	for _, id := range channels {
		if !access.ValidID(id) {
			return out, notFound()
		}
	}
	if auth.Entry == "project" {
		if _, e := access.RequireActorProjectTx(ctx, tx, authorizationActor(auth), access.ProjectResourceOperation("channel", "detail"), auth.Tenant, auth.Project); e != nil {
			return out, e
		}
	} else {
		if e := access.RequireScopedTx(ctx, tx, auth.Account, access.ResourceOperation("channel", "detail"), auth.Tenant); e != nil {
			return out, e
		}
	}
	if capability == "history" {
		if auth.Subject != nil {
			return nil, access.ErrDenied
		}
		var ok bool
		code := access.TenantMonitoringHistoryRead
		if auth.Entry == "project" {
			code = access.ProjectMonitoringHistoryRead
		}
		if auth.Entry == "project" {
			snap, e := access.ProjectSnapshotTx(ctx, tx, auth.Account, auth.Tenant, auth.Project, auth.Revision)
			if e != nil {
				return out, e
			}
			ok = slices.Contains(snap.Permissions, code)
		} else {
			_, snap, e := access.TenantSnapshotTx(ctx, tx, auth.Account, auth.Tenant, auth.Revision)
			if e != nil {
				return out, e
			}
			ok = slices.Contains(snap.Permissions, code)
		}
		if !ok {
			return out, access.ErrDenied
		}
	}
	query := `SELECT c.id::text,NULL::bigint,NULL::bigint FROM engineering_channels c WHERE c.tenant_id=$1 AND c.id=ANY($2::uuid[]) AND c.deleted_at IS NULL`
	args := []any{auth.Tenant, channels}
	if auth.Entry == "project" {
		query = `SELECT c.id::text,lower(d.permitted),upper(d.permitted) FROM engineering_channels c JOIN monitoring_data_grants d ON d.tenant_id=c.tenant_id AND d.channel_id=c.id JOIN project_resource_grants g ON g.id=d.resource_grant_id JOIN project_facility_links l ON l.id=g.link_id WHERE c.tenant_id=$1 AND c.id=ANY($2::uuid[]) AND c.deleted_at IS NULL AND d.project_id=$3 AND d.revoked_at IS NULL AND $4=ANY(d.capabilities) AND g.ended_at IS NULL AND l.status='active'`
		args = append(args, auth.Project, capability)
	}
	rows, e := tx.Query(ctx, query, args...)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var w Window
		if e = rows.Scan(&id, &w.From, &w.To); e != nil {
			return out, e
		}
		out[id] = append(out[id], w)
	}
	if e = rows.Err(); e != nil {
		return out, e
	}
	if auth.Subject != nil {
		if auth.Subject.Windows == nil {
			return nil, access.ErrDenied
		}
		ceilings, e := auth.Subject.Windows(ctx, tx, channels)
		if e != nil {
			return nil, e
		}
		for _, id := range channels {
			var clipped []Window
			for _, w := range out[id] {
				for _, limit := range ceilings[id] {
					if v, ok := w.Intersect(Window{From: limit.From, To: limit.To}); ok {
						clipped = append(clipped, v)
					}
				}
			}
			out[id] = clipped
		}
	}
	for _, id := range channels {
		if len(out[id]) == 0 {
			if strict {
				return map[string][]Window{}, notFound()
			}
			continue
		}
		out[id] = UnionWindows(out[id])
	}
	return out, nil
}

func stringRevision(n int64) string { return strconv.FormatInt(n, 10) }

func authorizationActor(auth authorization) access.Actor {
	if auth.Subject != nil {
		return auth.Subject.ReadActor()
	}
	return access.Actor{ID: auth.Account}
}

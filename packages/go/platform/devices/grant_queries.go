package devices

import (
	"context"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
)

func (s *Service) Grants(ctx context.Context, a access.Actor, tenant, project string, f Filter) (GrantPage, error) {
	out := GrantPage{Items: []Grant{}}
	e := s.accessTransact(ctx, a, tenant, "detail", false, func(tx pgx.Tx, auth k.Authorization) error {
		state, e := grantStateTx(ctx, tx, tenant, project)
		if e != nil {
			return e
		}
		out.GrantState = state
		auth.DeviceAccessRevision = state.AccessRevision
		if e = f.Validate(); e != nil {
			return e
		}
		if f.Kind != "" && f.Kind != "device" && f.Kind != "output" {
			return k.Problem("invalid_request", "授权目标类型无效。", 400)
		}
		cursor, e := k.DecodeCursor(f.Cursor, k.CursorContext(auth, f, []any{"device-grants", project}))
		if e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT `+grantJSON+grantFrom+` WHERE g.tenant_id=$1 AND g.project_id=$2 AND g.ended_at IS NULL AND ($3='' OR g.kind=$3) AND ($4='' OR d.code ILIKE '%'||$4||'%' OR d.name ILIKE '%'||$4||'%' OR o.name ILIKE '%'||$4||'%') AND ($5::timestamptz IS NULL OR (g.granted_at,g.id)>($5,$6::uuid)) ORDER BY g.granted_at,g.id LIMIT $7`, tenant, project, f.Kind, f.Query, cursor.TimeValue(), k.Nullable(cursor.ID), f.Limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var raw []byte
			var g Grant
			if e = rows.Scan(&raw); e != nil {
				return e
			}
			if e = json.Unmarshal(raw, &g); e != nil {
				return e
			}
			out.Items = append(out.Items, g)
		}
		if e = rows.Err(); e != nil {
			return e
		}
		if len(out.Items) > f.Limit {
			out.Items = out.Items[:f.Limit]
			last := out.Items[f.Limit-1]
			cursor.ID = last.ID
			cursor.At = last.GrantedAt
			out.NextCursor = cursor.Encode()
		}
		return nil
	})
	return out, e
}
func (s *Service) GrantCandidates(ctx context.Context, a access.Actor, tenant, project string, f Filter) (k.Page[GrantCandidate], error) {
	out := k.Page[GrantCandidate]{Items: []GrantCandidate{}}
	e := s.accessTransact(ctx, a, tenant, "candidates", false, func(tx pgx.Tx, auth k.Authorization) error {
		state, e := grantStateTx(ctx, tx, tenant, project)
		if e != nil {
			return e
		}
		auth.DeviceAccessRevision = state.AccessRevision
		if e = f.Validate(); e != nil {
			return e
		}
		if f.Kind != "device" && f.Kind != "output" {
			return k.Problem("invalid_request", "请选择设备档案或具体输出。", 400)
		}
		cursor, e := k.DecodeCursor(f.Cursor, k.CursorContext(auth, f, []any{"device-grant-candidates", project}))
		if e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `WITH candidates AS (
 SELECT 'device'::text AS kind,d.id,d.id AS device_id,d.code AS device_code,d.name AS device_name,d.name,false AS ended,d.created_at FROM devices d WHERE d.tenant_id=$1 AND d.deleted_at IS NULL AND $3='device'
 UNION ALL SELECT 'output',o.id,d.id,d.code,d.name,o.name,o.ended_at IS NOT NULL,o.started_at FROM device_outputs o JOIN devices d ON d.id=o.device_id WHERE o.tenant_id=$1 AND d.deleted_at IS NULL AND o.ended_at IS NULL AND $3='output'
 ) SELECT c.kind,c.id::text,c.device_id::text,c.device_code,c.device_name,c.name,c.ended,c.created_at FROM candidates c
 WHERE NOT EXISTS(SELECT 1 FROM project_device_grants g WHERE g.tenant_id=$1 AND g.project_id=$2 AND g.kind=c.kind AND CASE WHEN g.kind='device' THEN g.device_id ELSE g.output_id END=c.id AND g.ended_at IS NULL)
 AND ($4='' OR c.name ILIKE '%'||$4||'%' OR c.device_code ILIKE '%'||$4||'%' OR c.device_name ILIKE '%'||$4||'%') AND ($5::timestamptz IS NULL OR (c.created_at,c.id)>($5,$6::uuid)) ORDER BY c.created_at,c.id LIMIT $7`, tenant, project, f.Kind, f.Query, cursor.TimeValue(), k.Nullable(cursor.ID), f.Limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var v GrantCandidate
			if e = rows.Scan(&v.Kind, &v.ID, &v.DeviceID, &v.DeviceCode, &v.DeviceName, &v.Name, &v.Ended, &v.CreatedAt); e != nil {
				return e
			}
			out.Items = append(out.Items, v)
		}
		if e = rows.Err(); e != nil {
			return e
		}
		if len(out.Items) > f.Limit {
			out.Items = out.Items[:f.Limit]
			last := out.Items[f.Limit-1]
			cursor.ID = last.ID
			cursor.At = last.CreatedAt
			out.NextCursor = cursor.Encode()
		}
		return nil
	})
	return out, e
}
func (s *Service) GrantChanges(ctx context.Context, a access.Actor, tenant, project string, f Filter) (k.Page[GrantHistory], error) {
	out := k.Page[GrantHistory]{Items: []GrantHistory{}}
	e := s.accessTransact(ctx, a, tenant, "history", false, func(tx pgx.Tx, auth k.Authorization) error {
		state, e := grantStateTx(ctx, tx, tenant, project)
		if e != nil {
			return e
		}
		auth.DeviceAccessRevision = state.AccessRevision
		if e = f.Validate(); e != nil {
			return e
		}
		cursor, e := k.DecodeCursor(f.Cursor, k.CursorContext(auth, f, []any{"device-access-history", project}))
		if e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT to_jsonb(c)||jsonb_build_object('access_revision',access_revision::text) FROM project_device_access_changes c WHERE tenant_id=$1 AND project_id=$2 AND ($3::timestamptz IS NULL OR (changed_at,id)<($3,$4::uuid)) ORDER BY changed_at DESC,id DESC LIMIT $5`, tenant, project, cursor.TimeValue(), k.Nullable(cursor.ID), f.Limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var raw []byte
			var v GrantHistory
			if e = rows.Scan(&raw); e != nil {
				return e
			}
			if e = json.Unmarshal(raw, &v); e != nil {
				return e
			}
			out.Items = append(out.Items, v)
		}
		if e = rows.Err(); e != nil {
			return e
		}
		if len(out.Items) > f.Limit {
			out.Items = out.Items[:f.Limit]
			last := out.Items[f.Limit-1]
			cursor.ID = last.ID
			cursor.At = last.ChangedAt
			out.NextCursor = cursor.Encode()
		}
		return nil
	})
	return out, e
}
